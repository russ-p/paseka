package review_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/gitroot"
	"github.com/russ-p/paseka/internal/homestate"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/review"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/trailfork"
	"github.com/russ-p/paseka/internal/worktree"
)

func TestApproveExperimentalFinalGateCompletesWithoutDelivery(t *testing.T) {
	repo, traceID, slug, stub := setupPublishFixture(t)
	ctxColony := colony.Context{
		ColonyRoot: repo,
		Slug:       slug,
		Home:       colony.HomeConfig{Forge: colony.ForgeConfig{Command: []string{stub}}},
	}
	createForkBinding(t, ctxColony, traceID)

	ledger := finalIsolatedLedger(t, traceID)
	res, err := review.Approve(context.Background(), ctxColony, nil, ledger, review.ApproveInput{
		TraceID: traceID,
		TaskID:  taskledger.FinalReviewTaskID,
		PRTitle: "Experiment",
		PRBody:  "should not publish",
	}, review.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Experimental {
		t.Fatal("fork approval must be marked experimental")
	}
	if res.Published || res.PRURL != "" || res.PRNumber != 0 {
		t.Fatalf("fork approval delivered: %+v", res)
	}
	if res.CommitSHA != "" {
		t.Fatalf("fork approval created a merge commit %q", res.CommitSHA)
	}

	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Tasks[taskledger.FinalReviewTaskID].Status != protocol.TaskStatusCompleted {
		t.Fatalf("final review status = %q, want completed", snap.Tasks[taskledger.FinalReviewTaskID].Status)
	}
	if _, ok, err := homestate.FindPullRequest(slug, traceID); err != nil || ok {
		t.Fatalf("fork published a pull request: ok=%v err=%v", ok, err)
	}
	if !gitroot.IsInsideWorkTree(worktree.Path(repo, traceID)) {
		t.Fatal("fork worktree must remain after internal approval")
	}
	msg := review.ApproveMessage(review.ApproveMessageOptions{Experimental: true})
	if !strings.Contains(msg, "experimental") || !strings.Contains(msg, "no branch merge") {
		t.Fatalf("approve message = %q", msg)
	}
}

func TestReconcileExperimentalTrailRefusesDelivery(t *testing.T) {
	repo, traceID, slug, stub := setupPublishFixture(t)
	ctxColony := colony.Context{
		ColonyRoot: repo,
		Slug:       slug,
		Home:       colony.HomeConfig{Forge: colony.ForgeConfig{Command: []string{stub}}},
	}
	createForkBinding(t, ctxColony, traceID)

	if err := homestate.UpsertPullRequest(slug, homestate.PullRequestEntry{
		TraceID: traceID,
		URL:     "https://example.test/pr/42",
		Number:  42,
		Head:    "feature/x",
		Base:    "main",
		State:   "open",
	}); err != nil {
		t.Fatal(err)
	}
	merged := `#!/bin/sh
echo '{"protocolVersion":1,"found":true,"number":42,"url":"https://example.test/pr/42","head":"feature/x","base":"main","state":"merged","draft":false}'
`
	if err := os.WriteFile(stub, []byte(merged), 0o755); err != nil {
		t.Fatal(err)
	}

	ledger := finalIsolatedLedger(t, traceID)
	_, err := review.ReconcilePublished(context.Background(), ctxColony, nil, ledger, traceID, review.WriteOptions{})
	if err == nil || !strings.Contains(err.Error(), "experimental") {
		t.Fatalf("reconcile err = %v, want experimental refusal", err)
	}

	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Tasks[taskledger.FinalReviewTaskID].Status != protocol.TaskStatusWaitingReview {
		t.Fatalf("reconcile completed the fork: status = %q", snap.Tasks[taskledger.FinalReviewTaskID].Status)
	}
	if !gitroot.IsInsideWorkTree(worktree.Path(repo, traceID)) {
		t.Fatal("reconcile removed the fork worktree")
	}
}

func createForkBinding(t *testing.T, ctxColony colony.Context, traceID string) {
	t.Helper()
	source, err := trailconfig.Ensure(ctxColony, "trace-source")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := trailconfig.CreateForkBinding(trailconfig.ForkBindingOptions{
		Context:       ctxColony,
		SourceTraceID: "trace-source",
		TraceID:       traceID,
		ConfigRef:     source.ActiveConfigRef,
		WorkspaceRoot: worktree.Path(ctxColony.ColonyRoot, traceID),
		RequestID:     "req-" + traceID,
		Actor:         "test",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestIsExperimentalTrailFallsBackToForkLineage(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-fork"
	if err := os.MkdirAll(filepath.Dir(trailfork.LineagePath(root, traceID)), 0o755); err != nil {
		t.Fatal(err)
	}
	lineage := `{"version":1,"forkId":"fork-x","traceId":"trace-fork","sourceTrace":"trace-source","policy":"pinned"}`
	if err := os.WriteFile(trailfork.LineagePath(root, traceID), []byte(lineage), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := review.IsExperimentalTrail(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("fork lineage without a readable binding must still be experimental")
	}
	if ordinary, err := review.IsExperimentalTrail(root, "trace-ordinary"); err != nil || ordinary {
		t.Fatalf("ordinary trail = %v, err = %v", ordinary, err)
	}
}
