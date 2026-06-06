package coordinator

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/verialabs/ctf-agent/internal/cost"
	"github.com/verialabs/ctf-agent/internal/prompt"
	"github.com/verialabs/ctf-agent/internal/skills"
	"github.com/verialabs/ctf-agent/internal/solver"
	"github.com/verialabs/ctf-agent/internal/swarm"
)

// Coordinator manages multiple challenge swarms and tracks overall competition state.
type Coordinator struct {
	challengesDir string
	modelSpecs    []string
	apiKeys       map[string]string
	sandboxImage  string
	memoryLimit   string
	maxConcurrent int
	skillsDir     string
	costs         *cost.Tracker

	mu      sync.Mutex
	swarms  map[string]*swarm.Swarm
	pending map[string]bool
	results map[string]*solver.Result
	solved  map[string]bool
	ctx     context.Context
}

// New creates a new coordinator.
func New(challengesDir string, modelSpecs []string, apiKeys map[string]string, sandboxImage string, maxConcurrent int) *Coordinator {
	return NewWithOptions(challengesDir, modelSpecs, apiKeys, sandboxImage, "16g", maxConcurrent, "")
}

// NewWithSkills creates a coordinator with optional skill directory.
func NewWithSkills(challengesDir string, modelSpecs []string, apiKeys map[string]string, sandboxImage string, maxConcurrent int, skillsDir string) *Coordinator {
	return NewWithOptions(challengesDir, modelSpecs, apiKeys, sandboxImage, "16g", maxConcurrent, skillsDir)
}

// NewWithOptions creates a coordinator with sandbox and prompt options.
func NewWithOptions(challengesDir string, modelSpecs []string, apiKeys map[string]string, sandboxImage, memoryLimit string, maxConcurrent int, skillsDir string) *Coordinator {
	return NewWithOptionsAndTracker(challengesDir, modelSpecs, apiKeys, sandboxImage, memoryLimit, maxConcurrent, skillsDir, nil)
}

func NewWithOptionsAndTracker(challengesDir string, modelSpecs []string, apiKeys map[string]string, sandboxImage, memoryLimit string, maxConcurrent int, skillsDir string, costs *cost.Tracker) *Coordinator {
	return &Coordinator{
		challengesDir: challengesDir,
		modelSpecs:    modelSpecs,
		apiKeys:       apiKeys,
		sandboxImage:  sandboxImage,
		memoryLimit:   memoryLimit,
		maxConcurrent: maxConcurrent,
		skillsDir:     skillsDir,
		costs:         costs,
		swarms:        make(map[string]*swarm.Swarm),
		pending:       make(map[string]bool),
		results:       make(map[string]*solver.Result),
		solved:        make(map[string]bool),
	}
}

