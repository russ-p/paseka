package colonyinit_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/colonyinit"
)

func TestInitScaffold(t *testing.T) {
	repo := initTestRepo(t)

	res, err := colonyinit.Init(colonyinit.InitOptions{StartDir: repo})
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
		".paseka/bees/hivewright.yaml",
		".paseka/prompts/builder.md",
		".paseka/prompts/scout.md",
		".paseka/prompts/hivewright-system.md",
		".paseka/prompts/hivewright-task.md",
		".paseka/prompts/_partials/builder-intent-general.md",
		".paseka/prompts/_partials/builder-intent-feature.md",
		".paseka/prompts/_partials/scout-intent-survey.md",
		".paseka/prompts/_partials/scout-intent-intake.md",
		".paseka/prompts/_partials/scout-emit-intake.md",
		".paseka/prompts/_partials/emit-howto.md",
		".paseka/prompts/_partials/emit-insight.md",
		".paseka/prompts/_partials/emit-signal.md",
		".paseka/prompts/_partials/emit-verification.md",
		".paseka/prompts/_partials/emit-task-completed.md",
		".paseka/cues/feature.yaml",
		".paseka/cues/hotfix.yaml",
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

	res2, err := colonyinit.Init(colonyinit.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Created) != 0 {
		t.Fatalf("second init should not create project files, got %v", res2.Created)
	}
}

func TestNormalizeInitAdapter(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "cursor"},
		{"cursor", "cursor"},
		{"pi", "pi"},
		{"PI", "pi"},
		{" claude ", "cursor"},
		{"unknown", "cursor"},
	}
	for _, tc := range tests {
		if got := colonyinit.NormalizeInitAdapter(tc.in); got != tc.want {
			t.Errorf("NormalizeInitAdapter(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestInitScaffoldWithPiAdapter(t *testing.T) {
	repo := initTestRepo(t)
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	res, err := colonyinit.Init(colonyinit.InitOptions{StartDir: repo, Adapter: "pi"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Adapter != "pi" {
		t.Fatalf("adapter = %q, want pi", res.Adapter)
	}

	scout, err := os.ReadFile(filepath.Join(repo, ".paseka", "bees", "scout.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(scout), "adapter: pi") {
		t.Fatalf("scout bee should use pi adapter:\n%s", scout)
	}
	if !strings.Contains(string(scout), "output_format: json") {
		t.Fatalf("scout bee should use json output:\n%s", scout)
	}

	builder, err := os.ReadFile(filepath.Join(repo, ".paseka", "bees", "builder.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(builder), "adapter: pi") {
		t.Fatalf("builder bee should use pi adapter:\n%s", builder)
	}

	hivewright, err := os.ReadFile(filepath.Join(repo, ".paseka", "bees", "hivewright.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(hivewright), "adapter: pi") {
		t.Fatalf("hivewright bee should use pi adapter:\n%s", hivewright)
	}

	piYAML := filepath.Join(res.HomeDir, "adapters", "pi.yaml")
	if _, err := os.Stat(piYAML); err != nil {
		t.Fatalf("missing pi adapter config: %v", err)
	}

	cfg, err := os.ReadFile(filepath.Join(res.HomeDir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfg), "pi: {}") {
		t.Fatalf("home config should reference pi adapter:\n%s", cfg)
	}
}

func TestInitUnsupportedAdapterFallsBackToCursor(t *testing.T) {
	repo := initTestRepo(t)
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	res, err := colonyinit.Init(colonyinit.InitOptions{StartDir: repo, Adapter: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Adapter != "cursor" {
		t.Fatalf("adapter = %q, want cursor", res.Adapter)
	}

	scout, err := os.ReadFile(filepath.Join(repo, ".paseka", "bees", "scout.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(scout), "adapter: cursor") {
		t.Fatalf("scout bee should fall back to cursor:\n%s", scout)
	}

	if _, err := os.Stat(filepath.Join(res.HomeDir, "adapters", "pi.yaml")); !os.IsNotExist(err) {
		t.Fatalf("pi.yaml should not be scaffolded for cursor init: %v", err)
	}
}

func initTestRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
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
