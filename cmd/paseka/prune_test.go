package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/runs"
)

func TestPruneCLIRemovesOldRuns(t *testing.T) {
	repo := initTopologyFixtureRepoCLI(t)
	setupCLIHome(t, repo)

	d := runs.Dir{ColonyRoot: repo, TraceID: "trace-cli-old", AgentID: "agent-a"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d.ResultPath(), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	setCLITreeModTime(t, filepath.Join(repo, ".paseka", "runs", "trace-cli-old"), time.Now().Add(-30*24*time.Hour))

	root := newRoot()
	root.SetArgs([]string{"prune", "-C", repo, "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".paseka", "runs", "trace-cli-old")); !os.IsNotExist(err) {
		t.Fatalf("old trace still exists: %v", err)
	}
}

func TestPruneCLINothingToPrune(t *testing.T) {
	repo := initTopologyFixtureRepoCLI(t)
	setupCLIHome(t, repo)

	root := newRoot()
	root.SetArgs([]string{"prune", "-C", repo, "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestPruneCLIRejectsBadRetention(t *testing.T) {
	repo := initTopologyFixtureRepoCLI(t)
	setupCLIHome(t, repo)

	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"prune", "-C", repo, "--older-than", "not-a-duration"})
	if err := root.Execute(); err == nil {
		t.Fatalf("expected retention parse error, output: %s", out.String())
	}
}

func setCLITreeModTime(t *testing.T, root string, mod time.Time) {
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
