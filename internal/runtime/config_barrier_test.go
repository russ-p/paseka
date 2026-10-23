package runtime_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/runtime"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
)

func TestPendingApplyHoldsNewWorkAndKeepsExistingChains(t *testing.T) {
	root := t.TempDir()
	bees := map[string]colony.Bee{
		"builder": {Role: "builder", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "task.ready"}, Dispatch: colony.DispatchTask},
			{EventRule: colony.EventRule{Type: "VERIFICATION", Kind: "verification.failed"}, Dispatch: colony.DispatchDirect},
		}},
	}
	mustWriteTestColony(t, root, bees)
	promptPath := filepath.Join(root, ".paseka", "prompts", "default.md")
	if err := os.WriteFile(promptPath, []byte("OLD {{.Task}}"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, rec := taskAttemptReactor(t, root, bees)
	traceID := "trace-config-barrier"
	plan, ready := barrierTaskEvents(t, traceID, "task-1")
	if err := r.ProcessEvent(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ready); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 1 || !strings.Contains(rec.lastReq.Prompt, "OLD") {
		t.Fatalf("initial dispatch calls=%d prompt=%q", rec.calls, rec.lastReq.Prompt)
	}
	writeRunCompleted(t, root, traceID, rec.lastReq.AgentID)

	if err := os.WriteFile(promptPath, []byte("NEW {{.Task}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest, err := trailconfig.Load(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	expected := manifest.ConfigRevision
	applied, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: colony.Context{ColonyRoot: root, Slug: "test"},
		TraceID: traceID, Source: trailconfig.SourceColony, Actor: "operator",
		ExpectedRevision: &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if applied.Change.Outcome != trailconfig.OutcomePending {
		t.Fatalf("apply outcome = %q, want pending", applied.Change.Outcome)
	}
	if applied.Manifest.ActiveConfigRef != manifest.ActiveConfigRef {
		t.Fatal("pending apply changed the active configuration")
	}

	plan2, ready2 := barrierTaskEvents(t, traceID, "task-2")
	if err := r.ProcessEvent(context.Background(), plan2); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ready2); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 1 {
		t.Fatalf("new task admitted while pending: calls=%d", rec.calls)
	}

	rework, err := protocol.NewEvent(traceID, "guard", 3, protocol.EventVerification, protocol.VerificationPayload{
		Kind: protocol.VerificationFailed, TaskID: "task-1", Summary: "fix it",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), rework); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 2 || !strings.Contains(rec.lastReq.Prompt, "OLD") {
		t.Fatalf("existing rework calls=%d prompt=%q", rec.calls, rec.lastReq.Prompt)
	}
	if rec.lastReq.ConfigRevision != 1 {
		t.Fatalf("rework revision = %d, want 1", rec.lastReq.ConfigRevision)
	}
	writeRunCompleted(t, root, traceID, rec.lastReq.AgentID)

	failed, err := protocol.NewEvent(traceID, "runtime", 4, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind: protocol.TaskEventCompleted, TaskID: "task-1", Status: protocol.TaskStatusFailed, Summary: "give up",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), failed); err != nil {
		t.Fatal(err)
	}

	if rec.calls != 3 || !strings.Contains(rec.lastReq.Prompt, "NEW") {
		t.Fatalf("held work did not resume on the new config: calls=%d prompt=%q", rec.calls, rec.lastReq.Prompt)
	}
	if rec.lastReq.ConfigRevision != 2 {
		t.Fatalf("resumed work revision = %d, want 2", rec.lastReq.ConfigRevision)
	}
	activated, err := trailconfig.Load(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if activated.ConfigRevision != 2 || activated.Pending != nil {
		t.Fatalf("activation = %+v", activated)
	}
	attempts, err := trailconfig.Attempts(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	var taskTwo *trailconfig.Attempt
	for i := range attempts {
		if attempts[i].TaskID == "task-2" {
			taskTwo = &attempts[i]
		}
	}
	if taskTwo == nil || taskTwo.ConfigRevision != 2 || taskTwo.ConfigRef != activated.ActiveConfigRef {
		t.Fatalf("task-2 attempt = %+v", taskTwo)
	}

	// Recovering the barrier after a restart must not apply the transition twice.
	if err := r.RecoverPendingConfigs(context.Background()); err != nil {
		t.Fatal(err)
	}
	recovered, err := trailconfig.Load(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.ConfigRevision != activated.ConfigRevision || len(recovered.History) != len(activated.History) {
		t.Fatalf("recovery applied the transition again: %+v", recovered)
	}
}

func TestKillCancelsPendingConfiguration(t *testing.T) {
	root := t.TempDir()
	bees := map[string]colony.Bee{"builder": {Role: "builder"}}
	mustWriteTestColony(t, root, bees)
	ctx := colony.Context{ColonyRoot: root, Slug: "test"}
	manifest, err := trailconfig.Ensure(ctx, "trace-kill-config")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".paseka", "prompts", "default.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected := manifest.ConfigRevision
	pending, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-kill-config", Source: trailconfig.SourceColony,
		Actor: "operator", ExpectedRevision: &expected, Blockers: []string{"task task-1 is running"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if pending.Change.Outcome != trailconfig.OutcomePending {
		t.Fatalf("apply outcome = %q", pending.Change.Outcome)
	}

	d := runtime.NewDispatcher()
	reg := runtime.NewBeeRegistryFromBees(bees)
	d.SetBeeRegistry(reg)
	r := runtime.NewTestReactor(runtime.TestReactorOptions{
		ColonyRoot: root,
		Dispatcher: d,
		Registry:   reg,
		Ledger:     taskledger.NewMemoryLedger(),
	})
	kill, err := protocol.NewEvent("trace-kill-config", "cli", 1, protocol.EventSignal, protocol.SystemKillPayload{
		Kind: protocol.SignalSystemKill, Reason: "stop",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), kill); err != nil {
		t.Fatal(err)
	}
	after, err := trailconfig.Load(root, "trace-kill-config")
	if err != nil {
		t.Fatal(err)
	}
	if after.Pending != nil || after.ActiveConfigRef != manifest.ActiveConfigRef {
		t.Fatalf("kill did not cancel pending config: %+v", after)
	}
	if len(after.History) == 0 || after.History[len(after.History)-1].Outcome != trailconfig.OutcomeCancelled {
		t.Fatalf("kill cancel not recorded: %+v", after.History)
	}
}

func barrierTaskEvents(t *testing.T, traceID, taskID string) (protocol.Event, protocol.Event) {
	t.Helper()
	plan, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{
			TaskID: taskID, Title: "implement", Body: "do work", Bee: "builder", Intent: "feature",
			Review: protocol.TaskReviewRequired,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := protocol.NewEvent(traceID, "scout", 1, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind: protocol.TaskEventReady, TaskID: taskID, Bee: "builder",
	})
	if err != nil {
		t.Fatal(err)
	}
	return plan, ready
}

func writeRunCompleted(t *testing.T, root, traceID, agentID string) {
	t.Helper()
	runDir := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: agentID}
	if err := runDir.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusCompleted,
		FinishedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
}
