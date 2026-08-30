package worktree_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/gitroot"
	"github.com/paseka/paseka/internal/homestate"
	"github.com/paseka/paseka/internal/worktree"
)

func TestListRegistryAndGit(t *testing.T) {
	repo := initTestRepo(t)
	slug := "list-colony"
	setupHome(t, repo, slug)
	traceID := "trace-list"
	entry, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       slug,
	})
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := worktree.List(repo, slug)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 1 {
		t.Fatalf("len = %d %+v", len(snaps), snaps)
	}
	if snaps[0].TraceID != traceID || snaps[0].Path != entry.Path {
		t.Fatalf("snap = %+v", snaps[0])
	}
	if snaps[0].Branch == "" || snaps[0].BaseSHA == "" {
		t.Fatalf("missing branch/sha: %+v", snaps[0])
	}

	if err := gitroot.DeleteBranch(repo, entry.Branch); err == nil || !errors.Is(err, gitroot.ErrRefused) {
		t.Fatalf("delete worktree branch: %v", err)
	}
}

func TestPruneOrphanRegistryAndKeepLive(t *testing.T) {
	repo := initTestRepo(t)
	slug := "prune-colony"
	setupHome(t, repo, slug)
	traceID := "trace-live"
	if _, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       slug,
	}); err != nil {
		t.Fatal(err)
	}

	staleID := "trace-gone"
	stalePath := filepath.Join(repo, ".paseka", "worktrees", staleID)
	if err := homestate.RegisterWorktree(slug, homestate.WorktreeEntry{
		TraceID: staleID,
		Path:    stalePath,
		BaseSHA: "deadbeef",
		Branch:  "paseka/" + staleID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stalePath, 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := worktree.PruneOrphans(repo, slug)
	if err != nil {
		t.Fatal(err)
	}
	foundStale := false
	for _, id := range res.Unregistered {
		if id == staleID {
			foundStale = true
		}
	}
	if !foundStale {
		t.Fatalf("expected unregister %s: %+v", staleID, res)
	}
	st, err := homestate.LoadState(slug)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range st.Worktrees {
		if w.TraceID == staleID {
			t.Fatal("stale registry row still present")
		}
		if w.TraceID == traceID && !gitroot.IsInsideWorkTree(w.Path) {
			t.Fatal("live worktree was removed")
		}
	}
	if !gitroot.IsInsideWorkTree(worktree.Path(repo, traceID)) {
		t.Fatal("live checkout must remain")
	}
}

func TestMergeSkipsFailingCommitMsgHook(t *testing.T) {
	repo, traceID, slug := setupMergeFixture(t)
	hook := filepath.Join(repo, ".git", "hooks", "commit-msg")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
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
	if res.CommitSHA == "" {
		t.Fatal("expected merge commit despite failing commit-msg hook")
	}
}
