package tasks

import (
	"fmt"
	"sort"
	"strings"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

// DiagnoseDependencies scans every trail in the task ledger for planned tasks
// that can never become eligible. The final gate opens only when every task but
// the merge gate has completed, so each finding is a trail that is closed for
// good until an operator repairs it.
func DiagnoseDependencies(ctx colony.Context) []string {
	session, err := OpenLedger(ctx)
	if err != nil {
		return []string{fmt.Sprintf("task ledger: %v", err)}
	}
	defer session.Close()
	if session.Ledger == nil {
		return nil
	}
	lister, ok := session.Ledger.(interface{ TraceIDs() ([]string, error) })
	if !ok {
		return nil
	}
	traceIDs, err := lister.TraceIDs()
	if err != nil {
		return []string{fmt.Sprintf("task ledger: %v", err)}
	}
	sort.Strings(traceIDs)

	var warnings []string
	for _, traceID := range traceIDs {
		snap, err := session.Ledger.Snapshot(traceID)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("trace %s: read task ledger: %v", traceID, err))
			continue
		}
		warnings = append(warnings, DiagnoseTraceDependencies(snap)...)
	}
	return warnings
}

// DiagnoseTraceDependencies reports why the planned tasks of one trace can never
// run: a dependsOn id that no task carries (the typo case), a self dependency, a
// cycle, or a dependency on a task that is cancelled or blocked. Tasks that
// already completed, failed, or are in flight keep their status but drop their
// edges, so a healthy finished task with a stale dependency is not reported.
func DiagnoseTraceDependencies(trace taskledger.TraceSnapshot) []string {
	if trace.Killed || !hasPlannedTask(trace) || taskledger.AllAFKTasksCompleted(trace) {
		return nil
	}

	var warnings []string
	if err := taskledger.ValidateDependencies(plannedSubgraph(trace)); err != nil {
		for _, problem := range strings.Split(err.Error(), "\n") {
			warnings = append(warnings, fmt.Sprintf("trace %s: %s; the trail cannot finish and the final gate stays closed", trace.TraceID, problem))
		}
	}

	taskIDs := make([]string, 0, len(trace.Tasks))
	for taskID := range trace.Tasks {
		taskIDs = append(taskIDs, taskID)
	}
	sort.Strings(taskIDs)
	for _, taskID := range taskIDs {
		task := trace.Tasks[taskID]
		if task.Status != protocol.TaskStatusPlanned {
			continue
		}
		for _, depID := range task.DependsOn {
			dep, ok := trace.Tasks[depID]
			if !ok {
				continue // already reported as an unknown dependency
			}
			switch dep.Status {
			case protocol.TaskStatusCancelled, protocol.TaskStatusBlocked:
				warnings = append(warnings, fmt.Sprintf("trace %s: task %q depends on task %q, which is %s; the trail cannot finish and the final gate stays closed", trace.TraceID, taskID, depID, dep.Status))
			}
		}
	}
	return warnings
}

// plannedSubgraph keeps every task id as a node but leaves edges only on planned
// tasks. A dependency on a completed task then still resolves, while a stale edge
// on a finished task cannot invent an unknown id or a cycle that blocks nothing.
func plannedSubgraph(trace taskledger.TraceSnapshot) taskledger.TraceSnapshot {
	reduced := trace
	reduced.Tasks = make(map[string]taskledger.TaskSnapshot, len(trace.Tasks))
	for taskID, task := range trace.Tasks {
		if task.Status != protocol.TaskStatusPlanned {
			task.DependsOn = nil
		}
		reduced.Tasks[taskID] = task
	}
	return reduced
}

func hasPlannedTask(trace taskledger.TraceSnapshot) bool {
	for _, task := range trace.Tasks {
		if task.Status == protocol.TaskStatusPlanned {
			return true
		}
	}
	return false
}
