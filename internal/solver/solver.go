package solver

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"github.com/verialabs/ctf-agent/internal/bus"
	"github.com/verialabs/ctf-agent/internal/cost"
	"github.com/verialabs/ctf-agent/internal/loopdetect"
	"github.com/verialabs/ctf-agent/internal/models"
	"github.com/verialabs/ctf-agent/internal/sandbox"
	sandboxTools "github.com/verialabs/ctf-agent/internal/tools"
	"github.com/verialabs/ctf-agent/internal/trace"
)

// Status represents the solver's current state.
type Status int

const (
	Running Status = iota
	FlagFound
	GaveUp
	Error
	Cancelled
)

// Result holds the solver's final output.
type Result struct {
	Status   Status
	Flag     string
	Method   string
	Findings []string
	Steps    int
	Cost     float64
	LogPath  string
}

// Solver is a single AI agent solving one challenge.
type Solver struct {
	model      model.ToolCallingChatModel
	sandbox    sandbox.Sandbox
	bus        *bus.MessageBus
	busCursor  int
	detector   *loopdetect.Detector
	result     *Result
	challenge  string
	agentName  string
	modelInfo  models.SpecInfo
	messages   []*schema.Message
	tracer     *trace.Tracer
	reporter   *sandboxTools.FlagReporter
	costs      *cost.Tracker
	commentary []Commentary
	stepCount  int
	mu         sync.Mutex
}

var commentaryMu sync.Mutex

// Commentary is a short assistant progress update emitted before or after tool
// calls. It is shown in the operator UI as a live finding stream.
type Commentary struct {
	Agent     string
	Content   string
	Timestamp time.Time
	Step      int
}

// New creates a new solver instance.
func New(m model.ToolCallingChatModel, sb sandbox.Sandbox, b *bus.MessageBus) *Solver {
	return NewWithName(m, sb, b, "solver")
}

func NewWithName(m model.ToolCallingChatModel, sb sandbox.Sandbox, b *bus.MessageBus, agentName string) *Solver {
	return NewWithNameForChallenge(m, sb, b, agentName, "challenge")
}

func NewWithNameForChallenge(m model.ToolCallingChatModel, sb sandbox.Sandbox, b *bus.MessageBus, agentName, challenge string) *Solver {
	return NewWithOptions(m, sb, b, agentName, challenge, nil)
}

func NewWithOptions(m model.ToolCallingChatModel, sb sandbox.Sandbox, b *bus.MessageBus, agentName, challenge string, costs *cost.Tracker) *Solver {
	tracer, err := trace.NewSolverTracer("logs", challenge, agentName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "trace disabled for %s: %v\n", agentName, err)
	}
	return &Solver{
		model:     m,
		sandbox:   sb,
		bus:       b,
		detector:  loopdetect.New(),
		result:    &Result{Status: Running},
		challenge: challenge,
		agentName: agentName,
		modelInfo: models.InspectSpec(agentName),
		tracer:    tracer,
		reporter:  sandboxTools.NewFlagReporter(),
		costs:     costs,
	}
}

// Run starts the solver on the given system prompt.
// It returns the result channel which receives one value when done.
func (s *Solver) Run(ctx context.Context, systemPrompt string) <-chan *Result {
	resultCh := make(chan *Result, 1)
	go func() {
		defer close(resultCh)
		result, err := s.run(ctx, systemPrompt)
		if err != nil {
			result = &Result{Status: Error, Findings: []string{err.Error()}}
		}
		resultCh <- result
	}()
	return resultCh
}

func (s *Solver) run(ctx context.Context, systemPrompt string) (*Result, error) {
	return s.runWithUserMessage(ctx, systemPrompt,
		"Use the available tools immediately and solve this CTF challenge. Follow the system instructions for the first action.",
		false)
}

