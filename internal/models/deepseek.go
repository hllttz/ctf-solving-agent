package models

import (
	"fmt"

	"github.com/cloudwego/eino/components/model"
)

func newDeepSeekModel(spec, apiKey string) (model.ToolCallingChatModel, error) {
	_, modelID := splitSpec(spec)
	if apiKey == "" {
		return nil, fmt.Errorf("deepseek: DEEPSEEK_API_KEY not set")
	}

	// DeepSeek uses OpenAI-compatible API
	return &providerModel{
		provider: &openaiProvider{
			modelID: modelID,
			apiKey:  apiKey,
			baseURL: "https://api.deepseek.com/v1/chat/completions",
		},
	}, nil
}
