package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/trailfork"
	"github.com/russ-p/paseka/internal/worktree"
)

func TestWorkspaceForDispatchReusesForkExecutionRoot(t *testing.T) {
	repo := initProposalDispatchRepo(t)
	forkRoot := worktree.Path(repo, "trace-fork")
	if err := os.MkdirAll(forkRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	ctxColony := colony.Context{ColonyRoot: repo, Slug: "test-colony", WorkspaceRoot: forkRoot}
	manifest, err := colony.LoadColony(repo)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name         string
		bee          colony.Bee
		proposalKind string
	}{
		{name: "root working", bee: colony.Bee{Role: "builder", Worktree: false}},
		{name: "worktree enabled", bee: colony.Bee{Role: "builder", Worktree: true}},
		{name: "root proposal", bee: colony.Bee{Role: "main-guard", Worktree: true}, proposalKind: string(protocol.MutationCodeProposalRoot)},
		{name: "isolated proposal", bee: colony.Bee{Role: "guard", Worktree: false}, proposalKind: string(protocol.MutationCodeProposalIsolated)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workspace, _, err := workspaceForDispatch(ctxColony, manifest, tc.bee, "trace-fork", "", tc.proposalKind)
			if err != nil {
				t.Fatal(err)
			}
			if workspace != forkRoot {
				t.Fatalf("workspace = %q, want fork root %q", workspace, forkRoot)
			}
			if workspace == repo {
				t.Fatal("fork dispatch fell back to the primary colony checkout")
			}
		})
	}
}

