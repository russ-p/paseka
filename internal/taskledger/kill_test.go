package taskledger_test

import (
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

// IsKillableTaskStatus is the guard that keeps superseded work from being
// rewritten as a cancellation on kill. The table lists every current status
// explicitly: Go has no exhaustive enum switch, so a new status has to show up
// here in review rather than silently inheriting the default.
func TestIsKillableTaskStatus(t *testing.T) {
	cases := []struct {
		status protocol.TaskStatus
		want   bool
	}{
		{protocol.TaskStatusPlanned, true},
		{protocol.TaskStatusReady, true},
		{protocol.TaskStatusRunning, true},
		{protocol.TaskStatusWaitingReview, true},
		{protocol.TaskStatusBlocked, true},
		{protocol.TaskStatusCompleted, false},
		{protocol.TaskStatusFailed, false},
		{protocol.TaskStatusCancelled, false},
		{protocol.TaskStatusSkipped, false},
	}
	for _, tc := range cases {
		if got := taskledger.IsKillableTaskStatus(tc.status); got != tc.want {
			t.Errorf("IsKillableTaskStatus(%q) = %t, want %t", tc.status, got, tc.want)
		}
	}
}

// A task superseded before the kill was never work in flight, so kill must leave
// it skipped with its supersession summary while a non-terminal task is the one
// that becomes cancelled.
func TestApplyEventSystemKillKeepsSkippedTask(t *testing.T) {
	superseded := protocol.SupersededByStipendSummary(4)
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"skipped": {TaskID: "skipped", Status: protocol.TaskStatusSkipped, Summary: superseded},
			"running": {TaskID: "running", Status: protocol.TaskStatusRunning},
		},
	}
	ev, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.SystemKillPayload{
		Kind:   protocol.SignalSystemKill,
		Reason: "stop",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Trace.Tasks["skipped"].Status; got != protocol.TaskStatusSkipped {
		t.Fatalf("skipped task status = %q, want skipped", got)
	}
	if got := res.Trace.Tasks["skipped"].Summary; got != superseded {
		t.Fatalf("skipped task summary = %q, want the supersession summary kept", got)
	}
	if got := res.Trace.Tasks["running"].Status; got != protocol.TaskStatusCancelled {
		t.Fatalf("running task status = %q, want cancelled", got)
	}
	if got := res.Trace.Tasks["running"].Summary; got != "stop" {
		t.Fatalf("running task summary = %q, want the kill reason", got)
	}
}

func TestApplyEventSystemKill(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusRunning},
			"task-2": {TaskID: "task-2", Status: protocol.TaskStatusCompleted},
			"task-3": {TaskID: "task-3", Status: protocol.TaskStatusPlanned},
		},
	}
	ev, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.SystemKillPayload{
		Kind:   protocol.SignalSystemKill,
		Reason: "avalanche",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Trace.Killed {
		t.Fatal("expected trace killed")
	}
	if res.Trace.Tasks["task-1"].Status != protocol.TaskStatusCancelled {
		t.Fatalf("task-1 status = %q", res.Trace.Tasks["task-1"].Status)
	}
	if res.Trace.Tasks["task-1"].Summary != "avalanche" {
		t.Fatalf("task-1 summary = %q", res.Trace.Tasks["task-1"].Summary)
	}
	if res.Trace.Tasks["task-2"].Status != protocol.TaskStatusCompleted {
		t.Fatalf("task-2 status = %q", res.Trace.Tasks["task-2"].Status)
	}
	if res.Trace.Tasks["task-3"].Status != protocol.TaskStatusCancelled {
		t.Fatalf("task-3 status = %q", res.Trace.Tasks["task-3"].Status)
	}
	if len(res.Ready) != 0 {
		t.Fatalf("ready = %+v, want none", res.Ready)
	}
}

