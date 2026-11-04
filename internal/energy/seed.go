package energy

import (
	"context"
	"fmt"
	"strings"

	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

// SeedInput describes one authoritative seed request.
type SeedInput struct {
	TraceID string
	Budget  int
	AgentID string
}

// Seed performs the authoritative seed transition for one trail. The ledger
// records the winning allocation before anything is published, then this helper
// publishes exactly one SIGNAL/energy.seeded observation for it.
//
// A second request observes the recorded winner instead of allocating a second
// budget. An unpublished record left by an interrupted run is retried with the
// same seed identity; a legacy allocation without provenance produces no
// observation and keeps its unknown history.
func Seed(ctx context.Context, ledger taskledger.Ledger, pub Publisher, in SeedInput) (taskledger.EnergySeedSnapshot, error) {
	if ledger == nil {
		return taskledger.EnergySeedSnapshot{}, fmt.Errorf("energy: task ledger is required")
	}
	if strings.TrimSpace(in.TraceID) == "" {
		return taskledger.EnergySeedSnapshot{}, fmt.Errorf("energy: trace id is required")
	}
	seed, _, err := ledger.EnsureEnergySeed(in.TraceID, in.Budget)
	if err != nil {
		return taskledger.EnergySeedSnapshot{}, err
	}
	return publishRecordedSeed(ctx, ledger, pub, in.TraceID, in.AgentID, seed)
}

// EnsureSeeded seeds the trace honey reserve from colony defaults when not yet
// seeded, and repairs a pending seed observation when one is already recorded.
// It never infers a provenance record for a legacy allocation.
func EnsureSeeded(ctx context.Context, ledger taskledger.Ledger, pub Publisher, colonyRoot, traceID, agentID string) error {
	if ledger == nil {
		return fmt.Errorf("energy: task ledger is required")
	}
	if strings.TrimSpace(traceID) == "" {
		return fmt.Errorf("energy: trace id is required")
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	if snap.EnergySeed != nil {
		if snap.EnergySeed.Published {
			return nil
		}
		_, err := publishRecordedSeed(ctx, ledger, pub, traceID, agentID, *snap.EnergySeed)
		return err
	}
	if snap.EnergyBudget > 0 {
		// Legacy allocation written before seed provenance existed. It stays
		// unknown: no identity, budget, or timestamp is invented for it.
		return nil
	}
	budget := protocol.DefaultEnergyBudget
	if strings.TrimSpace(colonyRoot) != "" {
		manifest, err := colony.LoadColony(colonyRoot)
		if err != nil {
			return err
		}
		budget = manifest.ResolvedEnergyBudget()
	}
	_, err = Seed(ctx, ledger, pub, SeedInput{TraceID: traceID, Budget: budget, AgentID: agentID})
	return err
}

// PublishPendingSeedObservation retries the observation for an already recorded
// seed. It never seeds, so a restart can repair an interrupted publication
// without changing the winning allocation.
func PublishPendingSeedObservation(ctx context.Context, ledger taskledger.Ledger, pub Publisher, traceID, agentID string) error {
	if ledger == nil {
		return fmt.Errorf("energy: task ledger is required")
	}
	if strings.TrimSpace(traceID) == "" {
		return fmt.Errorf("energy: trace id is required")
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	if snap.EnergySeed == nil || snap.EnergySeed.Published {
		return nil
	}
	_, err = publishRecordedSeed(ctx, ledger, pub, traceID, agentID, *snap.EnergySeed)
	return err
}

func publishRecordedSeed(ctx context.Context, ledger taskledger.Ledger, pub Publisher, traceID, agentID string, seed taskledger.EnergySeedSnapshot) (taskledger.EnergySeedSnapshot, error) {
	if seed.SeedID == "" || seed.Published {
		return seed, nil
	}
	if !bus.PublisherAvailable(pub) {
		// The durable record stays pending so a later process can retry the
		// same identity once a publisher exists.
		return seed, nil
	}
	if agentID == "" {
		agentID = "cli"
	}
	ev, err := protocol.NewEvent(traceID, agentID, 0, protocol.EventSignal, protocol.EnergySeededPayload{
		Kind:     protocol.SignalEnergySeeded,
		SeedID:   seed.SeedID,
		Budget:   seed.Budget,
		SeededAt: seed.SeededAt,
	})
	if err != nil {
		return seed, err
	}
	if err := pub.PublishEvent(ctx, ev); err != nil {
		return seed, fmt.Errorf("publish energy.seeded: %w", err)
	}
	if err := ledger.MarkEnergySeedPublished(traceID, seed.SeedID); err != nil {
		return seed, err
	}
	seed.Published = true
	return seed, nil
}