func TestForkDispatchUsesOwnCheckoutAndPinnedRegistry(t *testing.T) {
	repo := initProposalDispatchRepo(t)
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "prompts", "default.md"), []byte("PINNED {{.Task}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builder := `role: builder
adapter: cursor
prompt_template: default.md
subscribes:
  - type: SIGNAL
    kind: feature.requested
    dispatch: direct
`
	if err := os.MkdirAll(filepath.Join(repo, ".paseka", "bees"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "bees", "builder.yaml"), []byte(builder), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitProposal(t, repo, "add", ".")
	runGitProposal(t, repo, "commit", "-m", "source config")

	ctxColony := colony.Context{ColonyRoot: repo, Slug: "test-colony"}
	source, err := trailconfig.Ensure(ctxColony, "trace-source")
	if err != nil {
		t.Fatal(err)
	}
	forkWorktree, err := worktree.Ensure(worktree.EnsureOptions{ColonyRoot: repo, TraceID: "trace-fork"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := trailconfig.CreateForkBinding(trailconfig.ForkBindingOptions{
		Context:       ctxColony,
		SourceTraceID: "trace-source",
		TraceID:       "trace-fork",
		ConfigRef:     source.ActiveConfigRef,
		WorkspaceRoot: forkWorktree.Path,
		RequestID:     "req-fork",
	}); err != nil {
		t.Fatal(err)
	}

	// Replace the live configuration after the source snapshot was captured.
	// The fork must keep dispatching through its pinned registry and must not
	// read the edited live prompt.
	livePrompt := "LIVE {{.Task}}\n"
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "prompts", "default.md"), []byte(livePrompt), 0o644); err != nil {
		t.Fatal(err)
	}
	worker := `role: worker
adapter: cursor
prompt_template: default.md
subscribes:
  - type: SIGNAL
    kind: feature.requested
    dispatch: direct
`
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "bees", "worker.yaml"), []byte(worker), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(repo, ".paseka", "bees", "builder.yaml")); err != nil {
		t.Fatal(err)
	}
	liveRegistry, err := BuildBeeRegistry(repo)
	if err != nil {
		t.Fatal(err)
	}

	d := NewDispatcher()
	d.SetBeeRegistry(liveRegistry)
	rec := &proposalRecordingAdapter{}
	d.RegisterAdapter("cursor", rec)
	r := NewTestReactor(TestReactorOptions{
		ColonyRoot: repo,
		Dispatcher: d,
		Registry:   liveRegistry,
		Ledger:     taskledger.NewMemoryLedger(),
	})

	forkEvent, err := protocol.NewEvent("trace-fork", "agent-fork", 1, protocol.EventSignal, map[string]any{
		"kind":  "feature.requested",
		"title": "fork work",
		"body":  "fork work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), forkEvent); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 1 {
		t.Fatalf("fork dispatch calls = %d, want 1 from the pinned registry", rec.calls)
	}
	if rec.lastReq.Bee != "builder" {
		t.Fatalf("fork dispatch bee = %q, want pinned builder", rec.lastReq.Bee)
	}
	if rec.lastReq.Workspace != forkWorktree.Path {
		t.Fatalf("fork dispatch workspace = %q, want %q", rec.lastReq.Workspace, forkWorktree.Path)
	}
	if !strings.Contains(rec.lastReq.Prompt, "PINNED") || strings.Contains(rec.lastReq.Prompt, "LIVE") {
		t.Fatalf("fork dispatch prompt = %q, want pinned prompt", rec.lastReq.Prompt)
	}

	ordinaryEvent, err := protocol.NewEvent("trace-ordinary", "agent-ordinary", 1, protocol.EventSignal, map[string]any{
		"kind":  "feature.requested",
		"title": "ordinary work",
		"body":  "ordinary work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ordinaryEvent); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 2 {
		t.Fatalf("ordinary dispatch calls = %d, want 2", rec.calls)
	}
	if rec.lastReq.Bee != "worker" {
		t.Fatalf("ordinary dispatch bee = %q, want live worker", rec.lastReq.Bee)
	}
	if rec.lastReq.Workspace != repo {
		t.Fatalf("ordinary dispatch workspace = %q, want primary checkout %q", rec.lastReq.Workspace, repo)
	}
	if !strings.Contains(rec.lastReq.Prompt, "LIVE") {
		t.Fatalf("ordinary dispatch prompt = %q, want live prompt", rec.lastReq.Prompt)
	}
}

func TestForkInternalProposalAndVerificationChoreography(t *testing.T) {
	repo := initProposalDispatchRepo(t)
	bees := map[string]colony.Bee{
		"builder": {Role: "builder", Worktree: true, Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "VERIFICATION", Kind: "verification.failed"}, Dispatch: colony.DispatchDirect},
		}},
		"guard": {Role: "guard", Worktree: true, Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.isolated"}, Dispatch: colony.DispatchDirect},
		}},
	}
	if err := writeProposalDispatchColony(repo, bees); err != nil {
		t.Fatal(err)
	}
	runGitProposal(t, repo, "add", ".")
	runGitProposal(t, repo, "commit", "-m", "source config")

	ctxColony := colony.Context{ColonyRoot: repo, Slug: "test-colony"}
	source, err := trailconfig.Ensure(ctxColony, "trace-source")
	if err != nil {
		t.Fatal(err)
	}
	forkWorktree, err := worktree.Ensure(worktree.EnsureOptions{ColonyRoot: repo, TraceID: "trace-fork"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := trailconfig.CreateForkBinding(trailconfig.ForkBindingOptions{
		Context:       ctxColony,
		SourceTraceID: "trace-source",
		TraceID:       "trace-fork",
		ConfigRef:     source.ActiveConfigRef,
		WorkspaceRoot: forkWorktree.Path,
		RequestID:     "req-fork",
	}); err != nil {
		t.Fatal(err)
	}

	liveRegistry, err := BuildBeeRegistry(repo)
	if err != nil {
		t.Fatal(err)
	}
	d := NewDispatcher()
	d.SetBeeRegistry(liveRegistry)
	rec := &proposalRecordingAdapter{}
	d.RegisterAdapter("cursor", rec)
	r := NewTestReactor(TestReactorOptions{
		ColonyRoot: repo,
		Dispatcher: d,
		Registry:   liveRegistry,
		Ledger:     taskledger.NewMemoryLedger(),
	})

	plan, err := protocol.NewEvent("trace-fork", "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{
			TaskID: "task-1",
			Title:  "fork work",
			Bee:    "builder",
			Review: protocol.TaskReviewRequired,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	running, err := protocol.NewEvent("trace-fork", "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind: protocol.TaskEventStatus, TaskID: "task-1", Status: protocol.TaskStatusRunning,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), running); err != nil {
		t.Fatal(err)
	}

	proposal, err := protocol.NewEvent("trace-fork", "builder-1", 0, protocol.EventMutation, protocol.MutationPayload{
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
	if rec.calls != 1 {
		t.Fatalf("guard dispatch calls = %d, want 1", rec.calls)
	}
	if rec.lastReq.Workspace != forkWorktree.Path {
		t.Fatalf("guard workspace = %q, want fork checkout %q", rec.lastReq.Workspace, forkWorktree.Path)
	}

	verification, err := protocol.NewEvent("trace-fork", "guard-1", 0, protocol.EventVerification, protocol.VerificationPayload{
		Kind:    protocol.VerificationFailed,
		TaskID:  "task-1",
		Summary: "proposal needs rework",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), verification); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 2 {
		t.Fatalf("rework dispatch calls = %d, want 2", rec.calls)
	}
	if rec.lastReq.Bee != "builder" {
		t.Fatalf("rework bee = %q, want pinned builder", rec.lastReq.Bee)
	}
	if rec.lastReq.Workspace != forkWorktree.Path {
		t.Fatalf("rework workspace = %q, want fork checkout %q", rec.lastReq.Workspace, forkWorktree.Path)
	}
	snap, err := r.Ledger().Snapshot("trace-fork")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Tasks["task-1"].ProposalWorkspace != protocol.ProposalWorkspaceIsolated {
		t.Fatalf("proposal workspace = %q, want isolated", snap.Tasks["task-1"].ProposalWorkspace)
	}
}

func TestReactorRefusesExperimentalTrailWithoutBinding(t *testing.T) {
	repo := initProposalDispatchRepo(t)
	writeRuntimeLineage(t, repo, trailfork.Lineage{
		TraceID:     "trace-fork",
		ForkID:      "fork-1",
		SourceTrace: "trace-source",
		Delta:       trailfork.Delta{Form: trailfork.DeltaNone},
	})

	registry, err := BuildBeeRegistry(repo)
	if err != nil {
		t.Fatal(err)
	}
	r := NewTestReactor(TestReactorOptions{
		ColonyRoot: repo,
		Dispatcher: NewDispatcher(),
		Registry:   registry,
		Ledger:     taskledger.NewMemoryLedger(),
	})
	ev, err := protocol.NewEvent("trace-fork", "agent-fork", 1, protocol.EventSignal, map[string]any{
		"kind":  "feature.requested",
		"title": "fork work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ev); err == nil ||
		!strings.Contains(err.Error(), "no configuration binding") {
		t.Fatalf("err = %v, want experimental binding refusal", err)
	}
}

func writeRuntimeLineage(t *testing.T, root string, lineage trailfork.Lineage) {
	t.Helper()
	data, err := json.Marshal(lineage)
	if err != nil {
		t.Fatal(err)
	}
	path := trailfork.LineagePath(root, lineage.TraceID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDroppedForkRefusesDispatch(t *testing.T) {
	repo := initProposalDispatchRepo(t)
	ctxColony := colony.Context{ColonyRoot: repo, Slug: "test-colony"}
	source, err := trailconfig.Ensure(ctxColony, "trace-source")
	if err != nil {
		t.Fatal(err)
	}
	forkWorktree, err := worktree.Ensure(worktree.EnsureOptions{ColonyRoot: repo, TraceID: "trace-fork"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := trailconfig.CreateForkBinding(trailconfig.ForkBindingOptions{
		Context:       ctxColony,
		SourceTraceID: "trace-source",
		TraceID:       "trace-fork",
		ConfigRef:     source.ActiveConfigRef,
		WorkspaceRoot: forkWorktree.Path,
		RequestID:     "req-dropped",
	}); err != nil {
		t.Fatal(err)
	}
	lineage := trailfork.Lineage{
		Version:         1,
		ForkID:          "fork-dropped",
		TraceID:         "trace-fork",
		SourceTrace:     "trace-source",
		SourceConfigRef: source.ActiveConfigRef,
		ConfigRef:       source.ActiveConfigRef,
		Policy:          trailconfig.PolicyPinned,
		State:           trailfork.StateDropped,
		Workspace:       forkWorktree.Path,
		Branch:          forkWorktree.Branch,
	}
	data, err := json.Marshal(lineage)
	if err != nil {
		t.Fatal(err)
	}
	path := trailfork.LineagePath(repo, "trace-fork")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	d := NewDispatcher()
	rec := &proposalRecordingAdapter{}
	d.RegisterAdapter("cursor", rec)
	r := NewTestReactor(TestReactorOptions{ColonyRoot: repo, Dispatcher: d, Ledger: taskledger.NewMemoryLedger()})

	ev, err := protocol.NewEvent("trace-fork", "agent-fork", 1, protocol.EventSignal, map[string]any{
		"kind":  "feature.requested",
		"title": "dropped work",
		"body":  "dropped work",
	})
	if err != nil {
		t.Fatal(err)
	}
	err = r.ProcessEvent(context.Background(), ev)
	if err == nil || !strings.Contains(err.Error(), "was dropped") {
		t.Fatalf("dropped fork dispatch error = %v", err)
	}
	if rec.calls != 0 {
		t.Fatalf("dropped fork dispatched %d adapter call(s)", rec.calls)
	}
}
