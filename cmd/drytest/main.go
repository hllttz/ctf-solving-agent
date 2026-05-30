package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/verialabs/ctf-agent/internal/bus"
	"github.com/verialabs/ctf-agent/internal/config"
	"github.com/verialabs/ctf-agent/internal/models"
	"github.com/verialabs/ctf-agent/internal/prompt"
	"github.com/verialabs/ctf-agent/internal/sandbox"
	"github.com/verialabs/ctf-agent/internal/solver"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: drytest <challenge-dir>\n")
		os.Exit(1)
	}

	challengeDir := os.Args[1]
	cfg := config.Load()

	// Load challenge metadata
	metaPath := filepath.Join(challengeDir, "metadata.yml")
	meta, err := prompt.LoadMeta(metaPath)
	if err != nil {
		log.Fatalf("load meta: %v", err)
	}

	sysPrompt := prompt.Build(meta,
		filepath.Join(challengeDir, "distfiles"),
		filepath.Join(challengeDir, "workspace"))

	fmt.Printf("=== System Prompt ===\n%s\n\n", sysPrompt)
	fmt.Printf("=== Starting solver ===\n\n")

	// Get API keys
	apiKeys := map[string]string{
		"anthropic": cfg.AnthropicAPIKey,
		"openai":    cfg.OpenAIAPIKey,
		"gemini":    cfg.GeminiAPIKey,
		"deepseek":  cfg.DeepSeekAPIKey,
	}

	// Resolve model
	modelSpec := "anthropic/claude-sonnet-4-6"
	if len(cfg.ModelSpecs) > 0 {
		modelSpec = cfg.ModelSpecs[0]
	}
	fmt.Printf("Using model: %s\n", modelSpec)

	m, err := models.ResolveModel(modelSpec, apiKeys)
	if err != nil {
		log.Fatalf("resolve model: %v", err)
	}

	// Create dry-run sandbox
	sb := sandbox.NewDryRun("/workspace", "/challenge/distfiles")

	// Create solver
	b := bus.New()
	s := solver.New(m, sb, b)

	// Run
	ctx := context.Background()
	resultCh := s.Run(ctx, sysPrompt)
	result := <-resultCh

	fmt.Printf("\n=== Result ===\n")
	fmt.Printf("Status: %d\n", result.Status)
	fmt.Printf("Flag: %s\n", result.Flag)
	fmt.Printf("Method: %s\n", result.Method)
	for i, f := range result.Findings {
		fmt.Printf("Finding[%d]: %s\n", i, f)
	}
}
