package taskledger_test

import (
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

func TestFirstEligiblePlanned(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-c": {TaskID: "task-c", Status: protocol.TaskStatusPlanned},
			"task-a": {TaskID: "task-a", Status: protocol.TaskStatusPlanned},
			"task-b": {TaskID: "task-b", Status: protocol.TaskStatusPlanned},
		},
	}
	first, ok := taskledger.FirstEligiblePlanned(trace)
	if !ok || first.TaskID != "task-a" {
		t.Fatalf("first = %+v, ok = %v", first, ok)
	}
}

func TestEligiblePlannedIsFIFOByCreatedAt(t *testing.T) {
	base := time.Date(2026, 7, 7, 6, 0, 0, 0, time.UTC)
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-a": {TaskID: "task-a", Status: protocol.TaskStatusPlanned, CreatedAt: base.Add(2 * time.Minute)},
			"task-b": {TaskID: "task-b", Status: protocol.TaskStatusPlanned, CreatedAt: base},
			"task-c": {TaskID: "task-c", Status: protocol.TaskStatusPlanned, CreatedAt: base.Add(time.Minute)},
		},
	}
	eligible := taskledger.EligiblePlanned(trace)
	if len(eligible) != 3 {
		t.Fatalf("eligible = %+v", eligible)
	}
	want := []string{"task-b", "task-c", "task-a"}
	for i, id := range want {
		if eligible[i].TaskID != id {
			t.Fatalf("order = %v, want %v", ids(eligible), want)
		}
	}
}

func TestEligiblePlannedPutsUnstampedTasksLast(t *testing.T) {
	base := time.Date(2026, 7, 7, 6, 0, 0, 0, time.UTC)
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			// Unstamped, and lexicographically first: it must still go last.
			"task-a": {TaskID: "task-a", Status: protocol.TaskStatusPlanned},
			"task-b": {TaskID: "task-b", Status: protocol.TaskStatusPlanned, CreatedAt: base.Add(time.Minute)},
			"task-c": {TaskID: "task-c", Status: protocol.TaskStatusPlanned, CreatedAt: base},
		},
	}
	eligible := taskledger.EligiblePlanned(trace)
	if len(eligible) != 3 {
		t.Fatalf("eligible = %+v", eligible)
	}
	want := []string{"task-c", "task-b", "task-a"}
	for i, id := range want {
		if eligible[i].TaskID != id {
			t.Fatalf("order = %v, want %v", ids(eligible), want)
		}
	}
}

func TestEligiblePlannedTiesOnSameCreatedAtFallBackToTaskID(t *testing.T) {
	at := time.Date(2026, 7, 7, 6, 0, 0, 0, time.UTC)
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-b": {TaskID: "task-b", Status: protocol.TaskStatusPlanned, CreatedAt: at},
			"task-a": {TaskID: "task-a", Status: protocol.TaskStatusPlanned, CreatedAt: at},
		},
	}
	first, ok := taskledger.FirstEligiblePlanned(trace)
	if !ok || first.TaskID != "task-a" {
		t.Fatalf("first = %+v, ok = %v", first, ok)
	}
}

func ids(tasks []taskledger.TaskSnapshot) []string {
	out := make([]string, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, task.TaskID)
	}
	return out
}

func TestPromoteFirstEligibleSkipsWhenReadyExists(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-a": {TaskID: "task-a", Status: protocol.TaskStatusReady},
			"task-b": {TaskID: "task-b", Status: protocol.TaskStatusPlanned},
		},
	}
	_, _, ok := taskledger.PromoteFirstEligible(trace, trace.Tasks["task-a"].UpdatedAt)
	if ok {
		t.Fatal("expected no promotion when a task is already ready")
	}
}

func TestEligiblePlanned(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
			"task-2": {TaskID: "task-2", Status: protocol.TaskStatusPlanned, DependsOn: []string{"task-1"}},
			"task-3": {TaskID: "task-3", Status: protocol.TaskStatusPlanned, DependsOn: []string{"task-2"}},
		},
	}
	eligible := taskledger.EligiblePlanned(trace)
	if len(eligible) != 1 || eligible[0].TaskID != "task-2" {
		t.Fatalf("eligible = %+v", eligible)
	}
}

func TestCanStart(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
			"task-2": {TaskID: "task-2", Status: protocol.TaskStatusPlanned, DependsOn: []string{"task-1"}},
			"task-3": {TaskID: "task-3", Status: protocol.TaskStatusPlanned, DependsOn: []string{"task-2"}},
		},
	}
	if _, err := taskledger.CanStart(trace, "task-2"); err != nil {
		t.Fatalf("task-2 should be startable: %v", err)
	}
	if _, err := taskledger.CanStart(trace, "task-3"); err != taskledger.ErrDependenciesIncomplete {
		t.Fatalf("task-3 err = %v", err)
	}
}
