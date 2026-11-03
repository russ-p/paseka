package runtime

import (
	"context"
	"sort"

	"github.com/russ-p/paseka/internal/energy"
	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

const energyDispatchCost = 1

// seedObservationPublisher routes the seed observation through the reactor's
// local-event memory, so the reactor does not apply or route its own echo.
type seedObservationPublisher struct {
	reactor *Reactor
}

func (p seedObservationPublisher) PublishEvent(ctx context.Context, ev protocol.Event) error {
	return p.reactor.publishObservation(ctx, ev)
}

func (r *Reactor) ensureEnergySeeded(ctx context.Context, traceID string) error {
	return energy.EnsureSeeded(ctx, r.ledger, seedObservationPublisher{reactor: r}, r.colony.ColonyRoot, traceID, protocol.RuntimeAgentID)
}

// RecoverPendingSeedObservations repairs seed observations that never reached
// the publisher before the reactor stopped. It republishes the recorded winner
// with its original identity and never seeds a trail that has no record.
func (r *Reactor) RecoverPendingSeedObservations(ctx context.Context) error {
	if r.ledger == nil {
		return nil
	}
	lister, ok := r.ledger.(taskledger.TraceLister)
	if !ok {
		return nil
	}
	traceIDs, err := lister.TraceIDs()
	if err != nil {
		return err
	}
	sort.Strings(traceIDs)
	for _, traceID := range traceIDs {
		if err := energy.PublishPendingSeedObservation(ctx, r.ledger, seedObservationPublisher{reactor: r}, traceID, protocol.RuntimeAgentID); err != nil {
			runtimeLog.Warn("repair pending energy.seeded observation failed",
				logging.F("trace", traceID),
				logging.F("error", err.Error()),
			)
		}
	}
	return nil
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
	if snap.Killed {
		return nil
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
		if err := r.dispatchReady(ctx, traceID, task, nil, "energy-unblock:"+traceID+":"+task.TaskID); err != nil {
			return err
		}
	}
	return nil
}

// gateTaskDispatchEnergy never writes a task status. A denied task dispatch is
// recorded as deferred trail work and keeps its lifecycle state, so `ready`
// still means eligible and a waiting_review task never becomes blocked.
func (r *Reactor) gateTaskDispatchEnergy(ctx context.Context, traceID, taskID, beeRole, sourceEventID, reason string) (bool, error) {
	ok, err := r.consumeEnergy(ctx, traceID, energyDispatchCost, reason, taskID)
	if err != nil {
		return false, err
	}
	if ok {
		return true, nil
	}
	if taskID != "" {
		if err := r.deferTaskDispatch(ctx, traceID, taskID, beeRole, sourceEventID, protocol.HoneyReserveExhaustedSummary); err != nil {
			return false, err
		}
	}
	return false, nil
}

// gateDirectDispatchEnergy never mutates task status: a direct dispatch reacts
// to an event and can run while the task is waiting_review or running. Deferred
// direct work is recorded separately and resumed on energy.add.
func (r *Reactor) gateDirectDispatchEnergy(ctx context.Context, traceID, taskID, reason string) (bool, error) {
	return r.consumeEnergy(ctx, traceID, energyDispatchCost, reason, taskID)
}

func energyAddDetected(ev protocol.Event) bool {
	return ev.Type == protocol.EventSignal && protocol.PayloadKind(ev.Payload) == string(protocol.SignalEnergyAdd)
}
