package cursor_test

import (
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/adapters/cursor"
)

func TestSessionCommandInteractiveNoPrintFlag(t *testing.T) {
	a := cursor.NewSession()
	cmd, err := a.SessionCommand(adapters.SessionRequest{
		Workspace:     "/tmp/ws",
		InitialPrompt: "discuss feature",
		Params:        adapters.RunParams{Binary: "sh", Trust: true, Force: true, Model: "composer-2.5"},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, arg := range cmd.Args {
		switch arg {
		case "-p":
			t.Fatalf("interactive session must not include -p, args=%v", cmd.Args)
		case "--trust":
			t.Fatalf("interactive session must not include --trust (headless-only), args=%v", cmd.Args)
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

func TestSessionCommandDetachedUsesPrintMode(t *testing.T) {
	a := cursor.NewSession()
	cmd, err := a.SessionCommand(adapters.SessionRequest{
		Workspace:     "/tmp/ws",
		InitialPrompt: "implement feature",
		Detached:      true,
		Params:        adapters.RunParams{Binary: "sh", Trust: true, Force: true, Model: "composer-2.5"},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"-p", "--workspace", "/tmp/ws",
		"--output-format", "text",
		"--trust", "--force",
		"--model", "composer-2.5",
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
