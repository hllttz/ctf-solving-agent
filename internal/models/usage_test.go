package models

import (
	"encoding/json"
	"testing"
)

func TestConvertAnthropicResponseIncludesUsage(t *testing.T) {
	var resp anthropicResponse
	if err := json.Unmarshal([]byte(`{
		"content": [{"type": "text", "text": "done"}],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 70, "output_tokens": 9}
	}`), &resp); err != nil {
		t.Fatal(err)
	}

	msg := convertAnthropicResponse(&resp)
	if msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		t.Fatal("missing usage")
	}
	if msg.ResponseMeta.FinishReason != "end_turn" {
		t.Fatalf("finish reason = %q", msg.ResponseMeta.FinishReason)
	}
	if msg.ResponseMeta.Usage.PromptTokens != 70 || msg.ResponseMeta.Usage.CompletionTokens != 9 {
		t.Fatalf("usage = %#v", msg.ResponseMeta.Usage)
	}
}

func TestConvertGeminiResponseIncludesUsage(t *testing.T) {
	var resp geminiResponse
	if err := json.Unmarshal([]byte(`{
		"candidates": [{"content": {"role": "model", "parts": [{"text": "done"}]}, "finishReason": "STOP"}],
		"usageMetadata": {
			"promptTokenCount": 50,
			"candidatesTokenCount": 11,
			"totalTokenCount": 61,
			"cachedContentTokenCount": 20
		}
	}`), &resp); err != nil {
		t.Fatal(err)
	}

	msg := convertGeminiResponse(&resp)
	if msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		t.Fatal("missing usage")
	}
	if msg.ResponseMeta.FinishReason != "STOP" {
		t.Fatalf("finish reason = %q", msg.ResponseMeta.FinishReason)
	}
	if msg.ResponseMeta.Usage.PromptTokens != 50 || msg.ResponseMeta.Usage.CompletionTokens != 11 {
		t.Fatalf("usage = %#v", msg.ResponseMeta.Usage)
	}
	if msg.ResponseMeta.Usage.PromptTokenDetails.CachedTokens != 20 {
		t.Fatalf("cached = %d", msg.ResponseMeta.Usage.PromptTokenDetails.CachedTokens)
	}
}
