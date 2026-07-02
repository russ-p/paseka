package runtime_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/runtime"
)

func TestBeeRunUsesWorktreeForBuilder(t *testing.T) {
	repo := initBeeRunRepo(t)
	slug := setupBeeRunHome(t, repo)

	rec := &recordingAdapter{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)

	res, err := d.BeeRun(context.Background(), runtime.BeeRunRequest{
		StartDir: repo,
		Bee:      "builder",
		TraceID:  "trace-build-1",
		Task:     "add endpoint",
	})
	if err != nil {
		t.Fatal(err)
	}

	wantWT := filepath.Join(repo, ".paseka", "worktrees", "trace-build-1")
	if res.Workspace != wantWT {
		t.Fatalf("workspace = %q, want %q", res.Workspace, wantWT)
	}
	if rec.lastReq.Workspace != wantWT {
		t.Fatalf("adapter workspace = %q", rec.lastReq.Workspace)
	}
	if rec.lastReq.Params.Binary != "agent" {
		t.Fatalf("binary = %q", rec.lastReq.Params.Binary)
	}

	st, err := colony.LoadState(slug)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Worktrees) != 1 {
		t.Fatalf("worktrees = %+v", st.Worktrees)
	}
}

func TestBeeRunScoutUsesColonyRoot(t *testing.T) {
	repo := initBeeRunRepo(t)
	setupBeeRunHome(t, repo)

	rec := &recordingAdapter{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)

	res, err := d.BeeRun(context.Background(), runtime.BeeRunRequest{
		StartDir: repo,
		Bee:      "scout",
		Task:     "survey code",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Workspace != repo {
		t.Fatalf("workspace = %q, want %q", res.Workspace, repo)
	}
	if res.TraceID == "" {
		t.Fatal("expected generated trace id")
	}
}

func initBeeRunRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "test")

	colonyFiles := map[string]string{
		".paseka/colony.yaml": `slug: test-colony
defaults:
  prompt_template: default.md
`,
		".paseka/bees/scout.yaml": `role: scout
adapter: cursor
prompt_template: scout.md
worktree: false
`,
		".paseka/bees/builder.yaml": `role: builder
adapter: cursor
prompt_template: builder.md
worktree: true
`,
		".paseka/prompts/scout.md":   `Scout: {{.Task}}`,
		".paseka/prompts/builder.md": `Builder: {{.Task}}`,
		".paseka/prompts/default.md": `Default: {{.Task}}`,
	}
	for path, content := range colonyFiles {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")
	return dir
}

func setupBeeRunHome(t *testing.T, repo string) string {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	slug := "test-colony"

	homeDir, err := colony.HomeDir(slug)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(homeDir, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf("colony_root: %q\nslug: %q\n", repo, slug)
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "adapters", "cursor.yaml"), []byte("binary: agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return slug
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
