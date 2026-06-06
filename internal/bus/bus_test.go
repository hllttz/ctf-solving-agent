package bus

import "testing"

func TestCheckForReturnsBroadcastAndTargetedMessages(t *testing.T) {
	b := New()
	b.Post("solver-a", "a finding")
	b.PostTo(CoordinatorAuthor, "solver-a", "target a")
	b.PostTo(CoordinatorAuthor, "solver-b", "target b")
	b.Broadcast("all solvers")

	items, next := b.CheckFor("solver-a", 0)
	if next != 4 {
		t.Fatalf("next cursor = %d", next)
	}
	if len(items) != 2 {
		t.Fatalf("solver-a visible items = %d: %#v", len(items), items)
	}
	if items[0].Content != "target a" || items[1].Content != "all solvers" {
		t.Fatalf("solver-a items = %#v", items)
	}

	items, _ = b.CheckFor("solver-b", 0)
	if len(items) != 3 {
		t.Fatalf("solver-b visible items = %d: %#v", len(items), items)
	}
	if items[0].Content != "a finding" || items[1].Content != "target b" || items[2].Content != "all solvers" {
		t.Fatalf("solver-b items = %#v", items)
	}
}
