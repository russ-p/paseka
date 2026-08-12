package invites

import (
	"context"
	"errors"
	"fmt"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/energy"
	"github.com/paseka/paseka/internal/taskledger"
)

func (s *Service) consumeSessionEnergy(ctx context.Context, traceID string) error {
	pub := s.publisher()
	var client *bus.Client
	if c, ok := pub.(*bus.Client); ok && c != nil {
		client = c
	} else {
		var err error
		client, err = bus.ConnectColony(s.Colony, false)
		if err != nil {
			return err
		}
		if client == nil {
			return nil
		}
		defer client.Close()
		if pub == nil {
			pub = client
		}
	}

	kv, err := client.JetStream().KeyValue(bus.TaskLedgerBucket(s.Colony.Slug))
	if err != nil {
		return fmt.Errorf("invites: task ledger kv: %w", err)
	}
	ledger := taskledger.NewKVLedger(kv)

	if err := energy.EnsureSeeded(ledger, s.Colony.ColonyRoot, traceID); err != nil {
		return err
	}
	before, err := ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	if !taskledger.HasEnergy(before, 1) {
		return fmt.Errorf("invites: honey reserve exhausted for trace %q (use paseka energy add)", traceID)
	}

	snap, err := energy.Consume(ctx, s.Colony.Slug, s.Colony.ColonyRoot, ledger, pub, energy.ConsumeInput{
		TraceID: traceID,
		Amount:  1,
		Reason:  "session.start",
		AgentID: "beekeeper",
	})
	if errors.Is(err, energy.ErrHoneyReserveExhausted) {
		return fmt.Errorf("invites: honey reserve exhausted for trace %q (use paseka energy add)", traceID)
	}
	if err != nil {
		return err
	}
	target := before.EnergyRemaining - 1
	if snap.EnergyRemaining > target {
		return fmt.Errorf("invites: timed out waiting for energy.consume projection")
	}
	return nil
}
