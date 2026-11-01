package taskledger

import (
	"sort"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
)

func pendingReadyFromPayload(payload protocol.TaskReadyPayload) *PendingReadySnapshot {
	return &PendingReadySnapshot{
		TaskID: payload.TaskID,
		Title:  payload.Title,
		Body:   payload.Body,
		Bee:    payload.Bee,
		Sector: payload.Sector,
		Intent: payload.Intent,
		CueID:  payload.CueID,
	}
}

func applyReadyOverlays(task TaskSnapshot, payload protocol.TaskReadyPayload) TaskSnapshot {
	if payload.Title != "" {
		task.Title = payload.Title
	}
	if payload.Body != "" {
		task.Body = payload.Body
	}
	if payload.Bee != "" {
		task.Bee = payload.Bee
	}
	if payload.Sector != "" {
		task.Sector = payload.Sector
	}
	if payload.Intent != "" {
		task.Intent = payload.Intent
	}
	if payload.CueID != "" {
		task.CueID = payload.CueID
	}
	return task
}

func applyPendingOverlays(task TaskSnapshot, pending PendingReadySnapshot) TaskSnapshot {
	return applyReadyOverlays(task, protocol.TaskReadyPayload{
		Title:  pending.Title,
		Body:   pending.Body,
		Bee:    pending.Bee,
		Sector: pending.Sector,
		Intent: pending.Intent,
		CueID:  pending.CueID,
	})
}

// tryPromotePendingReady transitions a parked kick to ready when the matching task is now planned and first eligible.
func tryPromotePendingReady(trace TraceSnapshot, now time.Time) (TraceSnapshot, []TaskSnapshot, bool) {
	if trace.PendingReady == nil || trace.PendingReady.TaskID == "" {
		return trace, nil, false
	}
	if trace.Killed || HasReadyTask(trace) {
		return trace, nil, false
	}
	first, ok := FirstEligiblePlanned(trace)
	if !ok || first.TaskID != trace.PendingReady.TaskID {
		return trace, nil, false
	}
	task, ok := trace.Tasks[trace.PendingReady.TaskID]
	if !ok || task.Status != protocol.TaskStatusPlanned {
		return trace, nil, false
	}
	task = applyPendingOverlays(task, *trace.PendingReady)
	task.Status = protocol.TaskStatusReady
	task.UpdatedAt = now
	trace.Tasks[task.TaskID] = task
	trace.PendingReady = nil
	return trace, []TaskSnapshot{task}, true
}

// dropStalePendingReady clears a parked kick once that task is no longer planned (already promoted or terminal).
func dropStalePendingReady(trace TraceSnapshot) (TraceSnapshot, bool) {
	if trace.PendingReady == nil || trace.PendingReady.TaskID == "" {
		return trace, false
	}
	task, ok := trace.Tasks[trace.PendingReady.TaskID]
	if !ok || task.Status == "" || task.Status == protocol.TaskStatusPlanned {
		return trace, false
	}
	trace.PendingReady = nil
	return trace, true
}

// DeferPendingDispatch appends deferred work unless the same id is still pending.
// A killed trail records no new pending work: kill is the hard stop, so a
// deferral racing the kill must not leave runnable work behind.
func DeferPendingDispatch(trace TraceSnapshot, dispatch PendingDispatch) (TraceSnapshot, bool) {
	if dispatch.ID == "" || trace.Killed {
		return trace, false
	}
	now := time.Now().UTC()
	if dispatch.CreatedAt.IsZero() {
		dispatch.CreatedAt = now
	}
	dispatch.UpdatedAt = now
	if dispatch.Status == "" {
		dispatch.Status = PendingDispatchPending
	}
	for _, existing := range trace.PendingDispatches {
		if existing.ID == dispatch.ID && existing.Status.Active() {
			return trace, false
		}
	}
	trace.PendingDispatches = append(trace.PendingDispatches, dispatch)
	return trace, true
}

// ResolvePendingDispatch closes one deferred item without touching task state.
// The same dispatch key can appear more than once: an earlier entry keeps its
// evidence when the reaction is deferred again after a resume. The active entry
// is the one to close, because closing the stale entry would leave runnable
// work behind. A closed entry is targeted only when no active duplicate exists,
// so the legacy states stay addressable.
func ResolvePendingDispatch(trace TraceSnapshot, dispatchID string, status PendingDispatchStatus) (TraceSnapshot, bool) {
	if dispatchID == "" {
		return trace, false
	}
	if status == "" {
		status = PendingDispatchConsumed
	}
	target := -1
	for i := range trace.PendingDispatches {
		if trace.PendingDispatches[i].ID == dispatchID && trace.PendingDispatches[i].Status.Active() {
			target = i
			break
		}
	}
	if target == -1 {
		for i := len(trace.PendingDispatches) - 1; i >= 0; i-- {
			if trace.PendingDispatches[i].ID == dispatchID {
				target = i
				break
			}
		}
	}
	if target == -1 || trace.PendingDispatches[target].Status == status {
		return trace, false
	}
	trace.PendingDispatches[target].Status = status
	trace.PendingDispatches[target].UpdatedAt = time.Now().UTC()
	return trace, true
}

