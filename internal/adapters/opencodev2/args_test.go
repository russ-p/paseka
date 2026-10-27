package opencodev2

import (
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/adapters"
)

func TestBuildAFKArgs(t *testing.T) {
	req := adapters.RunRequest{
		AgentID: "agent-1",
		Params: adapters.RunParams{
			Model:     "anthropic/claude-sonnet-4",
			Reasoning: "high",
			Trust:     true,
		},
	}
	args := buildArgs(req, "do work")
	joined := strings.Join(args, " ")

	if strings.Contains(joined, "--dir") {
		t.Fatalf("V2 argv must not contain --dir: %v", args)
	}
	if strings.Contains(joined, "--variant") {
		t.Fatalf("V2 argv must not contain a separate variant flag: %v", args)
	}
	for _, want := range []string{
		"run", "--format", "json", "--auto", "--title", "agent-1",
		"--model", "anthropic/claude-sonnet-4#high", "--", "do work",
	} {
		if !containsArg(args, want) {
			t.Fatalf("argv %v missing %q", args, want)
		}
	}
}

func TestBuildAFKArgsPlanWinsOverAuto(t *testing.T) {
	args := buildArgs(adapters.RunRequest{
		Params: adapters.RunParams{Plan: true, Trust: true, Force: true, Model: "provider/model"},
	}, "plan it")

	if containsArg(args, "--auto") {
		t.Fatalf("plan argv must not auto-approve: %v", args)
	}
	if !containsArg(args, "--agent") || !containsArg(args, "plan") {
		t.Fatalf("plan argv missing --agent plan: %v", args)
	}
}

func TestBuildAFKArgsRawUsesTextFormat(t *testing.T) {
	args := buildArgs(adapters.RunRequest{Raw: true}, "hello")
	if !containsArg(args, "--format") || !containsArg(args, "default") {
		t.Fatalf("raw argv = %v", args)
	}
}

func TestBuildInteractiveArgsOnlyV2Flags(t *testing.T) {
	args := buildInteractiveArgs("http://127.0.0.1:1234", "ses_abc", "kickoff", true)
	want := []string{"--server", "http://127.0.0.1:1234", "--session", "ses_abc", "--auto", "--prompt", "kickoff"}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Fatalf("interactive argv = %v, want %v", args, want)
	}
	for _, forbidden := range []string{"--model", "--agent", "--variant", "--port", "--hostname"} {
		if containsArg(args, forbidden) {
			t.Fatalf("interactive argv contains forbidden %q: %v", forbidden, args)
		}
	}
}

func TestModelReferenceAndSessionModel(t *testing.T) {
	if got := modelReference("provider/model", "high"); got != "provider/model#high" {
		t.Fatalf("model reference = %q", got)
	}
	if got := modelReference("provider/model", ""); got != "provider/model" {
		t.Fatalf("model reference = %q", got)
	}
	if got := modelReference("", "high"); got != "" {
		t.Fatalf("model reference = %q", got)
	}

	model := sessionModelFor("provider/model", "high")
	if model == nil || model.ProviderID != "provider" || model.ModelID != "model" || model.Variant != "high" {
		t.Fatalf("session model = %+v", model)
	}
	if got := sessionModelFor("", ""); got != nil {
		t.Fatalf("empty session model = %+v", got)
	}
}

func TestNewSessionIDShape(t *testing.T) {
	id, err := NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != len(sessionIDPrefix)+sessionIDLength || !strings.HasPrefix(id, sessionIDPrefix) {
		t.Fatalf("session id = %q", id)
	}
	for _, r := range id[len(sessionIDPrefix):] {
		if (r < '0' || r > '9') && (r < 'a' || r > 'z') {
			t.Fatalf("session id has invalid rune %q in %q", r, id)
		}
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
