package taskledger_test

import (
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

func TestApplyEventTaskStatus(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusRunning},
		},
	}
	ev, err := protocol.NewEvent("trace-1", "runtime", 1, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind:    protocol.TaskEventStatus,
		TaskID:  "task-1",
		Status:  protocol.TaskStatusWaitingReview,
		Summary: "needs eyes",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.Tasks["task-1"].Status != protocol.TaskStatusWaitingReview {
		t.Fatalf("status = %q", res.Trace.Tasks["task-1"].Status)
	}
	if res.Trace.Tasks["task-1"].Summary != "needs eyes" {
		t.Fatalf("summary = %q", res.Trace.Tasks["task-1"].Summary)
	}
}

func TestApplyEventTaskStatusClearsSummary(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {
				TaskID:  "task-1",
				Status:  protocol.TaskStatusBlocked,
				Summary: protocol.HoneyReserveExhaustedSummary,
			},
		},
	}
	ev, err := protocol.NewEvent("trace-1", "runtime", 1, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind:   protocol.TaskEventStatus,
		TaskID: "task-1",
		Status: protocol.TaskStatusReady,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.Tasks["task-1"].Status != protocol.TaskStatusReady {
		t.Fatalf("status = %q", res.Trace.Tasks["task-1"].Status)
	}
	if res.Trace.Tasks["task-1"].Summary != "" {
		t.Fatalf("summary = %q, want cleared", res.Trace.Tasks["task-1"].Summary)
	}
}

func TestApplyEventTaskPlanPreservesReview(t *testing.T) {
	trace := taskledger.TraceSnapshot{TraceID: "trace-1"}
	ev, err := protocol.NewEvent("trace-1", "scout", 1, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{
			{TaskID: "task-1", Title: "Gate", Review: protocol.TaskReviewFinal},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.Tasks["task-1"].Review != protocol.TaskReviewFinal {
		t.Fatalf("review = %q", res.Trace.Tasks["task-1"].Review)
	}
}

func TestAllPreMergeCompletedRequiresReviewRequiredDone(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
			"task-2": {TaskID: "task-2", Review: protocol.TaskReviewRequired, Status: protocol.TaskStatusWaitingReview},
		},
	}
	if taskledger.AllPreMergeCompleted(trace) {
		t.Fatal("expected false while review: required task is not completed")
	}
	trace.Tasks["task-2"] = taskledger.TaskSnapshot{TaskID: "task-2", Review: protocol.TaskReviewRequired, Status: protocol.TaskStatusCompleted}
	if !taskledger.AllPreMergeCompleted(trace) {
		t.Fatal("expected true when all non-merge tasks are completed")
	}
}

func TestAllPreMergeCompletedIgnoresMergeTask(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
			"review": {TaskID: "review", Review: protocol.TaskReviewFinal, Status: protocol.TaskStatusPlanned},
		},
	}
	if !taskledger.AllPreMergeCompleted(trace) {
		t.Fatal("expected pre-merge tasks completed")
	}
	if taskledger.IsMergeTask(trace.Tasks["review"]) != true {
		t.Fatal("review: final must read as the merge phase")
	}
}

func TestEligiblePlannedNeverIncludesMergeTask(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusPlanned},
			"review": {TaskID: "review", Review: protocol.TaskReviewFinal, Status: protocol.TaskStatusPlanned},
		},
	}
	if got := taskledger.EligiblePlanned(trace); len(got) != 1 || got[0].TaskID != "task-1" {
		t.Fatalf("eligible = %+v, want only the work task", got)
	}
	trace.Tasks["task-1"] = taskledger.TaskSnapshot{TaskID: "task-1", Status: protocol.TaskStatusCompleted}
	if got := taskledger.EligiblePlanned(trace); len(got) != 0 {
		t.Fatalf("eligible = %+v, want no merge task", got)
	}
	if _, ok := taskledger.FirstEligiblePlanned(trace); ok {
		t.Fatal("a merge task must never be dispatched")
	}
}