// MarkPendingDispatchUnresolved closes a deferred item whose source event can
// no longer be recovered. The entry stays in the ledger so missing evidence is
// visible; it is never drained as runnable work. Only active work can become
// unresolved: a late diagnostic must not overwrite a consumed or abandoned
// outcome that another mutation already recorded.
func MarkPendingDispatchUnresolved(trace TraceSnapshot, dispatchID, diagnostic string) (TraceSnapshot, bool) {
	if dispatchID == "" {
		return trace, false
	}
	for i := range trace.PendingDispatches {
		entry := &trace.PendingDispatches[i]
		if entry.ID != dispatchID || !entry.Status.Active() {
			continue
		}
		entry.Status = PendingDispatchUnresolved
		entry.Diagnostic = diagnostic
		entry.UpdatedAt = time.Now().UTC()
		return trace, true
	}
	return trace, false
}

// AbandonPendingDispatches closes every active deferred item without dispatching
// it. A hard stop calls this inside the mutation that owns the decision, so the
// abandoned queue is never revived by a later top-up or a restart. Closed
// entries keep their own status: only active work is abandoned.
func AbandonPendingDispatches(trace TraceSnapshot, now time.Time) (TraceSnapshot, bool) {
	return abandonPendingDispatches(trace, now, func(PendingDispatch) bool { return true })
}

// SupersedeEnergyDeferredDispatches closes active work that a new honey
// allocation supersedes: entries that waited on an exhausted reserve, including
// legacy entries with no reason. Configuration-barrier work is not energy
// waiting — it belongs to a pending configuration transition, which releases or
// cancels it at its own safe boundary — so this path leaves it active.
//
// A superseded task entry is closed together with its task, in this same
// mutation: the task becomes skipped with the summary naming the new allocation.
// Pairing the two is what keeps a superseded tick from surfacing as a ready row
// with no pending record, and a duplicate stipend finds nothing left to abandon.
// Existing blocked tasks are never rewritten: legacy honey-blocked work has no
// pending entry and keeps its own unblock-on-energy.add path.
func SupersedeEnergyDeferredDispatches(trace TraceSnapshot, now time.Time, summary string) (TraceSnapshot, bool) {
	changed := false
	for i := range trace.PendingDispatches {
		entry := &trace.PendingDispatches[i]
		if !entry.Status.Active() || entry.Reason == protocol.ConfigBarrierHoldReason {
			continue
		}
		switch entry.Kind {
		case PendingDispatchTask:
			entry.Status = PendingDispatchAbandoned
			entry.UpdatedAt = now
			changed = true
			task, ok := trace.Tasks[entry.TaskID]
			if !ok || !supersedableTaskStatus(task) {
				continue
			}
			task.Status = protocol.TaskStatusSkipped
			task.Summary = summary
			task.UpdatedAt = now
			trace.Tasks[task.TaskID] = task
		case PendingDispatchDirect:
			entry.Status = PendingDispatchAbandoned
			entry.UpdatedAt = now
			changed = true
		}
	}
	return trace, changed
}

// supersedableTaskStatus reports whether a new allocation may mark this task
// skipped. Only a ready task is deferred energy work: anything else already has
// an owner or an outcome, and a legacy honey-blocked task is never rewritten.
func supersedableTaskStatus(task TaskSnapshot) bool {
	return task.Status == protocol.TaskStatusReady
}

func abandonPendingDispatches(trace TraceSnapshot, now time.Time, abandon func(PendingDispatch) bool) (TraceSnapshot, bool) {
	changed := false
	for i := range trace.PendingDispatches {
		entry := &trace.PendingDispatches[i]
		if !entry.Status.Active() || !abandon(*entry) {
			continue
		}
		entry.Status = PendingDispatchAbandoned
		entry.UpdatedAt = now
		changed = true
	}
	return trace, changed
}

// ActivePendingDispatches returns pending work in creation order: oldest first,
// with the id breaking ties.
func ActivePendingDispatches(trace TraceSnapshot) []PendingDispatch {
	var pending []PendingDispatch
	for _, dispatch := range trace.PendingDispatches {
		if dispatch.Status.Active() {
			pending = append(pending, dispatch)
		}
	}
	sort.SliceStable(pending, func(i, j int) bool {
		if !pending[i].CreatedAt.Equal(pending[j].CreatedAt) {
			return pending[i].CreatedAt.Before(pending[j].CreatedAt)
		}
		return pending[i].ID < pending[j].ID
	})
	return pending
}
