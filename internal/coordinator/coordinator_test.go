package coordinator

import (
	"testing"

	"github.com/verialabs/ctf-agent/internal/solver"
	"github.com/verialabs/ctf-agent/internal/swarm"
)

func TestRecordResultRemovesFinishedSwarm(t *testing.T) {
	coord := New(t.TempDir(), []string{"anthropic/test"}, nil, "ctf-sandbox", 1)
	coord.swarms["baby"] = swarm.New("baby", t.TempDir(), nil, nil, "ctf-sandbox")

	result := &solver.Result{Status: solver.FlagFound, Flag: "ctf{ok}"}
	coord.recordResult("baby", result)

	if coord.swarms["baby"] != nil {
		t.Fatalf("finished swarm was not removed")
	}
	if coord.results["baby"] != result {
		t.Fatalf("result was not recorded")
	}
	if !coord.solved["baby"] {
		t.Fatalf("solved flag was not recorded")
	}
}

func TestRecordResultHandlesNilResult(t *testing.T) {
	coord := New(t.TempDir(), []string{"anthropic/test"}, nil, "ctf-sandbox", 1)
	coord.swarms["baby"] = swarm.New("baby", t.TempDir(), nil, nil, "ctf-sandbox")
	coord.pending["baby"] = true

	coord.recordResult("baby", nil)

	if coord.swarms["baby"] != nil {
		t.Fatalf("finished swarm was not removed")
	}
	if _, ok := coord.results["baby"]; !ok {
		t.Fatalf("nil result was not recorded")
	}
	if coord.pending["baby"] {
		t.Fatalf("pending challenge was not removed")
	}
	if coord.solved["baby"] {
		t.Fatalf("nil result marked challenge solved")
	}
	if got := coord.Summary(); got != "Solved 0/1 challenges" {
		t.Fatalf("summary = %q", got)
	}
}

func TestSpawnRespectsMaxConcurrentActiveSwarms(t *testing.T) {
	coord := New(t.TempDir(), []string{"anthropic/test"}, nil, "ctf-sandbox", 1)
	coord.swarms["running"] = swarm.New("running", t.TempDir(), nil, nil, "ctf-sandbox")

	if coord.Spawn(nil, "next") {
		t.Fatalf("Spawn returned true while at capacity")
	}
}

func TestSpawnCountsPendingChallengesTowardCapacity(t *testing.T) {
	coord := New(t.TempDir(), []string{"anthropic/test"}, nil, "ctf-sandbox", 1)
	coord.pending["starting"] = true

	if coord.Spawn(nil, "next") {
		t.Fatalf("Spawn returned true while pending challenge occupies capacity")
	}
}

func TestRecordResultReleasesSpawnCapacity(t *testing.T) {
	coord := New(t.TempDir(), []string{"anthropic/test"}, nil, "ctf-sandbox", 1)
	coord.pending["done"] = true

	coord.recordResult("done", &solver.Result{Status: solver.GaveUp})

	if got := coord.activeCountLocked(); got != 0 {
		t.Fatalf("active count = %d", got)
	}
}

func TestReserveChallengeRejectsDuplicateAndCapacity(t *testing.T) {
	coord := New(t.TempDir(), []string{"anthropic/test"}, nil, "ctf-sandbox", 1)

	if !coord.reserveChallengeLocked("baby") {
		t.Fatalf("reserve first challenge returned false")
	}
	if coord.reserveChallengeLocked("baby") {
		t.Fatalf("reserve duplicate challenge returned true")
	}
	if coord.reserveChallengeLocked("next") {
		t.Fatalf("reserve exceeded capacity")
	}

	coord.recordResult("baby", &solver.Result{Status: solver.GaveUp})
	if !coord.reserveChallengeLocked("next") {
		t.Fatalf("reserve after capacity release returned false")
	}
}

func TestReserveChallengeRejectsActiveSwarm(t *testing.T) {
	coord := New(t.TempDir(), []string{"anthropic/test"}, nil, "ctf-sandbox", 2)
	coord.swarms["baby"] = swarm.New("baby", t.TempDir(), nil, nil, "ctf-sandbox")

	if coord.reserveChallengeLocked("baby") {
		t.Fatalf("reserved challenge with active swarm")
	}
}

func TestConcurrencyLimit(t *testing.T) {
	tests := []struct {
		name           string
		maxConcurrent  int
		challengeCount int
		want           int
	}{
		{name: "normal cap", maxConcurrent: 2, challengeCount: 5, want: 2},
		{name: "zero means unbounded", maxConcurrent: 0, challengeCount: 5, want: 5},
		{name: "negative means unbounded", maxConcurrent: -1, challengeCount: 5, want: 5},
		{name: "cap above challenge count", maxConcurrent: 10, challengeCount: 3, want: 3},
		{name: "empty challenge list still safe", maxConcurrent: 0, challengeCount: 0, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coord := New(t.TempDir(), []string{"anthropic/test"}, nil, "ctf-sandbox", tt.maxConcurrent)
			if got := coord.concurrencyLimit(tt.challengeCount); got != tt.want {
				t.Fatalf("concurrencyLimit(%d) = %d, want %d", tt.challengeCount, got, tt.want)
			}
		})
	}
}
