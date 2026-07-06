package cursor

import (
	"context"
	"errors"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/protocol"
)

func TestAdapterName(t *testing.T) {
	if New().Name() != "cursor" {
		t.Fatal("expected adapter name cursor")
	}
}

func TestBuildArgs(t *testing.T) {
	req := adapters.RunRequest{
		Workspace: "/colony/worktree",
		Params: adapters.RunParams{
			Model:        "composer-2.5",
			OutputFormat: "stream-json",
			Trust:        true,
			Force:        true,
		},
	}
	args := buildArgs(req, "implement feature")

	want := []string{
		"-p", "--workspace", "/colony/worktree",
		"--output-format", "stream-json",
		"--trust", "--force",
		"--model", "composer-2.5",
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

func TestResolveStatusProcessOutcome(t *testing.T) {
	t.Run("completed on clean exit", func(t *testing.T) {
		status, msg := resolveStatus(nil, nil)
		if status != protocol.StatusCompleted || msg != "" {
			t.Fatalf("status=%q msg=%q", status, msg)
		}
	})
	t.Run("failed on run error", func(t *testing.T) {
		runErr := errors.New("exit 1")
		status, _ := resolveStatus(nil, runErr)
		if status != protocol.StatusFailed {
			t.Fatalf("status=%q", status)
		}
	})
	t.Run("cancelled on context cancel", func(t *testing.T) {
		status, _ := resolveStatus(context.Canceled, nil)
		if status != protocol.StatusCancelled {
			t.Fatalf("status=%q", status)
		}
	})
}