func (s *Solver) runWithUserMessage(ctx context.Context, systemPrompt, userMessage string, preserveHistory bool) (*Result, error) {
	// Build tools for this solver
	tools := s.buildTools()
	s.traceEvent("start", 0, "", map[string]any{
		"preserve_history": preserveHistory,
		"provider":         s.modelInfo.Provider,
		"model_id":         s.modelInfo.ModelID,
		"effort":           s.modelInfo.Effort,
		"context_window":   s.modelInfo.ContextWindow,
		"supports_vision":  s.modelInfo.SupportsVision,
	})

	// Build the ReAct agent
	agent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: s.model,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: tools,
			ToolCallMiddlewares: []compose.ToolMiddleware{
				s.toolMiddleware(),
			},
		},
		MaxStep: 100,
	})
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}

	input := s.buildInput(systemPrompt, userMessage, preserveHistory)

	startTime := time.Now()

	// Run the agent
	msgFutureOpt, msgFuture := react.WithMessageFuture()
	watchDone := s.watchModelCommentary(ctx, msgFuture)
	output, err := agent.Generate(ctx, input, msgFutureOpt)
	<-watchDone
	if err != nil {
		s.traceEvent("error", 0, "", map[string]any{"error": err.Error()})
		return nil, fmt.Errorf("agent run: %w", err)
	}
	s.recordTurn(input, output)
	_, _, _, turnCost := s.recordUsage(output)
	s.traceEvent("model_response", s.currentStep(), "", map[string]any{
		"text": truncateString(output.Content, 2000),
	})

	result := &Result{
		Status:   GaveUp,
		Steps:    s.currentStep(),
		Findings: []string{output.Content},
		LogPath:  s.tracePath(),
		Cost:     turnCost,
	}
	_ = startTime

	if report, ok := s.reporter.Latest(); ok {
		result.Status = FlagFound
		result.Flag = report.Flag
		result.Method = report.Method
		if report.Confidence != "" {
			result.Findings = append(result.Findings, "confidence: "+report.Confidence)
		}
		if report.Evidence != "" {
			result.Findings = append(result.Findings, "evidence: "+report.Evidence)
		}
	}

	// Extract flag if present in the output
	if flag, method := extractFlagResult(output.Content); flag != "" && result.Status != FlagFound {
		result.Status = FlagFound
		result.Flag = flag
		result.Method = method
	}
	s.postSummary(output.Content, result)
	s.traceEvent("finish", result.Steps, "", map[string]any{
		"status": result.Status,
		"flag":   result.Flag,
		"method": result.Method,
	})

	return result, nil
}

func (s *Solver) watchModelCommentary(ctx context.Context, future react.MessageFuture) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		iter := future.GetMessages()
		for {
			msg, ok, err := iter.Next()
			if err != nil {
				s.traceEvent("commentary_error", s.currentStep(), "", map[string]any{"error": err.Error()})
				return
			}
			if !ok {
				return
			}
			select {
			case <-ctx.Done():
				return
			default:
			}
			s.emitCommentary(msg)
		}
	}()
	return done
}

func (s *Solver) emitCommentary(msg *schema.Message) {
	if msg == nil || msg.Role != schema.Assistant {
		return
	}
	text := strings.TrimSpace(msg.Content)
	if text == "" {
		if len(msg.ToolCalls) > 0 {
			s.traceEvent("commentary_empty", s.currentStep(), "", map[string]any{
				"tool_calls": len(msg.ToolCalls),
			})
		}
		return
	}
	s.traceEvent("commentary", s.currentStep(), "", map[string]any{
		"text": text,
	})
	s.recordCommentary(text)
	commentaryMu.Lock()
	defer commentaryMu.Unlock()
	fmt.Fprintf(os.Stderr, "[%s/%s] %s\n", s.challenge, s.agentName, text)
}

func (s *Solver) recordCommentary(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.commentary) >= 200 {
		s.commentary = s.commentary[1:]
	}
	s.commentary = append(s.commentary, Commentary{
		Agent:     s.agentName,
		Content:   text,
		Timestamp: time.Now(),
		Step:      s.stepCount,
	})
}

