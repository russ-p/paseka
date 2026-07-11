package claude_test

import (
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/adapters/claude"
)

func TestSessionCommandInteractiveNoPrintFlag(t *testing.T) {
	a := claude.NewSession()
	cmd, err := a.SessionCommand(adapters.SessionRequest{
		Workspace:     "/tmp/ws",
		InitialPrompt: "discuss feature",
		Params:        adapters.RunParams{Binary: "sh", Trust: true, Force: true, Model: "claude-opus-4-8"},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, arg := range cmd.Args {
		if arg == "-p" {
			t.Fatalf("interactive session must not include -p, args=%v", cmd.Args)
		}
	}
	if cmd.Dir != "/tmp/ws" {
		t.Fatalf("dir = %q", cmd.Dir)
	}
	last := cmd.Args[len(cmd.Args)-1]
	if last != "discuss feature" {
		t.Fatalf("prompt arg = %q", last)
	}
}

func TestSessionCommandDetachedStillInteractive(t *testing.T) {
	a := claude.NewSession()
	cmd, err := a.SessionCommand(adapters.SessionRequest{
		Workspace:     "/tmp/ws",
		InitialPrompt: "implement feature",
		Detached:      true,
		Params:        adapters.RunParams{Binary: "sh", Trust: true, Force: true, Model: "claude-opus-4-8"},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, arg := range cmd.Args {
		if arg == "-p" || arg == "--output-format" || arg == "--permission-mode" {
			t.Fatalf("detached session must stay interactive, args=%v", cmd.Args)
		}
	}
	want := []string{
		"--model", "claude-opus-4-8",
		"implement feature",
	}
	if len(cmd.Args) != len(want) {
		t.Fatalf("got %d args, want %d: %v", len(cmd.Args), len(want), cmd.Args)
	}
	for i := range want {
		if cmd.Args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q (full: %v)", i, cmd.Args[i], want[i], cmd.Args)
		}
	}
}

func TestSessionCommandSystemOnlyAppendsSystemPrompt(t *testing.T) {
	a := claude.NewSession()
	cmd, err := a.SessionCommand(adapters.SessionRequest{
		ColonyRoot:   "/colony",
		Workspace:    "/tmp/ws",
		TraceID:      "trace-1",
		AgentID:      "agent-1",
		SystemPrompt: "You are Scout.",
		Params:       adapters.RunParams{Binary: "sh", Model: "claude-opus-4-8"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantSystem := filepath.Join("/colony", ".paseka", "runs", "trace-1", "agent-1", "system.txt")
	assertArgPair(t, cmd.Args, "--append-system-prompt-file", wantSystem)
	if len(cmd.Args) > 0 && cmd.Args[len(cmd.Args)-1] == "You are Scout." {
		t.Fatalf("system prompt must not be positional arg, args=%v", cmd.Args)
	}
}

func assertArgPair(t *testing.T, args []string, flag, want string) {
	t.Helper()
	for i := 0; i < len(args); i++ {
		if args[i] != flag {
			continue
		}
		if i+1 >= len(args) {
			t.Fatalf("flag %q missing value in %v", flag, args)
		}
		if args[i+1] != want {
			t.Fatalf("%q = %q, want %q", flag, args[i+1], want)
		}
		return
	}
	t.Fatalf("flag %q not found in %v", flag, args)
}
