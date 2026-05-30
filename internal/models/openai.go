package models

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type openaiProvider struct {
	modelID string
	apiKey  string
	baseURL string
}

func newOpenAIModel(spec, apiKey string) (model.ToolCallingChatModel, error) {
	_, modelID := splitSpec(spec)
	if apiKey == "" {
		return nil, fmt.Errorf("openai: OPENAI_API_KEY not set")
	}
	return &providerModel{
		provider: &openaiProvider{
			modelID: modelID,
			apiKey:  apiKey,
			baseURL: "https://api.openai.com/v1/chat/completions",
		},
	}, nil
}

type openaiMessage struct {
	Role       string        `json:"role"`
	Content    interface{}   `json:"content"`
	Name       string        `json:"name,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	ToolCalls  []openaiTC    `json:"tool_calls,omitempty"`
}

type openaiTC struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function openaiFunc   `json:"function"`
}

type openaiFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openaiTool struct {
	Type     string            `json:"type"`
	Function openaiToolFunc    `json:"function"`
}

type openaiToolFunc struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type openaiRequest struct {
	Model     string         `json:"model"`
	Messages  []openaiMessage `json:"messages"`
	Tools     []openaiTool   `json:"tools,omitempty"`
	MaxTokens int            `json:"max_tokens,omitempty"`
	Stream    bool           `json:"stream"`
}

type openaiResponse struct {
	Choices []struct {
		Message struct {
			Role      string      `json:"role"`
			Content   interface{} `json:"content"`
			ToolCalls []openaiTC  `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

type openaiStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string      `json:"content"`
			ToolCalls []openaiTC  `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func (p *openaiProvider) Generate(ctx context.Context, messages []*schema.Message, tools []*schema.ToolInfo) (*schema.Message, error) {
	req, err := p.buildRequest(messages, tools, false)
	if err != nil {
		return nil, err
	}

	resp, err := p.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	var or openaiResponse
	if err := json.Unmarshal(resp, &or); err != nil {
		return nil, fmt.Errorf("openai: parse response: %w", err)
	}

	return convertOpenAIResponse(&or), nil
}

func (p *openaiProvider) Stream(ctx context.Context, messages []*schema.Message, tools []*schema.ToolInfo) (*schema.StreamReader[*schema.Message], error) {
	req, err := p.buildRequest(messages, tools, true)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL, bytes.NewReader(req))
	if err != nil {
		return nil, err
	}
	p.setHeaders(httpReq)

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}

	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		defer httpResp.Body.Close()
		defer sw.Close()

		scanner := bufio.NewScanner(httpResp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		var (
			fullText      string
			toolCallDelta []streamToolDelta
		)

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var chunk openaiStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			for _, choice := range chunk.Choices {
				if choice.Delta.Content != "" {
					fullText += choice.Delta.Content
				}
				for _, tc := range choice.Delta.ToolCalls {
					for len(toolCallDelta) <= tc.Index() {
						toolCallDelta = append(toolCallDelta, streamToolDelta{})
					}
					if tc.ID != "" {
						toolCallDelta[tc.Index()].id = tc.ID
					}
					if tc.Function.Name != "" {
						toolCallDelta[tc.Index()].name = tc.Function.Name
					}
					toolCallDelta[tc.Index()].args += tc.Function.Arguments
				}
			}
		}

		msg := &schema.Message{Role: schema.Assistant, Content: fullText}
		for _, td := range toolCallDelta {
			if td.name != "" {
				msg.ToolCalls = append(msg.ToolCalls, schema.ToolCall{
					ID:   td.id,
					Type: "function",
					Function: schema.FunctionCall{
						Name:      td.name,
						Arguments: td.args,
					},
				})
			}
		}
		sw.Send(msg, nil)
	}()

	return sr, nil
}

type streamToolDelta struct {
	id   string
	name string
	args string
}

func (tc openaiTC) Index() int {
	// Extract index from the position or use sequential
	for i := len(tc.ID) - 1; i >= 0; i-- {
		if c := tc.ID[i]; c >= '0' && c <= '9' {
			continue
		} else if i+1 < len(tc.ID) {
			var idx int
			fmt.Sscanf(tc.ID[i+1:], "%d", &idx)
			return idx
		}
		break
	}
	return 0
}

func (p *openaiProvider) buildRequest(messages []*schema.Message, tools []*schema.ToolInfo, stream bool) ([]byte, error) {
	req := openaiRequest{
		Model:    p.modelID,
		MaxTokens: 16000,
		Stream:   stream,
		Messages: convertToOpenAIMessages(messages),
		Tools:    convertToOpenAITools(tools),
	}
	return json.Marshal(req)
}

func (p *openaiProvider) doRequest(ctx context.Context, body []byte) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	p.setHeaders(httpReq)

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}
	if httpResp.StatusCode >= 400 {
		return nil, fmt.Errorf("openai: API error %d: %s", httpResp.StatusCode, string(respBody))
	}
	return respBody, nil
}

func (p *openaiProvider) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")
}

func convertToOpenAIMessages(msgs []*schema.Message) []openaiMessage {
	result := make([]openaiMessage, 0, len(msgs))
	for _, msg := range msgs {
		if msg == nil {
			continue
		}
		switch msg.Role {
		case schema.System:
			result = append(result, openaiMessage{
				Role:    "system",
				Content: msg.Content,
			})
		case schema.User:
			result = append(result, openaiMessage{
				Role:    "user",
				Content: msg.Content,
			})
		case schema.Assistant:
			m := openaiMessage{
				Role:    "assistant",
				Content: msg.Content,
			}
			if len(msg.ToolCalls) > 0 {
				m.ToolCalls = make([]openaiTC, 0, len(msg.ToolCalls))
				for _, tc := range msg.ToolCalls {
					m.ToolCalls = append(m.ToolCalls, openaiTC{
						ID:   tc.ID,
						Type: "function",
						Function: openaiFunc{
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					})
				}
			}
			result = append(result, m)
		case schema.Tool:
			result = append(result, openaiMessage{
				Role:       "tool",
				Content:    msg.Content,
				ToolCallID: msg.ToolCallID,
			})
		}
	}
	return result
}

func convertOpenAIResponse(resp *openaiResponse) *schema.Message {
	if len(resp.Choices) == 0 {
		return &schema.Message{Role: schema.Assistant}
	}
	choice := resp.Choices[0]
	msg := &schema.Message{Role: schema.Assistant}
	if s, ok := choice.Message.Content.(string); ok {
		msg.Content = s
	}
	for _, tc := range choice.Message.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, schema.ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: schema.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}
	return msg
}

func convertToOpenAITools(tools []*schema.ToolInfo) []openaiTool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]openaiTool, 0, len(tools))
	for _, t := range tools {
		result = append(result, openaiTool{
			Type: "function",
			Function: openaiToolFunc{
				Name:        t.Name,
				Description: t.Desc,
				Parameters:  buildInputSchema(t),
			},
		})
	}
	return result
}
