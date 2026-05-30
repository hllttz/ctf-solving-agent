package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	CTFDURL   string
	CTFDToken string

	AnthropicAPIKey string
	OpenAIAPIKey    string
	GeminiAPIKey    string
	DeepSeekAPIKey  string

	SandboxImage  string
	MaxConcurrent int
	MemoryLimit   string

	ModelSpecs         []string
	CoordinatorModel   string
	CoordinatorBackend string
	SingleChallenge    string
	ChallengesDir      string
	SkillsDir          string
	NoSubmit           bool
	Verbose            bool
}

func Load() *Config {
	return &Config{
		CTFDURL:   getEnv("CTFD_URL", ""),
		CTFDToken: getEnv("CTFD_TOKEN", ""),

		AnthropicAPIKey: getEnv("ANTHROPIC_API_KEY", ""),
		OpenAIAPIKey:    getEnv("OPENAI_API_KEY", ""),
		GeminiAPIKey:    getEnv("GEMINI_API_KEY", ""),
		DeepSeekAPIKey:  getEnv("DEEPSEEK_API_KEY", ""),

		SandboxImage:  getEnv("SANDBOX_IMAGE", "ctf-sandbox"),
		MaxConcurrent: getEnvInt("MAX_CONCURRENT_CHALLENGES", 10),
		MemoryLimit:   getEnv("CONTAINER_MEMORY_LIMIT", "16g"),

		ModelSpecs: splitEnv("MODEL_SPECS", []string{
			"claude-sdk/claude-opus-4-6",
			"claude-sdk/claude-sonnet-4-6",
			"openai/gpt-5.4",
		}),
		CoordinatorModel:   getEnv("COORDINATOR_MODEL", "claude-sdk/claude-opus-4-6"),
		CoordinatorBackend: getEnv("COORDINATOR_BACKEND", "claude"),
		ChallengesDir:      getEnv("CHALLENGES_DIR", "./challenges"),
		SkillsDir:          getEnv("SKILLS_DIR", "./skills"),
		NoSubmit:           getEnvBool("NO_SUBMIT", false),
		Verbose:            getEnvBool("VERBOSE", false),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return def
}

func splitEnv(key string, def []string) []string {
	if v := os.Getenv(key); v != "" {
		parts := strings.Split(v, ",")
		res := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				res = append(res, p)
			}
		}
		if len(res) > 0 {
			return res
		}
	}
	return def
}
