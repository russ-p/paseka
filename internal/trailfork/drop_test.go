package trailfork_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/gitroot"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/purge"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/tasks"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/trailfork"
	"github.com/russ-p/paseka/internal/trailview"
)

type dropProbePublisher struct {
	events      []protocol.Event
	failPublish bool
	onKill      func(protocol.Event)
}

func (p *dropProbePublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	if p.failPublish {
		return errors.New("bus unavailable")
	}
	if p.onKill != nil && protocol.PayloadKind(ev.Payload) == string(protocol.SignalSystemKill) {
		p.onKill(ev)
	}
	p.events = append(p.events, ev)
	return nil
}

func setupDropColony(t *testing.T) (string, string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	home, err := colony.HomeDir("test")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	root, base := setupGitRepo(t)
	ctx := testContext(root)
	ctx.Slug = "test"
	writeSource(t, ctx, "trace-source", "task-1", nil)
	return root, base
}

func createDropFork(t *testing.T, ctxRoot, sourceTrace string) (trailfork.Result, *dropProbePublisher, taskledger.Ledger) {
	t.Helper()
	ctx := testContext(ctxRoot)
	ctx.Slug = "test"
	ledger := taskledger.NewMemoryLedger()
	pub := &dropProbePublisher{}
	result, err := trailfork.Create(context.Background(), pub, ledger, trailfork.Options{
		Context:       ctx,
		SourceTraceID: sourceTrace,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result, pub, ledger
}

func TestDropStopsForkBeforeRemovingExecutionResources(t *testing.T) {
	root, _ := setupDropColony(t)
	ctx := testContext(root)
	ctx.Slug = "test"

	created, pub, ledger := createDropFork(t, root, "trace-source")
	fork := created.Plan
	worktreeAtKill := false
	pub.onKill = func(protocol.Event) {
		if _, err := os.Stat(created.Worktree.Path); err == nil {
			worktreeAtKill = true
		}
	}

	session := &tasks.LedgerSession{Colony: ctx, Publisher: pub, Ledger: ledger}
	dropped, err := trailfork.Drop(context.Background(), trailfork.DropOptions{
		Context: ctx,
		TraceID: fork.TraceID,
		Reason:  "not worth continuing",
		Actor:   "alice",
		Session: session,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !dropped.Stopped || !dropped.Killed {
		t.Fatalf("drop did not record the stop: %+v", dropped)
	}
	if !worktreeAtKill {
		t.Fatal("worktree was removed before the kill was recorded")
	}
	if dropped.RemovedWorktree != created.Worktree.Path {
		t.Fatalf("removed worktree = %q", dropped.RemovedWorktree)
	}
	if _, err := os.Stat(created.Worktree.Path); !os.IsNotExist(err) {
		t.Fatalf("worktree still exists: %v", err)
	}
	if gitroot.LocalBranchExists(root, fork.Branch) {
		t.Fatalf("fork branch %q was retained", fork.Branch)
	}
	if dropped.RemovedBranch != fork.Branch {
		t.Fatalf("removed branch = %q, want %q", dropped.RemovedBranch, fork.Branch)
	}

	snap, err := ledger.Snapshot(fork.TraceID)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Killed {
		t.Fatal("task ledger does not record the kill")
	}

	lineage, err := trailfork.LoadLineage(root, fork.TraceID)
	if err != nil {
		t.Fatal(err)
	}
	if lineage.State != trailfork.StateDropped || lineage.DroppedBy != "alice" || lineage.DroppedAt == nil {
		t.Fatalf("dropped lineage = %+v", lineage)
	}
	if lineage.ConfigRef != fork.ConfigRef || lineage.SourceConfigRef != fork.SourceConfigRef {
		t.Fatalf("drop lost configuration evidence: %+v", lineage)
	}
	if !strings.Contains(lineage.DropReason, "not worth continuing") {
		t.Fatalf("drop reason = %q", lineage.DropReason)
	}

	// Configuration snapshots, attempts, tasks, and the lineage record stay
	// readable after the execution resources are gone.
	if _, err := trailconfig.BindRef(ctx, fork.TraceID, fork.ConfigRef, 1); err != nil {
		t.Fatalf("retained configuration is unreadable: %v", err)
	}
	attempts, err := trailconfig.Attempts(root, fork.TraceID)
	if err != nil || len(attempts) == 0 {
		t.Fatalf("attempt ownership = %+v %v", attempts, err)
	}
	for _, attempt := range attempts {
		if attempt.Open() {
			t.Fatalf("attempt %s stayed open after drop", attempt.AttemptID)
		}
	}
	if _, err := os.Stat(dropped.RetainedLineagePath); err != nil {
		t.Fatalf("lineage record missing: %v", err)
	}

	// A repeated drop is idempotent and needs no live ledger.
	again, err := trailfork.Drop(context.Background(), trailfork.DropOptions{Context: ctx, TraceID: fork.TraceID})
	if err != nil {
		t.Fatal(err)
	}
	if !again.AlreadyDropped || again.RemovedWorktree == "" {
		t.Fatalf("second drop = %+v", again)
	}
}

func TestDropRefusesRemovalWhenTheStopCannotBeRecorded(t *testing.T) {
	root, _ := setupDropColony(t)
	ctx := testContext(root)
	ctx.Slug = "test"

	created, _, ledger := createDropFork(t, root, "trace-source")
	fork := created.Plan

	session := &tasks.LedgerSession{Colony: ctx, Publisher: &dropProbePublisher{failPublish: true}, Ledger: ledger}
	if _, err := trailfork.Drop(context.Background(), trailfork.DropOptions{
		Context: ctx,
		TraceID: fork.TraceID,
		Session: session,
	}); err == nil || !strings.Contains(err.Error(), "stop fork") {
		t.Fatalf("drop error = %v", err)
	}
	if _, err := os.Stat(created.Worktree.Path); err != nil {
		t.Fatalf("worktree removed without a recorded stop: %v", err)
	}
	lineage, err := trailfork.LoadLineage(root, fork.TraceID)
	if err != nil {
		t.Fatal(err)
	}
	if lineage.State != trailfork.StateRunnable {
		t.Fatalf("failed drop changed lineage state to %q", lineage.State)
	}

	// Without a ledger session the runtime cannot prove the stop either.
	if _, err := trailfork.Drop(context.Background(), trailfork.DropOptions{Context: ctx, TraceID: fork.TraceID}); err == nil {
		t.Fatal("drop removed resources without the task ledger")
	}
	if _, err := os.Stat(created.Worktree.Path); err != nil {
		t.Fatalf("worktree removed without a ledger session: %v", err)
	}
}

func TestWorktreePruneLeavesConfigurationEvidenceReadable(t *testing.T) {
	root, _ := setupDropColony(t)
	ctx := testContext(root)
	ctx.Slug = "test"

	created, _, _ := createDropFork(t, root, "trace-source")
	fork := created.Plan

	target := purge.PruneTarget{
		Worktrees: true,
		OlderThan: time.Hour,
		Now:       time.Now().Add(48 * time.Hour),
	}
	plan, err := purge.Prune(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Worktrees) != 1 || plan.Worktrees[0].TraceID != fork.TraceID {
		t.Fatalf("worktree prune plan = %+v", plan.Worktrees)
	}
	if _, err := purge.ExecutePrune(ctx, target, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(created.Worktree.Path); !os.IsNotExist(err) {
		t.Fatalf("pruned worktree still exists: %v", err)
	}
	if _, err := trailconfig.BindRef(ctx, fork.TraceID, fork.ConfigRef, 1); err != nil {
		t.Fatalf("worktree prune removed configuration evidence: %v", err)
	}
	if _, err := trailfork.LoadLineage(root, fork.TraceID); err != nil {
		t.Fatalf("worktree prune removed fork lineage: %v", err)
	}
}

func TestRemovingSourceLeavesRetainedForkExecutableAndInspectable(t *testing.T) {
	root, _ := setupDropColony(t)
	ctx := testContext(root)
	ctx.Slug = "test"

	created, _, _ := createDropFork(t, root, "trace-source")
	fork := created.Plan

	if _, err := purge.Execute(ctx, purge.PurgeTarget{Runs: true, Worktrees: true, TraceID: "trace-source"}); err != nil {
		t.Fatal(err)
	}
	if _, err := trailconfig.Load(root, "trace-source"); !errors.Is(err, trailconfig.ErrNoBinding) {
		t.Fatalf("source configuration survived whole-trail deletion: %v", err)
	}
	if _, err := os.Stat(created.Worktree.Path); err != nil {
		t.Fatalf("deleting the source removed the fork workspace: %v", err)
	}

	// The fork owns its selected source and candidate snapshots, so removing
	// the source requires no cross-trail update.
	if _, err := trailconfig.BindRef(ctx, fork.TraceID, fork.SourceConfigRef, fork.SourceConfigRevision); err != nil {
		t.Fatalf("retained source snapshot is unreadable: %v", err)
	}
	if _, err := trailconfig.BindRef(ctx, fork.TraceID, fork.ConfigRef, 1); err != nil {
		t.Fatalf("retained candidate snapshot is unreadable: %v", err)
	}
	trail, ok, err := trailview.LoadTrail(ctx, fork.TraceID)
	if err != nil || !ok {
		t.Fatalf("fork is not inspectable: ok=%v err=%v", ok, err)
	}
	if trail.ForkOf != "trace-source" || trail.ConfigRef != fork.ConfigRef || trail.Policy != trailconfig.PolicyPinned {
		t.Fatalf("inspectable fork = %+v", trail)
	}
}
