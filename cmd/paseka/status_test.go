package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"time"

	"github.com/paseka/paseka/internal/hiveview"
	"github.com/paseka/paseka/internal/homestate"
	"github.com/paseka/paseka/internal/runtime"
)

func TestStatusCLIStoppedRuntimeExitZero(t *testing.T) {
	repo := initStatusFixtureRepo(t)
	setupStatusCLIHome(t, repo, "")

	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"status", "-C", repo})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "stopped") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestStatusCLIJSONSchemaVersion(t *testing.T) {
	repo := initStatusFixtureRepo(t)
	setupStatusCLIHome(t, repo, "")

	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"status", "-C", repo, "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	var snap hiveview.ColonySnapshot
	if err := json.Unmarshal(out.Bytes(), &snap); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if snap.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d", snap.SchemaVersion)
	}
	if snap.Attention.WaitingReview == nil {
		t.Fatal("waitingReview should be present")
	}
	if snap.Agents.Items == nil {
		t.Fatal("agents.items should be present")
	}
	if strings.Contains(out.String(), `"items": null`) {
		t.Fatalf("agents.items must not be json null:\n%s", out.String())
	}
}

func TestStatusCLINotAColony(t *testing.T) {
	dir := t.TempDir()
	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"status", "-C", dir})
	if err := root.Execute(); err == nil {
		t.Fatalf("expected error, got output: %s", out.String())
	}
}

func TestStatusCLICheckFailsWhenStopped(t *testing.T) {
	repo := initStatusFixtureRepo(t)
	setupStatusCLIHome(t, repo, "")

	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"status", "-C", repo, "--check"})
	if err := root.Execute(); err == nil {
		t.Fatalf("expected check failure, output: %s", out.String())
	}
	got := out.String()
	if got == "" {
		t.Fatal("expected snapshot printed before check failure")
	}
	if strings.Contains(got, "Usage:") {
		t.Fatalf("check failure should not dump cobra usage:\n%s", got)
	}
}

func TestStatusCLICheckSucceedsWithAttentionButAliveRuntime(t *testing.T) {
	repo := initStatusFixtureRepo(t)
	setupStatusCLIHome(t, repo, "")
	if err := homestate.RegisterRuntime("topology-fixture", homestate.RuntimeEntry{
		PID:       os.Getpid(),
		StartedAt: time.Now().UTC(),
		Status:    runtime.RuntimeStatusRunning,
	}); err != nil {
		t.Fatal(err)
	}

	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"status", "-C", repo, "--check"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
}

func TestStatusCLICheckFailsWhenNATSConfiguredDown(t *testing.T) {
	repo := initStatusFixtureRepo(t)
	setupStatusCLIHome(t, repo, "nats://127.0.0.1:59999")
	if err := homestate.RegisterRuntime("topology-fixture", homestate.RuntimeEntry{
		PID:       os.Getpid(),
		StartedAt: time.Now().UTC(),
		Status:    runtime.RuntimeStatusRunning,
	}); err != nil {
		t.Fatal(err)
	}

	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"status", "-C", repo, "--check"})
	if err := root.Execute(); err == nil {
		t.Fatalf("expected check failure, output: %s", out.String())
	}
}

func initStatusFixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runStatusGit(t, dir, "init")
	runStatusGit(t, dir, "config", "user.email", "test@test.com")
	runStatusGit(t, dir, "config", "user.name", "test")

	src := filepath.Join("..", "..", "internal", "colony", "testdata", "topology-fixture", ".paseka")
	dst := filepath.Join(dir, ".paseka")
	cmd := exec.Command("cp", "-a", src, dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("copy fixture: %v\n%s", err, out)
	}
	runStatusGit(t, dir, "add", ".paseka")
	runStatusGit(t, dir, "commit", "-m", "status fixture")
	return dir
}

func setupStatusCLIHome(t *testing.T, repo, natsURL string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	homeDir := filepath.Join(home, "paseka", "topology-fixture")
	if err := os.MkdirAll(filepath.Join(homeDir, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "colony_root: " + repo + "\nslug: topology-fixture\n"
	if natsURL != "" {
		cfg += "nats:\n  url: " + natsURL + "\n"
	}
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "adapters", "cursor.yaml"), []byte("binary: agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runStatusGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}
