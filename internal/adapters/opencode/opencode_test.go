package opencode

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/runs"
)

func writeProxyFake(t *testing.T, version string, capture string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "opencode")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then echo "` + version + `"; exit 0; fi
if [ "$1" = "run" ]; then
  if [ -n "$CAPTURE_ARGS" ]; then
    : > "$CAPTURE_ARGS"
    for a in "$@"; do printf '%s\n' "$a" >> "$CAPTURE_ARGS"; done
  fi
  printf '%s\n' '{"type":"text","sessionID":"ses_proxy","part":{"text":"proxy ok"}}'
  exit 0
fi
exit 1
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CAPTURE_ARGS", capture)
	return path
}

func readMeta(t *testing.T, dir runs.Dir) runs.Meta {
	t.Helper()
	data, err := os.ReadFile(dir.MetaPath())
	if err != nil {
		t.Fatal(err)
	}
	var meta runs.Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	return meta
}

func TestProxyName(t *testing.T) {
	if New().Name() != "opencode" {
		t.Fatalf("name = %q", New().Name())
	}
}

func TestProxyRunDelegatesV2(t *testing.T) {
	ResetVersionCache()
	workspace := t.TempDir()
	colonyRoot := t.TempDir()
	capture := filepath.Join(t.TempDir(), "args.txt")
	binary := writeProxyFake(t, "2.0.27", capture)

	res, err := New().Run(context.Background(), adapters.RunRequest{
		Bee:        "scout",
		Workspace:  workspace,
		ColonyRoot: colonyRoot,
		TraceID:    "trace-proxy",
		AgentID:    "agent-proxy",
		Prompt:     "do work",
		Binary:     binary,
		Env:        os.Environ(),
		Params:     adapters.RunParams{Model: "provider/model", Trust: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary != "proxy ok" || res.Status != "completed" {
		t.Fatalf("result = %+v", res)
	}
	args, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	argv := strings.Split(strings.TrimSpace(string(args)), "\n")
	if contains(argv, "--dir") {
		t.Fatalf("V2 delegated argv contains --dir: %v", argv)
	}
	meta := readMeta(t, runs.Dir{ColonyRoot: colonyRoot, TraceID: "trace-proxy", AgentID: "agent-proxy"})
	if meta.Adapter != "opencode" || meta.Implementation != "opencodev2" {
		t.Fatalf("meta identity = %q/%q", meta.Adapter, meta.Implementation)
	}
}

func TestProxyRunDelegatesV1(t *testing.T) {
	ResetVersionCache()
	workspace := t.TempDir()
	colonyRoot := t.TempDir()
	capture := filepath.Join(t.TempDir(), "args.txt")
	binary := writeProxyFake(t, "1.18.35", capture)

	if _, err := New().Run(context.Background(), adapters.RunRequest{
		Workspace:  workspace,
		ColonyRoot: colonyRoot,
		TraceID:    "trace-proxy",
		AgentID:    "agent-proxy",
		Prompt:     "do work",
		Binary:     binary,
		Env:        os.Environ(),
	}); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	argv := strings.Split(strings.TrimSpace(string(args)), "\n")
	if !contains(argv, "--dir") || !contains(argv, workspace) {
		t.Fatalf("V1 delegated argv missing --dir workspace: %v", argv)
	}
	meta := readMeta(t, runs.Dir{ColonyRoot: colonyRoot, TraceID: "trace-proxy", AgentID: "agent-proxy"})
	if meta.Adapter != "opencode" || meta.Implementation != "opencodev1" {
		t.Fatalf("meta identity = %q/%q", meta.Adapter, meta.Implementation)
	}
}

func contains(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
