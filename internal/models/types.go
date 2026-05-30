package models

import (
	"context"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type SpecInfo struct {
	Provider       string
	ModelID        string
	Effort         string
	ContextWindow  int
	SupportsVision bool
}

var contextWindows = map[string]int{
	"claude-opus-4-6":        1_000_000,
	"claude-sonnet-4-6":      1_000_000,
	"gpt-5.4":                1_000_000,
	"gpt-5.4-mini":           400_000,
	"gpt-5.3-codex":          1_000_000,
	"gpt-5.3-codex-spark":    128_000,
	"gemini-3-flash-preview": 1_000_000,
}

var visionModels = map[string]bool{
	"claude-opus-4-6":        true,
	"claude-sonnet-4-6":      true,
	"gpt-5.4":                true,
	"gpt-5.4-mini":           true,
	"gemini-3-flash-preview": true,
}

// Provider abstracts a model backend (Anthropic, OpenAI, etc.).
type Provider interface {
	// Generate sends messages and returns the model's response.
	Generate(ctx context.Context, messages []*schema.Message, tools []*schema.ToolInfo) (*schema.Message, error)
	// Stream sends messages and returns a stream of response chunks.
	Stream(ctx context.Context, messages []*schema.Message, tools []*schema.ToolInfo) (*schema.StreamReader[*schema.Message], error)
}

// providerModel adapts a Provider to eino's model.ToolCallingChatModel.
type providerModel struct {
	provider Provider
	base     model.BaseChatModel
	tools    []*schema.ToolInfo
}

func (m *providerModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return m.provider.Generate(ctx, input, m.tools)
}

func (m *providerModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return m.provider.Stream(ctx, input, m.tools)
}

func (m *providerModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return &providerModel{
		provider: m.provider,
		tools:    tools,
	}, nil
}

// ResolveModel takes a model spec string like "anthropic/claude-opus-4-6"
// or "openai/gpt-5.4" and returns a ToolCallingChatModel.
func ResolveModel(spec string, apiKeys map[string]string) (model.ToolCallingChatModel, error) {
	provider, _ := splitSpec(spec)
	switch provider {
	case "anthropic", "claude-sdk", "claude":
		return newAnthropicModel(spec, apiKeys["anthropic"])
	case "openai", "codex":
		return newOpenAIModel(spec, apiKeys["openai"])
	case "deepseek":
		return newDeepSeekModel(spec, apiKeys["deepseek"])
	case "google", "gemini":
		return newGeminiModel(spec, apiKeys["gemini"])
	default:
		return newAnthropicModel(spec, apiKeys["anthropic"])
	}
}

func splitSpec(spec string) (provider, modelID string) {
	for i := 0; i < len(spec); i++ {
		if spec[i] == '/' {
			return spec[:i], spec[i+1:]
		}
	}
	return "anthropic", spec
}

func InspectSpec(spec string) SpecInfo {
	provider, rest := splitSpec(spec)
	parts := strings.Split(rest, "/")
	modelID := rest
	effort := ""
	if len(parts) > 0 {
		modelID = parts[0]
	}
	if len(parts) > 1 {
		switch parts[1] {
		case "low", "medium", "high", "max":
			effort = parts[1]
		}
	}
	contextWindow := contextWindows[modelID]
	if contextWindow == 0 {
		contextWindow = 200_000
	}
	return SpecInfo{
		Provider:       provider,
		ModelID:        modelID,
		Effort:         effort,
		ContextWindow:  contextWindow,
		SupportsVision: visionModels[modelID],
	}
}
