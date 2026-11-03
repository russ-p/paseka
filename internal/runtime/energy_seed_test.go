package runtime_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runtime"
	"github.com/russ-p/paseka/internal/taskledger"
)

// A seed observation interrupted between the ledger commit and the send is
// repaired at startup with the recorded identity, and a second recovery does not
// publish it again.
func TestReactorRepairsPendingSeedObservation(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	if err := ledger.SeedEnergy("trace-1", 9); err != nil {
		t.Fatal(err)
	}
	before, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if before.EnergySeed == nil || before.EnergySeed.Published {
		t.Fatalf("seed = %+v, want a pending record", before.EnergySeed)
	}
	recorded := *before.EnergySeed

	pub := &recordingPublisher{}
	r := runtime.NewTestReactor(runtime.TestReactorOptions{
		Ledger:    ledger,
		Publisher: pub,
	})

	if err := r.RecoverPendingSeedObservations(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(pub.events) != 1 {
		t.Fatalf("published events = %d, want one repair", len(pub.events))
	}
	var payload protocol.EnergySeededPayload
	if err := json.Unmarshal(pub.events[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SeedID != recorded.SeedID || payload.Budget != recorded.Budget || !payload.SeededAt.Equal(recorded.SeededAt) {
		t.Fatalf("payload = %+v, want the recorded seed %+v", payload, recorded)
	}

	if err := r.RecoverPendingSeedObservations(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(pub.events) != 1 {
		t.Fatalf("a second recovery published %d events, want none", len(pub.events)-1)
	}
	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyBudget != 9 || snap.EnergyRemaining != 9 {
		t.Fatalf("honey = %d/%d, want the seed applied once", snap.EnergyBudget, snap.EnergyRemaining)
	}
}
