package energy_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/energy"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestAddAppliesWhenReactorStopped(t *testing.T) {
	slug := "energy-add-stopped"
	root := t.TempDir()
	setupEnergyHome(t, slug, root)

	ledger := taskledger.NewMemoryLedger()
	var applyCount atomic.Int32
	wrapped := &applyCountingLedger{Ledger: ledger, onApply: func() { applyCount.Add(1) }}

	snap, err := energy.Add(context.Background(), slug, wrapped, &noopPublisher{}, energy.AddInput{
		TraceID: "trace-1",
		Amount:  5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyRemaining != 5 {
		t.Fatalf("remaining = %d, want 5", snap.EnergyRemaining)
	}
	if applyCount.Load() != 1 {
		t.Fatalf("ledger apply calls = %d, want 1", applyCount.Load())
	}
}

func TestAddDoesNotDoubleApplyWhenReactorRunning(t *testing.T) {
	slug := "energy-add-running"
	root := t.TempDir()
	setupEnergyHome(t, slug, root)

	if err := colony.RegisterRuntime(slug, colony.RuntimeEntry{
		PID:             os.Getpid(),
		StartedAt:       time.Now().UTC(),
		ColonyRoot:      root,
		Status:          "running",
		LastHeartbeatAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = colony.ClearRuntime(slug) })

	memory := taskledger.NewMemoryLedger()
	if err := memory.SeedEnergy("trace-1", 12); err != nil {
		t.Fatal(err)
	}

	var cliApplyCount atomic.Int32
	wrapped := &applyCountingLedger{
		Ledger:  memory,
		onApply: func() { cliApplyCount.Add(1) },
	}

	snap, err := energy.Add(context.Background(), slug, wrapped, &reactorSimPublisher{ledger: memory}, energy.AddInput{
		TraceID: "trace-1",
		Amount:  3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cliApplyCount.Load() != 0 {
		t.Fatalf("cli ledger apply calls = %d, want 0 when reactor is running", cliApplyCount.Load())
	}
	if snap.EnergyRemaining != 15 {
		t.Fatalf("remaining = %d, want 15 (12 + 3 once)", snap.EnergyRemaining)
	}
}

func TestConsumeSeedsAndDecrements(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	pub := &stubPublisher{}
	ctx := context.Background()
	snap, err := energy.Consume(ctx, "test-colony", "", ledger, pub, energy.ConsumeInput{
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

func TestConsumeExhausted(t *testing.T) {
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
	pub := &stubPublisher{}
	_, err = energy.Consume(context.Background(), "test-colony", "", ledger, pub, energy.ConsumeInput{
		TraceID: "trace-1",
		Amount:  1,
		Reason:  "session.start",
	})
	if err != energy.ErrHoneyReserveExhausted {
		t.Fatalf("err = %v", err)
	}
}

type noopPublisher struct{}

func (noopPublisher) PublishEvent(context.Context, protocol.Event) error { return nil }

type reactorSimPublisher struct {
	ledger taskledger.Ledger
}

func (p *reactorSimPublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	_, err := p.ledger.Apply(ev)
	return err
}

type stubPublisher struct {
	published []protocol.Event
}

func (s *stubPublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	s.published = append(s.published, ev)
	return nil
}

type applyCountingLedger struct {
	taskledger.Ledger
	onApply func()
}

func (l *applyCountingLedger) Apply(ev protocol.Event) (taskledger.ApplyResult, error) {
	if l.onApply != nil {
		l.onApply()
	}
	return l.Ledger.Apply(ev)
}

func setupEnergyHome(t *testing.T, slug, root string) {
	t.Helper()
	home, err := colony.HomeDir(slug)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf("colony_root: %q\n", root)
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".paseka"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".paseka", "colony.yaml"), []byte("slug: "+slug+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
