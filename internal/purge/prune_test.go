package purge_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/homestate"
	"github.com/russ-p/paseka/internal/purge"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/worktree"
)

func TestParseRetention(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"14d", 14 * 24 * time.Hour},
		{"2w", 14 * 24 * time.Hour},
		{"336h", 336 * time.Hour},
		{"90m", 90 * time.Minute},
		{"1.5d", 36 * time.Hour},
	}
	for _, tc := range cases {
		got, err := purge.ParseRetention(tc.in)
		if err != nil {
			t.Fatalf("ParseRetention(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseRetention(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
	for _, bad := range []string{"", "0", "-1d", "14x", "abc"} {
		if _, err := purge.ParseRetention(bad); err == nil {
			t.Fatalf("ParseRetention(%q) expected error", bad)
		}
	}
}

func TestPruneRequiresTarget(t *testing.T) {
	repo := initTestRepo(t)
	slug := setupPurgeHome(t, repo)
	ctx := colony.Context{ColonyRoot: repo, Slug: slug}

	_, err := purge.Prune(ctx, purge.PruneTarget{OlderThan: 14 * 24 * time.Hour})
	if err == nil {
		t.Fatal("expected error with no target selected")
	}
}

func TestPruneRunsByAge(t *testing.T) {
	repo := initTestRepo(t)
	slug := setupPurgeHome(t, repo)

	makeRun(t, repo, "trace-old")
	makeRun(t, repo, "trace-new")
	setTreeModTime(t, filepath.Join(repo, ".paseka", "runs", "trace-old"), time.Now().Add(-30*24*time.Hour))

	ctx := colony.Context{ColonyRoot: repo, Slug: slug}
	target := purge.PruneTarget{Runs: true, OlderThan: 14 * 24 * time.Hour, Now: time.Now()}
	plan, err := purge.Prune(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Runs) != 1 || plan.Runs[0].TraceID != "trace-old" {
		t.Fatalf("plan runs = %+v", plan.Runs)
	}

	res, err := purge.ExecutePrune(ctx, target, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Removed) != 1 {
		t.Fatalf("removed = %+v", res.Removed)
	}
	if _, err := os.Stat(filepath.Join(repo, ".paseka", "runs", "trace-old")); !os.IsNotExist(err) {
		t.Fatalf("old trace still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".paseka", "runs", "trace-new")); err != nil {
		t.Fatalf("recent trace removed: %v", err)
	}
}

func TestPruneWorktreesByAge(t *testing.T) {
	repo := initTestRepo(t)
	slug := setupPurgeHome(t, repo)

	if _, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    "trace-wt",
		Slug:       slug,
	}); err != nil {
		t.Fatal(err)
	}
	wtPath := filepath.Join(repo, ".paseka", "worktrees", "trace-wt")
	old := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(wtPath, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(wtPath, ".git"), old, old); err != nil {
		t.Fatal(err)
	}

	ctx := colony.Context{ColonyRoot: repo, Slug: slug}
	target := purge.PruneTarget{Worktrees: true, OlderThan: 14 * 24 * time.Hour, Now: time.Now()}
	plan, err := purge.Prune(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Worktrees) != 1 || plan.Worktrees[0].TraceID != "trace-wt" {
		t.Fatalf("plan worktrees = %+v", plan.Worktrees)
	}

	res, err := purge.ExecutePrune(ctx, target, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Removed) != 1 {
		t.Fatalf("removed = %+v", res.Removed)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("worktree still exists: %v", err)
	}
	st, err := homestate.LoadState(slug)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Worktrees) != 0 {
		t.Fatalf("state worktrees = %+v", st.Worktrees)
	}
}

func TestPruneKeepsWorktreeWithRecentRuns(t *testing.T) {
	repo := initTestRepo(t)
	slug := setupPurgeHome(t, repo)

	if _, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    "trace-wt",
		Slug:       slug,
	}); err != nil {
		t.Fatal(err)
	}
	wtPath := filepath.Join(repo, ".paseka", "worktrees", "trace-wt")
	old := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(wtPath, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(wtPath, ".git"), old, old); err != nil {
		t.Fatal(err)
	}

	// A recent run for the same trace must protect the worktree from pruning.
	makeRun(t, repo, "trace-wt")

	ctx := colony.Context{ColonyRoot: repo, Slug: slug}
	plan, err := purge.Prune(ctx, purge.PruneTarget{
		Worktrees: true,
		OlderThan: 14 * 24 * time.Hour,
		Now:       time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Worktrees) != 0 {
		t.Fatalf("expected recent runs to protect worktree, got %+v", plan.Worktrees)
	}
}

func TestPruneKeepsDirtyWorktree(t *testing.T) {
	repo := initTestRepo(t)
	slug := setupPurgeHome(t, repo)

	if _, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    "trace-dirty",
		Slug:       slug,
	}); err != nil {
		t.Fatal(err)
	}
	wtPath := filepath.Join(repo, ".paseka", "worktrees", "trace-dirty")
	if err := os.WriteFile(filepath.Join(wtPath, "uncommitted.txt"), []byte("work in progress"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(wtPath, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(wtPath, ".git"), old, old); err != nil {
		t.Fatal(err)
	}

	ctx := colony.Context{ColonyRoot: repo, Slug: slug}
	plan, err := purge.Prune(ctx, purge.PruneTarget{
		Worktrees: true,
		OlderThan: 14 * 24 * time.Hour,
		Now:       time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Worktrees) != 0 {
		t.Fatalf("expected dirty worktree to be protected, got %+v", plan.Worktrees)
	}
}

func TestFormatPrunePlan(t *testing.T) {
	plan := purge.PrunePlan{
		Cutoff: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Runs: []purge.PruneCandidate{{
			TraceID:  "trace-run",
			Path:     "/colony/.paseka/runs/trace-run",
			LastUsed: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC),
		}},
		Worktrees: []purge.PruneCandidate{{
			TraceID:  "trace-wt",
			Path:     "/colony/.paseka/worktrees/trace-wt",
			LastUsed: time.Date(2024, 12, 2, 0, 0, 0, 0, time.UTC),
		}},
		Bus: []string{"trace-bus"},
	}
	out := purge.FormatPrunePlan(plan)
	for _, needle := range []string{"runs (1 traces)", "trace-run", "worktrees (1)", "trace-wt", "bus (1 trace(s))", "trace-bus"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("format prune plan missing %q:\n%s", needle, out)
		}
	}
}

func TestPrunePlanEmpty(t *testing.T) {
	if !purge.PrunePlanEmpty(purge.PrunePlan{}) {
		t.Fatal("expected empty plan")
	}
	if purge.PrunePlanEmpty(purge.PrunePlan{Bus: []string{"trace-bus"}}) {
		t.Fatal("expected non-empty plan")
	}
}

func makeRun(t *testing.T, repo, traceID string) {
	t.Helper()
	d := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: "agent-a"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d.ResultPath(), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setTreeModTime(t *testing.T, root string, mod time.Time) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(path, mod, mod)
	})
	if err != nil {
		t.Fatal(err)
	}
}
