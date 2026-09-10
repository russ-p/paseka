package review_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/gitroot"
	"github.com/russ-p/paseka/internal/homestate"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/review"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/worktree"
)

func TestApprovePullRequestDoesNotMerge(t *testing.T) {
	repo, traceID, slug, stub := setupPublishFixture(t)
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
		PRBody:  "## Why\nPR",
	}, review.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Published || res.PRURL == "" {
		t.Fatalf("result = %+v", res)
	}
	if res.CommitSHA != "" {
		t.Fatal("pull_request approve must not create a merge commit")
	}

	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Tasks[taskledger.FinalReviewTaskID].Status != protocol.TaskStatusWaitingReview {
		t.Fatalf("status = %q, want waiting_review", snap.Tasks[taskledger.FinalReviewTaskID].Status)
	}
	if !gitroot.IsInsideWorkTree(worktree.Path(repo, traceID)) {
		t.Fatal("worktree must remain after publish")
	}

	def, err := gitroot.ResolvedDefaultBranch(repo)
	if err != nil {
		t.Fatal(err)
	}
	log := gitOutput(t, repo, "log", "--oneline", def)
	if strings.Contains(log, "Merge") {
		t.Fatalf("default branch log has merge: %s", log)
	}

	pr, ok, err := homestate.FindPullRequest(slug, traceID)
	if err != nil || !ok {
		t.Fatalf("pr identity missing: ok=%v err=%v", ok, err)
	}
	if pr.State != "open" {
		t.Fatalf("state = %q", pr.State)
	}
}

func TestApprovePullRequestRequiresOrigin(t *testing.T) {
	repo := initPublishRepo(t)
	slug := "pr-no-origin"
	setupPublishHome(t, repo, slug, "")
	writeColonyDelivery(t, repo, colony.DeliveryPullRequest)

	ledger := finalIsolatedLedger(t, "trace-1")
	_, err := review.Approve(context.Background(), colony.Context{
		ColonyRoot: repo,
		Slug:       slug,
		Home:       colony.HomeConfig{Forge: colony.ForgeConfig{Command: []string{"/bin/true"}}},
	}, nil, ledger, review.ApproveInput{
		TraceID: "trace-1",
		TaskID:  taskledger.FinalReviewTaskID,
	}, review.WriteOptions{})
	if err == nil || !strings.Contains(err.Error(), "origin") {
		t.Fatalf("err = %v", err)
	}
}

func TestApprovePullRequestRequiresForgeCommand(t *testing.T) {
	repo, _ := clonePublishBare(t)
	slug := "pr-no-forge"
	setupPublishHome(t, repo, slug, "")
	writeColonyDelivery(t, repo, colony.DeliveryPullRequest)
	traceID := "trace-1"
	if _, err := worktree.Ensure(worktree.EnsureOptions{ColonyRoot: repo, TraceID: traceID, Slug: slug}); err != nil {
		t.Fatal(err)
	}
	ledger := finalIsolatedLedger(t, traceID)
	_, err := review.Approve(context.Background(), colony.Context{
		ColonyRoot: repo,
		Slug:       slug,
	}, nil, ledger, review.ApproveInput{
		TraceID: traceID,
		TaskID:  taskledger.FinalReviewTaskID,
	}, review.WriteOptions{})
	if err == nil || !strings.Contains(err.Error(), "forge.command") {
		t.Fatalf("err = %v", err)
	}
}

func TestApprovePullRequestRejectsNonExecutableForge(t *testing.T) {
	repo, _ := clonePublishBare(t)
	slug := "pr-bad-forge"
	setupPublishHome(t, repo, slug, "")
	writeColonyDelivery(t, repo, colony.DeliveryPullRequest)
	traceID := "trace-1"
	if _, err := worktree.Ensure(worktree.EnsureOptions{ColonyRoot: repo, TraceID: traceID, Slug: slug}); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing-forge")
	ledger := finalIsolatedLedger(t, traceID)
	_, err := review.Approve(context.Background(), colony.Context{
		ColonyRoot: repo,
		Slug:       slug,
		Home:       colony.HomeConfig{Forge: colony.ForgeConfig{Command: []string{missing}}},
	}, nil, ledger, review.ApproveInput{
		TraceID: traceID,
		TaskID:  taskledger.FinalReviewTaskID,
	}, review.WriteOptions{})
	if err == nil || !strings.Contains(err.Error(), "not executable") {
		t.Fatalf("err = %v", err)
	}
}

