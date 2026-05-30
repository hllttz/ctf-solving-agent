package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/verialabs/ctf-agent/internal/config"
	"github.com/verialabs/ctf-agent/internal/coordinator"
	"github.com/verialabs/ctf-agent/internal/cost"
	"github.com/verialabs/ctf-agent/internal/prompt"
	"github.com/verialabs/ctf-agent/internal/sandbox"
	"github.com/verialabs/ctf-agent/internal/skills"
	"github.com/verialabs/ctf-agent/internal/solver"
	"github.com/verialabs/ctf-agent/internal/swarm"
)

func main() {
	cfg := config.Load()

	rootCmd := &cobra.Command{
		Use:   "ctf-agent",
		Short: "Autonomous CTF solving agent",
		Long:  "ctf-agent races multiple AI models against CTF challenges in parallel Docker sandboxes.",
	}

	rootCmd.AddCommand(solveCmd(cfg))
	rootCmd.AddCommand(singleCmd(cfg))

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func solveCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "solve [challenges-dir]",
		Short: "Solve all challenges in a directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			challengesDir := cfg.ChallengesDir
			if len(args) > 0 {
				challengesDir = args[0]
			}

			apiKeys := map[string]string{
				"anthropic": cfg.AnthropicAPIKey,
				"openai":    cfg.OpenAIAPIKey,
				"gemini":    cfg.GeminiAPIKey,
				"deepseek":  cfg.DeepSeekAPIKey,
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				log.Println("Shutting down...")
				cancel()
			}()

			costTracker := cost.NewTracker()
			skillsPrompt, err := skills.LoadDir(cfg.SkillsDir)
			if err != nil {
				return err
			}
			if err := sandbox.CleanupOrphans(ctx); err != nil {
				log.Printf("sandbox cleanup skipped: %v", err)
			}

			coord := coordinator.NewWithOptions(challengesDir,
				cfg.ModelSpecs, apiKeys, cfg.SandboxImage, cfg.MemoryLimit, cfg.MaxConcurrent, skillsPrompt)

			results := coord.SolveAll(ctx)

			fmt.Println(coord.Summary())
			fmt.Println()
			fmt.Println(costTracker.Summary())

			fmt.Println("\nResults:")
			for name, result := range results {
				status := "unsolved"
				if result.Status == solver.FlagFound {
					status = "SOLVED: " + result.Flag
				}
				fmt.Printf("  %s: %s\n", name, status)
			}

			return nil
		},
	}
}

func singleCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "single <challenge-dir>",
		Short: "Solve a single challenge",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			challengeDir := args[0]

			metaPath := filepath.Join(challengeDir, "metadata.yml")
			meta, err := prompt.LoadMeta(metaPath)
			if err != nil {
				return fmt.Errorf("load metadata: %w", err)
			}

			sysPrompt := prompt.Build(meta,
				filepath.Join(challengeDir, "distfiles"),
				filepath.Join(challengeDir, "workspace"))
			skillsPrompt, err := skills.LoadDir(cfg.SkillsDir)
			if err != nil {
				return err
			}
			if skillsPrompt != "" {
				sysPrompt += "\n\n" + skillsPrompt
			}

			apiKeys := map[string]string{
				"anthropic": cfg.AnthropicAPIKey,
				"openai":    cfg.OpenAIAPIKey,
				"gemini":    cfg.GeminiAPIKey,
				"deepseek":  cfg.DeepSeekAPIKey,
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				cancel()
			}()

			fmt.Printf("Solving: %s (%s)\n", meta.Name, meta.Category)
			fmt.Printf("Models: %v\n\n", cfg.ModelSpecs)
			if err := sandbox.CleanupOrphans(ctx); err != nil {
				log.Printf("sandbox cleanup skipped: %v", err)
			}

			sw := swarm.NewWithOptions(meta.Name, challengeDir, cfg.ModelSpecs, apiKeys, cfg.SandboxImage, cfg.MemoryLimit)
			result := sw.Run(ctx, sysPrompt)

			fmt.Println()
			fmt.Printf("Status: %d\n", result.Status)
			if result.Flag != "" {
				fmt.Printf("Flag: %s\n", result.Flag)
			}
			fmt.Printf("Method: %s\n", result.Method)
			fmt.Printf("Steps: %d\n", result.Steps)

			return nil
		},
	}
}
