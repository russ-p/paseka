package tasks_test

import (
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/tasks"
)

func TestDiagnoseTraceDependenciesReportsATypo(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
			"task-2": {TaskID: "task-2", Status: protocol.TaskStatusPlanned, DependsOn: []string{"taks-1"}},
		},
	}
	warnings := tasks.DiagnoseTraceDependencies(trace)
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v", warnings)
	}
	for _, want := range []string{"trace-1", `"task-2"`, `"taks-1"`, "final gate stays closed"} {
		if !strings.Contains(warnings[0], want) {
			t.Fatalf("warning %q missing %q", warnings[0], want)
		}
	}
}

func TestDiagnoseTraceDependenciesReportsATerminalDependency(t *testing.T) {
	for _, status := range []protocol.TaskStatus{protocol.TaskStatusCancelled, protocol.TaskStatusBlocked} {
		trace := taskledger.TraceSnapshot{
			TraceID: "trace-1",
			Tasks: map[string]taskledger.TaskSnapshot{
				"task-1": {TaskID: "task-1", Status: status},
				"task-2": {TaskID: "task-2", Status: protocol.TaskStatusPlanned, DependsOn: []string{"task-1"}},
			},
		}
		warnings := tasks.DiagnoseTraceDependencies(trace)
		if len(warnings) != 1 || !strings.Contains(warnings[0], string(status)) {
			t.Fatalf("%s dependency warnings = %v", status, warnings)
		}
	}
}

func TestDiagnoseTraceDependenciesReportsACycle(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusPlanned, DependsOn: []string{"task-2"}},
			"task-2": {TaskID: "task-2", Status: protocol.TaskStatusPlanned, DependsOn: []string{"task-1"}},
		},
	}
	warnings := tasks.DiagnoseTraceDependencies(trace)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "dependency cycle") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestDiagnoseTraceDependenciesIgnoresFinishedEdges(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			// A finished task with a stale typo must not be reported: it is not
			// holding anything shut.
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCompleted, DependsOn: []string{"taks-0"}},
			"task-2": {TaskID: "task-2", Status: protocol.TaskStatusPlanned, DependsOn: []string{"task-1"}},
		},
	}
	if warnings := tasks.DiagnoseTraceDependencies(trace); len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestDiagnoseTraceDependenciesIgnoresKilledAndSettledTrails(t *testing.T) {
	typo := func() map[string]taskledger.TaskSnapshot {
		return map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusPlanned, DependsOn: []string{"taks-1"}},
		}
	}
	killed := taskledger.TraceSnapshot{TraceID: "trace-1", Killed: true, Tasks: typo()}
	if warnings := tasks.DiagnoseTraceDependencies(killed); len(warnings) != 0 {
		t.Fatalf("killed warnings = %v", warnings)
	}

	completed := taskledger.TraceSnapshot{
		TraceID: "trace-2",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
			"review": {TaskID: "review", Status: protocol.TaskStatusPlanned, Review: protocol.TaskReviewFinal},
		},
	}
	if warnings := tasks.DiagnoseTraceDependencies(completed); len(warnings) != 0 {
		t.Fatalf("settled warnings = %v", warnings)
	}
}
