package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/verialabs/ctf-agent/internal/challenge"
	"github.com/verialabs/ctf-agent/internal/config"
	"github.com/verialabs/ctf-agent/internal/coordinator"
	"github.com/verialabs/ctf-agent/internal/cost"
	"github.com/verialabs/ctf-agent/internal/prompt"
	"github.com/verialabs/ctf-agent/internal/sandbox"
	"github.com/verialabs/ctf-agent/internal/solver"
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
			if err := validateModelSpecs(cfg.ModelSpecs, apiKeys); err != nil {
				return err
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
			if err := sandbox.CleanupOrphans(ctx); err != nil {
				log.Printf("sandbox cleanup skipped: %v", err)
			}

			printRunHeader("Solving challenge directory", challengesDir, cfg.ModelSpecs, cfg.SandboxImage, cfg.MemoryLimit)
			coord := coordinator.NewWithOptionsAndTracker(challengesDir,
				cfg.ModelSpecs, apiKeys, cfg.SandboxImage, cfg.MemoryLimit, cfg.MaxConcurrent, cfg.SkillsDir, costTracker)
			if url, err := coord.StartOperatorServer(ctx, cfg.MsgAddr); err != nil {
				log.Printf("operator message server disabled: %v", err)
			} else {
				printOperatorURLs(url)
			}

			results := coord.SolveAll(ctx)

			printResults(results)
			fmt.Println(costTracker.Summary())

			return nil
		},
	}
}

func singleCmd(cfg *config.Config) *cobra.Command {
	var waitUI bool
	cmd := &cobra.Command{
		Use:   "single <challenge-dir>",
		Short: "Solve a single challenge",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSingleChallenge(cfg, args[0], waitUI)
		},
	}
	cmd.Flags().BoolVar(&waitUI, "wait-ui", false, "Keep the operator UI running after the challenge finishes")
	return cmd
}

func runCmd(cfg *config.Config) *cobra.Command {
	var target string
	var category string
	var name string
	var description string
	var files []string
	var waitUI bool

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
			return runSingleChallenge(cfg, created.Dir, waitUI)
		},
	}

	cmd.Flags().StringVar(&target, "target", "", "Target connection string, e.g. \"nc host 31337\" or \"http://host:8080\"")
	cmd.Flags().StringArrayVar(&files, "file", nil, "Attachment file path; repeat for multiple files")
	cmd.Flags().StringVar(&category, "category", "misc", "Challenge category, e.g. pwn, web, crypto, rev, forensics")
	cmd.Flags().StringVar(&name, "name", "", "Challenge name; inferred from file or target when omitted")
	cmd.Flags().StringVar(&description, "description", "", "Optional challenge description")
	cmd.Flags().BoolVar(&waitUI, "wait-ui", false, "Keep the operator UI running after the challenge finishes")
	return cmd
}

func runSingleChallenge(cfg *config.Config, challengeDir string, waitUI bool) error {
	metaPath := filepath.Join(challengeDir, "metadata.yml")
	meta, err := prompt.LoadMeta(metaPath)
	if err != nil {
		return fmt.Errorf("load metadata: %w", err)
	}

	apiKeys := map[string]string{
		"anthropic": cfg.AnthropicAPIKey,
		"openai":    cfg.OpenAIAPIKey,
		"gemini":    cfg.GeminiAPIKey,
		"deepseek":  cfg.DeepSeekAPIKey,
	}
	if err := validateModelSpecs(cfg.ModelSpecs, apiKeys); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	printRunHeader(fmt.Sprintf("Solving %s (%s)", meta.Name, meta.Category), challengeDir, cfg.ModelSpecs, cfg.SandboxImage, cfg.MemoryLimit)
	if err := sandbox.CleanupOrphans(ctx); err != nil {
		log.Printf("sandbox cleanup skipped: %v", err)
	}

	costTracker := cost.NewTracker()
	challengesDir := filepath.Dir(challengeDir)
	challengeName := filepath.Base(challengeDir)
	coord := coordinator.NewWithOptionsAndTracker(challengesDir,
		cfg.ModelSpecs, apiKeys, cfg.SandboxImage, cfg.MemoryLimit, 1, cfg.SkillsDir, costTracker)
	if url, err := coord.StartOperatorServer(ctx, cfg.MsgAddr); err != nil {
		log.Printf("operator message server disabled: %v", err)
	} else {
		printOperatorURLs(url)
	}
	result := coord.SolveChallenge(ctx, challengeName)

	printSingleResult(result)
	if result.Flag != "" {
		fmt.Printf("Flag:   %s\n", result.Flag)
	}
	if result.Method != "" {
		fmt.Printf("Method: %s\n", result.Method)
	}
	fmt.Printf("Steps:  %d\n", result.Steps)
	if result.LogPath != "" {
		fmt.Printf("Trace:  %s\n", result.LogPath)
	}
	if len(result.Findings) > 0 {
		fmt.Println("Findings:")
		for _, finding := range result.Findings {
			finding = strings.TrimSpace(finding)
			if finding == "" {
				continue
			}
			fmt.Printf("  - %s\n", truncateForCLI(finding, 500))
		}
	}
	fmt.Println()
	fmt.Println(costTracker.Summary())
	if waitUI {
		fmt.Println("UI is still running. Press Ctrl+C to stop.")
		<-ctx.Done()
	}

	return nil
}

