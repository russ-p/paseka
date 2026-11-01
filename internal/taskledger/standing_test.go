package taskledger_test

import (
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

func TestOpenStandingTickBusyStatuses(t *testing.T) {
	for _, status := range []protocol.TaskStatus{
		protocol.TaskStatusPlanned,
		protocol.TaskStatusReady,
		protocol.TaskStatusRunning,
		protocol.TaskStatusWaitingReview,
	} {
		trace := taskledger.TraceSnapshot{
			TraceID: "trail-1",
			Tasks: map[string]taskledger.TaskSnapshot{
				"task-b": {TaskID: "task-b", Status: protocol.TaskStatusCompleted},
				"task-a": {TaskID: "task-a", Status: status},
			},
		}
		got, ok := taskledger.OpenStandingTick(trace)
		if !ok || got.TaskID != "task-a" || got.Status != status {
			t.Fatalf("status %s: got %+v ok=%v", status, got, ok)
		}
	}
}

func TestOpenStandingTickAllowsTerminalAndBlocked(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trail-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"done":    {TaskID: "done", Status: protocol.TaskStatusCompleted},
			"failed":  {TaskID: "failed", Status: protocol.TaskStatusFailed},
			"cancel":  {TaskID: "cancel", Status: protocol.TaskStatusCancelled},
			"skipped": {TaskID: "skipped", Status: protocol.TaskStatusSkipped, Summary: protocol.SupersededByStipendSummary(4)},
			"blocked": {TaskID: "blocked", Status: protocol.TaskStatusBlocked, Summary: protocol.HoneyReserveExhaustedSummary},
		},
	}
	if _, ok := taskledger.OpenStandingTick(trace); ok {
		t.Fatal("expected no open tick")
	}
}

// A previous tick's ready task that is only waiting for honey does not occupy
// the next tick: the stipend supersedes it instead of being refused.
func TestOpenStandingTickIgnoresEnergyDeferredTask(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trail-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-deferred": {TaskID: "task-deferred", Status: protocol.TaskStatusReady},
		},
		PendingDispatches: []taskledger.PendingDispatch{
			{
				ID:     "task:task-deferred:watch",
				Kind:   taskledger.PendingDispatchTask,
				TaskID: "task-deferred",
				Reason: protocol.HoneyReserveExhaustedSummary,
				Status: taskledger.PendingDispatchPending,
			},
		},
	}
	if got, ok := taskledger.OpenStandingTick(trace); ok {
		t.Fatalf("open tick = %+v, want energy-deferred work to leave the tick free", got)
	}
}

// Configuration-barrier work is not energy waiting, so a held task still
// occupies the tick even when a pending direct entry names it.
func TestOpenStandingTickKeepsConfigurationHeldTaskBusy(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trail-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-held": {TaskID: "task-held", Status: protocol.TaskStatusReady},
		},
		PendingDispatches: []taskledger.PendingDispatch{
			{
				ID:     "direct-held",
				Kind:   taskledger.PendingDispatchDirect,
				TaskID: "task-held",
				Reason: protocol.ConfigBarrierHoldReason,
				Status: taskledger.PendingDispatchPending,
			},
		},
	}
	got, ok := taskledger.OpenStandingTick(trace)
	if !ok || got.TaskID != "task-held" {
		t.Fatalf("open tick = %+v ok=%v, want the configuration-held task", got, ok)
	}
}

// A task-level configuration hold is also not energy deferral.
func TestOpenStandingTickKeepsConfigurationHeldTaskEntryBusy(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trail-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-held": {TaskID: "task-held", Status: protocol.TaskStatusReady},
		},
		PendingDispatches: []taskledger.PendingDispatch{
			{
				ID:     "task:task-held:watch",
				Kind:   taskledger.PendingDispatchTask,
				TaskID: "task-held",
				Reason: protocol.ConfigBarrierHoldReason,
				Status: taskledger.PendingDispatchPending,
			},
		},
	}
	got, ok := taskledger.OpenStandingTick(trace)
	if !ok || got.TaskID != "task-held" {
		t.Fatalf("open tick = %+v ok=%v, want the configuration-held task", got, ok)
	}
}
