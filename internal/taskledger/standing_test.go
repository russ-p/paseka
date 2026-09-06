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
			"blocked": {TaskID: "blocked", Status: protocol.TaskStatusBlocked, Summary: protocol.HoneyReserveExhaustedSummary},
		},
	}
	if _, ok := taskledger.OpenStandingTick(trace); ok {
		t.Fatal("expected no open tick")
	}
}
