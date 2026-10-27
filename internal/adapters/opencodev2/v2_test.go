package opencodev2

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

func readRunMeta(t *testing.T, dir runs.Dir) runs.Meta {
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

func writeAFKFake(t *testing.T, capture string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "opencode")
	script := `#!/bin/sh
if [ "$1" = "run" ]; then
  if [ -n "$CAPTURE_ARGS" ]; then
    : > "$CAPTURE_ARGS"
    for a in "$@"; do printf '%s\n' "$a" >> "$CAPTURE_ARGS"; done
  fi
  printf '%s\n' '{"type":"text","sessionID":"ses_run","part":{"text":"done"}}'
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

func TestRunUsesWorkspaceCwdAndRecordsIdentity(t *testing.T) {
	workspace := t.TempDir()
	colonyRoot := t.TempDir()
	capture := filepath.Join(t.TempDir(), "args.txt")
	binary := writeAFKFake(t, capture)

	res, err := New().Run(context.Background(), adapters.RunRequest{
		Bee:            "scout",
		Workspace:      workspace,
		ColonyRoot:     colonyRoot,
		TraceID:        "trace-v2",
		AgentID:        "agent-v2",
		Prompt:         "do work",
		Binary:         binary,
		Env:            os.Environ(),
		AdapterName:    "opencode",
		Implementation: adapterName,
		Params: adapters.RunParams{
			Model:     "provider/model",
			Reasoning: "high",
			Trust:     true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != string(protocol.StatusCompleted) || res.Summary != "done" || res.ProviderSessionID != "ses_run" {
		t.Fatalf("result = %+v", res)
	}

	args, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	argv := strings.Split(strings.TrimSpace(string(args)), "\n")
	for _, forbidden := range []string{"--dir", "--variant"} {
		if containsArg(argv, forbidden) {
			t.Fatalf("argv contains %q: %v", forbidden, argv)
		}
	}
	if !containsArg(argv, "provider/model#high") {
		t.Fatalf("argv missing combined model reference: %v", argv)
	}

	dir := runs.Dir{ColonyRoot: colonyRoot, TraceID: "trace-v2", AgentID: "agent-v2"}
	meta := readRunMeta(t, dir)
	if meta.Adapter != "opencode" || meta.Implementation != adapterName {
		t.Fatalf("meta identity = %q/%q", meta.Adapter, meta.Implementation)
	}
	if meta.ProviderSessionID != "ses_run" {
		t.Fatalf("meta provider session = %q", meta.ProviderSessionID)
	}

	result, err := dir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if result.ProviderSessionID != "ses_run" || result.Status != protocol.StatusCompleted {
		t.Fatalf("result json = %+v", result)
	}
}

func TestRunDirectPinRecordsOwnName(t *testing.T) {
	workspace := t.TempDir()
	colonyRoot := t.TempDir()
	binary := writeAFKFake(t, "")

	if _, err := New().Run(context.Background(), adapters.RunRequest{
		Workspace:  workspace,
		ColonyRoot: colonyRoot,
		TraceID:    "trace-v2",
		AgentID:    "agent-v2",
		Prompt:     "do work",
		Binary:     binary,
		Env:        os.Environ(),
	}); err != nil {
		t.Fatal(err)
	}

	meta := readRunMeta(t, runs.Dir{ColonyRoot: colonyRoot, TraceID: "trace-v2", AgentID: "agent-v2"})
	if meta.Adapter != adapterName || meta.Implementation != adapterName {
		t.Fatalf("meta identity = %q/%q", meta.Adapter, meta.Implementation)
	}
}

func TestRunRejectsMissingWorkspace(t *testing.T) {
	_, err := New().Run(context.Background(), adapters.RunRequest{
		ColonyRoot: t.TempDir(),
		TraceID:    "trace-v2",
		AgentID:    "agent-v2",
		Prompt:     "do work",
	})
	if err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunRejectsWorkspaceThatIsNotADirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := New().Run(context.Background(), adapters.RunRequest{
		Workspace:  file,
		ColonyRoot: t.TempDir(),
		TraceID:    "trace-v2",
		AgentID:    "agent-v2",
		Prompt:     "do work",
	})
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("error = %v", err)
	}
}
