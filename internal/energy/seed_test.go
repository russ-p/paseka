package energy_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/energy"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

type recordingPublisher struct {
	mu     sync.Mutex
	events []protocol.Event
	err    error
}

func (p *recordingPublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	p.events = append(p.events, ev)
	return nil
}

func (p *recordingPublisher) snapshot() []protocol.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]protocol.Event(nil), p.events...)
}

func TestEnsureSeededUsesDefaultBudget(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	if err := energy.EnsureSeeded(context.Background(), ledger, nil, "", "trace-1", "cli"); err != nil {
		t.Fatal(err)
	}
	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyBudget != protocol.DefaultEnergyBudget {
		t.Fatalf("budget = %d", snap.EnergyBudget)
	}
	if snap.EnergySeed == nil {
		t.Fatal("expected a durable seed record")
	}
	if snap.EnergySeed.Budget != protocol.DefaultEnergyBudget {
		t.Fatalf("seed budget = %d", snap.EnergySeed.Budget)
	}
	if snap.EnergySeed.Published {
		t.Fatal("a nil publisher must leave the observation pending")
	}
}

// A newly seeded trail records the winning allocation and publishes exactly one
// observation carrying the same identity, budget, and seed time.
func TestSeedRecordsAndPublishesWinningAllocation(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	pub := &recordingPublisher{}
	before := time.Now().Add(-time.Second)

	seed, err := energy.Seed(context.Background(), ledger, pub, energy.SeedInput{
		TraceID: "trace-1",
		Budget:  9,
		AgentID: "cli",
	})
	if err != nil {
		t.Fatal(err)
	}
	if seed.SeedID == "" {
		t.Fatal("expected a seed identity")
	}
	if seed.Budget != 9 {
		t.Fatalf("seed budget = %d, want 9", seed.Budget)
	}
	if seed.SeededAt.Before(before) || seed.SeededAt.After(time.Now().Add(time.Second)) {
		t.Fatalf("seed time = %v, want the actual seed time", seed.SeededAt)
	}
	if !seed.Published {
		t.Fatal("expected the returned record to be published")
	}

	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyBudget != 9 || snap.EnergyRemaining != 9 {
		t.Fatalf("honey = %+v, want 9/9", snap)
	}
	if snap.EnergySeed == nil || snap.EnergySeed.SeedID != seed.SeedID || snap.EnergySeed.Budget != 9 || !snap.EnergySeed.Published {
		t.Fatalf("seed record = %+v, want the published winner", snap.EnergySeed)
	}

	// A second request observes the winner instead of logging its own budget.
	again, err := energy.Seed(context.Background(), ledger, pub, energy.SeedInput{
		TraceID: "trace-1",
		Budget:  20,
		AgentID: "cli",
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.SeedID != seed.SeedID || again.Budget != seed.Budget {
		t.Fatalf("second seed = %+v, want the existing winner %+v", again, seed)
	}
	events := pub.snapshot()
	if len(events) != 1 {
		t.Fatalf("published events = %d, want exactly one", len(events))
	}
	payload := decodeSeeded(t, events[0])
	if payload.SeedID != seed.SeedID || payload.Budget != seed.Budget || !payload.SeededAt.Equal(seed.SeededAt) {
		t.Fatalf("payload = %+v, want seed %+v", payload, seed)
	}
}

// Concurrent seed requests converge on one allocation and one identity. The
// observation may be delivered more than once by design (at-least-once), but it
// always names the recorded winner.
func TestSeedConcurrentRequestsAllocateOnce(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	pub := &recordingPublisher{}

	const callers = 16
	var wg sync.WaitGroup
	seeds := make([]taskledger.EnergySeedSnapshot, callers)
	errs := make([]error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		budget := 5 + i
		go func(i, budget int) {
			defer wg.Done()
			seeds[i], errs[i] = energy.Seed(context.Background(), ledger, pub, energy.SeedInput{
				TraceID: "trace-1",
				Budget:  budget,
			})
		}(i, budget)
	}
	wg.Wait()

	var winner taskledger.EnergySeedSnapshot
	for i, err := range errs {
		if err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
		if winner.SeedID == "" {
			winner = seeds[i]
			continue
		}
		if seeds[i].SeedID != winner.SeedID {
			t.Fatalf("seed %d identity = %q, want %q", i, seeds[i].SeedID, winner.SeedID)
		}
		if seeds[i].Budget != winner.Budget {
			t.Fatalf("seed %d budget = %d, want %d", i, seeds[i].Budget, winner.Budget)
		}
	}

	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergySeed == nil || snap.EnergySeed.SeedID != winner.SeedID {
		t.Fatalf("snapshot seed = %+v, want %q", snap.EnergySeed, winner.SeedID)
	}
	if snap.EnergyBudget != winner.Budget || snap.EnergyRemaining != winner.Budget {
		t.Fatalf("honey = %d/%d, want %d allocated once", snap.EnergyBudget, snap.EnergyRemaining, winner.Budget)
	}
	for i, ev := range pub.snapshot() {
		payload := decodeSeeded(t, ev)
		if payload.SeedID != winner.SeedID {
			t.Fatalf("observation %d names seed %q, want %q", i, payload.SeedID, winner.SeedID)
		}
	}
}

// An interrupted publication is retried with the recorded identity and without
// reseeding, including after a fresh process reads the same ledger.
func TestSeedPublicationRetryReusesRecordedIdentity(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	failing := &recordingPublisher{err: errors.New("bus unavailable")}

	if _, err := energy.Seed(context.Background(), ledger, failing, energy.SeedInput{
		TraceID: "trace-1",
		Budget:  7,
	}); err == nil {
		t.Fatal("expected the failed publication to be reported")
	}
	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergySeed == nil || snap.EnergySeed.Published {
		t.Fatalf("seed = %+v, want a pending record", snap.EnergySeed)
	}
	recorded := *snap.EnergySeed

	// A fresh reactor/CLI process retries from the durable record.
	pub := &recordingPublisher{}
	if err := energy.EnsureSeeded(context.Background(), ledger, pub, "", "trace-1", protocol.RuntimeAgentID); err != nil {
		t.Fatal(err)
	}
	snap, err = ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergySeed == nil || !snap.EnergySeed.Published {
		t.Fatalf("seed = %+v, want the retried publication", snap.EnergySeed)
	}
	if snap.EnergySeed.SeedID != recorded.SeedID || snap.EnergySeed.Budget != recorded.Budget || !snap.EnergySeed.SeededAt.Equal(recorded.SeededAt) {
		t.Fatalf("seed = %+v, want the recorded winner %+v", snap.EnergySeed, recorded)
	}
	if snap.EnergyBudget != 7 || snap.EnergyRemaining != 7 {
		t.Fatalf("honey = %d/%d, want the seed applied once", snap.EnergyBudget, snap.EnergyRemaining)
	}
	events := pub.snapshot()
	if len(events) != 1 {
		t.Fatalf("repair published %d events, want one", len(events))
	}
	if payload := decodeSeeded(t, events[0]); payload.SeedID != recorded.SeedID {
		t.Fatalf("repair seed = %q, want %q", payload.SeedID, recorded.SeedID)
	}
}

// A ledger written before seed provenance existed keeps its allocation unknown.
// No seed record, timestamp, or observation is invented for it.
func TestEnsureSeededLeavesLegacyAllocationUnknown(t *testing.T) {
	ledger := legacyLedger{snapshot: taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		EnergyBudget:    12,
		EnergyRemaining: 3,
	}}
	pub := &recordingPublisher{}

	if err := energy.EnsureSeeded(context.Background(), ledger, pub, "", "trace-1", "cli"); err != nil {
		t.Fatal(err)
	}
	if events := pub.snapshot(); len(events) != 0 {
		t.Fatalf("published %d events for a legacy allocation", len(events))
	}
}

func TestValidateAddAmount(t *testing.T) {
	if err := energy.ValidateAddAmount(1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := energy.ValidateAddAmount(0); err == nil {
		t.Fatal("expected error for zero amount")
	}
	if err := energy.ValidateAddAmount(-1); err == nil {
		t.Fatal("expected error for negative amount")
	}
}

func decodeSeeded(t *testing.T, ev protocol.Event) protocol.EnergySeededPayload {
	t.Helper()
	if ev.Type != protocol.EventSignal {
		t.Fatalf("event type = %s, want SIGNAL", ev.Type)
	}
	var payload protocol.EnergySeededPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Kind != protocol.SignalEnergySeeded {
		t.Fatalf("kind = %q, want energy.seeded", payload.Kind)
	}
	return payload
}

// legacyLedger reports a snapshot the ledger already held before provenance
// existed. Any attempt to seed or publish it would be a bug in the caller.
type legacyLedger struct {
	taskledger.Ledger
	snapshot taskledger.TraceSnapshot
}

func (l legacyLedger) Snapshot(string) (taskledger.TraceSnapshot, error) {
	return l.snapshot, nil
}
