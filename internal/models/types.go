package models

import (
	"context"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

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