func TestApplyEventSystemKillIdempotent(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Killed:  true,
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCancelled, Summary: "first"},
		},
	}
	ev, err := protocol.NewEvent("trace-1", "cli", 2, protocol.EventSignal, protocol.SystemKillPayload{
		Kind:   protocol.SignalSystemKill,
		Reason: "again",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed {
		t.Fatal("expected no change on repeat kill")
	}
	if res.Trace.Tasks["task-1"].Summary != "first" {
		t.Fatalf("summary overwritten = %q", res.Trace.Tasks["task-1"].Summary)
	}
}

func TestApplyEventSystemKillAbandonsActivePendingDispatches(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		PendingDispatches: []taskledger.PendingDispatch{
			{ID: "active", Kind: taskledger.PendingDispatchDirect, Status: taskledger.PendingDispatchPending},
			{ID: "done", Kind: taskledger.PendingDispatchDirect, Status: taskledger.PendingDispatchConsumed},
			{ID: "gone", Kind: taskledger.PendingDispatchDirect, Status: taskledger.PendingDispatchUnresolved, Diagnostic: "source event missing"},
		},
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusWaitingReview},
		},
	}
	ev, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.SystemKillPayload{
		Kind:   protocol.SignalSystemKill,
		Reason: "stop",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed {
		t.Fatal("kill must report a change so the ledger persists the abandonment")
	}
	if !res.Trace.Killed {
		t.Fatal("expected trace killed")
	}
	if got := res.Trace.Tasks["task-1"].Status; got != protocol.TaskStatusCancelled {
		t.Fatalf("task status = %q, want cancelled", got)
	}
	statuses := map[string]taskledger.PendingDispatchStatus{}
	for _, item := range res.Trace.PendingDispatches {
		statuses[item.ID] = item.Status
	}
	if got := statuses["active"]; got != taskledger.PendingDispatchAbandoned {
		t.Fatalf("active entry status = %q, want abandoned", got)
	}
	if got := statuses["done"]; got != taskledger.PendingDispatchConsumed {
		t.Fatalf("consumed entry status = %q, want consumed", got)
	}
	if got := statuses["gone"]; got != taskledger.PendingDispatchUnresolved {
		t.Fatalf("unresolved entry status = %q, want unresolved", got)
	}
	if active := taskledger.ActivePendingDispatches(res.Trace); len(active) != 0 {
		t.Fatalf("active pending = %+v, want none after kill", active)
	}
	if len(res.Trace.PendingDispatches) != 3 {
		t.Fatalf("pending entries = %d, want 3 kept as evidence", len(res.Trace.PendingDispatches))
	}
}

func TestApplyEventEnergyAddDoesNotReviveKillAbandonedWork(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		PendingDispatches: []taskledger.PendingDispatch{
			{ID: "active", Kind: taskledger.PendingDispatchDirect, Status: taskledger.PendingDispatchPending},
		},
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusWaitingReview},
		},
	}
	kill, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.SystemKillPayload{
		Kind: protocol.SignalSystemKill,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, kill)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Trace.PendingDispatches[0].Status; got != taskledger.PendingDispatchAbandoned {
		t.Fatalf("status after kill = %q, want abandoned", got)
	}

	add, err := protocol.NewEvent("trace-1", "cli", 2, protocol.EventSignal, protocol.EnergyAddPayload{
		Kind:   protocol.SignalEnergyAdd,
		Amount: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err = taskledger.ApplyEvent(res.Trace, add)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.EnergyRemaining != 5 {
		t.Fatalf("remaining = %d, want 5", res.Trace.EnergyRemaining)
	}
	if got := res.Trace.PendingDispatches[0].Status; got != taskledger.PendingDispatchAbandoned {
		t.Fatalf("status after energy.add = %q, want abandoned", got)
	}
	if active := taskledger.ActivePendingDispatches(res.Trace); len(active) != 0 {
		t.Fatalf("active pending = %+v, want abandoned work to stay unrunnable", active)
	}
}

func TestDeferPendingDispatchRefusesKilledTrace(t *testing.T) {
	trace := taskledger.TraceSnapshot{TraceID: "trace-1", Killed: true}
	updated, added := taskledger.DeferPendingDispatch(trace, taskledger.PendingDispatch{
		ID:     "late",
		Kind:   taskledger.PendingDispatchDirect,
		Status: taskledger.PendingDispatchPending,
	})
	if added {
		t.Fatal("a killed trace must not record new pending work")
	}
	if len(updated.PendingDispatches) != 0 {
		t.Fatalf("pending entries = %d, want none", len(updated.PendingDispatches))
	}
}

func TestKVLedgerKillAbandonsPendingDispatchInSameMutation(t *testing.T) {
	kv := newFakeKV()
	ledger := taskledger.NewKVLedger(kv)
	if _, err := ledger.DeferPendingDispatch("trace-1", taskledger.PendingDispatch{
		ID:   "direct-key",
		Kind: taskledger.PendingDispatchDirect,
	}); err != nil {
		t.Fatal(err)
	}

	kill, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.SystemKillPayload{
		Kind:   protocol.SignalSystemKill,
		Reason: "stop",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(kill); err != nil {
		t.Fatal(err)
	}

	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Killed {
		t.Fatal("expected killed snapshot")
	}
	if len(snap.PendingDispatches) != 1 {
		t.Fatalf("pending entries = %d, want 1", len(snap.PendingDispatches))
	}
	if got := snap.PendingDispatches[0].Status; got != taskledger.PendingDispatchAbandoned {
		t.Fatalf("status = %q, want abandoned persisted by the kill mutation", got)
	}

	add, err := protocol.NewEvent("trace-1", "cli", 2, protocol.EventSignal, protocol.EnergyAddPayload{
		Kind:   protocol.SignalEnergyAdd,
		Amount: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(add); err != nil {
		t.Fatal(err)
	}
	snap, err = ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if got := snap.PendingDispatches[0].Status; got != taskledger.PendingDispatchAbandoned {
		t.Fatalf("status after top-up = %q, want abandoned", got)
	}
}

func TestApplyEventTaskReadyIgnoredWhenKilled(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Killed:  true,
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusPlanned},
		},
	}
	ev, err := protocol.NewEvent("trace-1", "reactor", 1, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: "task-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.Tasks["task-1"].Status != protocol.TaskStatusPlanned {
		t.Fatalf("status = %q, want planned", res.Trace.Tasks["task-1"].Status)
	}
}

func TestApplyEventEnergyAddDoesNotUnkill(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		Killed:          true,
		EnergyBudget:    12,
		EnergyRemaining: 0,
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCancelled},
		},
	}
	ev, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.EnergyAddPayload{
		Kind:   protocol.SignalEnergyAdd,
		Amount: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.EnergyRemaining != 5 {
		t.Fatalf("remaining = %d", res.Trace.EnergyRemaining)
	}
	if !res.Trace.Killed {
		t.Fatal("kill flag cleared")
	}
}

