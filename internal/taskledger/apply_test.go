package taskledger_test

import (
	"testing"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

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
