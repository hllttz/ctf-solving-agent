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

	"github.com/verialabs/ctf-agent/internal/challenge"
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
	rootCmd.AddCommand(runCmd(cfg))

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
			if url, err := coord.StartOperatorServer(ctx, cfg.MsgAddr); err != nil {
				log.Printf("operator message server disabled: %v", err)
			} else {
				log.Printf("operator message server listening on %s/msg", url)
			}

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
			return runSingleChallenge(cfg, args[0])
		},
	}
}

func runCmd(cfg *config.Config) *cobra.Command {
	var target string
	var category string
	var name string
	var description string
	var files []string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Create a challenge from target/files and solve it",
		Example: "  ctf-agent run --target \"nc host 31337\" --file ./chall.zip --category pwn\n" +
			"  ctf-agent run --target http://host:8080 --file ./source.zip --category web --name baby-web",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("run does not accept positional args; use --target and --file")
			}
			if target == "" && len(files) == 0 {
				return fmt.Errorf("provide at least --target or --file")
			}

			created, err := challenge.CreateManual(challenge.ManualOptions{
				Root:        cfg.ChallengesDir,
				Name:        name,
				Category:    category,
				Target:      target,
				Description: description,
				Files:       files,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Created challenge: %s\n", created.Dir)
			return runSingleChallenge(cfg, created.Dir)
		},
	}

	cmd.Flags().StringVar(&target, "target", "", "Target connection string, e.g. \"nc host 31337\" or \"http://host:8080\"")
	cmd.Flags().StringArrayVar(&files, "file", nil, "Attachment file path; repeat for multiple files")
	cmd.Flags().StringVar(&category, "category", "misc", "Challenge category, e.g. pwn, web, crypto, rev, forensics")
	cmd.Flags().StringVar(&name, "name", "", "Challenge name; inferred from file or target when omitted")
	cmd.Flags().StringVar(&description, "description", "", "Optional challenge description")
	return cmd
}

func runSingleChallenge(cfg *config.Config, challengeDir string) error {
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
}
