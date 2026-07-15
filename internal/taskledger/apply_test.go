package taskledger_test

import (
	"testing"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestApplyEventTaskPlanPreservesSector(t *testing.T) {
	trace := taskledger.TraceSnapshot{TraceID: "trace-1"}

	ev, err := protocol.NewEvent("trace-1", "scout", 1, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{
			{TaskID: "task-1", Title: "Frontend", Bee: "builder", Sector: "frontend"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.Tasks["task-1"].Sector != "frontend" {
		t.Fatalf("sector = %q", res.Trace.Tasks["task-1"].Sector)
	}
}

func TestApplyEventTaskReadyUpdatesSector(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Title: "Backend", Status: protocol.TaskStatusPlanned},
		},
	}

	ev, err := protocol.NewEvent("trace-1", "reactor", 1, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: "task-1",
		Bee:    "builder",
		Sector: "backend-users",
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.Tasks["task-1"].Sector != "backend-users" {
		t.Fatalf("sector = %q", res.Trace.Tasks["task-1"].Sector)
	}
}

func TestApplyEventTaskPlanPreservesIntent(t *testing.T) {
	trace := taskledger.TraceSnapshot{TraceID: "trace-1"}

	ev, err := protocol.NewEvent("trace-1", "scout", 1, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{
			{TaskID: "task-1", Title: "Fix login", Bee: "builder", Intent: "bugfix"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.Tasks["task-1"].Intent != "bugfix" {
		t.Fatalf("intent = %q", res.Trace.Tasks["task-1"].Intent)
	}
}

func TestApplyEventTaskReadyUpdatesIntent(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Title: "Backend", Status: protocol.TaskStatusPlanned},
		},
	}

	ev, err := protocol.NewEvent("trace-1", "reactor", 1, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: "task-1",
		Bee:    "builder",
		Intent: "feature",
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.Tasks["task-1"].Intent != "feature" {
		t.Fatalf("intent = %q", res.Trace.Tasks["task-1"].Intent)
	}
}

func TestApplyEventTaskPlan(t *testing.T) {
	trace := taskledger.TraceSnapshot{TraceID: "trace-1"}

	ev, err := protocol.NewEvent("trace-1", "scout", 1, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{
			{TaskID: "task-1", Title: "Backend", Bee: "builder"},
			{TaskID: "task-2", Title: "Frontend", Bee: "builder", DependsOn: []string{"task-1"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed {
		t.Fatal("expected changed")
	}
	if len(res.Trace.Tasks) != 2 {
		t.Fatalf("tasks = %d", len(res.Trace.Tasks))
	}
	if res.Trace.Tasks["task-1"].Status != protocol.TaskStatusPlanned {
		t.Fatalf("task-1 status = %q", res.Trace.Tasks["task-1"].Status)
	}
}

func TestApplyEventTaskReady(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Title: "Backend", Status: protocol.TaskStatusPlanned},
		},
	}

	ev, err := protocol.NewEvent("trace-1", "reactor", 1, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: "task-1",
		Bee:    "builder",
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Ready) != 1 {
		t.Fatalf("ready = %+v", res.Ready)
	}
	if res.Trace.Tasks["task-1"].Status != protocol.TaskStatusReady {
		t.Fatalf("status = %q", res.Trace.Tasks["task-1"].Status)
	}
}

func TestApplyEventTaskReadyFromFailed(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {
				TaskID:  "task-1",
				Title:   "Backend",
				Status:  protocol.TaskStatusFailed,
				Summary: "adapter failed: tests failed",
				Bee:     "builder",
				Intent:  "feature",
			},
		},
	}

	ev, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: "task-1",
		Bee:    "builder",
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Ready) != 1 {
		t.Fatalf("ready = %+v", res.Ready)
	}
	task := res.Trace.Tasks["task-1"]
	if task.Status != protocol.TaskStatusReady {
		t.Fatalf("status = %q, want ready", task.Status)
	}
	if task.Summary != "" {
		t.Fatalf("summary = %q, want cleared", task.Summary)
	}
	if task.Intent != "feature" {
		t.Fatalf("intent = %q", task.Intent)
	}
}

func TestApplyEventTaskReadyFromRunning(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Title: "Backend", Status: protocol.TaskStatusRunning, Bee: "builder"},
		},
	}

	ev, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: "task-1",
		Bee:    "builder",
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Ready) != 1 {
		t.Fatalf("ready = %+v", res.Ready)
	}
	if res.Trace.Tasks["task-1"].Status != protocol.TaskStatusReady {
		t.Fatalf("status = %q, want ready", res.Trace.Tasks["task-1"].Status)
	}
}

