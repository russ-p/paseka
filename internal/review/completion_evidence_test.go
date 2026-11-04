package review_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/gitroot"
	"github.com/russ-p/paseka/internal/homestate"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/review"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/worktree"
)

func TestApprovePullRequestCapturesCompletionEvidenceBeforePublish(t *testing.T) {
	repo, traceID, slug, stub := setupPublishFixture(t)
	seedCompletionTrace(t, repo, traceID)
	ctxColony := colony.Context{
		ColonyRoot: repo,
		Slug:       slug,
		Home:       colony.HomeConfig{Forge: colony.ForgeConfig{Command: []string{stub}}},
	}
	ledger := finalIsolatedLedger(t, traceID)

	res, err := review.Approve(context.Background(), ctxColony, nil, ledger, review.ApproveInput{
		TraceID: traceID,
		TaskID:  taskledger.FinalReviewTaskID,
		PRTitle: "Live bees",
	}, review.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Published {
		t.Fatalf("result = %+v", res)
	}
	if !gitroot.IsInsideWorkTree(worktree.Path(repo, traceID)) {
		t.Fatal("pull_request delivery must keep the worktree")
	}

	checkpoint := requireCompletionCheckpoint(t, repo, traceID)
	if checkpoint.PatchRef == "" || checkpoint.Base == "" || checkpoint.Revision == "" {
		t.Fatalf("checkpoint endpoints = %+v", checkpoint)
	}
	patch := readCompletionPatch(t, repo, traceID, checkpoint)
	if !strings.Contains(patch, "feature.txt") {
		t.Fatalf("retained patch does not contain the published change:\n%s", patch)
	}
}

func TestApproveLocalMergeCapturesEvidenceBeforeWorktreeRemoval(t *testing.T) {
	repo, traceID, slug := setupLocalMergeFixture(t)
	seedCompletionTrace(t, repo, traceID)
	ctxColony := colony.Context{ColonyRoot: repo, Slug: slug}
	ledger := finalIsolatedLedger(t, traceID)

	res, err := review.Approve(context.Background(), ctxColony, nil, ledger, review.ApproveInput{
		TraceID: traceID,
		TaskID:  taskledger.FinalReviewTaskID,
	}, review.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.CommitSHA == "" {
		t.Fatalf("local merge produced no commit: %+v", res)
	}
	if gitroot.IsInsideWorkTree(worktree.Path(repo, traceID)) {
		t.Fatal("local merge should have removed the trace worktree")
	}

	checkpoint := requireCompletionCheckpoint(t, repo, traceID)
	patch := readCompletionPatch(t, repo, traceID, checkpoint)
	if !strings.Contains(patch, "feature.txt") || !strings.Contains(patch, "+new feature") {
		t.Fatalf("retained patch lost the merged change:\n%s", patch)
	}
	if _, ok, err := runs.ReadWorkCompletionMetrics(repo, traceID, checkpoint.CompletionID); err != nil || !ok {
		t.Fatalf("metrics snapshot: ok=%v err=%v", ok, err)
	}
}

func TestApproveDeliversAfterCheckpointWithUnpublishedMilestone(t *testing.T) {
	repo, traceID, slug := setupLocalMergeFixture(t)
	seedCompletionTrace(t, repo, traceID)
	ctxColony := colony.Context{ColonyRoot: repo, Slug: slug}
	ledger := finalIsolatedLedger(t, traceID)

	// A publication failure leaves the completion recorded but unpublished. It
	// must not hold delivery: the checkpoint, not the bus announcement, is the
	// evidence delivery needs.
	completionID := taskledger.CompletionIDForRevision(traceID, 1)
	if _, _, err := ledger.RecordWorkCompletion(traceID, taskledger.WorkCompletionSnapshot{
		CompletionID: completionID,
		WorkRevision: 1,
		CompletedAt:  time.Now().UTC(),
		ResultRef:    runs.WorkCompletionResultRef(traceID, completionID),
		MetricsRef:   runs.WorkCompletionMetricsRef(traceID, completionID),
	}); err != nil {
		t.Fatal(err)
	}

	res, err := review.Approve(context.Background(), ctxColony, nil, ledger, review.ApproveInput{
		TraceID: traceID,
		TaskID:  taskledger.FinalReviewTaskID,
	}, review.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.CommitSHA == "" {
		t.Fatalf("delivery waited for an unpublished milestone: %+v", res)
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Tasks[taskledger.FinalReviewTaskID].Status != protocol.TaskStatusCompleted {
		t.Fatalf("merge gate = %q", snap.Tasks[taskledger.FinalReviewTaskID].Status)
	}
	requireCompletionCheckpoint(t, repo, traceID)
}

func TestApproveFailsVisiblyWhenCompletionEvidenceCannotPersist(t *testing.T) {
	repo, traceID, slug := setupLocalMergeFixture(t)
	seedCompletionTrace(t, repo, traceID)
	ctxColony := colony.Context{ColonyRoot: repo, Slug: slug}
	ledger := finalIsolatedLedger(t, traceID)
	// Block the completion directory with a regular file so the checkpoint
	// cannot be persisted.
	blocked := filepath.Join(repo, ".paseka", "runs", traceID, runs.WorkCompletionDirName)
	if err := os.WriteFile(blocked, []byte("blocked"), 0o644); err != nil {
		t.Fatal(err)
	}
	pub := &recordingPublisher{}

	_, err := review.Approve(context.Background(), ctxColony, pub, ledger, review.ApproveInput{
		TraceID: traceID,
		TaskID:  taskledger.FinalReviewTaskID,
	}, review.WriteOptions{})
	if err == nil || !strings.Contains(err.Error(), "capture work completion") {
		t.Fatalf("err = %v, want a visible capture failure", err)
	}
	if !gitroot.IsInsideWorkTree(worktree.Path(repo, traceID)) {
		t.Fatal("delivery removed the worktree despite a failed capture")
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Tasks[taskledger.FinalReviewTaskID].Status != protocol.TaskStatusWaitingReview {
		t.Fatalf("merge gate = %q, want waiting_review", snap.Tasks[taskledger.FinalReviewTaskID].Status)
	}
	for _, ev := range pub.events {
		if protocol.PayloadKind(ev.Payload) == protocol.SignalTrailWorkCompleted {
			t.Fatal("a failed capture announced a work-completion milestone")
		}
	}
	if len(pub.events) != 0 {
		t.Fatalf("a failed capture published %d events", len(pub.events))
	}
}

func TestReconcileMergedCapturesEvidenceBeforeRemovingWorktree(t *testing.T) {
	repo, traceID, slug, stub := setupPublishFixture(t)
	seedCompletionTrace(t, repo, traceID)
	ctxColony := colony.Context{
		ColonyRoot: repo,
		Slug:       slug,
		Home:       colony.HomeConfig{Forge: colony.ForgeConfig{Command: []string{stub}}},
	}
	ledger := finalIsolatedLedger(t, traceID)
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

	res, err := review.ReconcilePublished(context.Background(), ctxColony, nil, ledger, traceID, review.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Completed {
		t.Fatalf("reconcile = %+v", res)
	}
	if gitroot.IsInsideWorkTree(worktree.Path(repo, traceID)) {
		t.Fatal("merged PR reconcile should remove the worktree")
	}
	checkpoint := requireCompletionCheckpoint(t, repo, traceID)
	patch := readCompletionPatch(t, repo, traceID, checkpoint)
	if !strings.Contains(patch, "feature.txt") {
		t.Fatalf("retained patch lost the merged change:\n%s", patch)
	}
}

// setupLocalMergeFixture prepares an ordinary local_merge colony with a trace
// worktree that carries one committed change.
func setupLocalMergeFixture(t *testing.T) (repo, traceID, slug string) {
	t.Helper()
	repo, _ = clonePublishBare(t)
	slug = "merge-colony"
	setupPublishHome(t, repo, slug, "")
	traceID = "trace-merge-1"
	entry, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       slug,
		Branch:     "feature/x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entry.Path, "feature.txt"), []byte("new feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, entry.Path, "add", "feature.txt")
	runGit(t, entry.Path, "commit", "-m", "feat")
	return repo, traceID, slug
}

// seedCompletionTrace writes the retained run evidence a metrics projection
// needs, so a delivery path captures a checkpoint instead of treating the trail
// as evidence-free.
func seedCompletionTrace(t *testing.T, repo, traceID string) {
	t.Helper()
	at := time.Now().UTC().Add(-time.Minute)
	task := taskledger.TaskSnapshot{
		TaskID:    "task-1",
		Bee:       "builder",
		Status:    protocol.TaskStatusCompleted,
		CreatedAt: at,
		UpdatedAt: at,
	}
	if err := runs.WriteTaskSnapshot(repo, traceID, task); err != nil {
		t.Fatal(err)
	}
	if err := runs.WriteTrailWorkRecord(repo, traceID, runs.TrailWorkRecord{Revision: 1}); err != nil {
		t.Fatal(err)
	}
}

func requireCompletionCheckpoint(t *testing.T, repo, traceID string) runs.WorkCompletionCheckpoint {
	t.Helper()
	completionID := taskledger.CompletionIDForRevision(traceID, 1)
	checkpoint, ok, err := runs.ReadWorkCompletionCheckpoint(repo, traceID, completionID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("no completion checkpoint captured for %s", completionID)
	}
	return checkpoint
}

func readCompletionPatch(t *testing.T, repo, traceID string, checkpoint runs.WorkCompletionCheckpoint) string {
	t.Helper()
	path, err := runs.ResolveTrailRef(repo, checkpoint.PatchRef)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
