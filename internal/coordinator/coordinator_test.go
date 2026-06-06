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