func TestApplyEventTaskCompletedUnlocksDependent(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusReady},
			"task-2": {TaskID: "task-2", Status: protocol.TaskStatusPlanned, DependsOn: []string{"task-1"}},
		},
	}

	ev, err := protocol.NewEvent("trace-1", "guard", 1, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind:    protocol.TaskEventCompleted,
		TaskID:  "task-1",
		Status:  protocol.TaskStatusCompleted,
		Summary: "committed",
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.Tasks["task-1"].Status != protocol.TaskStatusCompleted {
		t.Fatalf("task-1 status = %q", res.Trace.Tasks["task-1"].Status)
	}
	if res.Trace.Tasks["task-2"].Status != protocol.TaskStatusReady {
		t.Fatalf("task-2 status = %q", res.Trace.Tasks["task-2"].Status)
	}
	if len(res.Ready) != 1 || res.Ready[0].TaskID != "task-2" {
		t.Fatalf("ready = %+v", res.Ready)
	}
}

func TestApplyEventTaskCompletedPromotesOnlyFirstEligible(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-a": {TaskID: "task-a", Status: protocol.TaskStatusReady},
			"task-b": {TaskID: "task-b", Status: protocol.TaskStatusPlanned},
			"task-c": {TaskID: "task-c", Status: protocol.TaskStatusPlanned},
		},
	}

	ev, err := protocol.NewEvent("trace-1", "guard", 1, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind:    protocol.TaskEventCompleted,
		TaskID:  "task-a",
		Status:  protocol.TaskStatusCompleted,
		Summary: "done",
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.Tasks["task-b"].Status != protocol.TaskStatusReady {
		t.Fatalf("task-b status = %q, want ready", res.Trace.Tasks["task-b"].Status)
	}
	if res.Trace.Tasks["task-c"].Status != protocol.TaskStatusPlanned {
		t.Fatalf("task-c status = %q, want planned", res.Trace.Tasks["task-c"].Status)
	}
	if len(res.Ready) != 1 || res.Ready[0].TaskID != "task-b" {
		t.Fatalf("ready = %+v", res.Ready)
	}
}

func TestApplyEventTaskCompletedPromotesNextAfterFirstCompletes(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-b": {TaskID: "task-b", Status: protocol.TaskStatusReady},
			"task-c": {TaskID: "task-c", Status: protocol.TaskStatusPlanned},
		},
	}

	ev, err := protocol.NewEvent("trace-1", "guard", 1, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind:   protocol.TaskEventCompleted,
		TaskID: "task-b",
		Status: protocol.TaskStatusCompleted,
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.Tasks["task-c"].Status != protocol.TaskStatusReady {
		t.Fatalf("task-c status = %q, want ready", res.Trace.Tasks["task-c"].Status)
	}
	if len(res.Ready) != 1 || res.Ready[0].TaskID != "task-c" {
		t.Fatalf("ready = %+v", res.Ready)
	}
}

func TestApplyEventTaskReadyRejectsNonFirstEligible(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-a": {TaskID: "task-a", Status: protocol.TaskStatusPlanned},
			"task-b": {TaskID: "task-b", Status: protocol.TaskStatusPlanned},
		},
	}

	ev, err := protocol.NewEvent("trace-1", "reactor", 1, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: "task-b",
		Bee:    "builder",
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.Tasks["task-b"].Status != protocol.TaskStatusPlanned {
		t.Fatalf("task-b status = %q, want planned", res.Trace.Tasks["task-b"].Status)
	}
	if len(res.Ready) != 0 {
		t.Fatalf("ready = %+v, want none", res.Ready)
	}
}

func TestApplyEventTaskReadyRejectsWhenAnotherReady(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-a": {TaskID: "task-a", Status: protocol.TaskStatusReady},
			"task-b": {TaskID: "task-b", Status: protocol.TaskStatusPlanned},
		},
	}

	ev, err := protocol.NewEvent("trace-1", "reactor", 1, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: "task-b",
		Bee:    "builder",
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.Tasks["task-b"].Status != protocol.TaskStatusPlanned {
		t.Fatalf("task-b status = %q, want planned", res.Trace.Tasks["task-b"].Status)
	}
	if len(res.Ready) != 0 {
		t.Fatalf("ready = %+v, want none", res.Ready)
	}
}

func TestApplyEventsSequence(t *testing.T) {
	events := []protocol.Event{}
	plan, _ := protocol.NewEvent("trace-1", "scout", 1, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{
			{TaskID: "task-1", Title: "A"},
			{TaskID: "task-2", Title: "B", DependsOn: []string{"task-1"}},
		},
	})
	ready, _ := protocol.NewEvent("trace-1", "reactor", 2, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind: protocol.TaskEventReady, TaskID: "task-1",
	})
	done, _ := protocol.NewEvent("trace-1", "guard", 3, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind: protocol.TaskEventCompleted, TaskID: "task-1", Status: protocol.TaskStatusCompleted,
	})
	events = append(events, plan, ready, done)

	trace, err := taskledger.ApplyEvents(taskledger.TraceSnapshot{TraceID: "trace-1"}, events)
	if err != nil {
		t.Fatal(err)
	}
	if trace.Tasks["task-1"].Status != protocol.TaskStatusCompleted {
		t.Fatalf("task-1 = %+v", trace.Tasks["task-1"])
	}
	if trace.Tasks["task-2"].Status != protocol.TaskStatusReady {
		t.Fatalf("task-2 = %+v", trace.Tasks["task-2"])
	}
}