func TestTaskPhaseOf(t *testing.T) {
	cases := []struct {
		name string
		task taskledger.TaskSnapshot
		want protocol.TaskPhase
	}{
		{
			name: "nothing recorded reads as work",
			task: taskledger.TaskSnapshot{TaskID: "task-1"},
			want: protocol.TaskPhaseWork,
		},
		{
			name: "legacy final review reads as merge",
			task: taskledger.TaskSnapshot{TaskID: "_review", Review: protocol.TaskReviewFinal},
			want: protocol.TaskPhaseMerge,
		},
		{
			name: "recorded closing reads as closing",
			task: taskledger.TaskSnapshot{TaskID: "closing-1", Phase: protocol.TaskPhaseClosing},
			want: protocol.TaskPhaseClosing,
		},
		{
			name: "merge without final review falls back to work",
			task: taskledger.TaskSnapshot{TaskID: "task-1", Phase: protocol.TaskPhaseMerge, Review: protocol.TaskReviewNone},
			want: protocol.TaskPhaseWork,
		},
		{
			name: "merge with final review stays merge",
			task: taskledger.TaskSnapshot{TaskID: "_review", Phase: protocol.TaskPhaseMerge, Review: protocol.TaskReviewFinal},
			want: protocol.TaskPhaseMerge,
		},
		{
			name: "unknown phase reads as work",
			task: taskledger.TaskSnapshot{TaskID: "task-1", Phase: protocol.TaskPhase("deploy")},
			want: protocol.TaskPhaseWork,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := taskledger.TaskPhaseOf(tc.task); got != tc.want {
				t.Fatalf("phase = %q, want %q", got, tc.want)
			}
			if got, want := taskledger.IsMergeTask(tc.task), tc.want == protocol.TaskPhaseMerge; got != want {
				t.Fatalf("IsMergeTask = %v, want %v", got, want)
			}
			if got, want := taskledger.IsClosingTask(tc.task), tc.want == protocol.TaskPhaseClosing; got != want {
				t.Fatalf("IsClosingTask = %v, want %v", got, want)
			}
		})
	}
}

func TestAllWorkCompletedAndAllPreMergeCompleted(t *testing.T) {
	cases := []struct {
		name        string
		tasks       map[string]taskledger.TaskSnapshot
		allWork     bool
		allPreMerge bool
	}{
		{
			name: "work complete with closing planned",
			tasks: map[string]taskledger.TaskSnapshot{
				"task-1":    {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
				"closing-1": {TaskID: "closing-1", Phase: protocol.TaskPhaseClosing, Status: protocol.TaskStatusPlanned},
			},
			allWork:     true,
			allPreMerge: false,
		},
		{
			name: "work and closing complete",
			tasks: map[string]taskledger.TaskSnapshot{
				"task-1":    {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
				"closing-1": {TaskID: "closing-1", Phase: protocol.TaskPhaseClosing, Status: protocol.TaskStatusCompleted},
			},
			allWork:     true,
			allPreMerge: true,
		},
		{
			name: "work still running with closing planned",
			tasks: map[string]taskledger.TaskSnapshot{
				"task-1":    {TaskID: "task-1", Status: protocol.TaskStatusRunning},
				"closing-1": {TaskID: "closing-1", Phase: protocol.TaskPhaseClosing, Status: protocol.TaskStatusPlanned},
			},
			allWork:     false,
			allPreMerge: false,
		},
		{
			name: "merge task is ignored by both",
			tasks: map[string]taskledger.TaskSnapshot{
				"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
				"_review": {
					TaskID: "_review", Review: protocol.TaskReviewFinal, Status: protocol.TaskStatusWaitingReview,
				},
			},
			allWork:     true,
			allPreMerge: true,
		},
		{
			name: "legacy closing round is pre-merge but not work",
			tasks: map[string]taskledger.TaskSnapshot{
				"task-1":    {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
				"closing-1": {TaskID: "closing-1", Phase: protocol.TaskPhaseClosing, Status: protocol.TaskStatusFailed},
			},
			allWork:     true,
			allPreMerge: false,
		},
		{
			name: "failed work keeps both false",
			tasks: map[string]taskledger.TaskSnapshot{
				"task-1": {TaskID: "task-1", Status: protocol.TaskStatusFailed},
			},
			allWork:     false,
			allPreMerge: false,
		},
		{
			name:        "empty trace",
			tasks:       map[string]taskledger.TaskSnapshot{},
			allWork:     false,
			allPreMerge: false,
		},
		{
			name: "merge only trace has no work to complete",
			tasks: map[string]taskledger.TaskSnapshot{
				"_review": {TaskID: "_review", Review: protocol.TaskReviewFinal, Status: protocol.TaskStatusPlanned},
			},
			allWork:     false,
			allPreMerge: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trace := taskledger.TraceSnapshot{TraceID: "trace-1", Tasks: tc.tasks}
			if got := taskledger.AllWorkCompleted(trace); got != tc.allWork {
				t.Fatalf("AllWorkCompleted = %v, want %v", got, tc.allWork)
			}
			if got := taskledger.AllPreMergeCompleted(trace); got != tc.allPreMerge {
				t.Fatalf("AllPreMergeCompleted = %v, want %v", got, tc.allPreMerge)
			}
		})
	}
}

func TestHasIsolatedProposal(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", ProposalWorkspace: protocol.ProposalWorkspaceRoot},
		},
	}
	if taskledger.HasIsolatedProposal(trace) {
		t.Fatal("root proposal must not count as isolated merge candidate")
	}
	trace.Tasks["task-1"] = taskledger.TaskSnapshot{
		TaskID:            "task-1",
		ProposalWorkspace: protocol.ProposalWorkspaceIsolated,
	}
	if !taskledger.HasIsolatedProposal(trace) {
		t.Fatal("expected isolated proposal")
	}
}

