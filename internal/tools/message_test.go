package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/verialabs/ctf-agent/internal/bus"
)

func TestNotifyCoordinatorToolPostsCoordinatorNotification(t *testing.T) {
	b := bus.New()
	tool := NewNotifyCoordinatorTool(b, "solver-a")

	out, err := tool.InvokableRun(context.Background(), `{"message":"shared flag prefix discovered"}`)
	if err != nil {
		t.Fatalf("InvokableRun error: %v", err)
	}
	if !strings.Contains(out, "Message sent") {
		t.Fatalf("output = %q", out)
	}

	items := b.All()
	if len(items) != 1 {
		t.Fatalf("items len = %d", len(items))
	}
	if items[0].Author != bus.CoordinatorNotificationAuthor {
		t.Fatalf("author = %q", items[0].Author)
	}
	if !strings.Contains(items[0].Content, "solver-a") || !strings.Contains(items[0].Content, "shared flag prefix") {
		t.Fatalf("content = %q", items[0].Content)
	}
}
