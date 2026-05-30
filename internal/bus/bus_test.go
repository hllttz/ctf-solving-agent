package bus

import "testing"

func TestCheckForSkipsOwnFindingsAndAdvancesCursor(t *testing.T) {
	b := New()
	b.Post("model-a", "own")
	b.Post("model-b", "sibling")

	items, next := b.CheckFor("model-a", 0)
	if next != 2 {
		t.Fatalf("next cursor = %d, want 2", next)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Author != "model-b" || items[0].Content != "sibling" {
		t.Fatalf("unexpected item: %#v", items[0])
	}

	items, next = b.CheckFor("model-a", next)
	if next != 2 {
		t.Fatalf("next cursor after no new items = %d, want 2", next)
	}
	if len(items) != 0 {
		t.Fatalf("len(items) after no new items = %d, want 0", len(items))
	}
}

func TestBroadcastPostsCoordinatorMessage(t *testing.T) {
	b := New()
	b.Broadcast("try another angle")

	items, next := b.CheckFor("model-a", 0)
	if next != 1 {
		t.Fatalf("next = %d, want 1", next)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Author != CoordinatorAuthor {
		t.Fatalf("author = %q", items[0].Author)
	}
	if items[0].Content != "try another angle" {
		t.Fatalf("content = %q", items[0].Content)
	}
}
