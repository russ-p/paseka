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
}
