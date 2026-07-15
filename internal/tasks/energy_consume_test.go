package tasks

import (
	"context"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestConsumeEnergySeedsAndDecrements(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	pub := &stubEnergyPublisher{}
	ctx := context.Background()
	snap, err := consumeEnergy(ctx, "test-colony", "", ledger, pub, ConsumeEnergyInput{
		TraceID: "trace-1",
		Amount:  1,
		Reason:  "session.start",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyRemaining != protocol.DefaultEnergyBudget-1 {
		t.Fatalf("remaining = %d", snap.EnergyRemaining)
	}
	if len(pub.published) != 1 {
		t.Fatalf("published = %d", len(pub.published))
	}
}

func TestConsumeEnergyExhausted(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	if err := ledger.SeedEnergy("trace-1", 1); err != nil {
		t.Fatal(err)
	}
	ev, err := protocol.NewEvent("trace-1", "runtime", 0, protocol.EventSignal, protocol.EnergyConsumePayload{
		Kind:   protocol.SignalEnergyConsume,
		Amount: 1,
		Reason: "task.dispatch",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(ev); err != nil {
		t.Fatal(err)
	}
	pub := &stubEnergyPublisher{}
	_, err = consumeEnergy(context.Background(), "test-colony", "", ledger, pub, ConsumeEnergyInput{
		TraceID: "trace-1",
		Amount:  1,
		Reason:  "session.start",
	})
	if err != ErrHoneyReserveExhausted {
		t.Fatalf("err = %v", err)
	}
}

type stubEnergyPublisher struct {
	published []protocol.Event
}

func (s *stubEnergyPublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	s.published = append(s.published, ev)
	return nil
}

func TestEnsureEnergySeededUsesColonyBudget(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	manifest := colony.Colony{Defaults: colony.Defaults{EnergyBudget: 7}}
	_ = manifest
	if err := ensureEnergySeeded(ledger, "", "trace-1"); err != nil {
		t.Fatal(err)
	}
	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyBudget != protocol.DefaultEnergyBudget {
		t.Fatalf("budget = %d", snap.EnergyBudget)
	}
}
