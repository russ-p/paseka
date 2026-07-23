package worktree_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/gitroot"
	"github.com/paseka/paseka/internal/worktree"
)

func TestMergeComposedMessageWithBody(t *testing.T) {
	repo, traceID, slug := setupMergeFixture(t)

	message := "paseka: merge trace " + traceID + "\n\nImplemented OAuth callback and added focused tests"
	res, err := worktree.Merge(worktree.MergeOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       slug,
		Message:    message,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.CommitSHA == "" {
		t.Fatal("expected merge commit")
	}

	fullMessage := gitOutput(t, repo, "log", "-1", "--format=%B")
	if !strings.Contains(fullMessage, "paseka: merge trace "+traceID) {
		t.Fatalf("commit subject missing: %q", fullMessage)
	}
	if !strings.Contains(fullMessage, "Implemented OAuth callback and added focused tests") {
		t.Fatalf("commit body missing: %q", fullMessage)
	}
}

func TestMergeCleanRoot(t *testing.T) {
	repo, traceID, slug := setupMergeFixture(t)

	res, err := worktree.Merge(worktree.MergeOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       slug,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.CommitSHA == "" {
		t.Fatal("expected merge commit")
	}
	if res.StashOutcome != worktree.StashOutcomeNone {
		t.Fatalf("stash outcome = %q, want none", res.StashOutcome)
	}
	if gitroot.IsInsideWorkTree(worktree.Path(repo, traceID)) {
		t.Fatal("expected worktree removed")
	}
	if !fileExists(filepath.Join(repo, "feature.txt")) {
		t.Fatal("expected merged feature.txt on colony root")
	}
}

func TestMergeDirtyTrackedRestoresLocalChanges(t *testing.T) {
	repo, traceID, slug := setupMergeFixture(t)

	localPath := filepath.Join(repo, "local-wip.txt")
	if err := os.WriteFile(localPath, []byte("tracked wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "local-wip.txt")

	res, err := worktree.Merge(worktree.MergeOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       slug,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.StashOutcome != worktree.StashOutcomeRestored {
		t.Fatalf("stash outcome = %q, want restored", res.StashOutcome)
	}
	if !fileExists(localPath) {
		t.Fatal("expected local tracked file restored after merge")
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "tracked wip\n" {
		t.Fatalf("restored content = %q", data)
	}
}

func TestMergeDirtyRootMergeFailureLeavesStash(t *testing.T) {
	repo, traceID, slug := setupMergeFixture(t)
	wtPath := worktree.Path(repo, traceID)

	readmeMain := filepath.Join(repo, "README.md")
	if err := os.WriteFile(readmeMain, []byte("# main version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "main readme change")

	readmeWT := filepath.Join(wtPath, "README.md")
	if err := os.WriteFile(readmeWT, []byte("# worktree version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, wtPath, "add", "README.md")
	runGit(t, wtPath, "commit", "-m", "worktree readme change")

	localPath := filepath.Join(repo, "local-wip.txt")
	if err := os.WriteFile(localPath, []byte("tracked wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "local-wip.txt")

	res, err := worktree.Merge(worktree.MergeOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       slug,
	})
	if err == nil {
		t.Fatal("expected merge error")
	}
	if res.StashOutcome != worktree.StashOutcomeLeftOnFailure {
		t.Fatalf("stash outcome = %q, want left_on_failure", res.StashOutcome)
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "git stash list") || !strings.Contains(errMsg, "git stash pop") {
		t.Fatalf("error = %q, want stash recovery guidance", errMsg)
	}
	if !strings.Contains(errMsg, "autostashed") {
		t.Fatalf("error = %q, want autostash mention", errMsg)
	}

	stashList := gitOutput(t, repo, "stash", "list")
	if strings.TrimSpace(stashList) == "" {
		t.Fatal("expected stash entry after failed merge")
	}
	if !strings.Contains(stashList, traceID) {
		t.Fatalf("stash list = %q, want trace id in autostash message", stashList)
	}
	if fileExists(localPath) {
		t.Fatal("expected stashed local file absent from working tree")
	}
}

func TestMergeDirtyRootPopConflictKeepsMergeSuccess(t *testing.T) {
	repo := initTestRepo(t)
	slug := "merge-colony"
	setupHome(t, repo, slug)
	traceID := "trace-merge-pop-conflict"

	sharedMain := filepath.Join(repo, "shared.txt")
	if err := os.WriteFile(sharedMain, []byte("base version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "shared.txt")
	runGit(t, repo, "commit", "-m", "main shared")

	entry, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       slug,
	})
	if err != nil {
		t.Fatal(err)
	}
	wtPath := entry.Path

	if err := os.WriteFile(filepath.Join(wtPath, "feature.txt"), []byte("new feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sharedWT := filepath.Join(wtPath, "shared.txt")
	if err := os.WriteFile(sharedWT, []byte("merged version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, wtPath, "add", "feature.txt", "shared.txt")
	runGit(t, wtPath, "commit", "-m", "worktree changes")

	if err := os.WriteFile(sharedMain, []byte("local wip version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "shared.txt")

	res, err := worktree.Merge(worktree.MergeOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       slug,
	})
	if err != nil {
		t.Fatalf("expected merge success despite pop conflict: %v", err)
	}
	if res.CommitSHA == "" {
		t.Fatal("expected merge commit")
	}
	if res.StashOutcome != worktree.StashOutcomeRestoreConflicted {
		t.Fatalf("stash outcome = %q, want restore_conflicted", res.StashOutcome)
	}
	if gitroot.IsInsideWorkTree(worktree.Path(repo, traceID)) {
		t.Fatal("expected worktree removed")
	}
	if !fileExists(filepath.Join(repo, "feature.txt")) {
		t.Fatal("expected merged feature.txt on colony root")
	}

	unmerged := gitOutput(t, repo, "diff", "--name-only", "--diff-filter=U")
	if strings.TrimSpace(unmerged) == "" {
		t.Fatal("expected unmerged files after conflicting stash pop")
	}
	if !strings.Contains(unmerged, "shared.txt") {
		t.Fatalf("unmerged files = %q, want shared.txt", unmerged)
	}
}

func TestMergeDirtyUntrackedRestoresLocalChanges(t *testing.T) {
	repo, traceID, slug := setupMergeFixture(t)

	localPath := filepath.Join(repo, "scratch.txt")
	if err := os.WriteFile(localPath, []byte("untracked wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := worktree.Merge(worktree.MergeOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       slug,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.StashOutcome != worktree.StashOutcomeRestored {
		t.Fatalf("stash outcome = %q, want restored", res.StashOutcome)
	}
	if !fileExists(localPath) {
		t.Fatal("expected untracked local file restored after merge")
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "untracked wip\n" {
		t.Fatalf("restored content = %q", data)
	}
}

func setupMergeFixture(t *testing.T) (repo, traceID, slug string) {
	t.Helper()
	repo = initTestRepo(t)
	slug = "merge-colony"
	setupHome(t, repo, slug)
	traceID = "trace-merge-autostash"

	entry, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       slug,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(entry.Path, "feature.txt"), []byte("new feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, entry.Path, "add", "feature.txt")
	runGit(t, entry.Path, "commit", "-m", "add feature")
	return repo, traceID, slug
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
