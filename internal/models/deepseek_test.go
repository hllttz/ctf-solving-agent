package models

import "testing"

func TestDeepSeekBaseURLDefault(t *testing.T) {
	t.Setenv("DEEPSEEK_BASE_URL", "")
	if got := deepSeekBaseURL(); got != "https://api.deepseek.com/chat/completions" {
		t.Fatalf("base url = %q", got)
	}
}

func TestDeepSeekBaseURLFromRoot(t *testing.T) {
	t.Setenv("DEEPSEEK_BASE_URL", "https://proxy.example.com")
	if got := deepSeekBaseURL(); got != "https://proxy.example.com/chat/completions" {
		t.Fatalf("base url = %q", got)
	}
}

func TestDeepSeekBaseURLFromFullEndpoint(t *testing.T) {
	t.Setenv("DEEPSEEK_BASE_URL", "https://proxy.example.com/v1/chat/completions")
	if got := deepSeekBaseURL(); got != "https://proxy.example.com/v1/chat/completions" {
		t.Fatalf("base url = %q", got)
	}
}

func TestNewDeepSeekModelRequiresAPIKey(t *testing.T) {
	if _, err := newDeepSeekModel("deepseek/deepseek-v4-flash", ""); err == nil {
		t.Fatal("expected missing key error")
	}
}

func TestResolveDeepSeekModel(t *testing.T) {
	m, err := ResolveModel("deepseek/deepseek-v4-flash", map[string]string{"deepseek": "sk-test"})
	if err != nil {
		t.Fatalf("ResolveModel error: %v", err)
	}
	pm, ok := m.(*providerModel)
	if !ok {
		t.Fatalf("model type = %T", m)
	}
	provider, ok := pm.provider.(*openaiProvider)
	if !ok {
		t.Fatalf("provider type = %T", pm.provider)
	}
	if provider.modelID != "deepseek-v4-flash" {
		t.Fatalf("model id = %q", provider.modelID)
	}
}
