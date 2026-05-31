package models

import (
	"encoding/json"
	"testing"
)

func TestOpenAIBaseURLDefault(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "")
	if got := openAIBaseURL(); got != "https://api.openai.com/v1/chat/completions" {
		t.Fatalf("base url = %q", got)
	}
}

func TestOpenAIBaseURLFromEnv(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "https://proxy.example.com/v1/chat/completions")
	if got := openAIBaseURL(); got != "https://proxy.example.com/v1/chat/completions" {
		t.Fatalf("base url = %q", got)
	}
}

func TestConvertOpenAIResponseIncludesUsage(t *testing.T) {
	var resp openaiResponse
	if err := json.Unmarshal([]byte(`{
		"choices": [{"message": {"role": "assistant", "content": "done"}, "finish_reason": "stop"}],
		"usage": {
			"prompt_tokens": 100,
			"completion_tokens": 20,
			"total_tokens": 120,
			"prompt_tokens_details": {"cached_tokens": 40}
		}
	}`), &resp); err != nil {
		t.Fatal(err)
	}

	msg := convertOpenAIResponse(&resp)
	if msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		t.Fatal("missing usage")
	}
	if msg.ResponseMeta.Usage.PromptTokens != 100 || msg.ResponseMeta.Usage.CompletionTokens != 20 {
		t.Fatalf("usage = %#v", msg.ResponseMeta.Usage)
	}
	if msg.ResponseMeta.Usage.PromptTokenDetails.CachedTokens != 40 {
		t.Fatalf("cached = %d", msg.ResponseMeta.Usage.PromptTokenDetails.CachedTokens)
	}
}

func TestConvertOpenAIResponseContentArray(t *testing.T) {
	var resp openaiResponse
	if err := json.Unmarshal([]byte(`{
		"choices": [{
			"message": {
				"role": "assistant",
				"content": [
					{"type": "text", "text": "I will inspect the archive. "},
					{"type": "text", "text": "Then I will identify the binary."}
				]
			},
			"finish_reason": "stop"
		}]
	}`), &resp); err != nil {
		t.Fatal(err)
	}

	msg := convertOpenAIResponse(&resp)
	if msg.Content != "I will inspect the archive. Then I will identify the binary." {
		t.Fatalf("content = %q", msg.Content)
	}
}
