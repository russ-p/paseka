package invites

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

var errHoneyReserveExhausted = errors.New("honey reserve exhausted")

func (s *Service) consumeSessionEnergy(ctx context.Context, traceID string) error {
	pub := s.publisher()
	var client *bus.Client
	if pub == nil {
		var err error
		client, err = bus.ConnectColony(s.Colony, false)
		if err != nil {
			return err
		}
		if client == nil {
			return nil
		}
		defer client.Close()
		pub = client
	} else if s.Bus == nil {
		var err error
		client, err = bus.ConnectColony(s.Colony, false)
		if err != nil {
			return err
		}
		if client == nil {
			return nil
		}
		defer client.Close()
	} else {
		client = s.Bus
	}

	kv, err := client.JetStream().KeyValue(bus.TaskLedgerBucket(s.Colony.Slug))
	if err != nil {
		return fmt.Errorf("invites: task ledger kv: %w", err)
	}
	ledger := taskledger.NewKVLedger(kv)

	if err := seedEnergyIfNeeded(ledger, s.Colony.ColonyRoot, traceID); err != nil {
		return err
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	if !taskledger.HasEnergy(snap, 1) {
		return fmt.Errorf("invites: honey reserve exhausted for trace %q (use paseka energy add)", traceID)
	}

	ev, err := protocol.NewEvent(traceID, "beekeeper", 0, protocol.EventSignal, protocol.EnergyConsumePayload{
		Kind:   protocol.SignalEnergyConsume,
		Amount: 1,
		Reason: "session.start",
	})
	if err != nil {
		return err
	}
	if err := pub.PublishEvent(ctx, ev); err != nil {
		return err
	}
	if !reactorRunning(s.Colony.Slug) {
		if _, err := ledger.Apply(ev); err != nil {
			if errors.Is(err, errHoneyReserveExhausted) {
				return fmt.Errorf("invites: honey reserve exhausted for trace %q (use paseka energy add)", traceID)
			}
			return err
		}
		return nil
	}
	return waitForEnergyDecrease(ledger, traceID, snap.EnergyRemaining, 1)
}

func seedEnergyIfNeeded(ledger taskledger.Ledger, colonyRoot, traceID string) error {
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	if snap.EnergyBudget > 0 {
		return nil
	}
	budget := protocol.DefaultEnergyBudget
	if colonyRoot != "" {
		manifest, err := colony.LoadColony(colonyRoot)
		if err != nil {
			return err
		}
		budget = manifest.ResolvedEnergyBudget()
	}
	return ledger.SeedEnergy(traceID, budget)
}

func reactorRunning(slug string) bool {
	entry, err := colony.RuntimeRegistry(slug)
	if err != nil || entry == nil || entry.PID <= 0 {
		return false
	}
	if !colony.ProcessAlive(entry.PID) {
		return false
	}
	status := entry.Status
	return status == "" || status == "running"
}

func waitForEnergyDecrease(ledger taskledger.Ledger, traceID string, beforeRemaining, amount int) error {
	target := beforeRemaining - amount
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap, err := ledger.Snapshot(traceID)
		if err != nil {
			return err
		}
		if snap.EnergyRemaining <= target {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	if snap.EnergyRemaining > target {
		return fmt.Errorf("invites: timed out waiting for energy.consume projection")
	}
	return nil
}
