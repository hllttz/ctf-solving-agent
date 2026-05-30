package trace

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Event represents a trace event.
type Event struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Agent     string    `json:"agent"`
	Challenge string    `json:"challenge"`
	Data      string    `json:"data"`
	Tokens    *TokenUsage `json:"tokens,omitempty"`
}

type TokenUsage struct {
	Input  int `json:"input"`
	Output int `json:"output"`
	Cache  int `json:"cache,omitempty"`
}

// Tracer records solver events to a JSONL file.
type Tracer struct {
	mu   sync.Mutex
	file *os.File
}

// New creates a new tracer writing to the given path.
func New(path string) (*Tracer, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &Tracer{file: f}, nil
}

// Log records an event.
func (t *Tracer) Log(eventType, agent, challenge, data string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	evt := Event{
		Timestamp: time.Now(),
		Type:      eventType,
		Agent:     agent,
		Challenge: challenge,
		Data:      data,
	}

	b, _ := json.Marshal(evt)
	t.file.Write(b)
	t.file.Write([]byte("\n"))
	t.file.Sync()
}

// LogTool records a tool call event.
func (t *Tracer) LogTool(agent, challenge, toolName, input, output string) {
	data := fmt.Sprintf(`{"tool":"%s","input":"%s","output":"%s"}`, toolName, input, output)
	t.Log("tool", agent, challenge, data)
}

// Close flushes and closes the tracer.
func (t *Tracer) Close() error {
	return t.file.Close()
}
