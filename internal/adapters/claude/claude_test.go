package claude

import (
	"context"
	"errors"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/protocol"
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
			Model:        "claude-opus-4-8",
			OutputFormat: "stream-json",
			Trust:        true,
			Force:        true,
		},
	}
	args := buildArgs(req, "implement feature")

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

func TestResolveStatusProcessOutcome(t *testing.T) {
	t.Run("completed on clean exit", func(t *testing.T) {
		status, msg := resolveStatus(nil, nil)
		if status != protocol.StatusCompleted || msg != "" {
			t.Fatalf("status=%q msg=%q", status, msg)
		}
	})
	t.Run("failed on run error", func(t *testing.T) {
		status, _ := resolveStatus(nil, errors.New("exit 1"))
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
