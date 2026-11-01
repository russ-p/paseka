package main

import (
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/taskledger"
)

// `paseka energy show` must make a deferral visible: the reserve alone cannot
// explain why a ready task has not started.
func TestPrintHoneyReserveIncludesPendingCount(t *testing.T) {
	restoreOut, capture := captureStdout(t)
	defer restoreOut()

	printHoneyReserve(taskledger.TraceSnapshot{
		EnergyBudget:    4,
		EnergyRemaining: 0,
		PendingDispatches: []taskledger.PendingDispatch{
			{ID: "task:task-1:builder", Kind: taskledger.PendingDispatchTask, Status: taskledger.PendingDispatchPending},
			{ID: "direct-key", Kind: taskledger.PendingDispatchDirect, Status: taskledger.PendingDispatchConsumed},
		},
	})

	out := capture()
	if !strings.Contains(out, "pending:   1") {
		t.Fatalf("output = %q, want the active pending count", out)
	}
	if !strings.Contains(out, "paused:    false") {
		t.Fatalf("output = %q, want the admission state even when admitting", out)
	}
}

// The reserve alone cannot say whether dispatch stopped; `energy show` must
// print the pause, its reason, and the work waiting behind it.
func TestPrintHoneyReserveShowsPauseReasonAndPending(t *testing.T) {
	restoreOut, capture := captureStdout(t)
	defer restoreOut()

	printHoneyReserve(taskledger.TraceSnapshot{
		EnergyBudget:    4,
		EnergyRemaining: 0,
		EnergyPause: &taskledger.EnergyPauseSnapshot{
			EpisodeID: "pause-1",
			Reason:    "Honey reserve exhausted",
		},
		PendingDispatches: []taskledger.PendingDispatch{
			{ID: "task:task-1:builder", Kind: taskledger.PendingDispatchTask, TaskID: "task-1", Status: taskledger.PendingDispatchPending},
			{ID: "direct-key", Kind: taskledger.PendingDispatchDirect, Status: taskledger.PendingDispatchPending},
		},
	})

	out := capture()
	for _, want := range []string{"paused:    true", "reason:    Honey reserve exhausted", "pending:   2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output = %q, want %q", out, want)
		}
	}
}
