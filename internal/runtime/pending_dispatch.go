package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
)

// configBarrierHoldReason marks direct work that was recorded instead of
// dropped because a pending Apply configuration had not reached its safe
// boundary yet. The item is drained only after that boundary is resolved.
const configBarrierHoldReason = "configuration change pending"

// barrierHeldState tracks, per trail and per process, whether this reactor has
// already looked for barrier-held work. The probe is deliberately cheap: a
// trail is read from the ledger once, unless a deferral happened in this
// process, because only then can new work have appeared since the last probe.
type barrierHeldState struct {
	deferred  bool   // a barrier hold was recorded by this reactor
	probed    bool   // the ledger snapshot was read at least once
	draining  bool   // a drain is running; nested drains must not recurse
	deferrals uint64 // total deferrals recorded by this reactor
	drainMark uint64 // deferrals counter when the running drain began
}

// deferDirectDispatch records a direct reaction that could not start yet. The
// task lifecycle status is untouched: the reaction belongs to an event, not to
// the task's next state.
func (r *Reactor) deferDirectDispatch(ev protocol.Event, beeRole, taskID, reason string) error {
	dispatch := taskledger.PendingDispatch{
		ID:            directDispatchKey(ev, beeRole),
		Kind:          taskledger.PendingDispatchDirect,
		TaskID:        taskID,
		Bee:           beeRole,
		SourceEventID: eventIdentity(ev),
		Reason:        reason,
		Status:        taskledger.PendingDispatchPending,
		CreatedAt:     time.Now().UTC(),
	}
	added, err := r.ledger.DeferPendingDispatch(ev.TraceID, dispatch)
	if err != nil {
		return err
	}
	if added {
		runtimeLog.Info("direct dispatch deferred",
			logging.F("trace", ev.TraceID),
			logging.F("task", taskID),
			logging.F("bee", beeRole),
			logging.F("reason", reason),
		)
	}
	return nil
}

// resumePendingDispatches drains deferred work after energy.add. It resumes only
// direct dispatches in this slice; task dispatch unblocking stays in the
// existing task-status path.
//
// Barrier-held items are never drained here: they must wait for the
// configuration boundary and are resumed by resumeBarrierHeldDispatches.
func (r *Reactor) resumePendingDispatches(ctx context.Context, traceID string) error {
	snap, err := r.ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	if snap.Killed || snap.EnergyRemaining <= 0 {
		return nil
	}
	pending := taskledger.ActivePendingDispatches(snap)
	if !hasPendingDirect(pending) {
		return nil
	}
	if !bus.ReplayerAvailable(r.replayer) {
		runtimeLog.Warn("pending direct dispatch cannot resume without an event replayer",
			logging.F("trace", traceID),
		)
		return nil
	}
	events, err := r.replayer.ReplayTrace(traceID)
	if err != nil {
		return fmt.Errorf("runtime: replay pending direct dispatches: %w", err)
	}
	byID := make(map[string]protocol.Event, len(events))
	for _, ev := range events {
		byID[eventIdentity(ev)] = ev
	}
	for _, item := range pending {
		if !resumableDirectDispatch(item) {
			continue
		}
		ev, ok := byID[item.SourceEventID]
		if !ok {
			runtimeLog.Warn("pending direct dispatch source event missing",
				logging.F("trace", traceID),
				logging.F("task", item.TaskID),
				logging.F("bee", item.Bee),
				logging.F("sourceEvent", item.SourceEventID),
			)
			continue
		}
		r.stampIncomingProvenance(&ev)
		if err := r.dispatchDirect(ctx, ev, item.Bee, nil); err != nil {
			return err
		}
		if r.directProcessedRecorded(item.ID) {
			if err := r.ledger.ResolvePendingDispatch(traceID, item.ID, taskledger.PendingDispatchConsumed); err != nil {
				return err
			}
		}
	}
	return nil
}

