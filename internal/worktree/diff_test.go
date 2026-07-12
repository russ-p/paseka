package worktree_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/worktree"
)

func TestMergeDiffThreeDot(t *testing.T) {
	repo := initTestRepo(t)
	slug := "diff-colony"
	setupHome(t, repo, slug)

	entry, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    "trace-merge",
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

	res, err := worktree.MergeDiff(worktree.MergeDiffOptions{
		ColonyRoot: repo,
		TraceID:    "trace-merge",
		Slug:       slug,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Missing {
		t.Fatal("expected worktree branch to exist")
	}
	if res.Empty {
		t.Fatal("expected non-empty diff")
	}
	if res.DefaultBranch == "" || res.Branch == "" {
		t.Fatalf("branches = %q / %q", res.DefaultBranch, res.Branch)
	}
	if res.BaseSHA == "" || res.HeadSHA == "" {
		t.Fatalf("shas = %q / %q", res.BaseSHA, res.HeadSHA)
	}
	if res.BaseSHA == res.HeadSHA {
		t.Fatal("expected different base and head SHAs")
	}
	if !strings.Contains(res.Stat, "feature.txt") {
		t.Fatalf("stat = %q", res.Stat)
	}
	if !strings.Contains(res.Diff, "feature.txt") || !strings.Contains(res.Diff, "+new feature") {
		t.Fatalf("diff = %q", res.Diff)
	}
}

func TestMergeDiffMissingBranch(t *testing.T) {
	repo := initTestRepo(t)
	res, err := worktree.MergeDiff(worktree.MergeDiffOptions{
		ColonyRoot: repo,
		TraceID:    "trace-missing",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Missing {
		t.Fatalf("expected missing branch, got %+v", res)
	}
}
