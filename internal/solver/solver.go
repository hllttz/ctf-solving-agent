package solver

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"github.com/verialabs/ctf-agent/internal/bus"
	"github.com/verialabs/ctf-agent/internal/loopdetect"
	"github.com/verialabs/ctf-agent/internal/sandbox"
	sandboxTools "github.com/verialabs/ctf-agent/internal/tools"
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
	model     model.ToolCallingChatModel
	sandbox   sandbox.Sandbox
	bus       *bus.MessageBus
	busCursor int
	detector  *loopdetect.Detector
	result    *Result
	agentName string
	messages  []*schema.Message
	mu        sync.Mutex
}

// New creates a new solver instance.
func New(m model.ToolCallingChatModel, sb sandbox.Sandbox, b *bus.MessageBus) *Solver {
	return NewWithName(m, sb, b, "solver")
}

func NewWithName(m model.ToolCallingChatModel, sb sandbox.Sandbox, b *bus.MessageBus, agentName string) *Solver {
	return &Solver{
		model:     m,
		sandbox:   sb,
		bus:       b,
		detector:  loopdetect.New(),
		result:    &Result{Status: Running},
		agentName: agentName,
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

	// Build the ReAct agent
	agent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: s.model,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: tools,
			ToolCallMiddlewares: []compose.ToolMiddleware{
				s.detector.Middleware(),
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
	output, err := agent.Generate(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("agent run: %w", err)
	}
	s.recordTurn(input, output)

	result := &Result{
		Status:   GaveUp,
		Steps:    len(s.detector.History()),
		Findings: []string{output.Content},
	}
	_ = startTime

	// Extract flag if present in the output
	if flag := extractFlag(output.Content); flag != "" {
		result.Status = FlagFound
		result.Flag = flag
		result.Method = "ReAct agent output"
	}
	s.postSummary(output.Content, result)

	return result, nil
}

// Bump injects insights from sibling solvers as a new message and re-runs.
func (s *Solver) Bump(ctx context.Context, systemPrompt string, insights string) <-chan *Result {
	resultCh := make(chan *Result, 1)
	go func() {
		defer close(resultCh)
		s.detector.Reset()
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

func (s *Solver) buildTools() []tool.BaseTool {
	return []tool.BaseTool{
		sandboxTools.NewBashTool(s.sandbox),
		sandboxTools.NewReadFileTool(s.sandbox),
		sandboxTools.NewWriteFileTool(s.sandbox),
		sandboxTools.NewListFilesTool(s.sandbox),
		sandboxTools.NewViewImageTool(s.sandbox),
		sandboxTools.NewWebFetchTool(),
		sandboxTools.NewWebhookCreateTool(),
		sandboxTools.NewWebhookGetRequestsTool(),
		sandboxTools.NewPostFindingTool(s.bus, s.agentName),
		sandboxTools.NewCheckFindingsToolFor(s.bus, &s.busCursor, s.agentName),
	}
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

// extractFlag attempts to extract a flag pattern from text.
func extractFlag(text string) string {
	patterns := []string{"CTF{", "flag{", "FLAG{", "ctf{", "hsctf{"}
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
						return text[pos : i+1]
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
