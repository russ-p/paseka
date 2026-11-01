package taskledger_test

import (
	"encoding/json"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

// The KV ledger claims the published flag through compare-and-set: the first
// claim wins and the second finds nothing, so a restarted reactor cannot send a
// second paused observation for the same episode.
func TestKVLedgerClaimsPausePublicationOnce(t *testing.T) {
	kv := newFakeKV()
	ledger := taskledger.NewKVLedger(kv)

	seed := taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		EnergyBudget:    4,
		EnergyRemaining: 0,
		Tasks:           map[string]taskledger.TaskSnapshot{},
	}
	data, err := json.Marshal(seed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kv.Create("trace-1", data); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.DeferPendingDispatch("trace-1", taskledger.PendingDispatch{
		ID:     "direct-a",
		Kind:   taskledger.PendingDispatchDirect,
		TaskID: "task-a",
		Reason: protocol.HoneyReserveExhaustedSummary,
	}); err != nil {
		t.Fatal(err)
	}

	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyPause == nil || snap.EnergyPause.Published {
		t.Fatalf("pause = %+v, want an unpublished episode", snap.EnergyPause)
	}
	episodeID := snap.EnergyPause.EpisodeID

	claimed, ok, err := ledger.ClaimEnergyPausePublished("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected the first claim to win")
	}
	if claimed.EpisodeID != episodeID {
		t.Fatalf("episode = %q, want %q", claimed.EpisodeID, episodeID)
	}
	if _, ok, err := ledger.ClaimEnergyPausePublished("trace-1"); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("a second claim must not win")
	}
	snap, err = ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyPause == nil || !snap.EnergyPause.Published {
		t.Fatalf("pause = %+v, want the publication persisted", snap.EnergyPause)
	}

	closed, ok, err := ledger.ClearEnergyPause("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || closed.EpisodeID != episodeID {
		t.Fatalf("cleared = %+v, ok = %v, want episode %q", closed, ok, episodeID)
	}
	if _, ok, err := ledger.ClearEnergyPause("trace-1"); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("a second clear must not find an episode")
	}
	snap, err = ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyPause != nil {
		t.Fatalf("pause = %+v, want the field cleared", snap.EnergyPause)
	}
}
