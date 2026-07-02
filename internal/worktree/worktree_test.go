package worktree_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/gitroot"
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

	st, err := colony.LoadState(slug)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Worktrees) != 1 || st.Worktrees[0].TraceID != "trace-abc" {
		t.Fatalf("state worktrees = %+v", st.Worktrees)
	}

	_ = homeDir
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
	runGit(t, dir, "add", "README.md")
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
