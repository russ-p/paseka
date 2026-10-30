package tasks

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
)

// DiagnoseDependencies scans every trail in the task ledger for planned tasks
// that can never become eligible. The final gate opens only when every non-merge
// task has completed, so each finding is a trail that is closed for
// good until an operator repairs it.
func DiagnoseDependencies(ctx colony.Context) []string {
	return diagnoseTraces(ctx, DiagnoseTraceDependencies)
}

// DiagnoseClosings scans every trail for closing-phase problems: a work-complete
// trail whose closing seed is missing or cannot be built, a closing round stuck
// behind an impossible dependency, a trail that reached the round cap with new
// work still unseeded, and a trail carrying more than one merge gate.
func DiagnoseClosings(ctx colony.Context) []string {
	manifest, err := colony.LoadColony(ctx.ColonyRoot)
	if err != nil {
		return []string{fmt.Sprintf("colony: %v", err)}
	}
	bees, err := colony.LoadAllBeesForDiagnosis(ctx.ColonyRoot)
	if err != nil {
		// Doctor already reports the unreadable bee set. Skip the bee check
		// here rather than reporting every closing policy as a missing bee.
		bees = nil
	}
	return diagnoseTraces(ctx, func(trace taskledger.TraceSnapshot) []string {
		pinned, ok, err := trailconfig.LoadManifestIfExists(ctx.ColonyRoot, trace.TraceID)
		if err != nil {
			return []string{fmt.Sprintf("trace %s: read configuration binding: %v", trace.TraceID, err)}
		}
		policy := colony.ClosingPolicy{}
		if ok {
			policy = pinned.ClosingPolicy()
		}
		return DiagnoseTraceClosing(ClosingDiagnosisInput{
			Trace:    trace,
			Policy:   policy,
			Bees:     bees,
			Defaults: manifest.Defaults,
		})
	})
}

// diagnoseTraces runs one diagnosis over every trace the ledger can list.
func diagnoseTraces(ctx colony.Context, diagnose func(taskledger.TraceSnapshot) []string) []string {
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
		warnings = append(warnings, diagnose(snap)...)
	}
	return warnings
}

// DiagnoseTraceDependencies reports why the planned tasks of one trace can never
// run: a dependsOn id that no task carries (the typo case), a self dependency, a
// cycle, or a dependency on a task that is cancelled or blocked. Tasks that
// already completed, failed, or are in flight keep their status but drop their
// edges, so a healthy finished task with a stale dependency is not reported.
func DiagnoseTraceDependencies(trace taskledger.TraceSnapshot) []string {
	if trace.Killed || !hasPlannedTask(trace) || taskledger.AllPreMergeCompleted(trace) {
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
				warnings = append(warnings, fmt.Sprintf("trace %s: %s %q depends on task %q, which is %s; the trail cannot finish and the final gate stays closed", trace.TraceID, taskNoun(task), taskID, depID, dep.Status))
			}
		}
	}
	return warnings
}

// ClosingDiagnosisInput is one trail's closing-phase diagnosis. Bees may be nil
// when the caller only wants ledger findings and not the bee-existence check.
type ClosingDiagnosisInput struct {
	Trace    taskledger.TraceSnapshot
	Policy   colony.ClosingPolicy
	Bees     map[string]colony.Bee
	Defaults colony.Defaults
}