func (s *Solver) recordUsage(output *schema.Message) (inputTokens, outputTokens, cacheTokens int, costUSD float64) {
	if output == nil || output.ResponseMeta == nil || output.ResponseMeta.Usage == nil {
		return 0, 0, 0, 0
	}
	usage := output.ResponseMeta.Usage
	inputTokens = usage.PromptTokens
	outputTokens = usage.CompletionTokens
	cacheTokens = usage.PromptTokenDetails.CachedTokens
	u := cost.Usage{
		Model:        s.modelInfo.ModelID,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		CacheTokens:  cacheTokens,
	}
	costUSD = u.Cost()
	if s.costs != nil {
		s.costs.Record(s.agentName, s.modelInfo.ModelID, inputTokens, outputTokens, cacheTokens)
	}
	if s.tracer != nil {
		s.tracer.LogUsage(s.agentName, s.challenge, s.currentStep(), inputTokens, outputTokens, cacheTokens)
	}
	log.Printf("[%s/%s] usage in=%d cached=%d out=%d cost=$%.4f", s.challenge, s.agentName, inputTokens, cacheTokens, outputTokens, costUSD)
	return inputTokens, outputTokens, cacheTokens, costUSD
}

// Bump injects insights from sibling solvers as a new message and re-runs.
func (s *Solver) Bump(ctx context.Context, systemPrompt string, insights string) <-chan *Result {
	resultCh := make(chan *Result, 1)
	go func() {
		defer close(resultCh)
		s.detector.Reset()
		s.traceEvent("bump", s.currentStep(), "", map[string]any{
			"insights": truncateString(insights, 2000),
		})
		result, err := s.runWithUserMessage(ctx, systemPrompt, "Your previous attempt did not find the flag. "+
			"Here are insights from other solver agents working on the same challenge:\n\n"+
			insights+"\n\nUse these insights to guide your approach. Try a different approach. "+
			"Do NOT repeat what has already been tried. Find the flag!", true)
		if err != nil {
			resultCh <- &Result{Status: Error, Findings: []string{err.Error()}}
			return
		}
		resultCh <- result
	}()
	return resultCh
}

func (s *Solver) buildInput(systemPrompt, userMessage string, preserveHistory bool) []*schema.Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	input := []*schema.Message{{Role: schema.System, Content: systemPrompt}}
	if preserveHistory && len(s.messages) > 0 {
		input = append(input, cloneMessages(s.messages)...)
	}
	input = append(input, &schema.Message{Role: schema.User, Content: userMessage})
	return input
}

func (s *Solver) recordTurn(input []*schema.Message, output *schema.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()

	start := 0
	if len(input) > 0 && input[0].Role == schema.System {
		start = 1
	}
	s.messages = append(cloneMessages(input[start:]), output)
}

func cloneMessages(messages []*schema.Message) []*schema.Message {
	out := make([]*schema.Message, len(messages))
	copy(out, messages)
	return out
}

func (s *Solver) toolMiddleware() compose.ToolMiddleware {
	return compose.ToolMiddleware{
		Invokable: func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
				step := s.nextStep()
				s.traceEvent("tool_call", step, input.Name, map[string]any{
					"arguments": truncateString(input.Arguments, 2000),
				})

				output, err := next(ctx, input)
				if err != nil {
					s.traceEvent("tool_error", step, input.Name, map[string]any{"error": err.Error()})
					return nil, err
				}

				if msg := s.detector.Check(input.Name, input.Arguments); msg != "" {
					if output.Result != "" {
						output.Result += "\n"
					}
					output.Result += msg
				}

				if step%5 == 0 {
					if findings := s.checkFindingsText(); findings != "" {
						if output.Result != "" {
							output.Result += "\n\n---\n"
						}
						output.Result += findings
						s.traceEvent("findings_injected", step, input.Name, map[string]any{
							"findings": truncateString(findings, 2000),
						})
					}
				}

				s.traceEvent("tool_result", step, input.Name, map[string]any{
					"result": truncateString(output.Result, 2000),
				})
				return output, nil
			}
		},
		Streamable: func(next compose.StreamableToolEndpoint) compose.StreamableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.StreamToolOutput, error) {
				step := s.nextStep()
				s.traceEvent("tool_call", step, input.Name, map[string]any{
					"arguments": truncateString(input.Arguments, 2000),
					"stream":    true,
				})
				output, err := next(ctx, input)
				if err != nil {
					s.traceEvent("tool_error", step, input.Name, map[string]any{"error": err.Error()})
					return nil, err
				}
				_ = s.detector.Check(input.Name, input.Arguments)
				return output, nil
			}
		},
	}
}

