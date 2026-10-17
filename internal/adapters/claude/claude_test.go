package claude

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/runs"
)

func TestAdapterName(t *testing.T) {
	if New().Name() != "claude" {
		t.Fatal("expected adapter name claude")
	}
}

func TestBuildArgs(t *testing.T) {
	req := adapters.RunRequest{
		Workspace: "/colony/worktree",
		Params: adapters.RunParams{
			Model: "claude-opus-4-8",
			Trust: true,
			Force: true,
		},
	}
	args := buildArgs(req, "implement feature", "")

	want := []string{
		"-p",
		"--output-format", "stream-json", "--verbose",
		"--permission-mode", "bypassPermissions",
		"--model", "claude-opus-4-8",
		"implement feature",
	}
	if len(args) != len(want) {
		t.Fatalf("got %d args, want %d: %v", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q (full: %v)", i, args[i], want[i], args)
		}
	}
}

func TestBuildArgsRawDropsVerbose(t *testing.T) {
	req := adapters.RunRequest{Workspace: "/ws", Raw: true}
	args := buildArgs(req, "debug", "")
	want := []string{
		"-p",
		"--output-format", "text",
		"--permission-mode", "acceptEdits",
		"debug",
	}
	if len(args) != len(want) {
		t.Fatalf("got %d args, want %d: %v", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q (full: %v)", i, args[i], want[i], args)
		}
	}
	for _, a := range args {
		if a == "--verbose" {
			t.Fatalf("raw text must not add --verbose: %v", args)
		}
	}
}

func TestPermissionMode(t *testing.T) {
	cases := []struct {
		name string
		p    adapters.RunParams
		want string
	}{
		{"plan wins", adapters.RunParams{Plan: true, Force: true}, "plan"},
		{"force bypasses", adapters.RunParams{Force: true}, "bypassPermissions"},
		{"trust bypasses", adapters.RunParams{Trust: true}, "bypassPermissions"},
		{"default accepts edits", adapters.RunParams{}, "acceptEdits"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := permissionMode(tc.p); got != tc.want {
				t.Fatalf("permissionMode = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAdapterRunKeepsStreamDiagnosticsOutOfEventLog(t *testing.T) {
	repo := t.TempDir()
	stdout := `{"type":"assistant","message":{"content":[{"type":"text","text":"working"}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"main.go"}}]}}
{"type":"result","subtype":"success","is_error":false,"result":"done"}`
	fake := filepath.Join(t.TempDir(), "fake-agent")
	script := "#!/bin/sh\nprintf '%s\n' '" + strings.ReplaceAll(stdout, "'", `'"'"'`) + "'\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := New().Run(context.Background(), adapters.RunRequest{
		Bee:        "builder",
		Prompt:     "do the task",
		ColonyRoot: repo,
		Workspace:  repo,
		TraceID:    "trace-1",
		AgentID:    "agent-1",
		Params:     adapters.RunParams{Binary: fake},
	})
	if err != nil {
		t.Fatal(err)
	}
	// The parser still reports them to the in-process completion checks; only the
	// run's event log is a domain-bus audit and must not carry them.
	if len(result.Events) == 0 {
		t.Fatal("expected the adapter to report parsed stream events")
	}

	runDir := runs.Dir{ColonyRoot: repo, TraceID: "trace-1", AgentID: "agent-1"}
	events, err := runDir.ReadEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("events.ndjson has %d line(s), want 0: %+v", len(events), events)
	}
}
