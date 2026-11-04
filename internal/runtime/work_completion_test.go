package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/runtime"
	"github.com/russ-p/paseka/internal/taskledger"
)

const workCompletionColony = `defaults:
  prompt_template: default.md
`

const workCompletionClosingColony = `defaults:
  prompt_template: default.md
closing:
  enabled: true
  when:
    min_work_tasks: 1
  task:
    bee: reviewer
    review: required
  max_rounds: 2
`

type workPublisher struct {
	mu        sync.Mutex
	events    []protocol.Event
	failOnce  bool
	failKind  string
	onPublish func(protocol.Event) error
}

func (p *workPublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failOnce && protocol.PayloadKind(ev.Payload) == p.failKind {
		p.failOnce = false
		return errors.New("publish failed")
	}
	if p.onPublish != nil {
		if err := p.onPublish(ev); err != nil {
			return err
		}
	}
	p.events = append(p.events, ev)
	return nil
}

func (p *workPublisher) published(kind string) []protocol.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []protocol.Event
	for _, ev := range p.events {
		if protocol.PayloadKind(ev.Payload) == kind {
			out = append(out, ev)
		}
	}
	return out
}

func (p *workPublisher) count(kind string) int {
	return len(p.published(kind))
}

func newWorkCompletionReactor(t *testing.T, colonyYAML string, bees map[string]colony.Bee) (*runtime.Reactor, *workPublisher, *recordingAdapter) {
	t.Helper()
	root := t.TempDir()
	mustWriteTestColony(t, root, bees)
	if err := os.WriteFile(filepath.Join(root, ".paseka/colony.yaml"), []byte(colonyYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	d := runtime.NewDispatcher()
	reg := runtime.NewBeeRegistryFromBees(bees)
	d.SetBeeRegistry(reg)
	adapter := &recordingAdapter{}
	d.RegisterAdapter("cursor", adapter)
	pub := &workPublisher{}
	d.SetPublisher(pub, false)
	return runtime.NewTestReactor(runtime.TestReactorOptions{
		ColonyRoot: root,
		Dispatcher: d,
		Registry:   reg,
		Ledger:     taskledger.NewMemoryLedger(),
		Publisher:  pub,
	}), pub, adapter
}

func completionPlan(t *testing.T, r *runtime.Reactor, traceID, agentID string, specs ...protocol.TaskSpec) {
	t.Helper()
	ev, err := protocol.NewEvent(traceID, agentID, 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind:  protocol.TaskEventPlan,
		Tasks: specs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
}

func completionStatus(t *testing.T, r *runtime.Reactor, traceID, taskID string, status protocol.TaskStatus) {
	t.Helper()
	ev, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind:   protocol.TaskEventStatus,
		TaskID: taskID,
		Status: status,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
}

func completionDone(t *testing.T, r *runtime.Reactor, traceID, agentID, taskID string) {
	t.Helper()
	ev, err := protocol.NewEvent(traceID, agentID, 0, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind:        protocol.TaskEventCompleted,
		TaskID:      taskID,
		Status:      protocol.TaskStatusCompleted,
		CompletedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
}

func decodeCompletion(t *testing.T, ev protocol.Event) protocol.TrailWorkCompletedPayload {
	t.Helper()
	var payload protocol.TrailWorkCompletedPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestWorkCompletionFiresWhenWorkAndChecksAreDone(t *testing.T) {
	r, pub, _ := newWorkCompletionReactor(t, workCompletionColony, map[string]colony.Bee{
		"builder": {Role: "builder"},
	})
	traceID := "trace-work-complete"
	// The milestone must never arrive before its evidence is readable.
	pub.onPublish = func(ev protocol.Event) error {
		if protocol.PayloadKind(ev.Payload) != protocol.SignalTrailWorkCompleted {
			return nil
		}
		payload := decodeCompletion(t, ev)
		for _, ref := range []string{payload.ResultRef, payload.MetricsRef} {
			path, err := runs.ResolveTrailRef(r.ColonyRoot(), ref)
			if err != nil {
				return err
			}
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("evidence %q missing at publication: %w", ref, err)
			}
		}
		return nil
	}
	completionPlan(t, r, traceID, "scout", protocol.TaskSpec{TaskID: "task-1", Title: "No-code result", Bee: "builder"})
	completionDone(t, r, traceID, "builder-agent", "task-1")

	signals := pub.published(protocol.SignalTrailWorkCompleted)
	if len(signals) != 1 {
		t.Fatalf("completion signals = %d, want 1", len(signals))
	}
	payload := decodeCompletion(t, signals[0])
	if payload.WorkRevision != 1 || payload.CompletionID == "" {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.CompletedAt.IsZero() {
		t.Fatal("completedAt is zero")
	}
	for _, ref := range []string{payload.ResultRef, payload.MetricsRef} {
		path, err := runs.ResolveTrailRef(r.ColonyRoot(), ref)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("evidence %q does not resolve: %v", ref, err)
		}
	}
	if filepath.Base(payload.MetricsRef) != runs.TrailMetricsFileName {
		t.Fatalf("metricsRef = %q, want a metrics snapshot file", payload.MetricsRef)
	}
	snapshot, ok, err := runs.ReadWorkCompletionMetrics(r.ColonyRoot(), traceID, payload.CompletionID)
	if err != nil || !ok {
		t.Fatalf("metrics snapshot: ok=%v err=%v", ok, err)
	}
	if snapshot.Work.Revision == nil || *snapshot.Work.Revision != 1 {
		t.Fatalf("snapshot revision = %v", snapshot.Work.Revision)
	}
	if snapshot.Work.Completion == nil || snapshot.Work.Completion.CompletionID != payload.CompletionID {
		t.Fatalf("snapshot completion = %+v", snapshot.Work.Completion)
	}
	checkpoint, ok, err := runs.ReadWorkCompletionCheckpoint(r.ColonyRoot(), traceID, payload.CompletionID)
	if err != nil || !ok {
		t.Fatalf("checkpoint: ok=%v err=%v", ok, err)
	}
	if checkpoint.ResultRef != payload.ResultRef || checkpoint.MetricsRef != payload.MetricsRef {
		t.Fatalf("checkpoint refs = %q / %q", checkpoint.ResultRef, checkpoint.MetricsRef)
	}
}

func TestWorkCompletionAllowsWaitingMergeGate(t *testing.T) {
	r, pub, _ := newWorkCompletionReactor(t, workCompletionColony, map[string]colony.Bee{
		"builder": {Role: "builder"},
	})
	traceID := "trace-work-merge-waiting"
	completionPlan(t, r, traceID, "scout", protocol.TaskSpec{TaskID: "task-1", Title: "Code result", Bee: "builder"})
	proposal, err := protocol.NewEvent(traceID, "builder-agent", 0, protocol.EventMutation, protocol.MutationPayload{
		Kind:      protocol.MutationCodeProposalIsolated,
		TaskID:    "task-1",
		Workspace: protocol.ProposalWorkspaceIsolated,
		Diff:      "+line",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), proposal); err != nil {
		t.Fatal(err)
	}
	completionDone(t, r, traceID, "builder-agent", "task-1")

	snap, err := r.Ledger().Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	gate := snap.Tasks[taskledger.FinalReviewTaskID]
	if gate.Status != protocol.TaskStatusWaitingReview {
		t.Fatalf("merge gate = %q, want waiting_review", gate.Status)
	}
	if pub.count(protocol.SignalTrailWorkCompleted) != 1 {
		t.Fatal("a waiting merge gate blocked the work-completion milestone")
	}
}

func TestWorkCompletionHoldsForReviewRequiredWorkGate(t *testing.T) {
	r, pub, _ := newWorkCompletionReactor(t, workCompletionColony, map[string]colony.Bee{
		"builder": {Role: "builder"},
	})
	traceID := "trace-work-review-gate"
	completionPlan(t, r, traceID, "scout", protocol.TaskSpec{
		TaskID: "task-1", Title: "Needs review", Bee: "builder", Review: protocol.TaskReviewRequired,
	})
	completionStatus(t, r, traceID, "task-1", protocol.TaskStatusWaitingReview)
	if pub.count(protocol.SignalTrailWorkCompleted) != 0 {
		t.Fatal("a waiting work gate emitted a completion milestone")
	}

	completionDone(t, r, traceID, "reviewer", "task-1")
	if pub.count(protocol.SignalTrailWorkCompleted) != 1 {
		t.Fatal("completion did not fire after the work gate completed")
	}
}

func TestWorkCompletionWaitsForClosingRound(t *testing.T) {
	r, pub, _ := newWorkCompletionReactor(t, workCompletionClosingColony, map[string]colony.Bee{
		"builder":  {Role: "builder"},
		"reviewer": {Role: "reviewer"},
	})
	traceID := "trace-work-closing"
	completionPlan(t, r, traceID, "scout", protocol.TaskSpec{TaskID: "task-1", Title: "Work", Bee: "builder"})
	completionDone(t, r, traceID, "builder-agent", "task-1")

	snap, err := r.Ledger().Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	closing := snap.Tasks["closing-1"]
	if closing.Status != protocol.TaskStatusWaitingReview {
		t.Fatalf("closing-1 = %q, want the round seeded and in flight", closing.Status)
	}
	if pub.count(protocol.SignalTrailWorkCompleted) != 0 {
		t.Fatal("an in-flight closing round did not hold the milestone")
	}

	completionDone(t, r, traceID, "reviewer-agent", "closing-1")
	if pub.count(protocol.SignalTrailWorkCompleted) != 1 {
		t.Fatal("the milestone did not fire when the closing round completed")
	}
	snap, err = r.Ledger().Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	for id, task := range snap.Tasks {
		if taskledger.IsClosingTask(task) && id != "closing-1" {
			t.Fatalf("the milestone seeded another closing round %q", id)
		}
	}
	if _, err := os.Stat(filepath.Join(r.ColonyRoot(), ".paseka/bees/reviewer.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestWorkCompletionFiresAtWorkCompletionWhenClosingCapIsReached(t *testing.T) {
	colonyYAML := `defaults:
  prompt_template: default.md
closing:
  enabled: true
  when:
    min_work_tasks: 1
  task:
    bee: reviewer
    review: required
  max_rounds: 1
`
	r, pub, _ := newWorkCompletionReactor(t, colonyYAML, map[string]colony.Bee{
		"builder":  {Role: "builder"},
		"reviewer": {Role: "reviewer"},
	})
	traceID := "trace-work-cap"
	completionPlan(t, r, traceID, "scout", protocol.TaskSpec{TaskID: "task-1", Bee: "builder"})
	completionDone(t, r, traceID, "builder-agent", "task-1")
	completionDone(t, r, traceID, "reviewer-agent", "closing-1")

	completionPlan(t, r, traceID, "scout", protocol.TaskSpec{TaskID: "task-2", Bee: "builder"})
	completionDone(t, r, traceID, "builder-agent", "task-2")

	snap, err := r.Ledger().Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := snap.Tasks["closing-2"]; ok {
		t.Fatal("a new round was seeded past the cap")
	}
	signals := pub.published(protocol.SignalTrailWorkCompleted)
	if len(signals) != 2 {
		t.Fatalf("signals = %d, want one per accepted revision", len(signals))
	}
	first := decodeCompletion(t, signals[0])
	second := decodeCompletion(t, signals[1])
	if second.WorkRevision != 2 || second.CompletionID == first.CompletionID {
		t.Fatalf("second completion = %+v (first %+v)", second, first)
	}
	if _, err := os.Stat(runs.WorkCompletionCheckpointPath(r.ColonyRoot(), traceID, first.CompletionID)); err != nil {
		t.Fatalf("the older completion was not kept historical: %v", err)
	}
}

func TestWorkCompletionHoldsForDeferredWork(t *testing.T) {
	r, pub, _ := newWorkCompletionReactor(t, workCompletionColony, map[string]colony.Bee{
		"builder": {Role: "builder"},
	})
	traceID := "trace-work-deferred"
	if err := r.Ledger().SeedEnergy(traceID, 1); err != nil {
		t.Fatal(err)
	}
	completionPlan(t, r, traceID, "scout",
		protocol.TaskSpec{TaskID: "task-1", Bee: "builder"},
		protocol.TaskSpec{TaskID: "task-2", Bee: "builder"},
	)
	consume, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventSignal, protocol.EnergyConsumePayload{
		Kind: protocol.SignalEnergyConsume, Amount: 1, Reason: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), consume); err != nil {
		t.Fatal(err)
	}
	ready, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind: protocol.TaskEventReady, TaskID: "task-2", Bee: "builder",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ready); err != nil {
		t.Fatal(err)
	}

	completionDone(t, r, traceID, "builder-agent", "task-1")
	if pub.count(protocol.SignalTrailWorkCompleted) != 0 {
		t.Fatal("energy-deferred work emitted a success milestone")
	}
}

func TestWorkCompletionDoesNotFireForFailedCancelledOrSkippedWork(t *testing.T) {
	cases := []struct {
		name   string
		status protocol.TaskStatus
	}{
		{name: "failed", status: protocol.TaskStatusFailed},
		{name: "cancelled", status: protocol.TaskStatusCancelled},
		{name: "skipped", status: protocol.TaskStatusSkipped},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, pub, _ := newWorkCompletionReactor(t, workCompletionColony, map[string]colony.Bee{
				"builder": {Role: "builder"},
			})
			traceID := "trace-work-" + tc.name
			completionPlan(t, r, traceID, "scout", protocol.TaskSpec{TaskID: "task-1", Bee: "builder"})
			completionStatus(t, r, traceID, "task-1", tc.status)
			if pub.count(protocol.SignalTrailWorkCompleted) != 0 {
				t.Fatalf("%s work emitted a success milestone", tc.name)
			}
		})
	}
}

func TestWorkCompletionIgnoresAnalysisActivity(t *testing.T) {
	r, pub, _ := newWorkCompletionReactor(t, workCompletionColony, map[string]colony.Bee{
		"analyst": {Role: "analyst"},
	})
	traceID := "trace-work-analysis"
	completionPlan(t, r, traceID, "scout", protocol.TaskSpec{TaskID: "task-1", Bee: "builder"})
	// A deferred analysis reaction studies an already-recorded result and must
	// not hold the work milestone.
	added, err := r.Ledger().DeferPendingDispatch(traceID, taskledger.PendingDispatch{
		ID: "analysis-reaction", Kind: taskledger.PendingDispatchDirect,
		Status: taskledger.PendingDispatchPending, Purpose: protocol.PurposeAnalysis,
		AnalysisOf: "completion-pending",
	})
	if err != nil || !added {
		t.Fatalf("defer analysis: added=%v err=%v", added, err)
	}
	completionDone(t, r, traceID, "builder-agent", "task-1")
	if pub.count(protocol.SignalTrailWorkCompleted) != 1 {
		t.Fatal("analysis work held the work-completion milestone")
	}

	// Analysis notes after completion neither reopen the revision nor announce
	// another completion.
	note, err := protocol.NewEvent(traceID, "analyst-agent", 0, protocol.EventInsight, protocol.NarrativeInsightPayload{
		Kind:    protocol.InsightReviewNote,
		Summary: "retrospective finding",
		TaskID:  "task-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), note); err != nil {
		t.Fatal(err)
	}
	snap, err := r.Ledger().Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.WorkRevision != 1 {
		t.Fatalf("analysis advanced the work revision to %d", snap.WorkRevision)
	}
	if pub.count(protocol.SignalTrailWorkCompleted) != 1 {
		t.Fatal("analysis activity produced another completion")
	}
}

func TestWorkCompletionEmptyLedgerEmitsNothing(t *testing.T) {
	r, pub, _ := newWorkCompletionReactor(t, workCompletionColony, map[string]colony.Bee{
		"builder": {Role: "builder"},
	})
	if err := r.CheckWorkCompletion(context.Background(), "trace-empty"); err != nil {
		t.Fatal(err)
	}
	if pub.count(protocol.SignalTrailWorkCompleted) != 0 {
		t.Fatal("an empty ledger emitted a completion milestone")
	}
}

func TestWorkCompletionDoesNotFireForStandingTrail(t *testing.T) {
	r, pub, _ := newWorkCompletionReactor(t, workCompletionColony, map[string]colony.Bee{
		"builder": {Role: "builder"},
	})
	traceID := "trace-standing-work"
	cueDir := filepath.Join(r.ColonyRoot(), ".paseka", "cues")
	if err := os.MkdirAll(cueDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cue := `emit: task
bee: builder
intent: triage
standing:
  trace: ` + traceID + `
  stipend: 5
`
	if err := os.WriteFile(filepath.Join(cueDir, "standing-tick.yaml"), []byte(cue), 0o644); err != nil {
		t.Fatal(err)
	}

	completionPlan(t, r, traceID, "scout", protocol.TaskSpec{TaskID: "task-1", Bee: "builder"})
	completionDone(t, r, traceID, "builder-agent", "task-1")
	if pub.count(protocol.SignalTrailWorkCompleted) != 0 {
		t.Fatal("a standing tick emitted a success milestone")
	}
}

func TestWorkCompletionReopensRevisionForNewWork(t *testing.T) {
	r, pub, _ := newWorkCompletionReactor(t, workCompletionColony, map[string]colony.Bee{
		"builder": {Role: "builder"},
	})
	traceID := "trace-work-reopen"
	completionPlan(t, r, traceID, "scout", protocol.TaskSpec{TaskID: "task-1", Bee: "builder"})
	completionDone(t, r, traceID, "builder-agent", "task-1")

	completionPlan(t, r, traceID, "scout", protocol.TaskSpec{TaskID: "task-2", Bee: "builder"})
	snap, err := r.Ledger().Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.WorkRevision != 2 {
		t.Fatalf("revision after new work = %d, want 2", snap.WorkRevision)
	}
	if pub.count(protocol.SignalTrailWorkCompleted) != 1 {
		t.Fatal("new work admission announced a completion before it was complete")
	}

	completionDone(t, r, traceID, "builder-agent", "task-2")
	signals := pub.published(protocol.SignalTrailWorkCompleted)
	if len(signals) != 2 {
		t.Fatalf("signals = %d, want a new completion for the new revision", len(signals))
	}
	first := decodeCompletion(t, signals[0])
	second := decodeCompletion(t, signals[1])
	if first.CompletionID == second.CompletionID {
		t.Fatal("a new revision reused the previous completion identity")
	}
}

func TestWorkCompletionPublicationRetriesWithSameIdentity(t *testing.T) {
	r, pub, _ := newWorkCompletionReactor(t, workCompletionColony, map[string]colony.Bee{
		"builder": {Role: "builder"},
	})
	traceID := "trace-work-retry"
	pub.failOnce = true
	pub.failKind = protocol.SignalTrailWorkCompleted
	completionPlan(t, r, traceID, "scout", protocol.TaskSpec{TaskID: "task-1", Bee: "builder"})
	completionDone(t, r, traceID, "builder-agent", "task-1")

	if pub.count(protocol.SignalTrailWorkCompleted) != 0 {
		t.Fatal("the first publication was supposed to fail")
	}
	snap, err := r.Ledger().Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.WorkCompletion == nil || snap.WorkCompletion.Published {
		t.Fatalf("recorded completion = %+v, want an unpublished retry record", snap.WorkCompletion)
	}
	completionID := snap.WorkCompletion.CompletionID

	if err := r.CheckWorkCompletion(context.Background(), traceID); err != nil {
		t.Fatal(err)
	}
	signals := pub.published(protocol.SignalTrailWorkCompleted)
	if len(signals) != 1 {
		t.Fatalf("signals after retry = %d", len(signals))
	}
	if got := decodeCompletion(t, signals[0]).CompletionID; got != completionID {
		t.Fatalf("retry identity = %q, want %q", got, completionID)
	}
}

func TestWorkCompletionRestartRecoversPendingPublication(t *testing.T) {
	r, pub, _ := newWorkCompletionReactor(t, workCompletionColony, map[string]colony.Bee{
		"builder": {Role: "builder"},
	})
	traceID := "trace-work-restart"
	pub.failOnce = true
	pub.failKind = protocol.SignalTrailWorkCompleted
	completionPlan(t, r, traceID, "scout", protocol.TaskSpec{TaskID: "task-1", Bee: "builder"})
	completionDone(t, r, traceID, "builder-agent", "task-1")
	snap, err := r.Ledger().Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.WorkCompletion == nil {
		t.Fatal("no completion recorded before the simulated restart")
	}
	completionID := snap.WorkCompletion.CompletionID
	checkpointBefore, ok, err := runs.ReadWorkCompletionCheckpoint(r.ColonyRoot(), traceID, completionID)
	if err != nil || !ok {
		t.Fatalf("checkpoint before restart: ok=%v err=%v", ok, err)
	}

	restarted := runtime.NewTestReactor(runtime.TestReactorOptions{
		ColonyRoot: r.ColonyRoot(),
		Registry:   r.Registry(),
		Ledger:     r.Ledger(),
		Publisher:  pub,
	})
	if err := restarted.RecoverPendingWorkCompletions(context.Background()); err != nil {
		t.Fatal(err)
	}
	signals := pub.published(protocol.SignalTrailWorkCompleted)
	if len(signals) != 1 {
		t.Fatalf("signals after restart recovery = %d", len(signals))
	}
	payload := decodeCompletion(t, signals[0])
	if payload.CompletionID != completionID {
		t.Fatalf("restart identity = %q, want %q", payload.CompletionID, completionID)
	}
	if payload.ResultRef != checkpointBefore.ResultRef || payload.MetricsRef != checkpointBefore.MetricsRef {
		t.Fatalf("restart rewrote the evidence references: payload=%+v checkpoint=%+v", payload, checkpointBefore)
	}
	for _, ref := range []string{payload.ResultRef, payload.MetricsRef} {
		path, err := runs.ResolveTrailRef(r.ColonyRoot(), ref)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("restart evidence %q does not resolve: %v", ref, err)
		}
	}
}

func TestWorkCompletionDuplicateDeliveryKeepsIdentity(t *testing.T) {
	r, pub, _ := newWorkCompletionReactor(t, workCompletionColony, map[string]colony.Bee{
		"builder": {Role: "builder"},
	})
	traceID := "trace-work-duplicate"
	completionPlan(t, r, traceID, "scout", protocol.TaskSpec{TaskID: "task-1", Bee: "builder"})
	completionDone(t, r, traceID, "builder-agent", "task-1")
	signals := pub.published(protocol.SignalTrailWorkCompleted)
	if len(signals) != 1 {
		t.Fatalf("signals = %d, want 1", len(signals))
	}
	payload := decodeCompletion(t, signals[0])
	checkpointBefore, ok, err := runs.ReadWorkCompletionCheckpoint(r.ColonyRoot(), traceID, payload.CompletionID)
	if err != nil || !ok {
		t.Fatalf("checkpoint: ok=%v err=%v", ok, err)
	}
	resultBefore, err := os.ReadFile(runs.WorkCompletionResultPath(r.ColonyRoot(), traceID, payload.CompletionID))
	if err != nil {
		t.Fatal(err)
	}

	// Bus delivery is at least once. A redelivered milestone must resolve to
	// the same recorded identity and must not announce itself twice.
	duplicate, err := protocol.NewEvent(traceID, protocol.RuntimeAgentID, 0, protocol.EventSignal, protocol.TrailWorkCompletedPayload{
		Kind:         payload.Kind,
		CompletionID: payload.CompletionID,
		WorkRevision: payload.WorkRevision,
		CompletedAt:  payload.CompletedAt,
		ResultRef:    payload.ResultRef,
		MetricsRef:   payload.MetricsRef,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), duplicate); err != nil {
		t.Fatal(err)
	}

	if got := pub.count(protocol.SignalTrailWorkCompleted); got != 1 {
		t.Fatalf("duplicate delivery announced the completion again (%d signals)", got)
	}
	snap, err := r.Ledger().Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.WorkCompletion == nil || snap.WorkCompletion.CompletionID != payload.CompletionID {
		t.Fatalf("recorded completion = %+v", snap.WorkCompletion)
	}
	if snap.WorkCompletion.ResultRef != payload.ResultRef || snap.WorkCompletion.MetricsRef != payload.MetricsRef {
		t.Fatalf("duplicate delivery rewrote the references: %+v", snap.WorkCompletion)
	}
	checkpointAfter, ok, err := runs.ReadWorkCompletionCheckpoint(r.ColonyRoot(), traceID, payload.CompletionID)
	if err != nil || !ok {
		t.Fatalf("checkpoint after duplicate: ok=%v err=%v", ok, err)
	}
	if !checkpointAfter.CompletedAt.Equal(checkpointBefore.CompletedAt) ||
		checkpointAfter.ResultRef != checkpointBefore.ResultRef ||
		checkpointAfter.MetricsRef != checkpointBefore.MetricsRef {
		t.Fatalf("duplicate delivery rewrote the checkpoint: before=%+v after=%+v", checkpointBefore, checkpointAfter)
	}
	resultAfter, err := os.ReadFile(runs.WorkCompletionResultPath(r.ColonyRoot(), traceID, payload.CompletionID))
	if err != nil {
		t.Fatal(err)
	}
	if string(resultBefore) != string(resultAfter) {
		t.Fatal("duplicate delivery rewrote the immutable result evidence")
	}
}

func TestWorkCompletionOldEvidenceStaysResolvableAfterNewWork(t *testing.T) {
	r, pub, _ := newWorkCompletionReactor(t, workCompletionColony, map[string]colony.Bee{
		"builder": {Role: "builder"},
	})
	traceID := "trace-work-history"
	completionPlan(t, r, traceID, "scout", protocol.TaskSpec{TaskID: "task-1", Bee: "builder"})
	completionDone(t, r, traceID, "builder-agent", "task-1")
	first := decodeCompletion(t, pub.published(protocol.SignalTrailWorkCompleted)[0])
	firstMetrics, err := os.ReadFile(runs.WorkCompletionMetricsPath(r.ColonyRoot(), traceID, first.CompletionID))
	if err != nil {
		t.Fatal(err)
	}

	// A later accepted revision gets its own completion; the older one stays
	// historical and still resolves to its original result and metrics.
	completionPlan(t, r, traceID, "scout", protocol.TaskSpec{TaskID: "task-2", Bee: "builder"})
	completionDone(t, r, traceID, "builder-agent", "task-2")
	signals := pub.published(protocol.SignalTrailWorkCompleted)
	if len(signals) != 2 {
		t.Fatalf("signals = %d, want 2", len(signals))
	}
	second := decodeCompletion(t, signals[1])
	if second.CompletionID == first.CompletionID || second.WorkRevision != 2 {
		t.Fatalf("second completion = %+v (first %+v)", second, first)
	}

	for _, ref := range []string{first.ResultRef, first.MetricsRef} {
		path, err := runs.ResolveTrailRef(r.ColonyRoot(), ref)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("older completion evidence %q no longer resolves: %v", ref, err)
		}
	}
	oldSnapshot, ok, err := runs.ReadWorkCompletionMetrics(r.ColonyRoot(), traceID, first.CompletionID)
	if err != nil || !ok {
		t.Fatalf("old metrics snapshot: ok=%v err=%v", ok, err)
	}
	if oldSnapshot.Work.Revision == nil || *oldSnapshot.Work.Revision != 1 {
		t.Fatalf("old snapshot revision = %v, want 1", oldSnapshot.Work.Revision)
	}
	if oldSnapshot.Work.Completion == nil || oldSnapshot.Work.Completion.CompletionID != first.CompletionID {
		t.Fatalf("old snapshot completion = %+v", oldSnapshot.Work.Completion)
	}
	afterMetrics, err := os.ReadFile(runs.WorkCompletionMetricsPath(r.ColonyRoot(), traceID, first.CompletionID))
	if err != nil {
		t.Fatal(err)
	}
	if string(firstMetrics) != string(afterMetrics) {
		t.Fatal("new accepted work rewrote the older completion snapshot")
	}
	record, ok, err := runs.ReadTrailWorkRecord(r.ColonyRoot(), traceID)
	if err != nil || !ok {
		t.Fatalf("work record: ok=%v err=%v", ok, err)
	}
	if record.Completion == nil || record.Completion.CompletionID != second.CompletionID {
		t.Fatalf("work record current completion = %+v", record.Completion)
	}
}

func TestWorkCompletionIgnoresAcceptanceAndNarrativeEvents(t *testing.T) {
	r, pub, _ := newWorkCompletionReactor(t, workCompletionColony, map[string]colony.Bee{
		"builder": {Role: "builder"},
	})
	traceID := "trace-work-narrative"
	completionPlan(t, r, traceID, "scout", protocol.TaskSpec{TaskID: "task-1", Bee: "builder"})
	completionDone(t, r, traceID, "builder-agent", "task-1")
	if pub.count(protocol.SignalTrailWorkCompleted) != 1 {
		t.Fatal("initial completion missing")
	}

	note, err := protocol.NewEvent(traceID, "builder-agent", 0, protocol.EventInsight, protocol.NarrativeInsightPayload{
		Kind: protocol.InsightReviewNote, Summary: "late note",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), note); err != nil {
		t.Fatal(err)
	}
	// Final approval completes the merge task, which is acceptance, not work.
	mergePlan, err := protocol.NewEvent(traceID, protocol.RuntimeAgentID, 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind:  protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{TaskID: "_review", Review: protocol.TaskReviewFinal, Phase: protocol.TaskPhaseMerge}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), mergePlan); err != nil {
		t.Fatal(err)
	}
	completionDone(t, r, traceID, "human", "_review")

	snap, err := r.Ledger().Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.WorkRevision != 1 {
		t.Fatalf("revision after acceptance = %d, want 1", snap.WorkRevision)
	}
	if pub.count(protocol.SignalTrailWorkCompleted) != 1 {
		t.Fatal("acceptance or narrative events produced another completion")
	}
}

func TestWorkCompletionRecordsEvidenceWithoutPublisher(t *testing.T) {
	root := t.TempDir()
	mustWriteTestColony(t, root, map[string]colony.Bee{"builder": {Role: "builder"}})
	if err := os.WriteFile(filepath.Join(root, ".paseka/colony.yaml"), []byte(workCompletionColony), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := runtime.NewBeeRegistryFromBees(map[string]colony.Bee{"builder": {Role: "builder"}})
	d := runtime.NewDispatcher()
	d.SetBeeRegistry(reg)
	d.RegisterAdapter("cursor", &recordingAdapter{})
	r := runtime.NewTestReactor(runtime.TestReactorOptions{
		ColonyRoot: root,
		Dispatcher: d,
		Registry:   reg,
		Ledger:     taskledger.NewMemoryLedger(),
	})
	traceID := "trace-work-no-publisher"
	completionPlan(t, r, traceID, "scout", protocol.TaskSpec{TaskID: "task-1", Bee: "builder"})
	completionDone(t, r, traceID, "builder-agent", "task-1")

	snap, err := r.Ledger().Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.WorkCompletion == nil || snap.WorkCompletion.Published {
		t.Fatalf("completion = %+v, want a durable unpublished record", snap.WorkCompletion)
	}
	checkpoint, ok, err := runs.ReadWorkCompletionCheckpoint(root, traceID, snap.WorkCompletion.CompletionID)
	if err != nil || !ok {
		t.Fatalf("checkpoint without a publisher: ok=%v err=%v", ok, err)
	}

	// Once a channel exists, startup republishes the recorded identity.
	pub := &workPublisher{}
	restarted := runtime.NewTestReactor(runtime.TestReactorOptions{
		ColonyRoot: root,
		Registry:   reg,
		Ledger:     r.Ledger(),
		Publisher:  pub,
	})
	if err := restarted.RecoverPendingWorkCompletions(context.Background()); err != nil {
		t.Fatal(err)
	}
	signals := pub.published(protocol.SignalTrailWorkCompleted)
	if len(signals) != 1 {
		t.Fatalf("signals after publisher recovery = %d", len(signals))
	}
	payload := decodeCompletion(t, signals[0])
	if payload.CompletionID != checkpoint.CompletionID {
		t.Fatalf("recovered identity = %q, want %q", payload.CompletionID, checkpoint.CompletionID)
	}
}

// blockingAdapter signals when the run starts and waits until released, so a
// test can complete the last task while its dispatcher is still in flight.
type blockingAdapter struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (a *blockingAdapter) Name() string { return "cursor" }

func (a *blockingAdapter) Run(_ context.Context, _ adapters.RunRequest) (*adapters.RunResult, error) {
	a.once.Do(func() { close(a.started) })
	<-a.release
	return &adapters.RunResult{Status: "completed", Output: "ok"}, nil
}

func TestWorkCompletionWaitsForInFlightDispatcher(t *testing.T) {
	root := t.TempDir()
	mustWriteTestColony(t, root, map[string]colony.Bee{"builder": {Role: "builder"}})
	if err := os.WriteFile(filepath.Join(root, ".paseka/colony.yaml"), []byte(workCompletionColony), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := &blockingAdapter{started: make(chan struct{}), release: make(chan struct{})}
	reg := runtime.NewBeeRegistryFromBees(map[string]colony.Bee{"builder": {Role: "builder"}})
	d := runtime.NewDispatcher()
	d.SetBeeRegistry(reg)
	d.RegisterAdapter("cursor", adapter)
	pub := &workPublisher{}
	d.SetPublisher(pub, false)
	r := runtime.NewTestReactor(runtime.TestReactorOptions{
		ColonyRoot:    root,
		Dispatcher:    d,
		Registry:      reg,
		Ledger:        taskledger.NewMemoryLedger(),
		Publisher:     pub,
		AsyncDispatch: true,
	})
	traceID := "trace-work-inflight"
	completionPlan(t, r, traceID, "scout", protocol.TaskSpec{TaskID: "task-1", Bee: "builder"})
	ready, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind: protocol.TaskEventReady, TaskID: "task-1", Bee: "builder",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ready); err != nil {
		t.Fatal(err)
	}

	select {
	case <-adapter.started:
	case <-time.After(5 * time.Second):
		t.Fatal("adapter did not start")
	}

	// The last task completes while its dispatcher is still in flight: the
	// ledger says completed, but the invocation owns pending finalization.
	completionDone(t, r, traceID, "builder-agent", "task-1")
	if got := pub.count(protocol.SignalTrailWorkCompleted); got != 0 {
		t.Fatalf("completion fired with the dispatcher in flight (%d signals)", got)
	}

	close(adapter.release)
	deadline := time.After(5 * time.Second)
	for pub.count(protocol.SignalTrailWorkCompleted) != 1 {
		select {
		case <-deadline:
			t.Fatal("leaving the in-flight set did not trigger the completion check")
		case <-time.After(10 * time.Millisecond):
		}
	}

	// The completion announcement is written before the dispatch goroutine's
	// final retained-work write. Wait for that record to settle before TempDir
	// cleanup can race the trailing writer.
	waitForTrailWorkSettled(t, root, traceID)
}

// waitForTrailWorkSettled polls until the retained work record has carried a
// completion and its file has stopped changing. The async dispatch goroutine
// writes it twice (accepted revision, then completion), so existence alone is
// not enough for a clean TempDir teardown.
func waitForTrailWorkSettled(t *testing.T, root, traceID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var lastMod time.Time
	var lastSize int64
	stableSince := time.Time{}
	for {
		record, ok, err := runs.ReadTrailWorkRecord(root, traceID)
		if err == nil && ok && record.Completion != nil {
			if info, statErr := os.Stat(runs.TrailWorkPath(root, traceID)); statErr == nil {
				mod, size := info.ModTime(), info.Size()
				if mod.Equal(lastMod) && size == lastSize {
					if time.Since(stableSince) >= 100*time.Millisecond {
						return
					}
				} else {
					lastMod, lastSize = mod, size
					stableSince = time.Now()
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("completion did not retain a settled work record")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
