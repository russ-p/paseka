package review_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/review"
	"github.com/russ-p/paseka/internal/taskledger"
)

func TestActivateFinalReviewGateSeedsClosingRound(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	pub := &recordingPublisher{}
	traceID := "trace-closing-seed"
	applyPlan(t, ledger, traceID, "scout", protocol.TaskSpec{TaskID: "task-1", Title: "Build"})
	applyIsolatedProposal(t, ledger, traceID, "task-1")
	completeTaskAt(t, ledger, traceID, "task-1", time.Now().UTC())

	policies := review.GatePolicies{Closing: colony.ClosingPolicy{
		Enabled:   true,
		When:      colony.ClosingWhen{MinWorkTasks: 1, HasIsolatedProposal: true},
		Task:      colony.ClosingTask{Bee: "reviewer", Intent: "closing", Title: "Wrap up"},
		MaxRounds: intPtr(2),
	}}
	res, err := review.EnsureTrailPhases(context.Background(), pub, ledger, colony.Context{}, traceID, policies, review.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.ClosingSeeded == nil || res.ClosingSeeded.TaskID != "closing-1" {
		t.Fatalf("seeded = %+v, want closing-1", res.ClosingSeeded)
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	closing := snap.Tasks["closing-1"]
	if taskledger.TaskPhaseOf(closing) != protocol.TaskPhaseClosing {
		t.Fatalf("phase = %q, want closing", closing.Phase)
	}
	if closing.Status != protocol.TaskStatusReady {
		t.Fatalf("status = %q, want ready", closing.Status)
	}
	if closing.Bee != "reviewer" || closing.Intent != "closing" {
		t.Fatalf("closing = %+v", closing)
	}
	if _, ok := snap.Tasks[taskledger.FinalReviewTaskID]; ok {
		t.Fatal("merge gate must stay closed while the closing round is incomplete")
	}

	var sawPlan bool
	for _, ev := range pub.events {
		if protocol.PayloadKind(ev.Payload) != string(protocol.TaskEventPlan) || ev.AgentID != protocol.RuntimeAgentID {
			continue
		}
		var payload protocol.TaskPlanPayload
		decode(t, ev, &payload)
		for _, spec := range payload.Tasks {
			if spec.TaskID == "closing-1" && spec.Phase == protocol.TaskPhaseClosing {
				sawPlan = true
			}
		}
	}
	if !sawPlan {
		t.Fatalf("published events = %+v, want runtime closing plan", pub.events)
	}
}

func TestClosingRoundNumberingUsesHighestExistingRound(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	traceID := "trace-closing-number"
	base := time.Now().UTC().Add(-time.Hour)
	applyPlanAt(t, ledger, traceID, "scout", base, protocol.TaskSpec{TaskID: "task-1", Title: "Build"})
	applyPlanAt(t, ledger, traceID, protocol.RuntimeAgentID, base.Add(time.Minute), protocol.TaskSpec{TaskID: "closing-3", Title: "Round three", Phase: protocol.TaskPhaseClosing})
	completeTaskAt(t, ledger, traceID, "closing-3", base.Add(2*time.Minute))
	completeTaskAt(t, ledger, traceID, "task-1", base.Add(3*time.Minute))

	policies := review.GatePolicies{Closing: colony.ClosingPolicy{Enabled: true, MaxRounds: intPtr(5)}}
	res, err := review.EnsureTrailPhases(context.Background(), nil, ledger, colony.Context{}, traceID, policies, review.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.ClosingSeeded == nil || res.ClosingSeeded.TaskID != "closing-4" {
		t.Fatalf("seeded = %+v, want closing-4 from the highest existing round", res.ClosingSeeded)
	}
}

func TestNoClosingSeedWhileClosingInFlight(t *testing.T) {
	for _, status := range []protocol.TaskStatus{
		protocol.TaskStatusPlanned,
		protocol.TaskStatusReady,
		protocol.TaskStatusRunning,
		protocol.TaskStatusWaitingReview,
		protocol.TaskStatusBlocked,
	} {
		t.Run(string(status), func(t *testing.T) {
			ledger := taskledger.NewMemoryLedger()
			traceID := "trace-closing-inflight"
			base := time.Now().UTC().Add(-time.Hour)
			applyPlanAt(t, ledger, traceID, "scout", base, protocol.TaskSpec{TaskID: "task-1", Title: "Build"})
			completeTaskAt(t, ledger, traceID, "task-1", base.Add(time.Minute))
			applyPlanAt(t, ledger, traceID, protocol.RuntimeAgentID, base.Add(2*time.Minute), protocol.TaskSpec{TaskID: "closing-1", Title: "Round one", Phase: protocol.TaskPhaseClosing})
			applyStatusAt(t, ledger, traceID, "closing-1", status, base.Add(3*time.Minute))

			policies := review.GatePolicies{Closing: colony.ClosingPolicy{Enabled: true, MaxRounds: intPtr(3)}}
			res, err := review.EnsureTrailPhases(context.Background(), nil, ledger, colony.Context{}, traceID, policies, review.WriteOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if res.ClosingSeeded != nil {
				t.Fatalf("seeded %q while closing-1 is %s", res.ClosingSeeded.TaskID, status)
			}
			snap, err := ledger.Snapshot(traceID)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := snap.Tasks[taskledger.FinalReviewTaskID]; ok {
				t.Fatalf("merge gate opened while closing-1 is %s", status)
			}
		})
	}
}

func TestNoClosingSeedWhenPredicateFails(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	traceID := "trace-closing-predicate"
	base := time.Now().UTC().Add(-time.Hour)
	applyPlanAt(t, ledger, traceID, "scout", base, protocol.TaskSpec{TaskID: "task-1", Title: "Build"})
	completeTaskAt(t, ledger, traceID, "task-1", base.Add(time.Minute))

	policies := review.GatePolicies{Closing: colony.ClosingPolicy{Enabled: true, When: colony.ClosingWhen{MinWorkTasks: 2}, MaxRounds: intPtr(2)}}
	res, err := review.EnsureTrailPhases(context.Background(), nil, ledger, colony.Context{}, traceID, policies, review.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.ClosingSeeded != nil {
		t.Fatalf("seeded %q below min_work_tasks", res.ClosingSeeded.TaskID)
	}
}

func TestNoClosingSeedAfterMergeCompleted(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	traceID := "trace-closing-delivered"
	base := time.Now().UTC().Add(-time.Hour)
	applyPlanAt(t, ledger, traceID, "scout", base, protocol.TaskSpec{TaskID: "task-1", Title: "Build"})
	applyPlanAt(t, ledger, traceID, "scout", base.Add(time.Minute), protocol.TaskSpec{TaskID: "merge", Title: "Merge", Review: protocol.TaskReviewFinal})
	completeTaskAt(t, ledger, traceID, "task-1", base.Add(2*time.Minute))
	completeTaskAt(t, ledger, traceID, "merge", base.Add(3*time.Minute))

	policies := review.GatePolicies{Closing: colony.ClosingPolicy{Enabled: true, MaxRounds: intPtr(3)}}
	res, err := review.EnsureTrailPhases(context.Background(), nil, ledger, colony.Context{}, traceID, policies, review.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.ClosingSeeded != nil {
		t.Fatalf("seeded %q after the merge task completed", res.ClosingSeeded.TaskID)
	}
}

func TestClosingReseedsAfterFollowUpWorkCompletes(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	traceID := "trace-closing-followup"
	base := time.Now().UTC().Add(-time.Hour)
	applyPlanAt(t, ledger, traceID, "scout", base, protocol.TaskSpec{TaskID: "task-1", Title: "Build"})
	completeTaskAt(t, ledger, traceID, "task-1", base.Add(time.Minute))
	applyPlanAt(t, ledger, traceID, protocol.RuntimeAgentID, base.Add(2*time.Minute), protocol.TaskSpec{TaskID: "closing-1", Title: "Round one", Phase: protocol.TaskPhaseClosing})
	completeTaskAt(t, ledger, traceID, "closing-1", base.Add(3*time.Minute))
	applyPlanAt(t, ledger, traceID, "reviewer-1", base.Add(4*time.Minute), protocol.TaskSpec{TaskID: "fix-1", Title: "Fix", Review: protocol.TaskReviewRequired, DependsOn: []string{"closing-1"}})
	completeTaskAt(t, ledger, traceID, "fix-1", base.Add(5*time.Minute))

	policies := review.GatePolicies{Closing: colony.ClosingPolicy{Enabled: true, MaxRounds: intPtr(3)}}
	res, err := review.EnsureTrailPhases(context.Background(), nil, ledger, colony.Context{}, traceID, policies, review.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.ClosingSeeded == nil || res.ClosingSeeded.TaskID != "closing-2" {
		t.Fatalf("seeded = %+v, want closing-2 after follow-up work", res.ClosingSeeded)
	}
}

func TestClosingStopsAtMaxRoundsWithOperationalNote(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	pub := &recordingPublisher{}
	traceID := "trace-closing-cap"
	base := time.Now().UTC().Add(-time.Hour)
	applyPlanAt(t, ledger, traceID, "scout", base, protocol.TaskSpec{TaskID: "task-1", Title: "Build"})
	completeTaskAt(t, ledger, traceID, "task-1", base.Add(time.Minute))
	applyPlanAt(t, ledger, traceID, protocol.RuntimeAgentID, base.Add(2*time.Minute), protocol.TaskSpec{TaskID: "closing-1", Title: "Round one", Phase: protocol.TaskPhaseClosing})
	completeTaskAt(t, ledger, traceID, "closing-1", base.Add(3*time.Minute))
	applyPlanAt(t, ledger, traceID, "reviewer-1", base.Add(4*time.Minute), protocol.TaskSpec{TaskID: "fix-1", Title: "Fix"})
	completeTaskAt(t, ledger, traceID, "fix-1", base.Add(5*time.Minute))

	policies := review.GatePolicies{Closing: colony.ClosingPolicy{Enabled: true, MaxRounds: intPtr(1)}}
	res, err := review.EnsureTrailPhases(context.Background(), pub, ledger, colony.Context{}, traceID, policies, review.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.ClosingSeeded != nil {
		t.Fatalf("seeded %q past max_rounds", res.ClosingSeeded.TaskID)
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := snap.Tasks["closing-2"]; ok {
		t.Fatal("created a second closing round past max_rounds")
	}
	var note string
	for _, ev := range pub.events {
		if protocol.PayloadKind(ev.Payload) != string(protocol.InsightContextNote) {
			continue
		}
		var payload protocol.NarrativeInsightPayload
		decode(t, ev, &payload)
		note = payload.Summary
	}
	if !strings.Contains(note, "max_rounds") && !strings.Contains(note, "allows 1 round") {
		t.Fatalf("operational note = %q, want the cap explained", note)
	}
}

func TestFailedClosingTaskKeepsGateClosed(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	traceID := "trace-closing-failed"
	base := time.Now().UTC().Add(-time.Hour)
	applyPlanAt(t, ledger, traceID, "scout", base, protocol.TaskSpec{TaskID: "task-1", Title: "Build"})
	applyIsolatedProposal(t, ledger, traceID, "task-1")
	completeTaskAt(t, ledger, traceID, "task-1", base.Add(time.Minute))
	applyPlanAt(t, ledger, traceID, protocol.RuntimeAgentID, base.Add(2*time.Minute), protocol.TaskSpec{TaskID: "closing-1", Title: "Round one", Phase: protocol.TaskPhaseClosing})
	completeTaskAtWithStatus(t, ledger, traceID, "closing-1", protocol.TaskStatusFailed, base.Add(3*time.Minute))

	policies := review.GatePolicies{Closing: colony.ClosingPolicy{Enabled: true, MaxRounds: intPtr(3)}}
	if _, err := review.EnsureTrailPhases(context.Background(), nil, ledger, colony.Context{}, traceID, policies, review.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := snap.Tasks["closing-2"]; ok {
		t.Fatal("seeded a replacement round after a failed closing task")
	}
	if _, ok := snap.Tasks[taskledger.FinalReviewTaskID]; ok {
		t.Fatal("merge gate opened over a failed closing task")
	}
}

func TestMergeGateIsEnsuredExactlyOnceWithConfiguredCopy(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	traceID := "trace-gate-copy"
	base := time.Now().UTC().Add(-time.Hour)
	applyPlanAt(t, ledger, traceID, "scout", base, protocol.TaskSpec{TaskID: "task-1", Title: "Build", Intent: "code"})
	applyIsolatedProposal(t, ledger, traceID, "task-1")
	completeTaskAt(t, ledger, traceID, "task-1", base.Add(time.Minute))

	policies := review.GatePolicies{Merge: colony.MergePolicy{Task: colony.MergeTask{
		Title: "Review and merge: {{.TraceID}}",
		Body:  "{{range .Tasks}}- {{.TaskID}} ({{.Intent}})\n{{end}}",
	}}}
	for i := 0; i < 3; i++ {
		if _, err := review.EnsureTrailPhases(context.Background(), nil, ledger, colony.Context{}, traceID, policies, review.WriteOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	mergeCount := 0
	for _, task := range snap.Tasks {
		if taskledger.IsMergeTask(task) {
			mergeCount++
		}
	}
	if mergeCount != 1 {
		t.Fatalf("merge tasks = %d, want 1", mergeCount)
	}
	gate := snap.Tasks[taskledger.FinalReviewTaskID]
	if gate.Status != protocol.TaskStatusWaitingReview {
		t.Fatalf("gate status = %q, want waiting_review", gate.Status)
	}
	if gate.Title != "Review and merge: "+traceID {
		t.Fatalf("gate title = %q", gate.Title)
	}
	if !strings.Contains(gate.Body, "task-1 (code)") {
		t.Fatalf("gate body = %q, want the rendered inventory", gate.Body)
	}
	if taskledger.TaskPhaseOf(gate) != protocol.TaskPhaseMerge {
		t.Fatalf("gate phase = %q, want merge", gate.Phase)
	}
}

func TestExplicitFinalGateSuppressesMergeSynthesis(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	traceID := "trace-gate-explicit"
	base := time.Now().UTC().Add(-time.Hour)
	applyPlanAt(t, ledger, traceID, "scout", base, protocol.TaskSpec{TaskID: "task-1", Title: "Build"})
	applyPlanAt(t, ledger, traceID, "scout", base.Add(time.Minute), protocol.TaskSpec{TaskID: "merge", Title: "Merge", Review: protocol.TaskReviewFinal})
	applyIsolatedProposal(t, ledger, traceID, "task-1")
	completeTaskAt(t, ledger, traceID, "task-1", base.Add(2*time.Minute))

	if _, err := review.EnsureTrailPhases(context.Background(), nil, ledger, colony.Context{}, traceID, review.GatePolicies{}, review.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := review.EnsureTrailPhases(context.Background(), nil, ledger, colony.Context{}, traceID, review.GatePolicies{}, review.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := snap.Tasks[taskledger.FinalReviewTaskID]; ok {
		t.Fatal("runtime added a second merge gate")
	}
	if snap.Tasks["merge"].Status != protocol.TaskStatusWaitingReview {
		t.Fatalf("explicit gate status = %q", snap.Tasks["merge"].Status)
	}
}

func TestClosingSeedPlanEchoAndRestartAreNoOps(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	pub := &recordingPublisher{}
	traceID := "trace-closing-echo"
	base := time.Now().UTC().Add(-time.Hour)
	applyPlanAt(t, ledger, traceID, "scout", base, protocol.TaskSpec{TaskID: "task-1", Title: "Build"})
	completeTaskAt(t, ledger, traceID, "task-1", base.Add(time.Minute))

	policies := review.GatePolicies{Closing: colony.ClosingPolicy{Enabled: true, MaxRounds: intPtr(3)}}
	if _, err := review.EnsureTrailPhases(context.Background(), pub, ledger, colony.Context{}, traceID, policies, review.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	var plan protocol.Event
	for _, ev := range pub.events {
		if protocol.PayloadKind(ev.Payload) == string(protocol.TaskEventPlan) && ev.AgentID == protocol.RuntimeAgentID {
			plan = ev
			break
		}
	}
	if plan.TraceID == "" {
		t.Fatal("no closing plan published")
	}
	// The bus echo of the same task.plan must not change the recorded task.
	if _, err := ledger.Apply(plan); err != nil {
		t.Fatal(err)
	}
	// A restart re-runs ensure on the same ledger: the round is in flight.
	if _, err := review.EnsureTrailPhases(context.Background(), pub, ledger, colony.Context{}, traceID, policies, review.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	closingCount := 0
	for _, task := range snap.Tasks {
		if taskledger.IsClosingTask(task) {
			closingCount++
		}
	}
	if closingCount != 1 {
		t.Fatalf("closing tasks = %d, want 1", closingCount)
	}
	if snap.Tasks["closing-1"].Status != protocol.TaskStatusReady {
		t.Fatalf("closing-1 status = %q, want ready", snap.Tasks["closing-1"].Status)
	}
}

func applyPlan(t *testing.T, ledger taskledger.Ledger, traceID, agentID string, specs ...protocol.TaskSpec) {
	t.Helper()
	applyPlanAt(t, ledger, traceID, agentID, time.Now().UTC(), specs...)
}

func applyPlanAt(t *testing.T, ledger taskledger.Ledger, traceID, agentID string, at time.Time, specs ...protocol.TaskSpec) {
	t.Helper()
	ev, err := protocol.NewEvent(traceID, agentID, 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind:  protocol.TaskEventPlan,
		Tasks: specs,
	})
	if err != nil {
		t.Fatal(err)
	}
	ev.CreatedAt = at
	if _, err := ledger.Apply(ev); err != nil {
		t.Fatal(err)
	}
}

func applyIsolatedProposal(t *testing.T, ledger taskledger.Ledger, traceID, taskID string) {
	t.Helper()
	ev, err := protocol.NewEvent(traceID, "builder-1", 0, protocol.EventMutation, protocol.MutationPayload{
		Kind:      protocol.MutationCodeProposalIsolated,
		TaskID:    taskID,
		Workspace: protocol.ProposalWorkspaceIsolated,
		Diff:      "+line",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(ev); err != nil {
		t.Fatal(err)
	}
}

func completeTaskAt(t *testing.T, ledger taskledger.Ledger, traceID, taskID string, at time.Time) {
	t.Helper()
	completeTaskAtWithStatus(t, ledger, traceID, taskID, protocol.TaskStatusCompleted, at)
}

func completeTaskAtWithStatus(t *testing.T, ledger taskledger.Ledger, traceID, taskID string, status protocol.TaskStatus, at time.Time) {
	t.Helper()
	ev, err := protocol.NewEvent(traceID, protocol.RuntimeAgentID, 0, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind:        protocol.TaskEventCompleted,
		TaskID:      taskID,
		Status:      status,
		CompletedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	ev.CreatedAt = at
	if _, err := ledger.Apply(ev); err != nil {
		t.Fatal(err)
	}
}

func applyStatusAt(t *testing.T, ledger taskledger.Ledger, traceID, taskID string, status protocol.TaskStatus, at time.Time) {
	t.Helper()
	ev, err := protocol.NewEvent(traceID, protocol.RuntimeAgentID, 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind:   protocol.TaskEventStatus,
		TaskID: taskID,
		Status: status,
	})
	if err != nil {
		t.Fatal(err)
	}
	ev.CreatedAt = at
	if _, err := ledger.Apply(ev); err != nil {
		t.Fatal(err)
	}
}

func decode(t *testing.T, ev protocol.Event, out any) {
	t.Helper()
	if err := json.Unmarshal(ev.Payload, out); err != nil {
		t.Fatal(err)
	}
}

func intPtr(v int) *int { return &v }