// DiagnoseTraceClosing reports closing-phase problems for one trace under the
// policy pinned at its start.
func DiagnoseTraceClosing(in ClosingDiagnosisInput) []string {
	trace := in.Trace
	if trace.Killed {
		return nil
	}

	var warnings []string
	var mergeIDs, closingIDs, completedClosingIDs []string
	for taskID, task := range trace.Tasks {
		switch {
		case taskledger.IsMergeTask(task):
			mergeIDs = append(mergeIDs, taskID)
		case taskledger.IsClosingTask(task):
			closingIDs = append(closingIDs, taskID)
			if task.Status == protocol.TaskStatusCompleted {
				completedClosingIDs = append(completedClosingIDs, taskID)
			}
		}
	}
	sort.Strings(mergeIDs)
	sort.Strings(closingIDs)
	sort.Strings(completedClosingIDs)

	if len(mergeIDs) > 1 {
		warnings = append(warnings, fmt.Sprintf("trace %s: %d merge-phase tasks (%s); exactly one merge gate is allowed", trace.TraceID, len(mergeIDs), strings.Join(mergeIDs, ", ")))
	}

	if mergeCompleted(trace) {
		return warnings
	}

	// A closing round that ended failed, blocked, or cancelled without any
	// completed round cannot be repaired by seeding another one: the gate
	// requires every closing task to complete.
	if len(completedClosingIDs) == 0 {
		for _, taskID := range closingIDs {
			task := trace.Tasks[taskID]
			switch task.Status {
			case protocol.TaskStatusFailed, protocol.TaskStatusBlocked, protocol.TaskStatusCancelled:
				warnings = append(warnings, fmt.Sprintf("trace %s: closing round %q is %s; the final gate stays closed until a human repairs the round", trace.TraceID, taskID, task.Status))
			}
		}
	}

	if !in.Policy.Enabled || closingInFlight(trace) || !taskledger.AllWorkCompleted(trace) {
		return warnings
	}
	if !closingPredicatesHold(trace, in.Policy) {
		return warnings
	}

	if err := in.Policy.Validate(in.Bees, in.Defaults); err != nil {
		warnings = append(warnings, fmt.Sprintf("trace %s: work is complete but the closing seed cannot be built: %v", trace.TraceID, err))
		return warnings
	}

	lastClosing, hasClosing := latestClosingTask(trace)
	maxRounds := in.Policy.ResolvedMaxRounds()
	if len(closingIDs) >= maxRounds {
		if hasClosing && workCompletedAfter(trace, lastClosing) {
			warnings = append(warnings, fmt.Sprintf("trace %s: closing reached the cap of %d round(s) with follow-up work completed after %q; the final gate stays closed until a human adds a round or repairs the trail", trace.TraceID, maxRounds, lastClosing.TaskID))
		}
		return warnings
	}
	if !hasClosing {
		warnings = append(warnings, fmt.Sprintf("trace %s: work is complete but the closing seed has not been built; the final gate stays closed", trace.TraceID))
	}
	return warnings
}

// taskNoun names the ledger phase in a dependency warning so a closing round is
// distinguishable from ordinary work.
func taskNoun(task taskledger.TaskSnapshot) string {
	if taskledger.IsClosingTask(task) {
		return "closing task"
	}
	return "task"
}

func mergeCompleted(trace taskledger.TraceSnapshot) bool {
	for _, task := range trace.Tasks {
		if taskledger.IsMergeTask(task) && task.Status == protocol.TaskStatusCompleted {
			return true
		}
	}
	return false
}

func closingInFlight(trace taskledger.TraceSnapshot) bool {
	for _, task := range trace.Tasks {
		if !taskledger.IsClosingTask(task) {
			continue
		}
		switch task.Status {
		case protocol.TaskStatusPlanned, protocol.TaskStatusReady, protocol.TaskStatusRunning,
			protocol.TaskStatusWaitingReview, protocol.TaskStatusBlocked:
			return true
		}
	}
	return false
}

func closingPredicatesHold(trace taskledger.TraceSnapshot, policy colony.ClosingPolicy) bool {
	workTasks := 0
	for _, task := range trace.Tasks {
		if taskledger.TaskPhaseOf(task) == protocol.TaskPhaseWork {
			workTasks++
		}
	}
	return policy.Matches(workTasks, taskledger.HasIsolatedProposal(trace))
}

// latestClosingTask returns the closing round with the newest activity, using
// created time when an older record carries no update stamp.
func latestClosingTask(trace taskledger.TraceSnapshot) (taskledger.TaskSnapshot, bool) {
	var latest taskledger.TaskSnapshot
	found := false
	for _, task := range trace.Tasks {
		if !taskledger.IsClosingTask(task) {
			continue
		}
		if !found || taskTime(task).After(taskTime(latest)) ||
			(taskTime(task).Equal(taskTime(latest)) && task.TaskID > latest.TaskID) {
			latest = task
			found = true
		}
	}
	return latest, found
}

// workCompletedAfter reports whether a work-phase task completed after the
// closing round finished. That is the "new work completed after the last
// closing round" trigger the runtime uses to seed the next round.
func workCompletedAfter(trace taskledger.TraceSnapshot, closing taskledger.TaskSnapshot) bool {
	closingAt := taskTime(closing)
	if closingAt.IsZero() {
		return false
	}
	for _, task := range trace.Tasks {
		if taskledger.TaskPhaseOf(task) != protocol.TaskPhaseWork || task.Status != protocol.TaskStatusCompleted {
			continue
		}
		if taskTime(task).After(closingAt) {
			return true
		}
	}
	return false
}

func taskTime(task taskledger.TaskSnapshot) time.Time {
	if !task.UpdatedAt.IsZero() {
		return task.UpdatedAt
	}
	return task.CreatedAt
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