// DiscoverChallenges reads the challenges directory and returns challenge names.
func (c *Coordinator) DiscoverChallenges() ([]string, error) {
	entries, err := os.ReadDir(c.challengesDir)
	if err != nil {
		return nil, fmt.Errorf("read challenges dir: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		metaPath := filepath.Join(c.challengesDir, e.Name(), "metadata.yml")
		if _, err := os.Stat(metaPath); err == nil {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// SolveAll runs swarms for all challenges, limited by maxConcurrent.
func (c *Coordinator) SolveAll(ctx context.Context) map[string]*solver.Result {
	c.setContext(ctx)
	challenges, err := c.DiscoverChallenges()
	if err != nil {
		log.Printf("[coordinator] discover: %v", err)
		return nil
	}

	log.Printf("[coordinator] Found %d challenges", len(challenges))

	// Semaphore for concurrency control
	sem := make(chan struct{}, c.concurrencyLimit(len(challenges)))
	var wg sync.WaitGroup

	for _, ch := range challenges {
		wg.Add(1)
		go func(challenge string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			c.mu.Lock()
			if !c.reserveChallengeLocked(challenge) {
				c.mu.Unlock()
				return
			}
			c.mu.Unlock()

			result := c.solveOne(ctx, challenge)
			c.recordResult(challenge, result)
		}(ch)
	}

	wg.Wait()

	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]*solver.Result, len(c.results))
	for k, v := range c.results {
		out[k] = v
	}
	return out
}

// SolveChallenge runs one named challenge through the coordinator and records
// its result for the operator UI/status endpoints.
func (c *Coordinator) SolveChallenge(ctx context.Context, challenge string) *solver.Result {
	c.setContext(ctx)
	result := c.solveOne(ctx, challenge)
	c.recordResult(challenge, result)
	return result
}

func (c *Coordinator) setContext(ctx context.Context) {
	if ctx == nil {
		return
	}
	c.mu.Lock()
	c.ctx = ctx
	c.mu.Unlock()
}

func (c *Coordinator) context() context.Context {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ctx != nil {
		return c.ctx
	}
	return context.Background()
}

func (c *Coordinator) solveOne(ctx context.Context, challenge string) *solver.Result {
	dir := filepath.Join(c.challengesDir, challenge)
	metaPath := filepath.Join(dir, "metadata.yml")

	meta, err := prompt.LoadMeta(metaPath)
	if err != nil {
		return &solver.Result{Status: solver.Error, Findings: []string{fmt.Sprintf("load meta: %v", err)}}
	}

	sysPrompt := prompt.Build(meta, filepath.Join(dir, "distfiles"), filepath.Join(dir, "workspace"))
	skillsPrompt, err := skills.LoadForCategory(c.skillsDir, meta.Category)
	if err != nil {
		return &solver.Result{Status: solver.Error, Findings: []string{fmt.Sprintf("load skills: %v", err)}}
	}
	if skillsPrompt != "" {
		sysPrompt += "\n\n" + skillsPrompt
	}

	log.Printf("[coordinator] Starting swarm for %s (%s)", challenge, meta.Category)
	sw := swarm.NewWithOptionsAndTracker(challenge, dir, c.modelSpecs, c.apiKeys, c.sandboxImage, c.memoryLimit, initialStrategy(meta), c.costs)
	c.mu.Lock()
	c.swarms[challenge] = sw
	c.mu.Unlock()

	result := sw.Run(ctx, sysPrompt)
	log.Printf("[coordinator] Challenge %s: status=%d flag=%s", challenge, result.Status, result.Flag)
	return result
}

func (c *Coordinator) Spawn(ctx context.Context, challenge string) bool {
	if ctx == nil {
		ctx = c.context()
	} else {
		c.setContext(ctx)
	}
	challenge = strings.TrimSpace(challenge)
	if challenge == "" {
		return false
	}
	c.mu.Lock()
	if !c.reserveChallengeLocked(challenge) {
		c.mu.Unlock()
		return false
	}
	c.mu.Unlock()

	go func() {
		result := c.solveOne(ctx, challenge)
		c.recordResult(challenge, result)
	}()
	return true
}

func (c *Coordinator) reserveChallengeLocked(challenge string) bool {
	if strings.TrimSpace(challenge) == "" {
		return false
	}
	if sw := c.swarms[challenge]; sw != nil {
		return false
	}
	if c.pending[challenge] {
		return false
	}
	if c.maxConcurrent > 0 && c.activeCountLocked() >= c.maxConcurrent {
		return false
	}
	c.pending[challenge] = true
	return true
}

func (c *Coordinator) recordResult(challenge string, result *solver.Result) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.results[challenge] = result
	delete(c.pending, challenge)
	delete(c.swarms, challenge)
	if result != nil && result.Status == solver.FlagFound {
		c.solved[challenge] = true
	}
}

func (c *Coordinator) activeCountLocked() int {
	active := len(c.swarms)
	for name := range c.pending {
		if c.swarms[name] == nil {
			active++
		}
	}
	return active
}

func (c *Coordinator) concurrencyLimit(challengeCount int) int {
	if challengeCount <= 0 {
		return 1
	}
	if c.maxConcurrent <= 0 || c.maxConcurrent > challengeCount {
		return challengeCount
	}
	return c.maxConcurrent
}

func initialStrategy(meta *prompt.Meta) string {
	if meta == nil {
		return ""
	}
	var parts []string
	if meta.Host != "" && meta.Port > 0 {
		parts = append(parts, "Coordinator: prioritize the live service first; verify behavior remotely before spending time on local files.")
	}
	switch strings.ToLower(meta.Category) {
	case "web", "web exploitation":
		parts = append(parts, "Coordinator: enumerate HTTP surface early: headers, cookies, source, robots.txt, obvious params, then test injection/path traversal/SSRF paths.")
	case "pwn", "binary exploitation":
		parts = append(parts, "Coordinator: capture service I/O, run file/checksec, then build a minimal pwntools harness before deeper reversing.")
	case "rev", "reverse engineering":
		parts = append(parts, "Coordinator: start with file/strings, identify validation logic, and use pyghidra/r2 for focused decompilation.")
	case "crypto", "cryptography":
		parts = append(parts, "Coordinator: identify primitives and parameters first; look for weak randomness, reused nonce, small factors, padding oracles, or encoding layers.")
	case "forensics", "forensic":
		parts = append(parts, "Coordinator: preserve originals, inspect metadata/strings/binwalk first, then carve or repair files only in /workspace.")
	}
	return strings.Join(parts, "\n")
}

// Summary prints a results summary.
func (c *Coordinator) Summary() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	solved := 0
	total := len(c.results)
	for _, r := range c.results {
		if r != nil && r.Status == solver.FlagFound {
			solved++
		}
	}
	return fmt.Sprintf("Solved %d/%d challenges", solved, total)
}

// Results returns all results.
func (c *Coordinator) Results() map[string]*solver.Result {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]*solver.Result, len(c.results))
	for k, v := range c.results {
		out[k] = v
	}
	return out
}

func (c *Coordinator) CostSnapshot() map[string]cost.Usage {
	if c.costs == nil {
		return map[string]cost.Usage{}
	}
	return c.costs.Snapshot()
}

func (c *Coordinator) TotalCost() float64 {
	if c.costs == nil {
		return 0
	}
	return c.costs.TotalCost()
}
