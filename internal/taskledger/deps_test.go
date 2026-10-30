package taskledger_test

import (
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

func plannedTask(id string, deps ...string) taskledger.TaskSnapshot {
	return taskledger.TaskSnapshot{
		TaskID:    id,
		Status:    protocol.TaskStatusPlanned,
		DependsOn: deps,
	}
}

func TestValidateDependenciesAcceptsAChain(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1":         plannedTask("task-1"),
			"task-2":         plannedTask("task-2", "task-1"),
			"task-3":         plannedTask("task-3", "task-2", "task-3-sibling"),
			"task-3-sibling": plannedTask("task-3-sibling", "task-1"),
		},
	}
	if err := taskledger.ValidateDependencies(trace); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDependenciesReportsATypo(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": plannedTask("task-1"),
			"task-2": plannedTask("task-2", "taks-1"),
		},
	}
	err := taskledger.ValidateDependencies(trace)
	if err == nil {
		t.Fatal("expected an unknown dependency error")
	}
	if !strings.Contains(err.Error(), `task "task-2" depends on unknown task "taks-1"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateDependenciesRejectsSelfDependency(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": plannedTask("task-1", "task-1"),
		},
	}
	err := taskledger.ValidateDependencies(trace)
	if err == nil || !strings.Contains(err.Error(), `task "task-1" depends on itself`) {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateDependenciesReportsACycle(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": plannedTask("task-1", "task-3"),
			"task-2": plannedTask("task-2", "task-1"),
			"task-3": plannedTask("task-3", "task-2"),
		},
	}
	err := taskledger.ValidateDependencies(trace)
	if err == nil || !strings.Contains(err.Error(), "form a dependency cycle") {
		t.Fatalf("error = %v", err)
	}
	for _, id := range []string{`"task-1"`, `"task-2"`, `"task-3"`} {
		if !strings.Contains(err.Error(), id) {
			t.Fatalf("cycle does not name %s: %v", id, err)
		}
	}
}

func TestValidateDependenciesRejectsEmptyEntry(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": plannedTask("task-1", ""),
		},
	}
	err := taskledger.ValidateDependencies(trace)
	if err == nil || !strings.Contains(err.Error(), `task "task-1" has an empty dependency`) {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateDependenciesSeesTheWholePlan(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks:   map[string]taskledger.TaskSnapshot{},
	}
	specs := []protocol.TaskSpec{
		{TaskID: "task-1", Title: "first"},
		{TaskID: "task-2", Title: "second", DependsOn: []string{"task-1"}},
	}
	if err := taskledger.ValidateDependencies(trace, specs...); err != nil {
		t.Fatalf("a sibling in the same plan is a known id: %v", err)
	}
}

func TestValidateDependenciesUsesTheStoredEdgesOfAnExistingTask(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": plannedTask("task-1"),
		},
	}
	// The reducer ignores a re-plan for an id that already has a status, so the
	// spec's dependency must not be validated as if it would be applied.
	specs := []protocol.TaskSpec{{TaskID: "task-1", DependsOn: []string{"missing"}}}
	if err := taskledger.ValidateDependencies(trace, specs...); err != nil {
		t.Fatalf("stored edges should win: %v", err)
	}
}

func TestValidateDependenciesDuplicateEdgesAreNotACycle(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": plannedTask("task-1"),
			"task-2": plannedTask("task-2", "task-1", "task-1"),
		},
	}
	if err := taskledger.ValidateDependencies(trace); err != nil {
		t.Fatalf("a repeated dependency is not a cycle: %v", err)
	}
}
