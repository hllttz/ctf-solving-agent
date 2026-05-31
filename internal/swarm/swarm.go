package swarm

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/verialabs/ctf-agent/internal/bus"
	"github.com/verialabs/ctf-agent/internal/cost"
	"github.com/verialabs/ctf-agent/internal/models"
	"github.com/verialabs/ctf-agent/internal/sandbox"
	"github.com/verialabs/ctf-agent/internal/solver"
)

// Swarm manages multiple solvers racing on a single challenge.
type Swarm struct {
	challengeName string
	challengeDir  string
	modelSpecs    []string
	apiKeys       map[string]string
	sandboxImage  string
	memoryLimit   string
	strategyHint  string
	costs         *cost.Tracker

	bus *bus.MessageBus

	mu      sync.Mutex
	results []*solver.Result
	solvers map[string]*solver.Solver
	done    chan struct{}
	cancel  context.CancelFunc
}

type solverInst struct {
	s       *solver.Solver
	sb      sandbox.Sandbox
	modelID string
}

// New creates a new swarm for a challenge.
func New(name, dir string, modelSpecs []string, apiKeys map[string]string, image string) *Swarm {
	return NewWithOptions(name, dir, modelSpecs, apiKeys, image, "16g")
}

func NewWithOptions(name, dir string, modelSpecs []string, apiKeys map[string]string, image, memoryLimit string) *Swarm {
	return NewWithStrategy(name, dir, modelSpecs, apiKeys, image, memoryLimit, "")
}

func NewWithStrategy(name, dir string, modelSpecs []string, apiKeys map[string]string, image, memoryLimit, strategyHint string) *Swarm {
	return NewWithOptionsAndTracker(name, dir, modelSpecs, apiKeys, image, memoryLimit, strategyHint, nil)
}

func NewWithOptionsAndTracker(name, dir string, modelSpecs []string, apiKeys map[string]string, image, memoryLimit, strategyHint string, costs *cost.Tracker) *Swarm {
	return &Swarm{
		challengeName: name,
		challengeDir:  dir,
		modelSpecs:    modelSpecs,
		apiKeys:       apiKeys,
		sandboxImage:  image,
		memoryLimit:   memoryLimit,
		strategyHint:  strategyHint,
		costs:         costs,
		bus:           bus.New(),
		solvers:       make(map[string]*solver.Solver),
		done:          make(chan struct{}),
	}
}

// Run starts all solvers in parallel and returns the first flag found.
func (s *Swarm) Run(ctx context.Context, systemPrompt string) *solver.Result {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()

	if strings.TrimSpace(s.strategyHint) != "" {
		s.bus.Broadcast(s.strategyHint)
	}

	// Create sandboxes and solvers
	var instances []solverInst
	var sandboxes []sandbox.Sandbox

	for i, spec := range s.modelSpecs {
		m, err := models.ResolveModel(spec, s.apiKeys)
		if err != nil {
			log.Printf("[swarm:%s] skip model %s: %v", s.challengeName, spec, err)
			continue
		}

		containerName := fmt.Sprintf("ctf-%s-%d", sanitize(s.challengeName), i)
		sb, err := sandbox.NewDockerWithOptions(ctx, sandbox.DockerOptions{
			Image:        s.sandboxImage,
			Name:         containerName,
			ChallengeDir: s.challengeDir,
			MemoryLimit:  s.memoryLimit,
		})
		if err != nil {
			log.Printf("[swarm:%s] sandbox create failed for %s: %v", s.challengeName, spec, err)
			continue
		}
		sandboxes = append(sandboxes, sb)

		inst := solverInst{
			s:       solver.NewWithOptions(m, sb, s.bus, spec, s.challengeName, s.costs),
			sb:      sb,
			modelID: spec,
		}
		s.mu.Lock()
		s.solvers[spec] = inst.s
		s.mu.Unlock()
		instances = append(instances, inst)
	}

	if len(instances) == 0 {
		return &solver.Result{Status: solver.Error, Findings: []string{"no solvers could be created"}}
	}

	defer func() {
		for _, sb := range sandboxes {
			sb.Stop(context.Background())
			sb.Remove(context.Background())
		}
	}()

	// Race: first solver to find flag wins
	resultCh := make(chan *solver.Result, len(instances))
	var wg sync.WaitGroup

	for _, inst := range instances {
		wg.Add(1)
		go func(is solverInst) {
			defer wg.Done()
			s.runSolverLoop(ctx, systemPrompt, is, resultCh)
		}(inst)
	}

	var firstResult *solver.Result
	for i := 0; i < len(instances); i++ {
		select {
		case result := <-resultCh:
			if result == nil {
				continue
			}
			if firstResult == nil {
				firstResult = result
			}
			if result.Status == solver.FlagFound {
				firstResult = result
				cancel()
				i = len(instances)
			}
		case <-ctx.Done():
			firstResult = &solver.Result{Status: solver.Cancelled, Findings: []string{ctx.Err().Error()}}
			i = len(instances)
		}
	}

	wg.Wait()

	if firstResult == nil {
		firstResult = &solver.Result{Status: solver.GaveUp, Findings: []string{"all solvers finished without finding a flag"}}
	}

	if firstResult.Status == solver.FlagFound {
		log.Printf("[swarm:%s] FLAG FOUND: %s", s.challengeName, firstResult.Flag)
	}

	return firstResult
}