func printOperatorURLs(url string) {
	fmt.Printf("Operator: %s\n", url)
	fmt.Printf("  ui:     %s/ui\n", url)
	fmt.Printf("  status: %s/status\n", url)
	fmt.Printf("  hint:   curl -X POST %s/msg -H 'Content-Type: application/json' -d '{\"message\":\"...\"}'\n\n", url)
}

func validateModelSpecs(specs []string, apiKeys map[string]string) error {
	if len(specs) == 0 {
		return fmt.Errorf("MODEL_SPECS is empty")
	}
	missing := map[string]string{}
	for _, spec := range specs {
		provider := providerForSpec(spec)
		if apiKeys[provider] == "" {
			missing[provider] = envForProvider(provider)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	providers := make([]string, 0, len(missing))
	for provider := range missing {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	var parts []string
	for _, provider := range providers {
		parts = append(parts, fmt.Sprintf("%s for %s models", missing[provider], provider))
	}
	return fmt.Errorf("missing API key: %s", strings.Join(parts, ", "))
}

func providerForSpec(spec string) string {
	provider := spec
	if i := strings.IndexByte(spec, '/'); i >= 0 {
		provider = spec[:i]
	}
	switch provider {
	case "anthropic", "claude-sdk", "claude":
		return "anthropic"
	case "google", "gemini":
		return "gemini"
	case "deepseek":
		return "deepseek"
	case "openai", "codex":
		return "openai"
	default:
		return "anthropic"
	}
}

func envForProvider(provider string) string {
	switch provider {
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "openai":
		return "OPENAI_API_KEY"
	case "gemini":
		return "GEMINI_API_KEY"
	case "deepseek":
		return "DEEPSEEK_API_KEY"
	default:
		return strings.ToUpper(provider) + "_API_KEY"
	}
}

func printRunHeader(title, path string, models []string, image, memory string) {
	fmt.Println(title)
	fmt.Printf("Path:    %s\n", path)
	fmt.Printf("Models:  %s\n", strings.Join(models, ", "))
	fmt.Printf("Sandbox: %s", image)
	if memory != "" {
		fmt.Printf(" (%s)", memory)
	}
	fmt.Println()
	fmt.Println()
}

func printResults(results map[string]*solver.Result) {
	names := make([]string, 0, len(results))
	for name := range results {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Printf("Results (%d)\n", len(names))
	fmt.Println("=======")
	for _, name := range names {
		result := results[name]
		fmt.Printf("  %s: %s", name, statusText(result))
		if result != nil && result.Flag != "" {
			fmt.Printf(" %s", result.Flag)
		}
		if result != nil && result.LogPath != "" {
			fmt.Printf(" (%s)", result.LogPath)
		}
		fmt.Println()
	}
	fmt.Println()
}

func printSingleResult(result *solver.Result) {
	fmt.Println()
	fmt.Printf("Status: %s\n", statusText(result))
}

func statusText(result *solver.Result) string {
	if result == nil {
		return "unknown"
	}
	switch result.Status {
	case solver.Running:
		return "running"
	case solver.FlagFound:
		return "solved"
	case solver.GaveUp:
		return "gave up"
	case solver.Error:
		return "error"
	case solver.Cancelled:
		return "cancelled"
	default:
		return fmt.Sprintf("status %d", result.Status)
	}
}

func truncateForCLI(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max] + "... [truncated]"
}
