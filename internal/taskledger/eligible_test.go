package taskledger_test

import (
	"testing"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

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