func (s *Solver) checkFindingsText() string {
	items, nextCursor := s.bus.CheckFor(s.agentName, s.busCursor)
	s.busCursor = nextCursor
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("New guidance and findings:\n")
	for _, item := range items {
		b.WriteString(fmt.Sprintf("[%s] %s\n", item.Author, item.Content))
	}
	return strings.TrimSpace(b.String())
}

func (s *Solver) nextStep() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stepCount++
	return s.stepCount
}

func (s *Solver) currentStep() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stepCount
}

func (s *Solver) traceEvent(eventType string, step int, tool string, data map[string]any) {
	if s.tracer == nil {
		return
	}
	s.tracer.LogEvent(eventType, s.agentName, s.challenge, step, tool, data)

	switch eventType {
	case "start":
		log.Printf("[%s/%s] start provider=%v model=%v vision=%v", s.challenge, s.agentName, data["provider"], data["model_id"], data["supports_vision"])
	case "bump":
		log.Printf("[%s/%s] bump step=%d", s.challenge, s.agentName, step)
	case "tool_call":
		log.Printf("[%s/%s] step %d tool=%s", s.challenge, s.agentName, step, tool)
	case "tool_error":
		log.Printf("[%s/%s] step %d tool=%s error=%v", s.challenge, s.agentName, step, tool, data["error"])
	case "model_response":
		log.Printf("[%s/%s] step %d model responded", s.challenge, s.agentName, step)
	case "finish":
		log.Printf("[%s/%s] finish status=%v flag=%v method=%v steps=%d", s.challenge, s.agentName, data["status"], data["flag"], data["method"], step)
	case "error":
		log.Printf("[%s/%s] error=%v", s.challenge, s.agentName, data["error"])
	}
}

func (s *Solver) tracePath() string {
	if s.tracer == nil {
		return ""
	}
	return s.tracer.Path()
}

func (s *Solver) buildTools() []tool.BaseTool {
	tools := []tool.BaseTool{
		sandboxTools.NewBashTool(s.sandbox),
		sandboxTools.NewReadFileTool(s.sandbox),
		sandboxTools.NewWriteFileTool(s.sandbox),
		sandboxTools.NewListFilesTool(s.sandbox),
		sandboxTools.NewWebFetchTool(),
		sandboxTools.NewWebhookCreateTool(),
		sandboxTools.NewWebhookGetRequestsTool(),
		sandboxTools.NewReportFlagTool(s.reporter),
		sandboxTools.NewPostFindingTool(s.bus, s.agentName),
		sandboxTools.NewNotifyCoordinatorTool(s.bus, s.agentName),
		sandboxTools.NewCheckFindingsToolFor(s.bus, &s.busCursor, s.agentName),
	}
	if s.modelInfo.SupportsVision {
		tools = append(tools, sandboxTools.NewViewImageTool(s.sandbox))
	}
	return tools
}

func (s *Solver) postSummary(content string, result *Result) {
	summary := strings.TrimSpace(content)
	if summary == "" {
		return
	}
	if len(summary) > 1200 {
		summary = summary[:1200] + "... [truncated]"
	}
	if result.Status == FlagFound {
		summary = "FLAG FOUND: " + result.Flag + "\n" + summary
	}
	s.bus.Post(s.agentName, summary)
}

type flagOutput struct {
	Type   string `json:"type"`
	Flag   string `json:"flag"`
	Method string `json:"method"`
}

