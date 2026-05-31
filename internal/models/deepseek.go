package models

import (
	"fmt"
	"os"
	"strings"

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
			baseURL: deepSeekBaseURL(),
		},
	}, nil
}

func deepSeekBaseURL() string {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("DEEPSEEK_BASE_URL")), "/")
	if baseURL == "" {
		return "https://api.deepseek.com/chat/completions"
	}
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	return baseURL + "/chat/completions"
}
