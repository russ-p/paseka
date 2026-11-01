package runtime

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
)

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

// taskDispatchKey deduplicates deferred task work while it is active. A task
// dispatch is identified by the task and the Bee that would run it, not by the
// event that kicked it: a duplicate task.ready while the entry is pending adds
// no second entry and keeps the first source identity.
func taskDispatchKey(taskID, beeRole string) string {
	return fmt.Sprintf("task:%s:%s", taskID, beeRole)
}

// deferTaskDispatch records a task dispatch that could not start because the
// trail's honey reserve is exhausted. The task keeps its lifecycle status:
// honey belongs to the trail, not to one unit of work, so a deferral must not
// turn ready into blocked.
func (r *Reactor) deferTaskDispatch(traceID, taskID, beeRole, sourceEventID, reason string) error {
	dispatch := taskledger.PendingDispatch{
		ID:            taskDispatchKey(taskID, beeRole),
		Kind:          taskledger.PendingDispatchTask,
		TaskID:        taskID,
		Bee:           beeRole,
		SourceEventID: sourceEventID,
		Reason:        reason,
		Status:        taskledger.PendingDispatchPending,
		CreatedAt:     time.Now().UTC(),
	}
	added, err := r.ledger.DeferPendingDispatch(traceID, dispatch)
	if err != nil {
		return err
	}
	if added {
		runtimeLog.Info("task dispatch deferred",
			logging.F("trace", traceID),
			logging.F("task", taskID),
			logging.F("bee", beeRole),
			logging.F("reason", reason),
		)
	}
	return nil
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

// RecoverPendingDispatches drains direct work that a top-up approved while the
// reactor was down. The ledger is the only authority here: an `energy.add`
// applied by `paseka energy add` or another process has already raised the
// reserve, so startup resumes whatever that top-up made runnable.
//
// Kill and allocation boundaries are already durable in the ledger, so the
// drain inherits them: a killed trail is skipped and stipend-abandoned entries
// are not active. A trail without reserve cannot admit anything and is skipped
// too. Each trail drains oldest-first through the same resume path an
// `energy.add` uses, and a missing source event becomes unresolved there
// instead of blocking the rest of the queue.
func (r *Reactor) RecoverPendingDispatches(ctx context.Context) error {
	if r.ledger == nil {
		return nil
	}
	lister, ok := r.ledger.(taskledger.TraceLister)
	if !ok {
		return nil
	}
	traceIDs, err := lister.TraceIDs()
	if err != nil {
		return fmt.Errorf("runtime: list trails for pending dispatch recovery: %w", err)
	}
	sort.Strings(traceIDs)
	for _, traceID := range traceIDs {
		if err := r.resumePendingDispatches(ctx, traceID); err != nil {
			// One unreadable trail must not stop the others from resuming.
			runtimeLog.Warn("resume pending direct dispatches after restart failed",
				logging.F("trace", traceID),
				logging.F("error", err.Error()),
			)
		}
	}
	return nil
}

// resumePendingDispatches drains deferred work after energy.add. It resumes only
// direct dispatches in this slice; deferred task work is recorded here but
// resumes through the task dispatch path in the follow-up slice, and legacy
// honey-blocked tasks keep the existing task-status unblock. Entries drain
// oldest-first and the drain stops once the reserve is exhausted, so the rest
// stays pending for the next top-up.
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
			if err := r.markPendingUnresolved(traceID, item); err != nil {
				return err
			}
			continue
		}
		r.stampIncomingProvenance(&ev)
		if err := r.dispatchDirect(ctx, ev, item.Bee, nil); err != nil {
			return err
		}
		// Resolve against the dispatch the event actually produces, not the
		// stored id: a legacy entry may carry an id from an older key format,
		// and only a dispatch that really ran should be marked consumed.
		if r.directProcessedRecorded(directDispatchKey(ev, item.Bee)) {
			if err := r.ledger.ResolvePendingDispatch(traceID, item.ID, taskledger.PendingDispatchConsumed); err != nil {
				return err
			}
		}
		// Every admitted dispatch costs one honey. Once the reserve is gone,
		// nothing behind this entry can start, so stop instead of walking — and
		// re-deferring — the rest of the queue.
		left, err := r.ledger.Snapshot(traceID)
		if err != nil {
			return err
		}
		if left.EnergyRemaining <= 0 {
			break
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
			if err := r.markPendingUnresolved(traceID, item); err != nil {
				return err
			}
			continue
		}
		r.stampIncomingProvenance(&ev)
		if err := r.dispatchDirect(ctx, ev, item.Bee, nil); err != nil {
			return err
		}
		if !r.directProcessedRecorded(directDispatchKey(ev, item.Bee)) {
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

// markPendingUnresolved keeps a deferred direct dispatch in the ledger when the
// source event it reacted to can no longer be recovered. The entry becomes
// unresolved: visible as missing evidence, never drained as runnable work.
func (r *Reactor) markPendingUnresolved(traceID string, item taskledger.PendingDispatch) error {
	runtimeLog.Warn("pending direct dispatch source event missing",
		logging.F("trace", traceID),
		logging.F("task", item.TaskID),
		logging.F("bee", item.Bee),
		logging.F("sourceEvent", item.SourceEventID),
	)
	diagnostic := fmt.Sprintf("source event %q not found in trace replay", item.SourceEventID)
	return r.ledger.MarkPendingDispatchUnresolved(traceID, item.ID, diagnostic)
}

func hasPendingDirect(pending []taskledger.PendingDispatch) bool {
	for _, item := range pending {
		if resumableDirectDispatch(item) {
			return true
		}
	}
	return false
}

// resumableDirectDispatch reports whether the energy drain may run this entry.
// Configuration-barrier work is not energy waiting: a pending configuration
// transition owns it and releases it at its own safe boundary.
func resumableDirectDispatch(item taskledger.PendingDispatch) bool {
	return item.Kind == taskledger.PendingDispatchDirect && item.Reason != protocol.ConfigBarrierHoldReason
}

func barrierHeldDispatches(snap taskledger.TraceSnapshot) []taskledger.PendingDispatch {
	var held []taskledger.PendingDispatch
	for _, item := range taskledger.ActivePendingDispatches(snap) {
		if item.Kind == taskledger.PendingDispatchDirect && item.Reason == protocol.ConfigBarrierHoldReason {
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
