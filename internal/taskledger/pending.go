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
func ResolvePendingDispatch(trace TraceSnapshot, dispatchID string, status PendingDispatchStatus) (TraceSnapshot, bool) {
	if dispatchID == "" {
		return trace, false
	}
	if status == "" {
		status = PendingDispatchConsumed
	}
	for i := range trace.PendingDispatches {
		if trace.PendingDispatches[i].ID != dispatchID {
			continue
		}
		if trace.PendingDispatches[i].Status == status {
			return trace, false
		}
		trace.PendingDispatches[i].Status = status
		trace.PendingDispatches[i].UpdatedAt = time.Now().UTC()
		return trace, true
	}
	return trace, false
}

// MarkPendingDispatchUnresolved closes a deferred item whose source event can
// no longer be recovered. The entry stays in the ledger so missing evidence is
// visible; it is never drained as runnable work.
func MarkPendingDispatchUnresolved(trace TraceSnapshot, dispatchID, diagnostic string) (TraceSnapshot, bool) {
	if dispatchID == "" {
		return trace, false
	}
	for i := range trace.PendingDispatches {
		entry := &trace.PendingDispatches[i]
		if entry.ID != dispatchID {
			continue
		}
		if entry.Status == PendingDispatchUnresolved && entry.Diagnostic == diagnostic {
			return trace, false
		}
		entry.Status = PendingDispatchUnresolved
		entry.Diagnostic = diagnostic
		entry.UpdatedAt = time.Now().UTC()
		return trace, true
	}
	return trace, false
}

// AbandonPendingDispatches closes every active deferred item without dispatching
// it. A hard stop or a new allocation calls this inside the mutation that owns
// the decision, so the abandoned queue is never revived by a later top-up or a
// restart. Closed entries keep their own status: only active work is abandoned.
func AbandonPendingDispatches(trace TraceSnapshot, now time.Time) (TraceSnapshot, bool) {
	changed := false
	for i := range trace.PendingDispatches {
		entry := &trace.PendingDispatches[i]
		if !entry.Status.Active() {
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