func TestApplyEventEnergyStipendDoesNotUnkill(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		Killed:          true,
		EnergyBudget:    4,
		EnergyRemaining: 0,
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCancelled},
		},
	}
	ev, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.EnergyStipendPayload{
		Kind:   protocol.SignalEnergyStipend,
		Amount: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.EnergyRemaining != 4 {
		t.Fatalf("remaining = %d", res.Trace.EnergyRemaining)
	}
	if !res.Trace.Killed {
		t.Fatal("kill flag cleared")
	}
	if res.Trace.Tasks["task-1"].Status != protocol.TaskStatusCancelled {
		t.Fatalf("status = %q, want cancelled", res.Trace.Tasks["task-1"].Status)
	}
}

func TestApplyEventTaskStatusIgnoredWhenKilled(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Killed:  true,
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCancelled, Summary: "eval hard stop"},
		},
	}
	ev, err := protocol.NewEvent("trace-1", "runtime", 1, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind:    protocol.TaskEventStatus,
		TaskID:  "task-1",
		Status:  protocol.TaskStatusWaitingReview,
		Summary: "late finalize",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed {
		t.Fatal("expected no change for late task.status after kill")
	}
	task := res.Trace.Tasks["task-1"]
	if task.Status != protocol.TaskStatusCancelled {
		t.Fatalf("status = %q, want cancelled", task.Status)
	}
	if task.Summary != "eval hard stop" {
		t.Fatalf("summary = %q, want kill reason", task.Summary)
	}
}

func TestApplyEventTaskCompletedIgnoredWhenKilled(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Killed:  true,
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCancelled, Summary: "eval hard stop"},
		},
	}
	ev, err := protocol.NewEvent("trace-1", "runtime", 1, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind:    protocol.TaskEventCompleted,
		TaskID:  "task-1",
		Status:  protocol.TaskStatusCompleted,
		Summary: "late complete",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed {
		t.Fatal("expected no change for late task.completed after kill")
	}
	task := res.Trace.Tasks["task-1"]
	if task.Status != protocol.TaskStatusCancelled {
		t.Fatalf("status = %q, want cancelled", task.Status)
	}
}

func TestApplyEventTaskCompletedIgnoredForUnknownTaskWhenKilled(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Killed:  true,
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCancelled, Summary: "eval hard stop"},
		},
	}
	ev, err := protocol.NewEvent("trace-1", "runtime", 1, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind:   protocol.TaskEventCompleted,
		TaskID: "task-ghost",
		Status: protocol.TaskStatusCompleted,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed {
		t.Fatal("expected no change for late task.completed on unknown task after kill")
	}
	if _, ok := res.Trace.Tasks["task-ghost"]; ok {
		t.Fatal("late completed must not insert a task on a killed trace")
	}
}
