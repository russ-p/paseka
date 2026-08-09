package adapters_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
)

func TestAttributableDiffExcludesPreExistingDirty(t *testing.T) {
	repo := initGitRepo(t)

	readme := filepath.Join(repo, "README.md")
	if err := os.WriteFile(readme, []byte("base\npre-existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	baseline, err := adapters.CaptureWorkspaceBaseline(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(baseline.FileHashes) != 1 {
		t.Fatalf("baseline files = %v, want README.md dirty", baseline.FileHashes)
	}

	other := filepath.Join(repo, "tracked.txt")
	if err := os.WriteFile(other, []byte("tracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "tracked.txt")
	runGit(t, repo, "commit", "-m", "add tracked")

	if err := os.WriteFile(other, []byte("tracked\nrun change\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	diff, err := adapters.AttributableDiff(ctx, repo, baseline)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(diff, "pre-existing") {
		t.Fatalf("attributable diff must not include pre-existing README changes: %q", diff)
	}
	if !strings.Contains(diff, "tracked.txt") {
		t.Fatalf("attributable diff missing tracked.txt: %q", diff)
	}
}

func TestAttributableDiffEmptyWhenOnlyPreExistingDirty(t *testing.T) {
	repo := initGitRepo(t)

	readme := filepath.Join(repo, "README.md")
	if err := os.WriteFile(readme, []byte("base\npre-existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	baseline, err := adapters.CaptureWorkspaceBaseline(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}

	diff, err := adapters.AttributableDiff(ctx, repo, baseline)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(diff) != "" {
		t.Fatalf("expected empty attributable diff, got %q", diff)
	}
}

// Sector workspaces are subdirectory cwds; git still reports repo-root-relative paths.
func TestAttributableDiffFromSectorWorkspace(t *testing.T) {
	repo := initGitRepo(t)
	sector := filepath.Join(repo, "fizman-server")
	if err := os.MkdirAll(sector, 0o755); err != nil {
		t.Fatal(err)
	}
	tracked := filepath.Join(sector, "Report.java")
	if err := os.WriteFile(tracked, []byte("class Report {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "fizman-server/Report.java")
	runGit(t, repo, "commit", "-m", "add sector file")

	ctx := context.Background()
	baseline, err := adapters.CaptureWorkspaceBaseline(ctx, sector)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(tracked, []byte("class Report { int x; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "fizman-server/Report.java")

	diff, err := adapters.AttributableDiff(ctx, sector, baseline)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "fizman-server/Report.java") {
		t.Fatalf("attributable diff missing sector file: %q", diff)
	}
	if !strings.Contains(diff, "int x") {
		t.Fatalf("attributable diff missing content: %q", diff)
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "bee@test.local")
	runGit(t, repo, "config", "user.name", "Bee")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "init")
	return repo
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}
