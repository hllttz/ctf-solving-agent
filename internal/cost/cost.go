package cost

import (
	"fmt"
	"sync"
)

// Prices in USD per 1M tokens (input / output).
var modelPrices = map[string][2]float64{
	"claude-opus-4-6":     {15.0, 75.0},
	"claude-sonnet-4-6":   {3.0, 15.0},
	"claude-haiku-4-5":    {0.80, 4.0},
	"gpt-5.4":             {1.25, 10.0},
	"gpt-5.4-mini":        {0.15, 0.60},
	"gpt-5.3-codex":       {2.50, 10.0},
	"gemini-2.5-pro":      {1.25, 10.0},
}

// Usage accumulates token counts and cost for a model.
type Usage struct {
	Model       string
	InputTokens int
	OutputTokens int
	CacheTokens  int
}

// Cost returns the estimated cost in USD.
func (u *Usage) Cost() float64 {
	price, ok := modelPrices[u.Model]
	if !ok {
		price = [2]float64{3.0, 15.0}
	}
	return float64(u.InputTokens)/1e6*price[0] + float64(u.OutputTokens)/1e6*price[1]
}

// Tracker tracks cost across multiple agents.
type Tracker struct {
	mu    sync.Mutex
	usage map[string]*Usage
}

// NewTracker creates a new cost tracker.
func NewTracker() *Tracker {
	return &Tracker{usage: make(map[string]*Usage)}
}

// Record adds token usage for an agent.
func (t *Tracker) Record(agent, model string, inputTokens, outputTokens, cacheTokens int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := agent + "/" + model
	u, ok := t.usage[key]
	if !ok {
		u = &Usage{Model: model}
		t.usage[key] = u
	}
	u.InputTokens += inputTokens
	u.OutputTokens += outputTokens
	u.CacheTokens += cacheTokens
}

// Summary returns a formatted cost summary.
func (t *Tracker) Summary() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	totalCost := 0.0
	totalInput := 0
	totalOutput := 0
	result := "Cost Summary\n============\n"

	for key, u := range t.usage {
		c := u.Cost()
		totalCost += c
		totalInput += u.InputTokens
		totalOutput += u.OutputTokens
		result += fmt.Sprintf("  %s: in=%d out=%d cost=$%.4f\n", key, u.InputTokens, u.OutputTokens, c)
	}

	result += fmt.Sprintf("\nTotal: in=%d out=%d cost=$%.4f\n", totalInput, totalOutput, totalCost)
	return result
}

// TotalCost returns the total accumulated cost.
func (t *Tracker) TotalCost() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()

	var total float64
	for _, u := range t.usage {
		total += u.Cost()
	}
	return total
}
