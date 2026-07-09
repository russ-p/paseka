package pi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
)

func TestSessionCommandInteractiveNoPrintOrModeFlags(t *testing.T) {
	fakePi := writeFakePiBinary(t)
	a := NewSession()

	cmd, err := a.SessionCommand(adapters.SessionRequest{
		ColonyRoot:    "/colony",
		Workspace:     "/tmp/ws",
		TraceID:       "trace-1",
		AgentID:       "agent-1",
		InitialPrompt: "discuss feature",
		Params: adapters.RunParams{
			Binary:       fakePi,
			Model:        "gpt-4",
			Provider:     "gemini",
			Thinking:     "high",
			Plan:         true,
			APIKey:       "secret",
			OutputFormat: "json",
			Trust:        true,
			Force:        true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, arg := range cmd.Args {
		switch arg {
		case "-p", "--print":
			t.Fatalf("interactive session must not include print mode, args=%v", cmd.Args)
		case "--mode":
			t.Fatalf("interactive session must not include --mode, args=%v", cmd.Args)
		case "--trust", "--force":
			t.Fatalf("interactive session must not include cursor-only flags, args=%v", cmd.Args)
		}
	}

	wantSessionDir := filepath.Join("/colony", ".paseka", "runs", "trace-1", "agent-1", "pi-sessions")
	assertArgPair(t, cmd.Args, "--session-dir", wantSessionDir)
	assertArgPair(t, cmd.Args, "--session-id", "agent-1")
	assertArgPair(t, cmd.Args, "--model", "gpt-4")
	assertArgPair(t, cmd.Args, "--provider", "gemini")
	assertArgPair(t, cmd.Args, "--thinking", "high")
	if !containsArg(cmd.Args, "--plan") {
		t.Fatalf("expected --plan in args=%v", cmd.Args)
	}
	assertArgPair(t, cmd.Args, "--api-key", "secret")

	if cmd.Dir != "/tmp/ws" {
		t.Fatalf("dir = %q", cmd.Dir)
	}
	if cmd.Binary != fakePi {
		t.Fatalf("binary = %q", cmd.Binary)
	}
	last := cmd.Args[len(cmd.Args)-1]
	if last != "discuss feature" {
		t.Fatalf("prompt arg = %q", last)
	}
}

func TestSessionCommandDetachedStillInteractive(t *testing.T) {
	fakePi := writeFakePiBinary(t)
	a := NewSession()

	cmd, err := a.SessionCommand(adapters.SessionRequest{
		ColonyRoot:    "/colony",
		Workspace:     "/tmp/ws",
		TraceID:       "trace-1",
		AgentID:       "agent-1",
		InitialPrompt: "implement feature",
		Detached:      true,
		Params: adapters.RunParams{
			Binary:   fakePi,
			Model:    "gpt-4",
			Provider: "gemini",
			Thinking: "high",
			Plan:     true,
			APIKey:   "secret",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, arg := range cmd.Args {
		if arg == "-p" || arg == "--mode" {
			t.Fatalf("detached session must stay interactive, args=%v", cmd.Args)
		}
	}
	wantSessionDir := filepath.Join("/colony", ".paseka", "runs", "trace-1", "agent-1", "pi-sessions")
	assertArgPair(t, cmd.Args, "--session-dir", wantSessionDir)
	assertArgPair(t, cmd.Args, "--session-id", "agent-1")
	assertArgPair(t, cmd.Args, "--model", "gpt-4")
	assertArgPair(t, cmd.Args, "--provider", "gemini")
	assertArgPair(t, cmd.Args, "--thinking", "high")
	if !containsArg(cmd.Args, "--plan") {
		t.Fatalf("expected --plan in args=%v", cmd.Args)
	}
	assertArgPair(t, cmd.Args, "--api-key", "secret")
	last := cmd.Args[len(cmd.Args)-1]
	if last != "implement feature" {
		t.Fatalf("prompt arg = %q", last)
	}
}

func TestSessionCommandRequiresFields(t *testing.T) {
	fakePi := writeFakePiBinary(t)
	a := NewSession()
	base := adapters.SessionRequest{
		ColonyRoot:    "/colony",
		Workspace:     "/tmp/ws",
		TraceID:       "trace-1",
		AgentID:       "agent-1",
		InitialPrompt: "hello",
		Params:        adapters.RunParams{Binary: fakePi},
	}

	tests := []struct {
		name string
		req  adapters.SessionRequest
	}{
		{name: "workspace", req: func() adapters.SessionRequest { r := base; r.Workspace = ""; return r }()},
		{name: "prompt", req: func() adapters.SessionRequest { r := base; r.InitialPrompt = ""; return r }()},
		{name: "colony root", req: func() adapters.SessionRequest { r := base; r.ColonyRoot = ""; return r }()},
		{name: "trace id", req: func() adapters.SessionRequest { r := base; r.TraceID = ""; return r }()},
		{name: "agent id", req: func() adapters.SessionRequest { r := base; r.AgentID = ""; return r }()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := a.SessionCommand(tc.req); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func writeFakePiBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-pi")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
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

func containsArg(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}
