package opencode

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
)

func TestSessionCommandInteractive(t *testing.T) {
	fake := writeFakeBinary(t)
	cmd, err := NewSession().SessionCommand(adapters.SessionRequest{
		Workspace:     "/tmp/ws",
		InitialPrompt: "discuss feature",
		Params: adapters.RunParams{
			Binary:       fake,
			Model:        "claude-sonnet-4",
			Provider:     "anthropic",
			Thinking:     "high",
			Plan:         true,
			Trust:        true,
			Force:        true,
			OutputFormat: "json",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range cmd.Args {
		switch arg {
		case "run", "--auto", "--format", "--title", "--dir":
			t.Fatalf("interactive must not include %q, args=%v", arg, cmd.Args)
		}
	}
	assertArgPair(t, cmd.Args, "--agent", "plan")
	assertArgPair(t, cmd.Args, "--model", "anthropic/claude-sonnet-4")
	assertArgPair(t, cmd.Args, "--variant", "high")
	assertArgPair(t, cmd.Args, "--prompt", "discuss feature")
	if cmd.Dir != "/tmp/ws" {
		t.Fatalf("dir = %q", cmd.Dir)
	}
	if cmd.ProviderSessionID != "" {
		t.Fatalf("HITL provider session id must be empty, got %q", cmd.ProviderSessionID)
	}
}

func TestSessionCommandJoinsSystemPrompt(t *testing.T) {
	fake := writeFakeBinary(t)
	cmd, err := NewSession().SessionCommand(adapters.SessionRequest{
		Workspace:     "/tmp/ws",
		SystemPrompt:  "You are Scout.",
		InitialPrompt: "intake this idea",
		Params:        adapters.RunParams{Binary: fake},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertArgPair(t, cmd.Args, "--prompt", "You are Scout.\nintake this idea")
}

func TestSessionCommandSystemOnly(t *testing.T) {
	fake := writeFakeBinary(t)
	cmd, err := NewSession().SessionCommand(adapters.SessionRequest{
		Workspace:    "/tmp/ws",
		SystemPrompt: "You are Scout.",
		Params:       adapters.RunParams{Binary: fake},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertArgPair(t, cmd.Args, "--prompt", "You are Scout.")
}

func TestSessionCommandOverrideDoesNotInjectFlags(t *testing.T) {
	fake := writeFakeBinary(t)
	cmd, err := NewSession().SessionCommand(adapters.SessionRequest{
		Workspace:     "/tmp/ws",
		InitialPrompt: "discuss feature",
		Command:       []string{fake, "--prompt", "custom"},
		Params:        adapters.RunParams{Plan: true, Trust: true, Force: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range cmd.Args {
		if arg == "--auto" || arg == "--agent" || arg == "--dir" {
			t.Fatalf("override must not inject %q: %v", arg, cmd.Args)
		}
	}
	assertArgPair(t, cmd.Args, "--prompt", "custom")
}

func TestSessionCommandRequiresFields(t *testing.T) {
	fake := writeFakeBinary(t)
	a := NewSession()
	if _, err := a.SessionCommand(adapters.SessionRequest{InitialPrompt: "x", Params: adapters.RunParams{Binary: fake}}); err == nil {
		t.Fatal("expected workspace error")
	}
	if _, err := a.SessionCommand(adapters.SessionRequest{Workspace: "/tmp/ws", Params: adapters.RunParams{Binary: fake}}); err == nil {
		t.Fatal("expected prompt error")
	}
}

func writeFakeBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-opencode")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
