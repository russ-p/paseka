package worktree_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/gitroot"
	"github.com/paseka/paseka/internal/worktree"
)

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
