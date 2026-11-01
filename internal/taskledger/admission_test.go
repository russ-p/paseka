package taskledger_test

import (
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

// A snapshot written before the pause and pending fields existed must read as
// admitting with an empty queue, not as an error or an unknown state.
func TestAdmissionOfOlderSnapshotIsUnpausedAndEmpty(t *testing.T) {
	got := taskledger.AdmissionOf(taskledger.TraceSnapshot{TraceID: "legacy"})
	if got.Paused || got.PauseReason != "" || got.PendingCount != 0 {
		t.Fatalf("legacy admission = %+v, want unpaused/empty", got)
	}
}

func TestAdmissionOfPausedSnapshotReportsReasonAndPending(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Killed:  false,
		EnergyPause: &taskledger.EnergyPauseSnapshot{
			EpisodeID: "pause-1",
			Reason:    protocol.HoneyReserveExhaustedSummary,
		},
		PendingDispatches: []taskledger.PendingDispatch{
			{ID: "task:task-1:builder", Kind: taskledger.PendingDispatchTask, TaskID: "task-1", Status: taskledger.PendingDispatchPending},
			{ID: "direct-1", Kind: taskledger.PendingDispatchDirect, TaskID: "task-2", Status: taskledger.PendingDispatchPending},
			{ID: "task:done:builder", Kind: taskledger.PendingDispatchTask, TaskID: "done", Status: taskledger.PendingDispatchConsumed},
		},
	}

	got := taskledger.AdmissionOf(trace)
	if !got.Paused {
		t.Fatal("expected the open episode to read as paused")
	}
	if got.PauseReason != protocol.HoneyReserveExhaustedSummary {
		t.Fatalf("reason = %q", got.PauseReason)
	}
	if got.PendingCount != 2 {
		t.Fatalf("pending = %d, want the two active entries", got.PendingCount)
	}
}

func TestPendingTaskIDsMarksOnlyTaskDispatches(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		PendingDispatches: []taskledger.PendingDispatch{
			{ID: "task:task-1:builder", Kind: taskledger.PendingDispatchTask, TaskID: "task-1", Status: taskledger.PendingDispatchPending},
			{ID: "direct-1", Kind: taskledger.PendingDispatchDirect, TaskID: "task-2", Status: taskledger.PendingDispatchPending},
			{ID: "task:closed:builder", Kind: taskledger.PendingDispatchTask, TaskID: "closed", Status: taskledger.PendingDispatchAbandoned},
		},
	}

	got := taskledger.PendingTaskIDs(trace)
	if !got["task-1"] {
		t.Fatal("expected task-1 to be pending")
	}
	if got["task-2"] {
		t.Fatal("a direct dispatch must not mark its task as pending")
	}
	if got["closed"] {
		t.Fatal("an abandoned entry must not mark its task as pending")
	}
}

func TestFormatAdmissionReport(t *testing.T) {
	paused := taskledger.FormatAdmissionReport(taskledger.Admission{
		Paused:       true,
		PauseReason:  protocol.HoneyReserveExhaustedSummary,
		PendingCount: 3,
	})
	joined := strings.Join(paused, "\n")
	for _, want := range []string{"paused:    true", "reason:    Honey reserve exhausted", "pending:   3"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("report = %q, want %q", joined, want)
		}
	}

	admitting := taskledger.FormatAdmissionReport(taskledger.Admission{})
	joined = strings.Join(admitting, "\n")
	if !strings.Contains(joined, "paused:    false") || !strings.Contains(joined, "pending:   0") {
		t.Fatalf("admitting report = %q", joined)
	}
	if strings.Contains(joined, "reason:") {
		t.Fatalf("admitting report must not invent a reason: %q", joined)
	}
}
