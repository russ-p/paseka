package runtime

import (
	"context"
	"fmt"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

const energyDispatchCost = 1

// ValidateEnergyAddAmount checks CLI/API honey injection amounts.
func ValidateEnergyAddAmount(amount int) error {
	if amount <= 0 {
		return fmt.Errorf("energy amount must be positive")
	}
	return nil
}

func (r *Reactor) ensureEnergySeeded(ctx context.Context, traceID string) error {
	snap, err := r.ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	if snap.EnergyBudget > 0 {
		return nil
	}
	budget, err := r.resolvedEnergyBudget()
	if err != nil {
		return err
	}
	return r.ledger.SeedEnergy(traceID, budget)
}

func (r *Reactor) resolvedEnergyBudget() (int, error) {
	if r.colony.ColonyRoot == "" {
		return protocol.DefaultEnergyBudget, nil
	}
	manifest, err := colony.LoadColony(r.colony.ColonyRoot)
	if err != nil {
		return 0, err
	}
	return manifest.ResolvedEnergyBudget(), nil
}

func (r *Reactor) consumeEnergy(ctx context.Context, traceID string, amount int, reason, taskID string) (bool, error) {
	if amount <= 0 {
		return true, nil
	}
	if err := r.ensureEnergySeeded(ctx, traceID); err != nil {
		return false, err
	}
	snap, err := r.ledger.Snapshot(traceID)
	if err != nil {
		return false, err
	}
	if !taskledger.HasEnergy(snap, amount) {
		return false, nil
	}

	ev, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventSignal, protocol.EnergyConsumePayload{
		Kind:   protocol.SignalEnergyConsume,
		Amount: amount,
		Reason: reason,
		TaskID: taskID,
	})
	if err != nil {
		return false, err
	}
	return true, r.applyAndSync(ctx, ev)
}

func (r *Reactor) unblockEnergyBlockedTasks(ctx context.Context, traceID string) error {
	snap, err := r.ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	if snap.EnergyRemaining <= 0 {
		return nil
	}
	for _, task := range snap.Tasks {
		if !taskledger.IsEnergyBlockedTask(task) {
			continue
		}
		if err := r.setTaskStatus(ctx, traceID, task.TaskID, protocol.TaskStatusReady, ""); err != nil {
			return err
		}
		updated, err := r.ledger.Snapshot(traceID)
		if err != nil {
			return err
		}
		task = updated.Tasks[task.TaskID]
		if err := r.dispatchReady(ctx, traceID, task); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reactor) blockTaskForEnergy(ctx context.Context, traceID, taskID string) error {
	return r.setTaskStatus(ctx, traceID, taskID, protocol.TaskStatusBlocked, protocol.HoneyReserveExhaustedSummary)
}

func (r *Reactor) gateDispatchEnergy(ctx context.Context, traceID, taskID, reason string) (bool, error) {
	ok, err := r.consumeEnergy(ctx, traceID, energyDispatchCost, reason, taskID)
	if err != nil {
		return false, err
	}
	if ok {
		return true, nil
	}
	if taskID != "" {
		if err := r.blockTaskForEnergy(ctx, traceID, taskID); err != nil {
			return false, err
		}
	}
	return false, nil
}

func energyAddDetected(ev protocol.Event) bool {
	return ev.Type == protocol.EventSignal && protocol.PayloadKind(ev.Payload) == string(protocol.SignalEnergyAdd)
}
