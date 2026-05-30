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

type anthropicProvider struct {
	modelID string
	apiKey  string
	baseURL string
}

func newAnthropicModel(spec, apiKey string) (model.ToolCallingChatModel, error) {
	_, modelID := splitSpec(spec)
	if apiKey == "" {
		return nil, fmt.Errorf("anthropic: ANTHROPIC_API_KEY not set")
	}
	return &providerModel{
		provider: &anthropicProvider{
			modelID: modelID,
			apiKey:  apiKey,
			baseURL: "https://api.anthropic.com/v1/messages",
		},
	}, nil
}

type anthropicMessage struct {
	Role    string              `json:"role"`
	Content []anthropicContent  `json:"content"`
}

type anthropicContent struct {
	Type      string           `json:"type"`
	Text      string           `json:"text,omitempty"`
	Name      string           `json:"name,omitempty"`
	Input     json.RawMessage  `json:"input,omitempty"`
	ToolUseID string           `json:"tool_use_id,omitempty"`
	Source    *anthropicSource `json:"source,omitempty"`
	Content   string           `json:"content,omitempty"`
}

type anthropicSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type anthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type anthropicRequest struct {
	Model     string              `json:"model"`
	MaxTokens int                 `json:"max_tokens"`
	Messages  []anthropicMessage  `json:"messages"`
	Tools     []anthropicTool     `json:"tools,omitempty"`
	Stream    bool                `json:"stream"`
}

