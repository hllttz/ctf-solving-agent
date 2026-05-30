package trace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Event represents a trace event.
type Event struct {
	Timestamp time.Time      `json:"timestamp"`
	Type      string         `json:"type"`
	Agent     string         `json:"agent"`
	Challenge string         `json:"challenge"`
	Step      int            `json:"step,omitempty"`
	Tool      string         `json:"tool,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	Tokens    *TokenUsage    `json:"tokens,omitempty"`
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
	path string
}

// New creates a new tracer writing to the given path.
func New(path string) (*Tracer, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &Tracer{file: f, path: path}, nil
}

// NewSolverTracer creates a trace file for one challenge/model solver.
func NewSolverTracer(logDir, challenge, agent string) (*Tracer, error) {
	ts := time.Now().Format("20060102-150405")
	name := "trace-" + sanitize(challenge) + "-" + sanitize(agent) + "-" + ts + ".jsonl"
	return New(filepath.Join(logDir, name))
}

// Log records an event.
func (t *Tracer) Log(eventType, agent, challenge, data string) {
	t.LogEvent(eventType, agent, challenge, 0, "", map[string]any{"message": data})
}

// LogEvent records a structured event.
func (t *Tracer) LogEvent(eventType, agent, challenge string, step int, tool string, data map[string]any) {
	t.mu.Lock()
	defer t.mu.Unlock()

	evt := Event{
		Timestamp: time.Now(),
		Type:      eventType,
		Agent:     agent,
		Challenge: challenge,
		Step:      step,
		Tool:      tool,
		Data:      data,
	}

	b, _ := json.Marshal(evt)
	t.file.Write(b)
	t.file.Write([]byte("\n"))
	t.file.Sync()
}

// LogTool records a tool call event.
func (t *Tracer) LogTool(agent, challenge, toolName, input, output string) {
	t.LogEvent("tool", agent, challenge, 0, toolName, map[string]any{
		"input":  truncate(input, 2000),
		"output": truncate(output, 2000),
	})
}

// Close flushes and closes the tracer.
func (t *Tracer) Close() error {
	return t.file.Close()
}

// Path returns the trace file path.
func (t *Tracer) Path() string {
	if t == nil {
		return ""
	}
	return t.path
}

func sanitize(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, ":", "_")
	if s == "" {
		return "unknown"
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
