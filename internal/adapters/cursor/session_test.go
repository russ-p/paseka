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
		Params:        adapters.RunParams{Trust: true, Force: true, Model: "composer-2.5"},
	})
	if err != nil {
		t.Skip("agent binary not in PATH:", err)
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