func (s *Swarm) runSolverLoop(ctx context.Context, systemPrompt string, inst solverInst, resultCh chan<- *solver.Result) {
	const maxBumps = 3

	result := recvResult(ctx, inst.s.Run(ctx, systemPrompt))
	if result == nil {
		return
	}

	for bump := 1; result.Status != solver.FlagFound && bump <= maxBumps; bump++ {
		if result.Status == solver.Cancelled {
			break
		}
		if result.Status == solver.Error && result.Steps == 0 && len(result.Findings) > 0 {
			break
		}

		delay := time.Duration(bump*30) * time.Second
		if !sleepOrCancelled(ctx, delay) {
			break
		}

		insights := s.gatherSiblingInsights(inst.modelID)
		log.Printf("[swarm:%s/%s] bump %d with sibling insights", s.challengeName, inst.modelID, bump)
		next := recvResult(ctx, inst.s.Bump(ctx, systemPrompt, insights))
		if next == nil {
			break
		}
		result = next
	}

	select {
	case resultCh <- result:
	case <-ctx.Done():
	}
}

func recvResult(ctx context.Context, ch <-chan *solver.Result) *solver.Result {
	select {
	case result, ok := <-ch:
		if !ok {
			return nil
		}
		return result
	case <-ctx.Done():
		return &solver.Result{Status: solver.Cancelled, Findings: []string{ctx.Err().Error()}}
	}
}

func sleepOrCancelled(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *Swarm) gatherSiblingInsights(excludeModel string) string {
	findings := s.bus.All()
	parts := make([]string, 0, len(findings))
	for _, finding := range findings {
		if finding.Author == excludeModel || strings.TrimSpace(finding.Content) == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("[%s]: %s", finding.Author, finding.Content))
	}
	if len(parts) == 0 {
		return "No sibling insights available yet."
	}
	return strings.Join(parts, "\n\n")
}

// Bus returns the swarm's message bus.
func (s *Swarm) Bus() *bus.MessageBus { return s.bus }

func (s *Swarm) Kill() {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Swarm) Bump(modelSpec, insights string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.solvers[modelSpec]; !ok {
		return false
	}
	s.bus.Post(bus.CoordinatorAuthor, fmt.Sprintf("Targeted bump for %s: %s", modelSpec, insights))
	return true
}

func (s *Swarm) Status() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()

	agents := make(map[string]any, len(s.solvers))
	for spec, sol := range s.solvers {
		agent := map[string]any{
			"history_len": len(sol.History()),
		}
		if s.costs != nil {
			key := spec + "/" + sol.ModelID()
			if usage, ok := s.costs.Snapshot()[key]; ok {
				agent["usage"] = usage
				agent["cost_usd"] = usage.Cost()
			}
		}
		agents[spec] = agent
	}
	return map[string]any{
		"challenge": s.challengeName,
		"active":    true,
		"agents":    agents,
		"findings":  s.bus.All(),
	}
}

func sanitize(name string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return r + 32
		}
		return '-'
	}, name)
}