func TestLastCompletedIsolatedProposalTaskPicksNewest(t *testing.T) {
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {
				TaskID: "task-1", Bee: "builder", Status: protocol.TaskStatusCompleted,
				ProposalWorkspace: protocol.ProposalWorkspaceIsolated, UpdatedAt: older,
			},
			"task-2": {
				TaskID: "task-2", Bee: "builder", Status: protocol.TaskStatusCompleted,
				ProposalWorkspace: protocol.ProposalWorkspaceIsolated, UpdatedAt: newer,
			},
			"_review": {
				TaskID: "_review", Review: protocol.TaskReviewFinal, Status: protocol.TaskStatusWaitingReview,
			},
			"root-1": {
				TaskID: "root-1", Status: protocol.TaskStatusCompleted,
				ProposalWorkspace: protocol.ProposalWorkspaceRoot, UpdatedAt: newer.Add(time.Hour),
			},
		},
	}
	got, ok := taskledger.LastCompletedIsolatedProposalTask(trace)
	if !ok || got.TaskID != "task-2" {
		t.Fatalf("got %+v ok=%v, want task-2", got, ok)
	}
}

func TestLastCompletedIsolatedProposalTaskExcludesClosing(t *testing.T) {
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {
				TaskID: "task-1", Bee: "builder", Status: protocol.TaskStatusCompleted,
				ProposalWorkspace: protocol.ProposalWorkspaceIsolated, UpdatedAt: older,
			},
			"closing-1": {
				TaskID: "closing-1", Bee: "reviewer", Phase: protocol.TaskPhaseClosing,
				Status:            protocol.TaskStatusCompleted,
				ProposalWorkspace: protocol.ProposalWorkspaceIsolated, UpdatedAt: newer,
			},
		},
	}
	got, ok := taskledger.LastCompletedIsolatedProposalTask(trace)
	if !ok || got.TaskID != "task-1" {
		t.Fatalf("got %+v ok=%v, want task-1 (closing is never the rework source)", got, ok)
	}
}

func TestCanRequestChangesAndInFlightRework(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {
				TaskID: "task-1", Bee: "builder", Status: protocol.TaskStatusCompleted,
				ProposalWorkspace: protocol.ProposalWorkspaceIsolated,
			},
			"_review": {
				TaskID: "_review", Review: protocol.TaskReviewFinal, Status: protocol.TaskStatusWaitingReview,
			},
		},
	}
	if !taskledger.CanRequestChanges(trace, trace.Tasks["_review"]) {
		t.Fatal("expected can request changes")
	}
	trace.Tasks["task-rework"] = taskledger.TaskSnapshot{
		TaskID: "task-rework", Status: protocol.TaskStatusRunning,
	}
	if inflight, ok := taskledger.InFlightRework(trace); !ok || inflight.TaskID != "task-rework" {
		t.Fatalf("inflight = %+v ok=%v", inflight, ok)
	}
	if taskledger.CanRequestChanges(trace, trace.Tasks["_review"]) {
		t.Fatal("expected request changes disabled while rework running")
	}
}
