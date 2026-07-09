package claude_test

import (
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
