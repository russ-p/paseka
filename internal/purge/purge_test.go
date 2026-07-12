package purge_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/purge"
)

func TestPurgeBusRequiresTrace(t *testing.T) {
	repo := initTestRepo(t)
	slug := setupPurgeHome(t, repo)
	ctx := colony.Context{ColonyRoot: repo, Slug: slug}

	_, err := purge.Plan(ctx, colony.PurgeTarget{Bus: true})
	if err == nil || !strings.Contains(err.Error(), "--trace is required with --bus") {
		t.Fatalf("plan error = %v", err)
	}
	_, err = purge.Execute(ctx, colony.PurgeTarget{Bus: true})
	if err == nil || !strings.Contains(err.Error(), "--trace is required with --bus") {
		t.Fatalf("purge error = %v", err)
	}
}

func TestPurgeBusRequiresNATS(t *testing.T) {
	repo := initTestRepo(t)
	slug := setupPurgeHome(t, repo)
	ctx := colony.Context{
		ColonyRoot: repo,
		Slug:       slug,
		Home:       colony.HomeConfig{ColonyRoot: repo, Slug: slug},
	}

	_, err := purge.Plan(ctx, colony.PurgeTarget{Bus: true, TraceID: "trace-1"})
	if err == nil || !strings.Contains(err.Error(), "nats url not configured") {
		t.Fatalf("plan error = %v", err)
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

func setupPurgeHome(t *testing.T, repo string) string {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)

	slug := "purge-test"
	homeDir, err := colony.HomeDir(slug)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(homeDir, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := []byte("colony_root: " + repo + "\nslug: " + slug + "\n")
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), cfg, 0o600); err != nil {
		t.Fatal(err)
	}
	return slug
}
