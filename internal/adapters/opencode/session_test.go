package opencode

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/adapters"
)

func TestSessionCommandInteractive(t *testing.T) {
	fake := writeFakeBinary(t)
	rec := stubCreateSession(t, "ses_new", nil)
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
	assertArgPair(t, cmd.Args, "--session", "ses_new")
	assertArgPair(t, cmd.Args, "--prompt", "discuss feature")
	if cmd.Dir != "/tmp/ws" {
		t.Fatalf("dir = %q", cmd.Dir)
	}
	if cmd.ProviderSessionID != "ses_new" {
		t.Fatalf("provider session id = %q", cmd.ProviderSessionID)
	}
	if rec.calls != 1 {
		t.Fatalf("create session calls = %d", rec.calls)
	}
	if rec.binary != fake || rec.workspace != "/tmp/ws" {
		t.Fatalf("create session called with %q %q", rec.binary, rec.workspace)
	}
}

func TestSessionCommandJoinsSystemPrompt(t *testing.T) {
	fake := writeFakeBinary(t)
	stubCreateSession(t, "ses_join", nil)
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
	stubCreateSession(t, "ses_sys", nil)
	cmd, err := NewSession().SessionCommand(adapters.SessionRequest{
		Workspace:    "/tmp/ws",
		SystemPrompt: "You are Scout.",
		Params:       adapters.RunParams{Binary: fake},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertArgPair(t, cmd.Args, "--prompt", "You are Scout.")
	assertArgPair(t, cmd.Args, "--session", "ses_sys")
}

func TestSessionCommandResumeSkipsCreateSession(t *testing.T) {
	fake := writeFakeBinary(t)
	rec := stubCreateSession(t, "should-not-be-used", nil)
	cmd, err := NewSession().SessionCommand(adapters.SessionRequest{
		Workspace:       "/tmp/ws",
		ResumeSessionID: "ses_old",
		InitialPrompt:   "keep going",
		SystemPrompt:    "You are Scout.",
		Params:          adapters.RunParams{Binary: fake, Plan: true, Force: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.calls != 0 {
		t.Fatalf("resume must not create a session, calls=%d", rec.calls)
	}
	assertArgPair(t, cmd.Args, "--session", "ses_old")
	assertArgPair(t, cmd.Args, "--prompt", "keep going")
	for _, arg := range cmd.Args {
		if strings.Contains(arg, "You are Scout.") {
			t.Fatalf("resume must not join system prompt, args=%v", cmd.Args)
		}
		if arg == "--auto" || arg == "--format" || arg == "--title" || arg == "--dir" {
			t.Fatalf("resume must stay interactive, args=%v", cmd.Args)
		}
	}
	if cmd.ProviderSessionID != "ses_old" {
		t.Fatalf("provider session id = %q", cmd.ProviderSessionID)
	}
}

func TestSessionCommandResumeEmptyContinue(t *testing.T) {
	fake := writeFakeBinary(t)
	stubCreateSession(t, "should-not-be-used", nil)
	cmd, err := NewSession().SessionCommand(adapters.SessionRequest{
		Workspace:       "/tmp/ws",
		ResumeSessionID: "ses_old",
		Params:          adapters.RunParams{Binary: fake},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertArgPair(t, cmd.Args, "--session", "ses_old")
	if adapters.FlagValue(cmd.Args, "--prompt") != "" {
		t.Fatalf("empty continue must omit --prompt, args=%v", cmd.Args)
	}
}

func TestSessionCommandCreateSessionFailureStillLaunches(t *testing.T) {
	fake := writeFakeBinary(t)
	stubCreateSession(t, "", errors.New("serve unavailable"))
	cmd, err := NewSession().SessionCommand(adapters.SessionRequest{
		Workspace:     "/tmp/ws",
		InitialPrompt: "discuss feature",
		Params:        adapters.RunParams{Binary: fake, Model: "composer-2.5"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.ProviderSessionID != "" {
		t.Fatalf("provider session id = %q, want empty", cmd.ProviderSessionID)
	}
	for _, arg := range cmd.Args {
		if arg == "--session" || strings.HasPrefix(arg, "--session=") {
			t.Fatalf("must not inject --session after pre-create failure, args=%v", cmd.Args)
		}
	}
	assertArgPair(t, cmd.Args, "--prompt", "discuss feature")
}

func TestSessionCommandOverrideDoesNotInjectFlags(t *testing.T) {
	fake := writeFakeBinary(t)
	rec := stubCreateSession(t, "should-not-be-used", nil)
	cmd, err := NewSession().SessionCommand(adapters.SessionRequest{
		Workspace:     "/tmp/ws",
		InitialPrompt: "discuss feature",
		Command:       []string{fake, "--prompt", "custom"},
		Params:        adapters.RunParams{Plan: true, Trust: true, Force: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.calls != 0 {
		t.Fatalf("command override must not create a session, calls=%d", rec.calls)
	}
	for _, arg := range cmd.Args {
		if arg == "--auto" || arg == "--agent" || arg == "--dir" {
			t.Fatalf("override must not inject %q: %v", arg, cmd.Args)
		}
	}
	assertArgPair(t, cmd.Args, "--prompt", "custom")
}

func TestSessionCommandOverrideStoresSessionID(t *testing.T) {
	fake := writeFakeBinary(t)
	rec := stubCreateSession(t, "should-not-be-used", nil)
	for _, flag := range []string{"--session", "-s"} {
		cmd, err := NewSession().SessionCommand(adapters.SessionRequest{
			Workspace:     "/tmp/ws",
			InitialPrompt: "discuss feature",
			Command:       []string{fake, flag, "ses_override", "--prompt", "custom"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if rec.calls != 0 {
			t.Fatalf("command override must not create a session, calls=%d", rec.calls)
		}
		if cmd.ProviderSessionID != "ses_override" {
			t.Fatalf("flag %q provider session id = %q", flag, cmd.ProviderSessionID)
		}
	}
}

func TestSessionCommandOverrideOmitsIDWithoutSessionFlag(t *testing.T) {
	fake := writeFakeBinary(t)
	cmd, err := NewSession().SessionCommand(adapters.SessionRequest{
		Workspace:     "/tmp/ws",
		InitialPrompt: "discuss feature",
		Command:       []string{fake, "--prompt", "custom"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.ProviderSessionID != "" {
		t.Fatalf("provider session id = %q, want empty", cmd.ProviderSessionID)
	}
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

type createSessionCall struct {
	binary    string
	workspace string
	calls     int
}

func stubCreateSession(t *testing.T, id string, err error) *createSessionCall {
	t.Helper()
	rec := &createSessionCall{}
	prev := createSessionFunc
	createSessionFunc = func(binary, workspace string, _ []string) (string, error) {
		rec.calls++
		rec.binary = binary
		rec.workspace = workspace
		if err != nil {
			return "", err
		}
		return id, nil
	}
	t.Cleanup(func() { createSessionFunc = prev })
	return rec
}
