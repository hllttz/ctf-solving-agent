package solver

import "testing"

func TestCurrentSummaryReturnsLatestCommentary(t *testing.T) {
	s := NewWithOptions(nil, nil, nil, "openai/test-model", "baby", nil)

	if got := s.CurrentSummary(); got != "" {
		t.Fatalf("empty summary = %q", got)
	}

	s.recordCommentary("first summary")
	s.recordCommentary("latest summary")

	if got := s.CurrentSummary(); got != "latest summary" {
		t.Fatalf("summary = %q", got)
	}
}
