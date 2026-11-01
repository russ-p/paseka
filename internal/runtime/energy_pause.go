package runtime

import (
	"context"
	"strconv"

	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

// afterEnergyAdd runs the trail-side work a top-up unblocks: it publishes any
// pause episode that never reached the bus, drains deferred work, and closes the
// episode once the reserve can admit work again. The resumed observation is sent
// after the drain and after the ledger cleared the field, so a restart can lose
// at most one notification and never duplicates one.
func (r *Reactor) afterEnergyAdd(ctx context.Context, traceID string) error {
	if err := r.publishPauseObservation(ctx, traceID); err != nil {
		return err
	}
	before, err := r.ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	pendingBefore := taskledger.ActivePendingCount(before)
	if err := r.unblockEnergyBlockedTasks(ctx, traceID); err != nil {
		return err
	}
	dispatched, err := r.resumePendingDispatches(ctx, traceID)
	if err != nil {
		return err
	}
	return r.settlePauseAfterIncrease(ctx, traceID, protocol.EnergyResumeSourceAdd, pendingBefore, dispatched)
}

// publishPauseObservation claims the open pause episode through the ledger's
// compare-and-set path and sends its single paused observation. A claim that
// loses to another sender, a replay, or a restart reports false and sends
// nothing, so an episode never produces two pauses.
func (r *Reactor) publishPauseObservation(ctx context.Context, traceID string) error {
	pause, ok, err := r.ledger.ClaimEnergyPausePublished(traceID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	ev, err := protocol.NewEvent(traceID, protocol.RuntimeAgentID, 0, protocol.EventSignal, protocol.EnergyPausedPayload{
		Kind:         protocol.SignalEnergyPaused,
		EpisodeID:    pause.EpisodeID,
		Reason:       pause.Reason,
		Remaining:    pause.Remaining,
		Budget:       pause.Budget,
		PendingCount: pause.Pending,
	})
	if err != nil {
		return err
	}
	runtimeLog.Info("energy pause observation",
		logging.F("trace", traceID),
		logging.F("episode", pause.EpisodeID),
		logging.F("pending", strconv.Itoa(pause.Pending)),
	)
	return r.publishObservation(ctx, ev)
}

// settlePauseAfterIncrease closes the active pause episode when an increase let
// the reserve admit work again, and publishes the one resumed observation for
// it. A partial drain that leaves the reserve exhausted keeps the episode: the
// trail still cannot admit anything, so no false resume is sent.
func (r *Reactor) settlePauseAfterIncrease(ctx context.Context, traceID string, source protocol.EnergyResumeSource, pendingBefore, dispatched int) error {
	snap, err := r.ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	if snap.Killed || snap.EnergyPause == nil || snap.EnergyRemaining < energyDispatchCost {
		return nil
	}
	pause, ok, err := r.ledger.ClearEnergyPause(traceID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	after, err := r.ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	ev, err := protocol.NewEvent(traceID, protocol.RuntimeAgentID, 0, protocol.EventSignal, protocol.EnergyResumedPayload{
		Kind:          protocol.SignalEnergyResumed,
		EpisodeID:     pause.EpisodeID,
		Source:        source,
		Remaining:     after.EnergyRemaining,
		Budget:        taskledger.Allocated(after.EnergyBudget, after.EnergyAdded),
		PendingBefore: pendingBefore,
		PendingAfter:  taskledger.ActivePendingCount(after),
		Dispatched:    dispatched,
	})
	if err != nil {
		return err
	}
	runtimeLog.Info("energy resume observation",
		logging.F("trace", traceID),
		logging.F("episode", pause.EpisodeID),
		logging.F("source", string(source)),
		logging.F("dispatched", strconv.Itoa(dispatched)),
	)
	return r.publishObservation(ctx, ev)
}

// publishResumeObservation sends the resumed observation a reduction already
// prepared. The ledger cleared the episode inside its own mutation, so the send
// can happen at most once per episode.
func (r *Reactor) publishResumeObservation(ctx context.Context, traceID string, closed taskledger.EnergyPauseClose) error {
	ev, err := protocol.NewEvent(traceID, protocol.RuntimeAgentID, 0, protocol.EventSignal, protocol.EnergyResumedPayload{
		Kind:          protocol.SignalEnergyResumed,
		EpisodeID:     closed.Episode.EpisodeID,
		Source:        closed.Source,
		Remaining:     closed.Remaining,
		Budget:        closed.Budget,
		PendingBefore: closed.PendingBefore,
		PendingAfter:  closed.PendingAfter,
		Dispatched:    closed.Dispatched,
		Abandoned:     closed.Abandoned,
	})
	if err != nil {
		return err
	}
	runtimeLog.Info("energy resume observation",
		logging.F("trace", traceID),
		logging.F("episode", closed.Episode.EpisodeID),
		logging.F("source", string(closed.Source)),
		logging.F("abandoned", strconv.Itoa(closed.Abandoned)),
	)
	return r.publishObservation(ctx, ev)
}

// publishObservation sends a platform observation. It is remembered as a local
// event so the JetStream echo is not applied or routed again, and a colony with
// no publisher simply records it in the ledger state that produced it.
func (r *Reactor) publishObservation(ctx context.Context, ev protocol.Event) error {
	r.rememberLocalEvent(ev)
	if !bus.PublisherAvailable(r.publisher) {
		return nil
	}
	return r.publisher.PublishEvent(ctx, ev)
}
