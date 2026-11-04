package runtime_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/runtime"
	"github.com/russ-p/paseka/internal/taskledger"
)

func analysisReactionBee() colony.Bee {
	return colony.Bee{
		Role: "analyst",
		Subscribes: []colony.SubscriptionRule{{
			EventRule: colony.EventRule{Type: string(protocol.EventSignal), Kind: protocol.SignalTrailWorkCompleted},
			Dispatch:  colony.DispatchDirect,
			Purpose:   protocol.PurposeAnalysis,
		}},
	}
}

func newAnalysisReactionReactor(t *testing.T, root string, bees map[string]colony.Bee, adapter *recordingAdapter) *runtime.Reactor {
	t.Helper()
	d := runtime.NewDispatcher()
	reg := runtime.NewBeeRegistryFromBees(bees)
	d.SetBeeRegistry(reg)
	d.RegisterAdapter("cursor", adapter)
	pub := &workPublisher{}
	d.SetPublisher(pub, false)
	return runtime.NewTestReactor(runtime.TestReactorOptions{
		ColonyRoot: root,
		Dispatcher: d,
		Registry:   reg,
		Ledger:     taskledger.NewMemoryLedger(),
		Publisher:  pub,
	})
}

func milestoneEvent(t *testing.T, traceID, completionID string) protocol.Event {
	t.Helper()
	ev, err := protocol.NewEvent(traceID, protocol.RuntimeAgentID, 0, protocol.EventSignal, protocol.TrailWorkCompletedPayload{
		Kind:         protocol.SignalTrailWorkCompleted,
		CompletionID: completionID,
		WorkRevision: 1,
		CompletedAt:  time.Now().UTC(),
		ResultRef:    "runs/result.json",
		MetricsRef:   "runs/metrics.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

// TestConfiguredAnalysisReactionRunsOnceAcrossRestart pins the configured
// admission path: a direct subscription to trail.work.completed starts an
// analysis run in the source trail with purpose analysis and the studied
// milestone, and a restart does not dispatch the same milestone twice.
func TestConfiguredAnalysisReactionRunsOnceAcrossRestart(t *testing.T) {
	root := t.TempDir()
	bees := map[string]colony.Bee{"analyst": analysisReactionBee()}
	mustWriteTestColony(t, root, bees)

	traceID := "trace-analysis-reaction"
	completionID := "completion-r1-reaction"
	ev := milestoneEvent(t, traceID, completionID)

	first := &recordingAdapter{}
	r1 := newAnalysisReactionReactor(t, root, bees, first)
	if err := r1.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if first.calls != 1 {
		t.Fatalf("analysis dispatches = %d, want 1", first.calls)
	}
	if first.lastReq.Workspace != root {
		t.Fatalf("analysis workspace = %q, want the colony root %q", first.lastReq.Workspace, root)
	}
	meta, ok, err := runs.FindRun(root, traceID, first.lastReq.AgentID)
	if err != nil || !ok {
		t.Fatalf("find analysis run: ok=%v err=%v", ok, err)
	}
	if meta.Purpose != protocol.PurposeAnalysis || meta.AnalysisOf != completionID {
		t.Fatalf("analysis run purpose = %q analysisOf = %q", meta.Purpose, meta.AnalysisOf)
	}
	admissionID := runs.AnalysisAdmissionID(traceID, completionID, "analyst")
	admission, ok, err := runs.ReadAnalysisAdmission(root, traceID, admissionID)
	if err != nil || !ok {
		t.Fatalf("read analysis admission: ok=%v err=%v", ok, err)
	}
	if admission.Status != runs.AnalysisAdmissionCompleted || admission.AgentID != first.lastReq.AgentID {
		t.Fatalf("analysis admission = %+v", admission)
	}

	// Restart: a fresh reactor with an empty in-memory set must still see the
	// durable admission and leave the duplicate milestone alone.
	second := &recordingAdapter{}
	r2 := newAnalysisReactionReactor(t, root, bees, second)
	if err := r2.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if second.calls != 0 {
		t.Fatalf("restart re-dispatched the same analysis (%d calls)", second.calls)
	}

	// A later completion is a new studied milestone and runs again.
	next := milestoneEvent(t, traceID, "completion-r2-reaction")
	if err := r2.ProcessEvent(context.Background(), next); err != nil {
		t.Fatal(err)
	}
	if second.calls != 1 {
		t.Fatalf("new milestone dispatches = %d, want 1", second.calls)
	}
	nextMeta, ok, err := runs.FindRun(root, traceID, second.lastReq.AgentID)
	if err != nil || !ok {
		t.Fatalf("find second analysis run: ok=%v err=%v", ok, err)
	}
	if nextMeta.AnalysisOf != "completion-r2-reaction" {
		t.Fatalf("second analysis studied %q", nextMeta.AnalysisOf)
	}
}

// TestConfiguredAnalysisReactionFailureStaysAnalysis pins the accounting: a
// failed analysis is visible in the analysis bucket and total, and never turns
// completed work into failed work. The explicit operator action remains the
// retry path.
func TestConfiguredAnalysisReactionFailureStaysAnalysis(t *testing.T) {
	root := t.TempDir()
	bees := map[string]colony.Bee{
		"builder": {Role: "builder"},
		"analyst": analysisReactionBee(),
	}
	mustWriteTestColony(t, root, bees)
	traceID := "trace-analysis-failure"
	completionID := "completion-r1-failure"

	failing := &recordingAdapter{result: &adapters.RunResult{Status: string(protocol.StatusFailed), Summary: "analysis broke"}}
	r := newAnalysisReactionReactor(t, root, bees, failing)
	completionPlan(t, r, traceID, "scout", protocol.TaskSpec{TaskID: "task-1", Bee: "builder"})
	completionDone(t, r, traceID, "builder-agent", "task-1")
	if err := r.ProcessEvent(context.Background(), milestoneEvent(t, traceID, completionID)); err != nil {
		t.Fatal(err)
	}
	if failing.calls != 1 {
		t.Fatalf("analysis dispatches = %d, want 1", failing.calls)
	}

	admissionID := runs.AnalysisAdmissionID(traceID, completionID, "analyst")
	admission, ok, err := runs.ReadAnalysisAdmission(root, traceID, admissionID)
	if err != nil || !ok {
		t.Fatalf("read analysis admission: ok=%v err=%v", ok, err)
	}
	if admission.Status != runs.AnalysisAdmissionFailed {
		t.Fatalf("analysis admission status = %q, want failed", admission.Status)
	}

	metrics, err := runs.BuildTrailMetrics(root, traceID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if metrics.ByPurpose.Analysis.Failed != 1 {
		t.Fatalf("analysis failures = %d, want 1", metrics.ByPurpose.Analysis.Failed)
	}
	if metrics.ByPurpose.Work.Failed != 0 {
		t.Fatalf("failed analysis counted as failed work: %d", metrics.ByPurpose.Work.Failed)
	}
	if metrics.Runs.Failed != 1 {
		t.Fatalf("total failed runs = %d, want the analysis run", metrics.Runs.Failed)
	}
	if metrics.Work.State == nil || *metrics.Work.State != string(protocol.TaskStatusCompleted) {
		t.Fatalf("work state = %v, want completed", metrics.Work.State)
	}

	// The failed reaction stays a single recorded failure: a duplicate delivery
	// does not retry it automatically.
	second := &recordingAdapter{}
	r2 := newAnalysisReactionReactor(t, root, bees, second)
	if err := r2.ProcessEvent(context.Background(), milestoneEvent(t, traceID, completionID)); err != nil {
		t.Fatal(err)
	}
	if second.calls != 0 {
		t.Fatalf("failed analysis was retried automatically (%d calls)", second.calls)
	}
}

// TestAnalysisAdmissionRecordsSurviveWorktreeRemoval keeps the dedupe evidence
// with the trail rather than the removable checkout.
func TestAnalysisAdmissionRecordsSurviveWorktreeRemoval(t *testing.T) {
	root := t.TempDir()
	bees := map[string]colony.Bee{"analyst": analysisReactionBee()}
	mustWriteTestColony(t, root, bees)
	traceID := "trace-analysis-worktree"
	completionID := "completion-r1-worktree"

	adapter := &recordingAdapter{}
	r := newAnalysisReactionReactor(t, root, bees, adapter)
	if err := r.ProcessEvent(context.Background(), milestoneEvent(t, traceID, completionID)); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, ".paseka", "worktrees", traceID)); err != nil {
		t.Fatal(err)
	}
	admissionID := runs.AnalysisAdmissionID(traceID, completionID, "analyst")
	if _, ok, err := runs.ReadAnalysisAdmission(root, traceID, admissionID); err != nil || !ok {
		t.Fatalf("analysis admission lost with the worktree: ok=%v err=%v", ok, err)
	}
}

// TestAnalysisRunPlansAnalysisTasks pins the causal chain: a task planned by an
// analysis run inherits the run's purpose and studied milestone, so the task's
// executions stay in analysis accounting instead of reopening goal work.
func TestAnalysisRunPlansAnalysisTasks(t *testing.T) {
	root := t.TempDir()
	bees := map[string]colony.Bee{
		"builder": {Role: "builder"},
		"analyst": analysisReactionBee(),
	}
	mustWriteTestColony(t, root, bees)
	traceID := "trace-analysis-task-chain"
	completionID := "completion-r1-chain"

	adapter := &recordingAdapter{}
	r := newAnalysisReactionReactor(t, root, bees, adapter)
	completionPlan(t, r, traceID, "scout", protocol.TaskSpec{TaskID: "task-1", Bee: "builder"})
	completionDone(t, r, traceID, "builder-agent", "task-1")
	if err := r.ProcessEvent(context.Background(), milestoneEvent(t, traceID, completionID)); err != nil {
		t.Fatal(err)
	}
	if adapter.calls != 1 {
		t.Fatalf("analysis dispatches = %d, want 1", adapter.calls)
	}

	plan, err := protocol.NewEvent(traceID, adapter.lastReq.AgentID, 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{
			TaskID: "analysis-followup", Title: "read the captured evidence", Bee: "analyst",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	snap, err := r.Ledger().Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	task, ok := snap.Tasks["analysis-followup"]
	if !ok {
		t.Fatal("analysis task was not recorded")
	}
	if task.Purpose != protocol.PurposeAnalysis || task.AnalysisOf != completionID {
		t.Fatalf("analysis task attribution = purpose %q analysisOf %q", task.Purpose, task.AnalysisOf)
	}
	if snap.WorkRevision != 1 {
		t.Fatalf("analysis task advanced the work revision to %d", snap.WorkRevision)
	}
	if verdict := taskledger.EvaluateWorkCompletion(snap); !verdict.Complete {
		t.Fatalf("analysis task held completion: %s", verdict.Reason)
	}
}

// TestOperatorAnalysisRunsAfterWorktreeRemoval pins the operator admission
// path: an explicit analysis run starts in the source trail from the colony
// root, records its purpose and milestone, and never recreates the delivery
// worktree that a merge or reconcile removed.
func TestOperatorAnalysisRunsAfterWorktreeRemoval(t *testing.T) {
	root := t.TempDir()
	mustWriteTestColony(t, root, map[string]colony.Bee{"analyst": {Role: "analyst", Worktree: true}})
	traceID := "trace-operator-analysis"
	completionID := "completion-r1-operator"

	worktreePath := filepath.Join(root, ".paseka", "worktrees", traceID)
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(worktreePath); err != nil {
		t.Fatal(err)
	}

	rec := &recordingAdapter{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)
	res, err := d.DispatchColonyBee(context.Background(), colony.Context{ColonyRoot: root, Slug: "test"}, runtime.ColonyDispatchRequest{
		Bee:        "analyst",
		TraceID:    traceID,
		Task:       "study the retained result",
		Purpose:    protocol.PurposeAnalysis,
		AnalysisOf: completionID,
	}, runtime.DispatchModeCLI)
	if err != nil {
		t.Fatal(err)
	}
	if res.Workspace != root || rec.lastReq.Workspace != root {
		t.Fatalf("analysis workspace = %q / adapter %q, want colony root", res.Workspace, rec.lastReq.Workspace)
	}
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Fatalf("operator analysis recreated the removed worktree: %v", err)
	}
	meta, ok, err := runs.FindRun(root, traceID, res.AgentID)
	if err != nil || !ok {
		t.Fatalf("find operator analysis run: ok=%v err=%v", ok, err)
	}
	if meta.Purpose != protocol.PurposeAnalysis || meta.AnalysisOf != completionID {
		t.Fatalf("operator analysis attribution = purpose %q analysisOf %q", meta.Purpose, meta.AnalysisOf)
	}
}

// TestKilledTrailDoesNotAdmitAnalysis keeps the existing kill control in force
// for the analysis path: a killed trail records no analysis dispatch.
func TestKilledTrailDoesNotAdmitAnalysis(t *testing.T) {
	root := t.TempDir()
	bees := map[string]colony.Bee{"analyst": analysisReactionBee()}
	mustWriteTestColony(t, root, bees)
	traceID := "trace-analysis-killed"
	completionID := "completion-r1-killed"

	adapter := &recordingAdapter{}
	r := newAnalysisReactionReactor(t, root, bees, adapter)
	kill, err := protocol.NewEvent(traceID, "cli", 0, protocol.EventSignal, protocol.SystemKillPayload{
		Kind:   protocol.SignalSystemKill,
		Reason: "operator stop",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), kill); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), milestoneEvent(t, traceID, completionID)); err != nil {
		t.Fatal(err)
	}
	if adapter.calls != 0 {
		t.Fatalf("killed trail admitted %d analysis dispatch(es)", adapter.calls)
	}
	if _, ok, err := runs.ReadAnalysisAdmission(root, traceID, runs.AnalysisAdmissionID(traceID, completionID, "analyst")); err != nil || ok {
		t.Fatalf("killed trail recorded an analysis admission: ok=%v err=%v", ok, err)
	}
}
