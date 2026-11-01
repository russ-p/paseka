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
// turn ready into blocked. The ledger opens the pause episode in the same
// mutation; this call then publishes its one paused observation if it is new.
func (r *Reactor) deferTaskDispatch(ctx context.Context, traceID, taskID, beeRole, sourceEventID, reason string) error {
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
	return r.publishPauseObservation(ctx, traceID)
}

// deferDirectDispatch records a direct reaction that could not start yet. The
// task lifecycle status is untouched: the reaction belongs to an event, not to
// the task's next state. A configuration barrier hold is not energy waiting, so
// it does not open a pause episode.
func (r *Reactor) deferDirectDispatch(ctx context.Context, ev protocol.Event, beeRole, taskID, reason string) error {
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
	return r.publishPauseObservation(ctx, ev.TraceID)
}

// RecoverPendingDispatches drains work that a top-up approved while the reactor
// was down. The ledger is the only authority here: an `energy.add` applied by
// `paseka energy add` or another process has already raised the reserve, so
// startup resumes whatever that top-up made runnable.
//
// Kill and allocation boundaries are already durable in the ledger, so the
// drain inherits them: a killed trail is skipped and stipend-abandoned entries
// are not active. A trail without reserve cannot admit anything and is skipped
// too. Each trail drains task and direct work in one creation-order queue
// through the same resume path an `energy.add` uses, and a missing direct source
// event becomes unresolved there instead of blocking the rest of the queue.
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
		if err := r.recoverTracePendingDispatches(ctx, traceID); err != nil {
			// One unreadable trail must not stop the others from resuming.
			runtimeLog.Warn("resume pending dispatches after restart failed",
				logging.F("trace", traceID),
				logging.F("error", err.Error()),
			)
		}
	}
	return nil
}

// recoverTracePendingDispatches publishes any pause episode that never reached
// the bus, then drains work the ledger already approved. The paused observation
// goes out before the drain, so an operator sees why the trail stopped before
// it starts admitting work again. Once the reserve can admit work the episode
// closes and one resumed observation reports the drain.
func (r *Reactor) recoverTracePendingDispatches(ctx context.Context, traceID string) error {
	snap, err := r.ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	// Count before the drain: the ledger snapshot aliases its working slice, and
	// the drain closes entries as it admits them.
	pendingBefore := taskledger.ActivePendingCount(snap)
	if err := r.publishPauseObservation(ctx, traceID); err != nil {
		return err
	}
	if snap.Killed {
		return nil
	}
	dispatched, err := r.resumePendingDispatches(ctx, traceID)
	if err != nil {
		return err
	}
	return r.settlePauseAfterIncrease(ctx, traceID, protocol.EnergyResumeSourceAdd, pendingBefore, dispatched)
}

// resumePendingDispatches drains deferred work after energy.add and reports how
// many entries were admitted. Task and direct entries share one creation-order
// queue: the oldest entry is considered first, with the id breaking ties. A task
// entry resumes through the normal task dispatch path from the current task
// snapshot, so its in-flight, attempt, configuration, and kill guards still
// decide; the stored source event identity only marks the attempt the resumed
// dispatch belongs to, it is never replayed.
//
// Entries drain until the reserve is exhausted, so a partial top-up runs what it
// can and leaves the rest pending for the next one. Barrier-held items are never
// drained here: they must wait for the configuration boundary and are resumed by
// resumeBarrierHeldDispatches.
func (r *Reactor) resumePendingDispatches(ctx context.Context, traceID string) (int, error) {
	dispatched := 0
	snap, err := r.ledger.Snapshot(traceID)
	if err != nil {
		return dispatched, err
	}
	if snap.Killed || snap.EnergyRemaining <= 0 {
		return dispatched, nil
	}
	pending := taskledger.ActivePendingDispatches(snap)
	if !hasResumablePending(pending) {
		return dispatched, nil
	}

	// Direct work needs its source event to rerun the same reaction. Task work
	// does not: its current snapshot is the work to resume.
	var byID map[string]protocol.Event
	if hasPendingDirect(pending) {
		if !bus.ReplayerAvailable(r.replayer) {
			runtimeLog.Warn("pending direct dispatch cannot resume without an event replayer",
				logging.F("trace", traceID),
			)
		} else {
			events, err := r.replayer.ReplayTrace(traceID)
			if err != nil {
				return dispatched, fmt.Errorf("runtime: replay pending direct dispatches: %w", err)
			}
			byID = make(map[string]protocol.Event, len(events))
			for _, ev := range events {
				byID[eventIdentity(ev)] = ev
			}
		}
	}
	for _, item := range pending {
		var admitted bool
		switch item.Kind {
		case taskledger.PendingDispatchTask:
			admitted, err = r.resumeTaskPendingDispatch(ctx, traceID, item)
		case taskledger.PendingDispatchDirect:
			if !resumableDirectDispatch(item) || byID == nil {
				continue
			}
			admitted, err = r.resumeDirectPendingDispatch(ctx, traceID, item, byID)
		default:
			continue
		}
		if err != nil {
			return dispatched, err
		}
		if admitted {
			dispatched++
		}
		// Every admitted dispatch costs one honey. Once the reserve is gone,
		// nothing behind this entry can start, so stop instead of walking — and
		// re-deferring — the rest of the queue.
		left, err := r.ledger.Snapshot(traceID)
		if err != nil {
			return dispatched, err
		}
		if left.EnergyRemaining <= 0 {
			break
		}
	}
	return dispatched, nil
}

