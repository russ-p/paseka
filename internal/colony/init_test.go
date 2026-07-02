package colony_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/colony"
)

func TestComputeSlugFromRemote(t *testing.T) {
	got := colony.ComputeSlug("/tmp/x", "https://github.com/acme/api.git")
	if got != "acme-api" {
		t.Fatalf("got %q, want acme-api", got)
	}
}

func TestComputeSlugFromDir(t *testing.T) {
	got := colony.ComputeSlug("/home/dev/paseka", "")
	if got != "paseka" {
		t.Fatalf("got %q", got)
	}
}

func TestInitScaffold(t *testing.T) {
	repo := initTestRepo(t)

	res, err := colony.Init(colony.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	if res.Slug == "" {
		t.Fatal("expected slug")
	}

	for _, rel := range []string{
		".paseka/colony.yaml",
		".paseka/.gitignore",
		".paseka/bees/builder.yaml",
		".paseka/bees/scout.yaml",
		".paseka/prompts/builder.md",
		".paseka/prompts/scout.md",
	} {
		if _, err := os.Stat(filepath.Join(repo, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}

	gitignore, err := os.ReadFile(filepath.Join(repo, ".paseka", ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"worktrees/", "runs/", "*.local.yaml"} {
		if !strings.Contains(string(gitignore), line) {
			t.Fatalf("gitignore missing %q", line)
		}
	}

	if _, err := os.Stat(res.HomeDir); err != nil {
		t.Fatalf("home dir: %v", err)
	}

	res2, err := colony.Init(colony.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Created) != 0 {
		t.Fatalf("second init should not create project files, got %v", res2.Created)
	}
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

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
