package bus

import (
	"sync"
	"time"
)

const CoordinatorAuthor = "coordinator"
const CoordinatorNotificationAuthor = "coordinator_notification"

// Finding represents a discovery shared by a solver.
type Finding struct {
	Author    string
	Content   string
	Target    string `json:",omitempty"`
	Timestamp time.Time
}

// MessageBus enables solvers working on the same challenge to share findings.
// Thread-safe, append-only, capped list.
type MessageBus struct {
	mu       sync.Mutex
	findings []Finding
	maxItems int
}

func New() *MessageBus {
	return &MessageBus{maxItems: 200}
}

// Post adds a finding to the bus.
func (b *MessageBus) Post(author, content string) {
	b.PostTo(author, "", content)
}

// PostTo adds a finding targeted at a specific solver. An empty target is
// visible to all solvers.
func (b *MessageBus) PostTo(author, target, content string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.findings) >= b.maxItems {
		b.findings = b.findings[1:]
	}
	b.findings = append(b.findings, Finding{
		Author:    author,
		Content:   content,
		Target:    target,
		Timestamp: time.Now(),
	})
}

// Broadcast posts a coordinator-level strategy message to all solvers.
func (b *MessageBus) Broadcast(content string) {
	b.Post(CoordinatorAuthor, content)
}

// Check returns findings after the given cursor position.
func (b *MessageBus) Check(cursor int) []Finding {
	b.mu.Lock()
	defer b.mu.Unlock()

	if cursor >= len(b.findings) {
		return nil
	}
	return b.findings[cursor:]
}

// CheckFor returns findings after the given cursor, excluding findings posted by author.
func (b *MessageBus) CheckFor(author string, cursor int) ([]Finding, int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if cursor >= len(b.findings) {
		return nil, len(b.findings)
	}
	out := make([]Finding, 0, len(b.findings)-cursor)
	for _, item := range b.findings[cursor:] {
		if item.Author != author {
			if item.Target != "" && item.Target != author {
				continue
			}
			out = append(out, item)
		}
	}
	return out, len(b.findings)
}

// All returns all findings.
func (b *MessageBus) All() []Finding {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := make([]Finding, len(b.findings))
	copy(out, b.findings)
	return out
}
