package review_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/review"
	"github.com/russ-p/paseka/internal/taskledger"
)

func TestApproveMergeGateRefusedWhileNonMergeTaskIncomplete(t *testing.T) {
	for _, status := range []protocol.TaskStatus{
		protocol.TaskStatusPlanned,
		protocol.TaskStatusReady,
		protocol.TaskStatusRunning,
		protocol.TaskStatusWaitingReview,
		protocol.TaskStatusBlocked,
		protocol.TaskStatusFailed,
	} {
		t.Run(string(status), func(t *testing.T) {
			ledger := taskledger.NewMemoryLedger()
			traceID := "trace-approve-guard"
			base := time.Now().UTC().Add(-time.Hour)
			applyPlanAt(t, ledger, traceID, "scout", base, protocol.TaskSpec{TaskID: "task-1", Title: "Work"})
			applyStatusAt(t, ledger, traceID, "task-1", status, base.Add(time.Minute))
			applyPlanAt(t, ledger, traceID, "scout", base.Add(2*time.Minute), protocol.TaskSpec{
				TaskID: "merge", Title: "Merge", Review: protocol.TaskReviewFinal,
			})
			applyStatusAt(t, ledger, traceID, "merge", protocol.TaskStatusWaitingReview, base.Add(3*time.Minute))

			_, err := review.Approve(context.Background(), colonyCtx(t), nil, ledger, review.ApproveInput{
				TraceID: traceID,
				TaskID:  "merge",
			}, review.WriteOptions{})
			if err == nil {
				t.Fatalf("approved the merge gate while task-1 is %s", status)
			}
			if !strings.Contains(err.Error(), "task-1") || !strings.Contains(err.Error(), string(status)) {
				t.Fatalf("reason = %q, want the unfinished task and its status", err)
			}
			snap, snapErr := ledger.Snapshot(traceID)
			if snapErr != nil {
				t.Fatal(snapErr)
			}
			if snap.Tasks["merge"].Status != protocol.TaskStatusWaitingReview {
				t.Fatalf("merge status = %q, want waiting_review", snap.Tasks["merge"].Status)
			}
		})
	}
}

func TestApproveMergeGateAllowedWhenEveryNonMergeTaskCompleted(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	traceID := "trace-approve-complete"
	base := time.Now().UTC().Add(-time.Hour)
	applyPlanAt(t, ledger, traceID, "scout", base, protocol.TaskSpec{TaskID: "task-1", Title: "Work"})
	completeTaskAt(t, ledger, traceID, "task-1", base.Add(time.Minute))
	applyPlanAt(t, ledger, traceID, "scout", base.Add(2*time.Minute), protocol.TaskSpec{
		TaskID: "merge", Title: "Merge", Review: protocol.TaskReviewFinal,
	})
	applyStatusAt(t, ledger, traceID, "merge", protocol.TaskStatusWaitingReview, base.Add(3*time.Minute))

	if _, err := review.Approve(context.Background(), colonyCtx(t), nil, ledger, review.ApproveInput{
		TraceID: traceID,
		TaskID:  "merge",
	}, review.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Tasks["merge"].Status != protocol.TaskStatusCompleted {
		t.Fatalf("merge status = %q, want completed", snap.Tasks["merge"].Status)
	}
}

func TestApproveMidTrailReviewGateIgnoresPlannedDownstreamWork(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	traceID := "trace-approve-midtrail"
	base := time.Now().UTC().Add(-time.Hour)
	applyPlanAt(t, ledger, traceID, "scout", base, protocol.TaskSpec{
		TaskID: "task-1", Title: "Work", Review: protocol.TaskReviewRequired,
	})
	applyStatusAt(t, ledger, traceID, "task-1", protocol.TaskStatusWaitingReview, base.Add(time.Minute))
	applyPlanAt(t, ledger, traceID, "scout", base.Add(2*time.Minute), protocol.TaskSpec{
		TaskID: "task-2", Title: "Downstream", DependsOn: []string{"task-1"},
	})

	if _, err := review.Approve(context.Background(), colonyCtx(t), nil, ledger, review.ApproveInput{
		TraceID: traceID,
		TaskID:  "task-1",
	}, review.WriteOptions{}); err != nil {
		t.Fatalf("mid-trail review gate must stay approvable: %v", err)
	}
}

func TestClosingRoundGuaranteed(t *testing.T) {
	base := time.Now().UTC().Add(-time.Hour)
	completedWork := func() taskledger.TraceSnapshot {
		return taskledger.TraceSnapshot{TraceID: "trace", Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusRunning},
		}}
	}
	withClosing := func(status protocol.TaskStatus) taskledger.TraceSnapshot {
		return taskledger.TraceSnapshot{TraceID: "trace", Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusRunning},
			"closing-1": {
				TaskID: "closing-1", Phase: protocol.TaskPhaseClosing,
				Status: status, UpdatedAt: base,
			},
		}}
	}

	tests := []struct {
		name   string
		policy colony.ClosingPolicy
		trace  taskledger.TraceSnapshot
		want   bool
	}{
		{
			name:   "enabled policy with a round left",
			policy: colony.ClosingPolicy{Enabled: true, MaxRounds: intPtr(2)},
			trace:  completedWork(),
			want:   true,
		},
		{
			name:   "disabled policy",
			policy: colony.ClosingPolicy{},
			trace:  completedWork(),
			want:   false,
		},
		{
			name:   "min_work_tasks not met",
			policy: colony.ClosingPolicy{Enabled: true, When: colony.ClosingWhen{MinWorkTasks: 2}},
			trace:  completedWork(),
			want:   false,
		},
		{
			name:   "has_isolated_proposal not met",
			policy: colony.ClosingPolicy{Enabled: true, When: colony.ClosingWhen{HasIsolatedProposal: true}},
			trace:  completedWork(),
			want:   false,
		},
		{
			name:   "cap reached by a completed round",
			policy: colony.ClosingPolicy{Enabled: true, MaxRounds: intPtr(1)},
			trace:  withClosing(protocol.TaskStatusCompleted),
			want:   false,
		},
		{
			name:   "failed round is never replaced",
			policy: colony.ClosingPolicy{Enabled: true, MaxRounds: intPtr(3)},
			trace:  withClosing(protocol.TaskStatusFailed),
			want:   false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := review.ClosingRoundGuaranteed(tc.policy, tc.trace); got != tc.want {
				t.Fatalf("ClosingRoundGuaranteed() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMergeGateFallsBackToDefaultCopy(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	traceID := "trace-gate-default-copy"
	base := time.Now().UTC().Add(-time.Hour)
	applyPlanAt(t, ledger, traceID, "scout", base, protocol.TaskSpec{TaskID: "task-1", Title: "Build"})
	applyIsolatedProposal(t, ledger, traceID, "task-1")
	completeTaskAt(t, ledger, traceID, "task-1", base.Add(time.Minute))

	if _, err := review.EnsureTrailPhases(context.Background(), nil, ledger, colony.Context{}, traceID, review.GatePolicies{}, review.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	gate := snap.Tasks[taskledger.FinalReviewTaskID]
	if gate.Title != colony.DefaultMergeTaskTitle {
		t.Fatalf("gate title = %q, want today's literal copy", gate.Title)
	}
	if gate.Body != colony.DefaultMergeTaskBody {
		t.Fatalf("gate body = %q, want today's literal copy", gate.Body)
	}
}