// resumeTaskPendingDispatch resumes one deferred task dispatch. The pending
// entry is only a marker: the ledger's current task snapshot is authoritative,
// so a task that closed or turned terminal while waiting is abandoned instead
// of replaying stale work. A dispatch that re-defers because the reserve ran out
// again leaves the entry pending, so the drain never claims a false resume.
func (r *Reactor) resumeTaskPendingDispatch(ctx context.Context, traceID string, item taskledger.PendingDispatch) (bool, error) {
	// One process must not start two live dispatches for the same entry. Two
	// energy events can arrive close enough together that both see the entry
	// active; the claim lets only the first one dispatch.
	if !r.claimPendingResume(traceID, item.ID) {
		return false, nil
	}
	defer r.releasePendingResume(traceID, item.ID)

	snap, err := r.ledger.Snapshot(traceID)
	if err != nil {
		return false, err
	}
	if snap.Killed {
		return false, nil
	}
	entry := findPendingDispatch(snap, item.ID)
	if entry == nil || !entry.Status.Active() {
		// Another mutation already closed the entry.
		return false, nil
	}
	task, ok := snap.Tasks[item.TaskID]
	if closed := taskPendingClosed(task, ok); closed != "" {
		runtimeLog.Info("pending task dispatch abandoned",
			logging.F("trace", traceID),
			logging.F("task", item.TaskID),
			logging.F("bee", item.Bee),
			logging.F("reason", closed),
		)
		return false, r.ledger.ResolvePendingDispatch(traceID, item.ID, taskledger.PendingDispatchAbandoned)
	}

	if err := r.dispatchReady(ctx, traceID, task, nil, item.SourceEventID); err != nil {
		return false, err
	}
	after, err := r.ledger.Snapshot(traceID)
	if err != nil {
		return false, err
	}
	entry = findPendingDispatch(after, item.ID)
	if entry == nil || !entry.Status.Active() {
		// The dispatch (or a concurrent guard) closed the entry.
		return false, nil
	}
	if moved, ok := after.Tasks[item.TaskID]; !ok || moved.Status == protocol.TaskStatusReady {
		// The dispatch was not admitted: it re-deferred because the reserve ran
		// out, awaited a configuration boundary, or a guard skipped it. The
		// entry stays pending for the next top-up.
		return false, nil
	}
	return true, r.ledger.ResolvePendingDispatch(traceID, item.ID, taskledger.PendingDispatchConsumed)
}

// taskPendingClosed reports why a deferred task dispatch can no longer run, or
// an empty string when the task is still `ready` and dispatchable. A task that
// moved past ready already has an owner or outcome; replaying it would duplicate
// or override that work.
func taskPendingClosed(task taskledger.TaskSnapshot, ok bool) string {
	if !ok || task.TaskID == "" {
		return "task no longer exists"
	}
	if task.Status == protocol.TaskStatusReady {
		return ""
	}
	return fmt.Sprintf("task is %q, not ready", task.Status)
}

// resumeDirectPendingDispatch resumes one deferred direct reaction through the
// same dispatch path its original event used.
func (r *Reactor) resumeDirectPendingDispatch(ctx context.Context, traceID string, item taskledger.PendingDispatch, byID map[string]protocol.Event) (bool, error) {
	ev, ok := byID[item.SourceEventID]
	if !ok {
		return false, r.markPendingUnresolved(traceID, item)
	}
	r.stampIncomingProvenance(&ev)
	if err := r.dispatchDirect(ctx, ev, item.Bee, nil); err != nil {
		return false, err
	}
	// Resolve against the dispatch the event actually produces, not the stored
	// id: a legacy entry may carry an id from an older key format, and only a
	// dispatch that really ran should be marked consumed.
	if r.directProcessedRecorded(directDispatchKey(ev, item.Bee)) {
		return true, r.ledger.ResolvePendingDispatch(traceID, item.ID, taskledger.PendingDispatchConsumed)
	}
	return false, nil
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

// hasResumablePending reports whether the drain has work it may consider. Task
// entries and energy-deferred direct entries share the queue; configuration-held
// direct work is owned by its transition, not by an energy increase.
func hasResumablePending(pending []taskledger.PendingDispatch) bool {
	for _, item := range pending {
		if item.Kind == taskledger.PendingDispatchTask || resumableDirectDispatch(item) {
			return true
		}
	}
	return false
}

func findPendingDispatch(snap taskledger.TraceSnapshot, dispatchID string) *taskledger.PendingDispatch {
	for i := range snap.PendingDispatches {
		if snap.PendingDispatches[i].ID == dispatchID {
			return &snap.PendingDispatches[i]
		}
	}
	return nil
}

func pendingResumeKey(traceID, dispatchID string) string {
	return traceID + "\x00" + dispatchID
}

// claimPendingResume marks a pending entry as being resumed by this process, so
// two close energy events cannot both start a live dispatch for it.
func (r *Reactor) claimPendingResume(traceID, dispatchID string) bool {
	key := pendingResumeKey(traceID, dispatchID)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pendingResume == nil {
		r.pendingResume = make(map[string]struct{})
	}
	if _, ok := r.pendingResume[key]; ok {
		return false
	}
	r.pendingResume[key] = struct{}{}
	return true
}

func (r *Reactor) releasePendingResume(traceID, dispatchID string) {
	key := pendingResumeKey(traceID, dispatchID)
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.pendingResume, key)
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
