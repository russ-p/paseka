package tasks_test

import (
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
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

func timeAt(minute int) time.Time {
	return time.Date(2026, 1, 1, 12, minute, 0, 0, time.UTC)
}

func completedWorkTask(id string, at time.Time) taskledger.TaskSnapshot {
	return taskledger.TaskSnapshot{TaskID: id, Status: protocol.TaskStatusCompleted, CreatedAt: at, UpdatedAt: at}
}

func closingTask(id string, status protocol.TaskStatus, at time.Time) taskledger.TaskSnapshot {
	return taskledger.TaskSnapshot{TaskID: id, Phase: protocol.TaskPhaseClosing, Status: status, CreatedAt: at, UpdatedAt: at}
}

func mergeTask(id string, status protocol.TaskStatus, at time.Time) taskledger.TaskSnapshot {
	return taskledger.TaskSnapshot{TaskID: id, Review: protocol.TaskReviewFinal, Status: status, CreatedAt: at, UpdatedAt: at}
}

func closingPolicy(enabled bool, bee string, maxRounds int) colony.ClosingPolicy {
	policy := colony.ClosingPolicy{Enabled: enabled}
	policy.Task.Bee = bee
	if maxRounds > 0 {
		policy.MaxRounds = &maxRounds
	}
	return policy
}

func closingBees() map[string]colony.Bee {
	return map[string]colony.Bee{"builder": {Role: "builder"}}
}

func TestDiagnoseTraceClosingDefaultDisabledIsSilent(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": completedWorkTask("task-1", timeAt(0)),
		},
	}
	if warnings := tasks.DiagnoseTraceClosing(tasks.ClosingDiagnosisInput{Trace: trace}); len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestDiagnoseTraceClosingReportsMoreThanOneMergeTask(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1":   completedWorkTask("task-1", timeAt(0)),
			"review-a": mergeTask("review-a", protocol.TaskStatusWaitingReview, timeAt(1)),
			"review-b": mergeTask("review-b", protocol.TaskStatusWaitingReview, timeAt(2)),
		},
	}
	warnings := tasks.DiagnoseTraceClosing(tasks.ClosingDiagnosisInput{Trace: trace})
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v", warnings)
	}
	for _, want := range []string{"2 merge-phase tasks", "review-a", "review-b", "exactly one merge gate"} {
		if !strings.Contains(warnings[0], want) {
			t.Fatalf("warning %q missing %q", warnings[0], want)
		}
	}
}

func TestDiagnoseTraceClosingReportsMissingSeed(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": completedWorkTask("task-1", timeAt(0)),
		},
	}
	warnings := tasks.DiagnoseTraceClosing(tasks.ClosingDiagnosisInput{
		Trace:  trace,
		Policy: closingPolicy(true, "builder", 0),
		Bees:   closingBees(),
	})
	if len(warnings) != 1 || !strings.Contains(warnings[0], "closing seed has not been built") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestDiagnoseTraceClosingReportsUnbuildableSeed(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": completedWorkTask("task-1", timeAt(0)),
		},
	}
	warnings := tasks.DiagnoseTraceClosing(tasks.ClosingDiagnosisInput{
		Trace:  trace,
		Policy: closingPolicy(true, "ghost", 0),
		Bees:   closingBees(),
	})
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v", warnings)
	}
	for _, want := range []string{"cannot be built", "ghost", "closing.task.bee"} {
		if !strings.Contains(warnings[0], want) {
			t.Fatalf("warning %q missing %q", warnings[0], want)
		}
	}
}

func TestDiagnoseTraceClosingReportsCapWithFollowUpWork(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1":    completedWorkTask("task-1", timeAt(0)),
			"closing-1": closingTask("closing-1", protocol.TaskStatusCompleted, timeAt(1)),
			"task-2":    completedWorkTask("task-2", timeAt(2)),
		},
	}
	warnings := tasks.DiagnoseTraceClosing(tasks.ClosingDiagnosisInput{
		Trace:  trace,
		Policy: closingPolicy(true, "builder", 1),
		Bees:   closingBees(),
	})
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v", warnings)
	}
	for _, want := range []string{"cap of 1 round", "closing-1", "follow-up work"} {
		if !strings.Contains(warnings[0], want) {
			t.Fatalf("warning %q missing %q", warnings[0], want)
		}
	}
}

func TestDiagnoseTraceClosingSilentForDeliveredTrail(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1":    completedWorkTask("task-1", timeAt(0)),
			"closing-1": closingTask("closing-1", protocol.TaskStatusCompleted, timeAt(1)),
			"review":    mergeTask("review", protocol.TaskStatusCompleted, timeAt(2)),
		},
	}
	if warnings := tasks.DiagnoseTraceClosing(tasks.ClosingDiagnosisInput{
		Trace:  trace,
		Policy: closingPolicy(true, "builder", 1),
		Bees:   closingBees(),
	}); len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestDiagnoseTraceClosingSilentWhileClosingIsInFlight(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1":    completedWorkTask("task-1", timeAt(0)),
			"closing-1": closingTask("closing-1", protocol.TaskStatusPlanned, timeAt(1)),
		},
	}
	if warnings := tasks.DiagnoseTraceClosing(tasks.ClosingDiagnosisInput{
		Trace:  trace,
		Policy: closingPolicy(true, "builder", 0),
		Bees:   closingBees(),
	}); len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestDiagnoseTraceClosingReportsFailedClosingRound(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1":    completedWorkTask("task-1", timeAt(0)),
			"closing-1": closingTask("closing-1", protocol.TaskStatusFailed, timeAt(1)),
		},
	}
	warnings := tasks.DiagnoseTraceClosing(tasks.ClosingDiagnosisInput{
		Trace:  trace,
		Policy: closingPolicy(true, "builder", 0),
		Bees:   closingBees(),
	})
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v", warnings)
	}
	for _, want := range []string{"closing round", "closing-1", "failed", "final gate stays closed"} {
		if !strings.Contains(warnings[0], want) {
			t.Fatalf("warning %q missing %q", warnings[0], want)
		}
	}
}

func TestDiagnoseTraceDependenciesNamesAStuckClosingTask(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1":    {TaskID: "task-1", Status: protocol.TaskStatusBlocked},
			"closing-1": {TaskID: "closing-1", Phase: protocol.TaskPhaseClosing, Status: protocol.TaskStatusPlanned, DependsOn: []string{"task-1"}},
		},
	}
	warnings := tasks.DiagnoseTraceDependencies(trace)
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v", warnings)
	}
	for _, want := range []string{"closing task", `"closing-1"`, "blocked", "final gate stays closed"} {
		if !strings.Contains(warnings[0], want) {
			t.Fatalf("warning %q missing %q", warnings[0], want)
		}
	}
}
