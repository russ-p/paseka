package taskledger_test

import (
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

func planWork(t *testing.T, trace taskledger.TraceSnapshot, agentID string, specs ...protocol.TaskSpec) taskledger.TraceSnapshot {
	t.Helper()
	ev, err := protocol.NewEvent(trace.TraceID, agentID, 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind:  protocol.TaskEventPlan,
		Tasks: specs,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	return res.Trace
}

func applyStatus(t *testing.T, trace taskledger.TraceSnapshot, taskID string, status protocol.TaskStatus) taskledger.TraceSnapshot {
	t.Helper()
	ev, err := protocol.NewEvent(trace.TraceID, "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind:   protocol.TaskEventStatus,
		TaskID: taskID,
		Status: status,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	return res.Trace
}

func TestWorkRevisionAdvancesForAcceptedWorkOnly(t *testing.T) {
	trace := taskledger.TraceSnapshot{TraceID: "trace-1", Tasks: map[string]taskledger.TaskSnapshot{}}

	trace = planWork(t, trace, "scout", protocol.TaskSpec{TaskID: "task-1", Title: "work"})
	if trace.WorkRevision != 1 {
		t.Fatalf("revision after first work plan = %d, want 1", trace.WorkRevision)
	}

	// Replaying the same plan does not reopen the revision.
	trace = planWork(t, trace, "scout", protocol.TaskSpec{TaskID: "task-1", Title: "work"})
	if trace.WorkRevision != 1 {
		t.Fatalf("revision after duplicate plan = %d, want 1", trace.WorkRevision)
	}

	// A closing round is overhead, not accepted goal work.
	trace = planWork(t, trace, protocol.RuntimeAgentID, protocol.TaskSpec{
		TaskID: "closing-1", Phase: protocol.TaskPhaseClosing,
	})
	if trace.WorkRevision != 1 {
		t.Fatalf("revision after closing seed = %d, want 1", trace.WorkRevision)
	}

	// A merge gate is acceptance, not accepted goal work.
	trace = planWork(t, trace, protocol.RuntimeAgentID, protocol.TaskSpec{
		TaskID: "_review", Review: protocol.TaskReviewFinal,
	})
	if trace.WorkRevision != 1 {
		t.Fatalf("revision after merge plan = %d, want 1", trace.WorkRevision)
	}

	// A rework return opens a new revision.
	trace = applyStatus(t, trace, "task-1", protocol.TaskStatusCompleted)
	if trace.WorkRevision != 1 {
		t.Fatalf("revision after completion = %d, want 1", trace.WorkRevision)
	}
	trace = applyStatus(t, trace, "task-1", protocol.TaskStatusWaitingReview)
	trace = applyStatus(t, trace, "task-1", protocol.TaskStatusReady)
	if trace.WorkRevision != 2 {
		t.Fatalf("revision after rework return = %d, want 2", trace.WorkRevision)
	}

	// New work planned from a gate rejection opens another revision.
	trace = planWork(t, trace, "human", protocol.TaskSpec{TaskID: "rework-1", Title: "rework"})
	if trace.WorkRevision != 3 {
		t.Fatalf("revision after rework plan = %d, want 3", trace.WorkRevision)
	}
}

func TestEvaluateWorkCompletion(t *testing.T) {
	cases := []struct {
		name     string
		tasks    map[string]taskledger.TaskSnapshot
		pending  []taskledger.PendingDispatch
		complete bool
	}{
		{
			name: "work complete",
			tasks: map[string]taskledger.TaskSnapshot{
				"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
			},
			complete: true,
		},
		{
			name: "work and closing complete",
			tasks: map[string]taskledger.TaskSnapshot{
				"task-1":    {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
				"closing-1": {TaskID: "closing-1", Phase: protocol.TaskPhaseClosing, Status: protocol.TaskStatusCompleted},
			},
			complete: true,
		},
		{
			name: "waiting merge gate does not block",
			tasks: map[string]taskledger.TaskSnapshot{
				"task-1":  {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
				"_review": {TaskID: "_review", Review: protocol.TaskReviewFinal, Status: protocol.TaskStatusWaitingReview},
			},
			complete: true,
		},
		{
			name: "review required work gate blocks",
			tasks: map[string]taskledger.TaskSnapshot{
				"task-1": {TaskID: "task-1", Review: protocol.TaskReviewRequired, Status: protocol.TaskStatusWaitingReview},
			},
			complete: false,
		},
		{
			name: "unprovable gate purpose holds",
			tasks: map[string]taskledger.TaskSnapshot{
				"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
				// An empty phase and non-final review cannot prove final
				// acceptance, so it reads as an unfinished work gate.
				"gate": {TaskID: "gate", Status: protocol.TaskStatusWaitingReview},
			},
			complete: false,
		},
		{
			name:     "empty ledger is not complete by vacuous truth",
			tasks:    map[string]taskledger.TaskSnapshot{},
			complete: false,
		},
		{
			name: "merge-only trail has no work",
			tasks: map[string]taskledger.TaskSnapshot{
				"_review": {TaskID: "_review", Review: protocol.TaskReviewFinal, Status: protocol.TaskStatusCompleted},
			},
			complete: false,
		},
		{
			name: "failed work blocks",
			tasks: map[string]taskledger.TaskSnapshot{
				"task-1": {TaskID: "task-1", Status: protocol.TaskStatusFailed},
			},
			complete: false,
		},
		{
			name: "cancelled work blocks",
			tasks: map[string]taskledger.TaskSnapshot{
				"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCancelled},
			},
			complete: false,
		},
		{
			name: "skipped work is not completion",
			tasks: map[string]taskledger.TaskSnapshot{
				"task-1": {TaskID: "task-1", Status: protocol.TaskStatusSkipped},
			},
			complete: false,
		},
		{
			name: "energy-deferred work blocks even at completed",
			tasks: map[string]taskledger.TaskSnapshot{
				"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
			},
			pending: []taskledger.PendingDispatch{{
				ID: "task:task-1:builder", Kind: taskledger.PendingDispatchTask,
				TaskID: "task-1", Status: taskledger.PendingDispatchPending,
			}},
			complete: false,
		},
		{
			name: "analysis deferral does not hold work completion",
			tasks: map[string]taskledger.TaskSnapshot{
				"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
			},
			pending: []taskledger.PendingDispatch{{
				ID: "analysis", Kind: taskledger.PendingDispatchDirect,
				Status: taskledger.PendingDispatchPending, Purpose: protocol.PurposeAnalysis,
			}},
			complete: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trace := taskledger.TraceSnapshot{TraceID: "trace-1", Tasks: tc.tasks, PendingDispatches: tc.pending}
			verdict := taskledger.EvaluateWorkCompletion(trace)
			if verdict.Complete != tc.complete {
				t.Fatalf("complete = %v (%s), want %v", verdict.Complete, verdict.Reason, tc.complete)
			}
		})
	}
}

// TestAnalysisTasksStayOutsideTheWorkingSet pins the analysis boundary: an
// analysis task may be planned, running, or failed without reopening the work
// revision, holding completion, or blocking the final gate.
func TestAnalysisTasksStayOutsideTheWorkingSet(t *testing.T) {
	trace := taskledger.TraceSnapshot{TraceID: "trace-1", Tasks: map[string]taskledger.TaskSnapshot{}}
	trace = planWork(t, trace, "scout", protocol.TaskSpec{TaskID: "task-1", Title: "work"})
	trace = applyStatus(t, trace, "task-1", protocol.TaskStatusCompleted)

	ev, err := protocol.NewEvent(trace.TraceID, "analyst-agent", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{
			TaskID: "analysis-1", Title: "study the result", Bee: "analyst",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ev.Purpose = protocol.PurposeAnalysis
	ev.AnalysisOf = "completion-r1"
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	trace = res.Trace
	if trace.WorkRevision != 1 {
		t.Fatalf("analysis plan advanced the revision to %d", trace.WorkRevision)
	}
	task := trace.Tasks["analysis-1"]
	if !taskledger.IsAnalysisTask(task) || task.AnalysisOf != "completion-r1" {
		t.Fatalf("analysis task attribution = %+v", task)
	}

	// An unfinished analysis task neither holds completion nor the final gate.
	if verdict := taskledger.EvaluateWorkCompletion(trace); !verdict.Complete {
		t.Fatalf("analysis task held completion: %s", verdict.Reason)
	}
	if !taskledger.AllPreMergeCompleted(trace) {
		t.Fatal("analysis task held the final gate")
	}
	if _, ok := taskledger.InFlightRework(trace); ok {
		t.Fatal("analysis task became in-flight product rework")
	}

	// A failed analysis task stays an analysis failure, not failed goal work.
	trace = applyStatus(t, trace, "analysis-1", protocol.TaskStatusFailed)
	if trace.WorkRevision != 1 {
		t.Fatalf("analysis failure advanced the revision to %d", trace.WorkRevision)
	}
	if verdict := taskledger.EvaluateWorkCompletion(trace); !verdict.Complete {
		t.Fatalf("failed analysis held completion: %s", verdict.Reason)
	}
	if !taskledger.AllPreMergeCompleted(trace) {
		t.Fatal("failed analysis held the final gate")
	}

	// Accepting substantive findings is ordinary work admission: it reopens the
	// episode and advances the revision.
	trace = planWork(t, trace, "human", protocol.TaskSpec{TaskID: "rework-1", Title: "implement finding"})
	if trace.WorkRevision != 2 {
		t.Fatalf("accepted rework revision = %d, want 2", trace.WorkRevision)
	}
	if verdict := taskledger.EvaluateWorkCompletion(trace); verdict.Complete {
		t.Fatal("accepted rework did not reopen work completion")
	}
}

func TestCompletionIdentityIsStablePerRevision(t *testing.T) {
	a := taskledger.CompletionIDForRevision("trace-1", 1)
	b := taskledger.CompletionIDForRevision("trace-1", 1)
	c := taskledger.CompletionIDForRevision("trace-1", 2)
	if a != b {
		t.Fatalf("same revision produced %q and %q", a, b)
	}
	if a == c {
		t.Fatal("a new revision must get a new completion identity")
	}
}

func TestRecordWorkCompletionKeepsFirstIdentity(t *testing.T) {
	trace := taskledger.TraceSnapshot{TraceID: "trace-1", Tasks: map[string]taskledger.TaskSnapshot{}}
	trace, first, created, err := taskledger.RecordWorkCompletion(trace, taskledger.WorkCompletionSnapshot{
		CompletionID: "completion-r1-a", WorkRevision: 1,
	})
	if err != nil || !created {
		t.Fatalf("first record: created=%v err=%v", created, err)
	}
	trace, second, created, err := taskledger.RecordWorkCompletion(trace, taskledger.WorkCompletionSnapshot{
		CompletionID: "completion-r1-b", WorkRevision: 1,
	})
	if err != nil || created {
		t.Fatalf("duplicate record: created=%v err=%v", created, err)
	}
	if second.CompletionID != first.CompletionID {
		t.Fatalf("identity = %q, want %q", second.CompletionID, first.CompletionID)
	}
	trace, changed, err := taskledger.MarkWorkCompletionPublished(trace, second.CompletionID)
	if err != nil || !changed {
		t.Fatalf("mark published: changed=%v err=%v", changed, err)
	}
	if _, changed, err = taskledger.MarkWorkCompletionPublished(trace, "other"); err == nil || changed {
		t.Fatal("marking a different identity must fail closed")
	}
}

func TestAnalysisTaskDoesNotBlockChangeRequest(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {
				TaskID: "task-1", Status: protocol.TaskStatusCompleted,
				ProposalWorkspace: protocol.ProposalWorkspaceIsolated,
			},
			"_review": {
				TaskID: "_review", Review: protocol.TaskReviewFinal,
				Phase: protocol.TaskPhaseMerge, Status: protocol.TaskStatusWaitingReview,
			},
			"analysis-1": {
				TaskID: "analysis-1", Status: protocol.TaskStatusRunning,
				Purpose: protocol.PurposeAnalysis, AnalysisOf: "completion-r1",
			},
		},
	}
	final, ok := trace.Tasks["_review"]
	if !ok {
		t.Fatal("merge task missing")
	}
	if !taskledger.CanRequestChanges(trace, final) {
		t.Fatal("an in-flight analysis suppressed a legitimate change request")
	}
	if !taskledger.AllPreMergeCompleted(trace) {
		t.Fatal("an analysis task held the final gate")
	}
}
