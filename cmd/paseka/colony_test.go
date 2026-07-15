package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestColonyTopologyCLIStdout(t *testing.T) {
	repo := initTopologyFixtureRepoCLI(t)
	setupCLIHome(t, repo)

	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"colony", "topology", "-C", repo})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	got := strings.TrimRight(out.String(), "\n")
	want := readTopologyGoldenMermaid(t)
	if got != want {
		t.Fatalf("mermaid mismatch:\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestColonyTopologyCLIOutFile(t *testing.T) {
	repo := initTopologyFixtureRepoCLI(t)
	setupCLIHome(t, repo)

	outPath := filepath.Join(t.TempDir(), "topology.mmd")
	root := newRoot()
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	root.SetArgs([]string{"colony", "topology", "-C", repo, "--out", outPath})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimRight(string(raw), "\n")
	want := readTopologyGoldenMermaid(t)
	if got != want {
		t.Fatalf("mermaid file mismatch:\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func readTopologyGoldenMermaid(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "internal", "colony", "testdata", "topology-fixture", "topology.golden.mermaid")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimRight(string(raw), "\n")
}

func initTopologyFixtureRepoCLI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGitCLI(t, dir, "init")
	runGitCLI(t, dir, "config", "user.email", "test@test.com")
	runGitCLI(t, dir, "config", "user.name", "test")

	src := filepath.Join("..", "..", "internal", "colony", "testdata", "topology-fixture", ".paseka")
	dst := filepath.Join(dir, ".paseka")
	cmd := exec.Command("cp", "-a", src, dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("copy topology fixture: %v\n%s", err, out)
	}
	runGitCLI(t, dir, "add", ".paseka")
	runGitCLI(t, dir, "commit", "-m", "topology fixture")
	return dir
}

func setupCLIHome(t *testing.T, repo string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	homeDir := filepath.Join(home, "paseka", "topology-fixture")
	if err := os.MkdirAll(filepath.Join(homeDir, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "colony_root: " + repo + "\nslug: topology-fixture\n"
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

func runGitCLI(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
}