func TestReconcileMergedCompletesAndRemovesWorktree(t *testing.T) {
	repo, traceID, slug, stub := setupPublishFixture(t)
	ctxColony := colony.Context{
		ColonyRoot: repo,
		Slug:       slug,
		Home:       colony.HomeConfig{Forge: colony.ForgeConfig{Command: []string{stub}}},
	}
	ledger := finalIsolatedLedger(t, traceID)
	if _, err := review.Approve(context.Background(), ctxColony, nil, ledger, review.ApproveInput{
		TraceID: traceID,
		TaskID:  taskledger.FinalReviewTaskID,
	}, review.WriteOptions{}); err != nil {
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
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Tasks[taskledger.FinalReviewTaskID].Status != protocol.TaskStatusCompleted {
		t.Fatalf("status = %q", snap.Tasks[taskledger.FinalReviewTaskID].Status)
	}
	if gitroot.IsInsideWorkTree(worktree.Path(repo, traceID)) {
		t.Fatal("expected worktree removed after forge merge")
	}
}

func TestReconcileClosedKeepsWaitingReview(t *testing.T) {
	repo, traceID, slug, stub := setupPublishFixture(t)
	ctxColony := colony.Context{
		ColonyRoot: repo,
		Slug:       slug,
		Home:       colony.HomeConfig{Forge: colony.ForgeConfig{Command: []string{stub}}},
	}
	ledger := finalIsolatedLedger(t, traceID)
	if _, err := review.Approve(context.Background(), ctxColony, nil, ledger, review.ApproveInput{
		TraceID: traceID,
		TaskID:  taskledger.FinalReviewTaskID,
	}, review.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	closed := `#!/bin/sh
echo '{"protocolVersion":1,"found":true,"number":42,"url":"https://example.test/pr/42","head":"feature/x","base":"main","state":"closed","draft":false}'
`
	if err := os.WriteFile(stub, []byte(closed), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := review.ReconcilePublished(context.Background(), ctxColony, nil, ledger, traceID, review.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Completed {
		t.Fatal("closed PR must not complete the gate")
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Tasks[taskledger.FinalReviewTaskID].Status != protocol.TaskStatusWaitingReview {
		t.Fatalf("status = %q", snap.Tasks[taskledger.FinalReviewTaskID].Status)
	}
}

func TestApproveRootDoesNotSpawnForge(t *testing.T) {
	dir := t.TempDir()
	called := filepath.Join(dir, "called")
	stub := filepath.Join(dir, "forge.sh")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\ntouch "+called+"\necho fail >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	ledger := taskledger.NewMemoryLedger()
	traceID := "trace-root"
	plan, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{
			{TaskID: "task-1", Title: "Cfg", Bee: "hivewright", Review: protocol.TaskReviewRequired},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(plan); err != nil {
		t.Fatal(err)
	}
	proposal, err := protocol.NewEvent(traceID, "hivewright-1", 0, protocol.EventMutation, protocol.MutationPayload{
		Kind:      protocol.MutationCodeProposalRoot,
		TaskID:    "task-1",
		Workspace: protocol.ProposalWorkspaceRoot,
		Summary:   "cfg",
		Diff:      "+yaml",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(proposal); err != nil {
		t.Fatal(err)
	}
	waiting, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind: protocol.TaskEventStatus, TaskID: "task-1", Status: protocol.TaskStatusWaitingReview,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(waiting); err != nil {
		t.Fatal(err)
	}

	repo := t.TempDir()
	writeColonyDelivery(t, repo, colony.DeliveryPullRequest)
	_, err = review.Approve(context.Background(), colony.Context{
		ColonyRoot: repo,
		Slug:       "root-pr",
		Home:       colony.HomeConfig{Forge: colony.ForgeConfig{Command: []string{stub}}},
	}, nil, ledger, review.ApproveInput{TraceID: traceID, TaskID: "task-1"}, review.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(called); err == nil {
		t.Fatal("root approve must not spawn forge")
	}
}

func finalIsolatedLedger(t *testing.T, traceID string) taskledger.Ledger {
	t.Helper()
	ledger := taskledger.NewMemoryLedger()
	plan, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{
			{TaskID: taskledger.FinalReviewTaskID, Title: "Final", Review: protocol.TaskReviewFinal},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(plan); err != nil {
		t.Fatal(err)
	}
	mutation, err := protocol.NewEvent(traceID, "builder-1", 0, protocol.EventMutation, protocol.MutationPayload{
		Kind:      protocol.MutationCodeProposalIsolated,
		TaskID:    "task-1",
		Workspace: protocol.ProposalWorkspaceIsolated,
		Diff:      "+line",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(mutation); err != nil {
		t.Fatal(err)
	}
	waiting, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind: protocol.TaskEventStatus, TaskID: taskledger.FinalReviewTaskID, Status: protocol.TaskStatusWaitingReview,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(waiting); err != nil {
		t.Fatal(err)
	}
	return ledger
}

func setupPublishFixture(t *testing.T) (repo, traceID, slug, stub string) {
	t.Helper()
	repo, _ = clonePublishBare(t)
	slug = "pr-colony"
	stub = writePublishStub(t)
	setupPublishHome(t, repo, slug, stub)
	writeColonyDelivery(t, repo, colony.DeliveryPullRequest)
	traceID = "trace-pr-1"
	entry, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       slug,
		Branch:     "feature/x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entry.Path, "feature.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, entry.Path, "add", "feature.txt")
	runGit(t, entry.Path, "commit", "-m", "feat")
	return repo, traceID, slug, stub
}

func writeColonyDelivery(t *testing.T, repo, delivery string) {
	t.Helper()
	dir := filepath.Join(repo, ".paseka")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf("slug: test\ndefaults:\n  delivery: %s\n", delivery)
	if err := os.WriteFile(filepath.Join(dir, "colony.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setupPublishHome(t *testing.T, repo, slug, stub string) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	homeDir, err := colony.HomeDir(slug)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf("colony_root: %q\nslug: %q\n", repo, slug)
	if stub != "" {
		cfg += fmt.Sprintf("forge:\n  command: [%q]\n", stub)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePublishStub(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "forge.sh")
	state := filepath.Join(dir, "state.json")
	script := `#!/bin/sh
op="$1"
state="` + state + `"
case "$op" in
capabilities) echo '{"protocolVersion":1,"ops":["upsert","get"]}' ;;
get)
  if [ -f "$state" ]; then cat "$state"; else echo '{"protocolVersion":1,"found":false}'; fi
  ;;
upsert)
  echo '{"protocolVersion":1,"found":true,"number":42,"url":"https://example.test/pr/42","head":"feature/x","base":"main","state":"open","draft":false}' | tee "$state"
  ;;
*) echo unknown >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func clonePublishBare(t *testing.T) (clone, bare string) {
	t.Helper()
	src := initPublishRepo(t)
	bare = t.TempDir()
	runGit(t, bare, "init", "--bare")
	runGit(t, src, "remote", "add", "origin", bare)
	runGit(t, src, "push", "-u", "origin", "HEAD:main")
	clone = t.TempDir()
	cmd := exec.Command("git", "clone", "-b", "main", bare, clone)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clone: %v\n%s", err, out)
	}
	runGit(t, clone, "config", "user.email", "test@test.com")
	runGit(t, clone, "config", "user.name", "test")
	return clone, bare
}

func initPublishRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "init")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}