// resumeBarrierHeldDispatches resumes direct chains that a pending configuration
// change held instead of dropping. It does nothing while a change is still
// pending: the existing admission barrier owns that decision. Once the change is
// activated or cancelled it replays the source events and redispatches them
// through dispatchDirect, so the now-active execution context is resolved
// normally. An item is resolved only when its dispatch actually ran; a killed or
// dropped trail therefore keeps its held work durable.
func (r *Reactor) resumeBarrierHeldDispatches(ctx context.Context, traceID string) error {
	if r.ledger == nil || traceID == "" || r.colony.ColonyRoot == "" {
		return nil
	}
	// Claim the drain before touching disk: an unchanged trail is never probed
	// twice by the same process.
	if !r.beginBarrierHeldDrain(traceID) {
		return nil
	}
	remaining := true
	defer func() { r.endBarrierHeldDrain(traceID, remaining) }()

	held, err := trailconfig.HasPending(r.colony.ColonyRoot, traceID)
	if err != nil {
		return err
	}
	if held {
		// The admission barrier still owns this decision.
		return nil
	}

	snap, err := r.ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	r.markBarrierHeldProbed(traceID)
	if snap.Killed {
		// Held work stays durable, but a killed trail never resumes it.
		remaining = false
		return nil
	}
	pending := barrierHeldDispatches(snap)
	if len(pending) == 0 {
		remaining = false
		return nil
	}
	if !bus.ReplayerAvailable(r.replayer) {
		runtimeLog.Warn("barrier-held direct dispatch cannot resume without an event replayer",
			logging.F("trace", traceID),
		)
		return nil
	}
	events, err := r.replayer.ReplayTrace(traceID)
	if err != nil {
		return fmt.Errorf("runtime: replay barrier-held direct dispatches: %w", err)
	}
	byID := make(map[string]protocol.Event, len(events))
	for _, ev := range events {
		byID[eventIdentity(ev)] = ev
	}
	unresolved := 0
	for _, item := range pending {
		ev, ok := byID[item.SourceEventID]
		if !ok {
			unresolved++
			runtimeLog.Warn("barrier-held direct dispatch source event missing",
				logging.F("trace", traceID),
				logging.F("task", item.TaskID),
				logging.F("bee", item.Bee),
				logging.F("sourceEvent", item.SourceEventID),
			)
			continue
		}
		r.stampIncomingProvenance(&ev)
		if err := r.dispatchDirect(ctx, ev, item.Bee, nil); err != nil {
			return err
		}
		if !r.directProcessedRecorded(item.ID) {
			unresolved++
			continue
		}
		if err := r.ledger.ResolvePendingDispatch(traceID, item.ID, taskledger.PendingDispatchConsumed); err != nil {
			return err
		}
	}
	remaining = unresolved > 0
	return nil
}

func hasPendingDirect(pending []taskledger.PendingDispatch) bool {
	for _, item := range pending {
		if resumableDirectDispatch(item) {
			return true
		}
	}
	return false
}

func resumableDirectDispatch(item taskledger.PendingDispatch) bool {
	return item.Kind == taskledger.PendingDispatchDirect && item.Reason != configBarrierHoldReason
}

func barrierHeldDispatches(snap taskledger.TraceSnapshot) []taskledger.PendingDispatch {
	var held []taskledger.PendingDispatch
	for _, item := range taskledger.ActivePendingDispatches(snap) {
		if item.Kind == taskledger.PendingDispatchDirect && item.Reason == configBarrierHoldReason {
			held = append(held, item)
		}
	}
	return held
}

func (r *Reactor) directProcessedRecorded(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.directProcessed[key]
	return ok
}

// markBarrierHeldDeferral remembers that this reactor recorded a barrier hold,
// so the next boundary retries even if the trail was already probed.
func (r *Reactor) markBarrierHeldDeferral(traceID string) {
	if traceID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state := r.barrierHeldLocked(traceID)
	state.deferred = true
	state.deferrals++
}

// beginBarrierHeldDrain claims the right to read the ledger for held work. It
// reports false when a drain is already running or when this process already
// probed the trail and no deferral happened since.
func (r *Reactor) beginBarrierHeldDrain(traceID string) bool {
	if traceID == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state := r.barrierHeldLocked(traceID)
	if state.draining {
		return false
	}
	if state.probed && !state.deferred {
		return false
	}
	state.draining = true
	state.drainMark = state.deferrals
	return true
}

// markBarrierHeldProbed records that the ledger was read successfully, so an
// unchanged trail is not read again by this process.
func (r *Reactor) markBarrierHeldProbed(traceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.barrierHeldLocked(traceID).probed = true
}

// endBarrierHeldDrain releases the drain claim and forgets the deferral once no
// held item remains and no new deferral arrived while the drain was running.
func (r *Reactor) endBarrierHeldDrain(traceID string, remaining bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state := r.barrierHeldLocked(traceID)
	state.draining = false
	if !remaining && state.deferrals == state.drainMark {
		state.deferred = false
	}
}

func (r *Reactor) barrierHeldLocked(traceID string) *barrierHeldState {
	if r.barrierHeld == nil {
		r.barrierHeld = make(map[string]*barrierHeldState)
	}
	state := r.barrierHeld[traceID]
	if state == nil {
		state = &barrierHeldState{}
		r.barrierHeld[traceID] = state
	}
	return state
}
