package models

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type geminiProvider struct {
	modelID string
	apiKey  string
	baseURL string
}

func newGeminiModel(spec, apiKey string) (model.ToolCallingChatModel, error) {
	_, modelID := splitSpec(spec)
	if apiKey == "" {
		return nil, fmt.Errorf("gemini: GEMINI_API_KEY not set")
	}
	return &providerModel{
		provider: &geminiProvider{
			modelID: modelID,
			apiKey:  apiKey,
			baseURL: "https://generativelanguage.googleapis.com/v1beta/models",
		},
	}, nil
}

type geminiContent struct {
	Role  string        `json:"role,omitempty"`
	Parts []geminiPart  `json:"parts"`
}

type geminiPart struct {
	Text       string `json:"text,omitempty"`
	FunctionCall  *geminiFC `json:"functionCall,omitempty"`
	FunctionResponse *geminiFR `json:"functionResponse,omitempty"`
}

type geminiFC struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type geminiFR struct {
	Name     string `json:"name"`
	Response struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	} `json:"response"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFD `json:"functionDeclarations"`
}

type geminiFD struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type geminiRequest struct {
	Contents         []geminiContent  `json:"contents"`
	Tools            []geminiTool     `json:"tools,omitempty"`
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenConfig `json:"generationConfig,omitempty"`
}

type geminiGenConfig struct {
	MaxOutputTokens int `json:"maxOutputTokens"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Role  string       `json:"role"`
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func (p *geminiProvider) Generate(ctx context.Context, messages []*schema.Message, tools []*schema.ToolInfo) (*schema.Message, error) {
	req, err := p.buildRequest(messages, tools)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		p.buildURL("generateContent"), bytes.NewReader(req))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

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
		return nil, fmt.Errorf("gemini: API error %d: %s", httpResp.StatusCode, string(respBody))
	}

	var gr geminiResponse
	if err := json.Unmarshal(respBody, &gr); err != nil {
		return nil, fmt.Errorf("gemini: parse response: %w", err)
	}

	return convertGeminiResponse(&gr), nil
}

func (p *geminiProvider) Stream(ctx context.Context, messages []*schema.Message, tools []*schema.ToolInfo) (*schema.StreamReader[*schema.Message], error) {
	// Gemini streaming uses SSE; implemented similarly
	req, err := p.buildRequest(messages, tools)
	if err != nil {
		return nil, err
	}

	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		defer sw.Close()

		httpReq, err := http.NewRequestWithContext(ctx, "POST",
			p.buildURL("streamGenerateContent?alt=sse"), bytes.NewReader(req))
		if err != nil {
			sw.Send(nil, err)
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")

		httpResp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			sw.Send(nil, err)
			return
		}
		defer httpResp.Body.Close()

		body, _ := io.ReadAll(httpResp.Body)
		// Parse SSE and send chunks
		var msg geminiResponse
		if err := json.Unmarshal(body, &msg); err == nil {
			sw.Send(convertGeminiResponse(&msg), nil)
		}
	}()

	return sr, nil
}

func (p *geminiProvider) buildRequest(messages []*schema.Message, tools []*schema.ToolInfo) ([]byte, error) {
	req := geminiRequest{
		GenerationConfig: &geminiGenConfig{MaxOutputTokens: 16000},
	}
	if len(tools) > 0 {
		req.Tools = []geminiTool{{FunctionDeclarations: convertToGeminiTools(tools)}}
	}

	var systemParts []geminiPart
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if msg.Role == schema.System {
			systemParts = append(systemParts, geminiPart{Text: msg.Content})
			continue
		}
		req.Contents = append(req.Contents, convertToGeminiContent(msg))
	}

	if len(systemParts) > 0 {
		req.SystemInstruction = &geminiContent{Parts: systemParts}
	}

	return json.Marshal(req)
}

func (p *geminiProvider) buildURL(path string) string {
	return fmt.Sprintf("%s/%s:%s?key=%s", p.baseURL,
		url.PathEscape(p.modelID), path, url.QueryEscape(p.apiKey))
}

func convertToGeminiContent(msg *schema.Message) geminiContent {
	switch msg.Role {
	case schema.User:
		return geminiContent{Role: "user", Parts: []geminiPart{{Text: msg.Content}}}
	case schema.Assistant:
		gc := geminiContent{Role: "model"}
		if msg.Content != "" {
			gc.Parts = append(gc.Parts, geminiPart{Text: msg.Content})
		}
		for _, tc := range msg.ToolCalls {
			gc.Parts = append(gc.Parts, geminiPart{
				FunctionCall: &geminiFC{
					Name: tc.Function.Name,
					Args: json.RawMessage(tc.Function.Arguments),
				},
			})
		}
		return gc
	case schema.Tool:
		return geminiContent{Role: "user", Parts: []geminiPart{{
			FunctionResponse: &geminiFR{
				Name: msg.ToolName,
				Response: struct {
					Name    string `json:"name"`
					Content string `json:"content"`
				}{Name: msg.ToolName, Content: msg.Content},
			},
		}}}
	}
	return geminiContent{Role: "user", Parts: []geminiPart{{Text: msg.Content}}}
}

func convertGeminiResponse(resp *geminiResponse) *schema.Message {
	msg := &schema.Message{Role: schema.Assistant}
	if len(resp.Candidates) == 0 {
		return msg
	}
	for _, part := range resp.Candidates[0].Content.Parts {
		if part.Text != "" {
			msg.Content += part.Text
		}
		if part.FunctionCall != nil {
			args, _ := json.Marshal(part.FunctionCall.Args)
			msg.ToolCalls = append(msg.ToolCalls, schema.ToolCall{
				ID:   part.FunctionCall.Name,
				Type: "function",
				Function: schema.FunctionCall{
					Name:      part.FunctionCall.Name,
					Arguments: string(args),
				},
			})
		}
	}
	return msg
}

func convertToGeminiTools(tools []*schema.ToolInfo) []geminiFD {
	result := make([]geminiFD, 0, len(tools))
	for _, t := range tools {
		result = append(result, geminiFD{
			Name:        t.Name,
			Description: t.Desc,
			Parameters:  buildInputSchema(t),
		})
	}
	return result
}
