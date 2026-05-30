package loopdetect

import (
	"context"
	"sync"

	"github.com/cloudwego/eino/compose"
)

const (
	maxHistory  = 12
	warnAt      = 3
	forceBreakAt = 5
)

// Detector tracks tool call patterns and detects when a solver is looping.
type Detector struct {
	mu       sync.Mutex
	history  []string
	warnings int
}

func New() *Detector {
	return &Detector{}
}

// Check returns a warning/break message if a loop is detected, empty string otherwise.
func (d *Detector) Check(toolName, argsJSON string) string {
	d.mu.Lock()
	defer d.mu.Unlock()

	sig := toolName + ":" + truncate(argsJSON, 500)
	d.history = append(d.history, sig)
	if len(d.history) > maxHistory {
		d.history = d.history[1:]
	}

	count := 0
	for _, h := range d.history {
		if h == sig {
			count++
		}
	}

	if count >= forceBreakAt {
		return loopBreakMessage
	}
	if count >= warnAt && d.warnings < 2 {
		d.warnings++
		return loopWarningMessage
	}
	return ""
}

// Reset clears the detector state.
// History returns a copy of the tool call history for debugging.
func (d *Detector) History() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, len(d.history))
	copy(out, d.history)
	return out
}

// Reset clears the detector state.
func (d *Detector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.history = nil
	d.warnings = 0
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

const loopWarningMessage = `
[SYSTEM WARNING] You appear to be calling the same tool with the same arguments repeatedly.
This suggests you're stuck in a loop. Consider:
1. Trying a completely different approach
2. Using check_findings to see what other solvers have discovered
3. Notifying the coordinator if you're truly stuck
`

const loopBreakMessage = `
[SYSTEM] LOOP DETECTED - You have called the same tool with the same arguments 5 times.
PLEASE take a completely different approach. Review what you've learned and try something new.
`

// Middleware returns an eino ToolMiddleware that injects loop detection.
// It wraps tool calls and checks for repetitive patterns.
func (d *Detector) Middleware() compose.ToolMiddleware {
	return compose.ToolMiddleware{
		Invokable: func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
				output, err := next(ctx, input)
				if err != nil {
					return nil, err
				}
				if msg := d.Check(input.Name, input.Arguments); msg != "" {
					if output.Result != "" {
						output.Result += "\n"
					}
					output.Result += msg
				}
				return output, nil
			}
		},
		Streamable: func(next compose.StreamableToolEndpoint) compose.StreamableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.StreamToolOutput, error) {
				output, err := next(ctx, input)
				if err != nil {
					return nil, err
				}
				_ = d.Check(input.Name, input.Arguments)
				return output, nil
			}
		},
	}
}
