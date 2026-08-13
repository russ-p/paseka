package taskledger

import (
	"time"

	"github.com/paseka/paseka/internal/protocol"
)

func pendingReadyFromPayload(payload protocol.TaskReadyPayload) *PendingReadySnapshot {
	return &PendingReadySnapshot{
		TaskID: payload.TaskID,
		Title:  payload.Title,
		Body:   payload.Body,
		Bee:    payload.Bee,
		Sector: payload.Sector,
		Intent: payload.Intent,
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
	return task
}

func applyPendingOverlays(task TaskSnapshot, pending PendingReadySnapshot) TaskSnapshot {
	return applyReadyOverlays(task, protocol.TaskReadyPayload{
		Title:  pending.Title,
		Body:   pending.Body,
		Bee:    pending.Bee,
		Sector: pending.Sector,
		Intent: pending.Intent,
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
