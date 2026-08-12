package taskledger_test

import (
	"testing"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

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