type anthropicResponse struct {
	Content []struct {
		Type      string          `json:"type"`
		Text      string          `json:"text"`
		Name      string          `json:"name"`
		Input     json.RawMessage `json:"input"`
		ID        string          `json:"id"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type anthropicStreamEvent struct {
	Type    string `json:"type"`
	Delta   *struct {
		Type       string          `json:"type"`
		Text       string          `json:"text"`
		PartialJSON string         `json:"partial_json"`
	} `json:"delta"`
	ContentBlock *struct {
		Type string `json:"type"`
		Name string `json:"name"`
		ID   string `json:"id"`
	} `json:"content_block"`
	Usage *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (p *anthropicProvider) Generate(ctx context.Context, messages []*schema.Message, tools []*schema.ToolInfo) (*schema.Message, error) {
	req, err := p.buildRequest(messages, tools, false)
	if err != nil {
		return nil, err
	}

	resp, err := p.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	var ar anthropicResponse
	if err := json.Unmarshal(resp, &ar); err != nil {
		return nil, fmt.Errorf("anthropic: parse response: %w", err)
	}

	return convertAnthropicResponse(&ar), nil
}

func (p *anthropicProvider) Stream(ctx context.Context, messages []*schema.Message, tools []*schema.ToolInfo) (*schema.StreamReader[*schema.Message], error) {
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

		// Track tool call accumulation state
		var (
			toolUseAccum  []toolCallAccum
			currentText   string
			textSent      bool
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

			var event anthropicStreamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			switch event.Type {
			case "content_block_start":
				if event.ContentBlock != nil && event.ContentBlock.Type == "tool_use" {
					toolUseAccum = append(toolUseAccum, toolCallAccum{
						id:   event.ContentBlock.ID,
						name: event.ContentBlock.Name,
					})
				}
			case "content_block_delta":
				if event.Delta == nil {
					continue
				}
				switch event.Delta.Type {
				case "text_delta":
					currentText += event.Delta.Text
				case "input_json_delta":
					if len(toolUseAccum) > 0 {
						toolUseAccum[len(toolUseAccum)-1].input += event.Delta.PartialJSON
					}
				}
			case "content_block_stop":
				// Flush current chunk
				if len(toolUseAccum) > 0 {
					chunk := buildStreamChunk(nil, toolUseAccum)
					if chunk != nil {
						sw.Send(chunk, nil)
					}
					toolUseAccum = nil
				}
			case "message_delta":
				// Final usage info
			case "message_stop":
				if currentText != "" && !textSent {
					sw.Send(&schema.Message{
						Role:    schema.Assistant,
						Content: currentText,
					}, nil)
				}
			}
		}
	}()

	return sr, nil
}

func (p *anthropicProvider) buildRequest(messages []*schema.Message, tools []*schema.ToolInfo, stream bool) ([]byte, error) {
	req := anthropicRequest{
		Model:     p.modelID,
		MaxTokens: 16000,
		Stream:    stream,
		Messages:  convertToAnthropicMessages(messages),
		Tools:     convertToAnthropicTools(tools),
	}
	return json.Marshal(req)
}

func (p *anthropicProvider) doRequest(ctx context.Context, body []byte) ([]byte, error) {
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
		return nil, fmt.Errorf("anthropic: API error %d: %s", httpResp.StatusCode, string(respBody))
	}
	return respBody, nil
}

func (p *anthropicProvider) setHeaders(req *http.Request) {
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")
}

type toolCallAccum struct {
	id    string
	name  string
	input string
}

func buildStreamChunk(text *string, tools []toolCallAccum) *schema.Message {
	msg := &schema.Message{Role: schema.Assistant}
	if text != nil && *text != "" {
		msg.Content = *text
	}
	if len(tools) > 0 {
		msg.ToolCalls = make([]schema.ToolCall, 0, len(tools))
		for _, tc := range tools {
			if tc.name != "" && tc.input != "" {
				msg.ToolCalls = append(msg.ToolCalls, schema.ToolCall{
					ID:   tc.id,
					Type: "function",
					Function: schema.FunctionCall{
						Name:      tc.name,
						Arguments: tc.input,
					},
				})
			}
		}
	}
	if msg.Content == "" && len(msg.ToolCalls) == 0 {
		return nil
	}
	return msg
}

func convertToAnthropicMessages(msgs []*schema.Message) []anthropicMessage {
	result := make([]anthropicMessage, 0, len(msgs))
	var systemContent []anthropicContent

	for _, msg := range msgs {
		if msg == nil {
			continue
		}
		switch msg.Role {
		case schema.System:
			systemContent = append(systemContent, anthropicContent{
				Type: "text",
				Text: msg.Content,
			})
		case schema.User:
			m := anthropicMessage{Role: "user"}
			m.Content = append(m.Content, anthropicContent{
				Type: "text",
				Text: msg.Content,
			})
			result = append(result, m)
		case schema.Assistant:
			m := anthropicMessage{Role: "assistant"}
			c := anthropicContent{Type: "text", Text: msg.Content}
			m.Content = append(m.Content, c)

			// Add tool calls if present
			for _, tc := range msg.ToolCalls {
				m.Content = append(m.Content, anthropicContent{
					Type:  "tool_use",
					Name:  tc.Function.Name,
					Input: json.RawMessage(tc.Function.Arguments),
				})
			}

			// If there's no text but there are tool calls, don't add empty text
			if msg.Content == "" && len(msg.ToolCalls) > 0 {
				m.Content = m.Content[1:] // Remove empty text
			}
			result = append(result, m)
		case schema.Tool:
			m := anthropicMessage{Role: "user"}
			m.Content = append(m.Content, anthropicContent{
				Type:      "tool_result",
				ToolUseID: msg.ToolCallID,
				Content:   msg.Content,
			})
			result = append(result, m)
		}
	}

	// Anthropic requires the first message to be user
	if len(result) > 0 && result[0].Role != "user" {
		result = append([]anthropicMessage{{Role: "user", Content: []anthropicContent{{Type: "text", Text: "Hello"}}}}, result...)
	}

	_ = systemContent
	return result
}

func convertAnthropicResponse(resp *anthropicResponse) *schema.Message {
	msg := &schema.Message{Role: schema.Assistant}
	for _, c := range resp.Content {
		switch c.Type {
		case "text":
			msg.Content += c.Text
		case "tool_use":
			msg.ToolCalls = append(msg.ToolCalls, schema.ToolCall{
				ID:   c.ID,
				Type: "function",
				Function: schema.FunctionCall{
					Name:      c.Name,
					Arguments: string(c.Input),
				},
			})
		}
	}
	return msg
}

func convertToAnthropicTools(tools []*schema.ToolInfo) []anthropicTool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]anthropicTool, 0, len(tools))
	for _, t := range tools {
		at := anthropicTool{
			Name:        t.Name,
			Description: t.Desc,
			InputSchema: buildInputSchema(t),
		}
		result = append(result, at)
	}
	return result
}

func buildInputSchema(t *schema.ToolInfo) map[string]interface{} {
	if t.ParamsOneOf == nil {
		return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
	}

	js, err := t.ParamsOneOf.ToJSONSchema()
	if err != nil || js == nil {
		return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
	}

	// Marshal to JSON and back to map for a clean generic representation
	b, err := json.Marshal(js)
	if err != nil {
		return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
	}

	var result map[string]interface{}
	json.Unmarshal(b, &result)
	return result
}
