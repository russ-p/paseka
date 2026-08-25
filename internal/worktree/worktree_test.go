package worktree_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/gitroot"
	"github.com/paseka/paseka/internal/homestate"
	"github.com/paseka/paseka/internal/worktree"
)

func TestEnsureCreatesWorktree(t *testing.T) {
	repo := initTestRepo(t)
	slug := "test-colony"
	homeDir := setupHome(t, repo, slug)

	entry, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    "trace-abc",
		Slug:       slug,
	})
	if err != nil {
		t.Fatal(err)
	}

	wantPath := filepath.Join(repo, ".paseka", "worktrees", "trace-abc")
	if entry.Path != wantPath {
		t.Fatalf("path = %q, want %q", entry.Path, wantPath)
	}
	if !gitroot.IsInsideWorkTree(entry.Path) {
		t.Fatal("expected git worktree at path")
	}
	if entry.BaseSHA == "" {
		t.Fatal("expected base SHA")
	}

	entry2, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    "trace-abc",
		Slug:       slug,
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry2.Path != entry.Path {
		t.Fatalf("reuse path = %q, want %q", entry2.Path, entry.Path)
	}

	st, err := homestate.LoadState(slug)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Worktrees) != 1 || st.Worktrees[0].TraceID != "trace-abc" {
		t.Fatalf("state worktrees = %+v", st.Worktrees)
	}

	_ = homeDir
}

func TestEnsureCreatesNamedBranch(t *testing.T) {
	repo := initTestRepo(t)
	slug := "named-colony"
	setupHome(t, repo, slug)

	entry, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    "trace-feature",
		Slug:       slug,
		Branch:     "feature/live-bees",
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.Branch != "feature/live-bees" {
		t.Fatalf("branch = %q", entry.Branch)
	}
	got := gitBranch(t, entry.Path)
	if got != "feature/live-bees" {
		t.Fatalf("HEAD branch = %q", got)
	}
}

func TestApplyBranchInsightRenames(t *testing.T) {
	repo := initTestRepo(t)
	slug := "rename-colony"
	setupHome(t, repo, slug)
	traceID := "trace-rename"

	_, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       slug,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := worktree.ApplyBranchInsight(worktree.ApplyBranchOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       slug,
		Branch:     "hotfix/windows-path",
	}); err != nil {
		t.Fatal(err)
	}

	path := worktree.Path(repo, traceID)
	got := gitBranch(t, path)
	if got != "hotfix/windows-path" {
		t.Fatalf("HEAD branch = %q", got)
	}
	st, err := homestate.LoadState(slug)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Worktrees) != 1 || st.Worktrees[0].Branch != "hotfix/windows-path" {
		t.Fatalf("registry = %+v", st.Worktrees)
	}
}

func TestEnsureReuseKeepsNamedBranchWithoutInsight(t *testing.T) {
	repo := initTestRepo(t)
	slug := "keep-named"
	setupHome(t, repo, slug)
	traceID := "trace-keep"

	_, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       slug,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := worktree.ApplyBranchInsight(worktree.ApplyBranchOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       slug,
		Branch:     "feature/keep-me",
	}); err != nil {
		t.Fatal(err)
	}

	entry, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       slug,
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.Branch != "feature/keep-me" {
		t.Fatalf("branch = %q, want feature/keep-me (must not revert to default)", entry.Branch)
	}
	if gitBranch(t, entry.Path) != "feature/keep-me" {
		t.Fatalf("HEAD = %q", gitBranch(t, entry.Path))
	}
}

func TestApplyBranchInsightCollisionLeavesOldName(t *testing.T) {
	repo := initTestRepo(t)
	slug := "rename-collision"
	setupHome(t, repo, slug)
	traceID := "trace-collide"

	entry, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       slug,
	})
	if err != nil {
		t.Fatal(err)
	}
	old := gitBranch(t, entry.Path)
	runGit(t, repo, "branch", "feature/taken")

	err = worktree.ApplyBranchInsight(worktree.ApplyBranchOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       slug,
		Branch:     "feature/taken",
	})
	if err == nil {
		t.Fatal("expected collision error")
	}
	if gitBranch(t, entry.Path) != old {
		t.Fatalf("HEAD = %q, want %q", gitBranch(t, entry.Path), old)
	}
	st, err := homestate.LoadState(slug)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Worktrees) != 1 || st.Worktrees[0].Branch != old {
		t.Fatalf("registry = %+v, want branch %q", st.Worktrees, old)
	}
}

func TestMergeDiffUsesRenamedBranch(t *testing.T) {
	repo := initTestRepo(t)
	slug := "diff-renamed"
	setupHome(t, repo, slug)
	traceID := "trace-diff-rename"

	entry, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       slug,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entry.Path, "feature.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, entry.Path, "add", "feature.txt")
	runGit(t, entry.Path, "commit", "-m", "add feature")
	if err := worktree.ApplyBranchInsight(worktree.ApplyBranchOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       slug,
		Branch:     "feature/renamed-diff",
	}); err != nil {
		t.Fatal(err)
	}

	res, err := worktree.MergeDiff(worktree.MergeDiffOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       slug,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Branch != "feature/renamed-diff" {
		t.Fatalf("merge-diff branch = %q", res.Branch)
	}
	if res.Missing || res.Empty {
		t.Fatalf("unexpected merge-diff result %+v", res)
	}
}

func TestEnsureBranchCollisionFails(t *testing.T) {
	repo := initTestRepo(t)
	slug := "collision-colony"
	setupHome(t, repo, slug)
	runGit(t, repo, "branch", "feature/collision")

	_, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    "trace-collision",
		Slug:       slug,
		Branch:     "feature/collision",
	})
	if err == nil {
		t.Fatal("expected collision error")
	}
}

func gitBranch(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".paseka/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "README.md", ".gitignore")
	runGit(t, dir, "commit", "-m", "init")
	return dir
}

func setupHome(t *testing.T, repo, slug string) string {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)

	homeDir, err := colony.HomeDir(slug)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(homeDir, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := []byte(fmt.Sprintf("colony_root: %q\nslug: %q\n", repo, slug))
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), cfg, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return homeDir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
