package coordinator

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/verialabs/ctf-agent/internal/prompt"
	"github.com/verialabs/ctf-agent/internal/solver"
	"github.com/verialabs/ctf-agent/internal/swarm"
)

// Coordinator manages multiple challenge swarms and tracks overall competition state.
type Coordinator struct {
	challengesDir string
	modelSpecs    []string
	apiKeys       map[string]string
	sandboxImage  string
	maxConcurrent int
	skillsPrompt  string

	mu      sync.Mutex
	swarms  map[string]*swarm.Swarm
	results map[string]*solver.Result
	solved  map[string]bool
}

// New creates a new coordinator.
func New(challengesDir string, modelSpecs []string, apiKeys map[string]string, sandboxImage string, maxConcurrent int) *Coordinator {
	return NewWithSkills(challengesDir, modelSpecs, apiKeys, sandboxImage, maxConcurrent, "")
}

// NewWithSkills creates a coordinator with optional prompt skill content.
func NewWithSkills(challengesDir string, modelSpecs []string, apiKeys map[string]string, sandboxImage string, maxConcurrent int, skillsPrompt string) *Coordinator {
	return &Coordinator{
		challengesDir: challengesDir,
		modelSpecs:    modelSpecs,
		apiKeys:       apiKeys,
		sandboxImage:  sandboxImage,
		maxConcurrent: maxConcurrent,
		skillsPrompt:  skillsPrompt,
		swarms:        make(map[string]*swarm.Swarm),
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
	challenges, err := c.DiscoverChallenges()
	if err != nil {
		log.Printf("[coordinator] discover: %v", err)
		return nil
	}

	log.Printf("[coordinator] Found %d challenges", len(challenges))

	// Semaphore for concurrency control
	sem := make(chan struct{}, c.maxConcurrent)
	var wg sync.WaitGroup

	for _, ch := range challenges {
		wg.Add(1)
		go func(challenge string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result := c.solveOne(ctx, challenge)
			c.mu.Lock()
			c.results[challenge] = result
			if result.Status == solver.FlagFound {
				c.solved[challenge] = true
			}
			c.mu.Unlock()
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

func (c *Coordinator) solveOne(ctx context.Context, challenge string) *solver.Result {
	dir := filepath.Join(c.challengesDir, challenge)
	metaPath := filepath.Join(dir, "metadata.yml")

	meta, err := prompt.LoadMeta(metaPath)
	if err != nil {
		return &solver.Result{Status: solver.Error, Findings: []string{fmt.Sprintf("load meta: %v", err)}}
	}

	sysPrompt := prompt.Build(meta, filepath.Join(dir, "distfiles"), filepath.Join(dir, "workspace"))
	if c.skillsPrompt != "" {
		sysPrompt += "\n\n" + c.skillsPrompt
	}

	log.Printf("[coordinator] Starting swarm for %s (%s)", challenge, meta.Category)
	sw := swarm.New(challenge, dir, c.modelSpecs, c.apiKeys, c.sandboxImage)
	c.mu.Lock()
	c.swarms[challenge] = sw
	c.mu.Unlock()

	result := sw.Run(ctx, sysPrompt)
	log.Printf("[coordinator] Challenge %s: status=%d flag=%s", challenge, result.Status, result.Flag)
	return result
}

// Summary prints a results summary.
func (c *Coordinator) Summary() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	solved := 0
	total := len(c.results)
	for _, r := range c.results {
		if r.Status == solver.FlagFound {
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