// extractFlagResult attempts to extract a final flag from structured output first,
// then falls back to explicit FLAG lines and common flag patterns.
func extractFlagResult(text string) (string, string) {
	if flag, method := extractJSONFlag(text); flag != "" {
		return flag, method
	}
	if isStructuredNonFlag(text) {
		return "", ""
	}
	if flag := extractFlagLine(text); flag != "" {
		return flag, "FLAG line"
	}
	if flag := extractFlag(text); flag != "" {
		return flag, "flag pattern fallback"
	}
	return "", ""
}

func extractJSONFlag(text string) (string, string) {
	candidates := []string{strings.TrimSpace(text)}
	candidates = append(candidates, fencedJSONBlocks(text)...)
	candidates = append(candidates, jsonObjects(text)...)

	for _, candidate := range candidates {
		var out flagOutput
		if err := json.Unmarshal([]byte(candidate), &out); err != nil {
			continue
		}
		flag := strings.TrimSpace(out.Flag)
		if flag == "" {
			continue
		}
		if !sandboxTools.IsPlausibleFlag(flag) {
			continue
		}
		if out.Type != "" && out.Type != "flag_found" {
			continue
		}
		method := strings.TrimSpace(out.Method)
		if method == "" {
			method = "structured output"
		}
		return flag, method
	}
	return "", ""
}

func fencedJSONBlocks(text string) []string {
	re := regexp.MustCompile("(?is)```(?:json)?\\s*(\\{.*?\\})\\s*```")
	matches := re.FindAllStringSubmatch(text, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			out = append(out, strings.TrimSpace(match[1]))
		}
	}
	return out
}

func jsonObjects(text string) []string {
	var out []string
	for start := strings.IndexByte(text, '{'); start >= 0 && start < len(text); {
		depth := 0
		inString := false
		escaped := false
		for i := start; i < len(text); i++ {
			ch := text[i]
			if inString {
				if escaped {
					escaped = false
					continue
				}
				if ch == '\\' {
					escaped = true
					continue
				}
				if ch == '"' {
					inString = false
				}
				continue
			}
			switch ch {
			case '"':
				inString = true
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					out = append(out, text[start:i+1])
					next := strings.IndexByte(text[i+1:], '{')
					if next == -1 {
						return out
					}
					start = i + 1 + next
					i = len(text)
				}
			}
		}
		if depth != 0 {
			return out
		}
	}
	return out
}

func extractFlagLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "FLAG:") {
			flag := strings.TrimSpace(line[len("FLAG:"):])
			if sandboxTools.IsPlausibleFlag(flag) {
				return flag
			}
		}
	}
	return ""
}

func isStructuredNonFlag(text string) bool {
	var out flagOutput
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &out); err != nil {
		return false
	}
	return strings.TrimSpace(out.Flag) != "" && out.Type != "" && out.Type != "flag_found"
}

// extractFlag attempts to extract a flag pattern from text.
func extractFlag(text string) string {
	patterns := []string{"hsctf{", "CTF{", "flag{", "FLAG{", "ctf{"}
	for _, prefix := range patterns {
		idx := 0
		for {
			pos := -1
			for i := idx; i <= len(text)-len(prefix); i++ {
				if text[i:i+len(prefix)] == prefix {
					pos = i
					break
				}
			}
			if pos == -1 {
				break
			}
			// Find closing brace
			depth := 0
			for i := pos + len(prefix) - 1; i < len(text); i++ {
				if text[i] == '{' {
					depth++
				} else if text[i] == '}' {
					depth--
					if depth == 0 {
						flag := text[pos : i+1]
						if sandboxTools.IsPlausibleFlag(flag) {
							return flag
						}
						break
					}
				}
			}
			idx = pos + len(prefix)
		}
	}
	return ""
}

// History exposes the detector history for debugging.
func (s *Solver) History() []string {
	return s.detector.History()
}

func (s *Solver) Commentary() []Commentary {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Commentary, len(s.commentary))
	copy(out, s.commentary)
	return out
}

func (s *Solver) ModelID() string {
	return s.modelInfo.ModelID
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
