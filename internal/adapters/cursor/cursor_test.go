package cursor

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/runs"
)

func TestAdapterName(t *testing.T) {
	if New().Name() != "cursor" {
		t.Fatal("expected adapter name cursor")
	}
}

func TestJoinPrompt(t *testing.T) {
	cases := []struct {
		system, prompt, want string
	}{
		{"", "task", "task"},
		{"role", "", "role"},
		{"role", "task", "role\ntask"},
	}
	for _, tc := range cases {
		if got := JoinPrompt(tc.system, tc.prompt); got != tc.want {
			t.Fatalf("JoinPrompt(%q, %q) = %q, want %q", tc.system, tc.prompt, got, tc.want)
		}
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

func TestAdapterRunPersistsProviderSessionID(t *testing.T) {
	repo := initCursorRepo(t)
	stdout := `{"type":"system","subtype":"init","session_id":"c6b62c6f-7ead-4fd6-9922-e952131177ff"}
{"type":"result","subtype":"success","result":"done","session_id":"c6b62c6f-7ead-4fd6-9922-e952131177ff"}`
	fake := writeFakeAgent(t, stdout)

	result, err := New().Run(context.Background(), adapters.RunRequest{
		Bee:        "builder",
		Prompt:     "do the task",
		ColonyRoot: repo,
		Workspace:  repo,
		TraceID:    "trace-1",
		AgentID:    "agent-1",
		Params: adapters.RunParams{
			Binary:       fake,
			OutputFormat: "stream-json",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProviderSessionID != "c6b62c6f-7ead-4fd6-9922-e952131177ff" {
		t.Fatalf("run result session id = %q", result.ProviderSessionID)
	}

	runDir := runs.Dir{ColonyRoot: repo, TraceID: "trace-1", AgentID: "agent-1"}
	protoResult, err := runDir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if protoResult.ProviderSessionID != "c6b62c6f-7ead-4fd6-9922-e952131177ff" {
		t.Fatalf("result.json session id = %q", protoResult.ProviderSessionID)
	}

	data, err := os.ReadFile(runDir.MetaPath())
	if err != nil {
		t.Fatal(err)
	}
	var meta runs.Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.ProviderSessionID != "c6b62c6f-7ead-4fd6-9922-e952131177ff" {
		t.Fatalf("meta.json session id = %q", meta.ProviderSessionID)
	}
}

func TestAdapterRunOmitsProviderSessionIDOnText(t *testing.T) {
	repo := initCursorRepo(t)
	fake := writeFakeAgent(t, "plain output")

	result, err := New().Run(context.Background(), adapters.RunRequest{
		Prompt:     "task",
		ColonyRoot: repo,
		Workspace:  repo,
		TraceID:    "trace-1",
		AgentID:    "agent-1",
		Params: adapters.RunParams{
			Binary:       fake,
			OutputFormat: "text",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProviderSessionID != "" {
		t.Fatalf("expected empty session id, got %q", result.ProviderSessionID)
	}

	runDir := runs.Dir{ColonyRoot: repo, TraceID: "trace-1", AgentID: "agent-1"}
	protoResult, err := runDir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if protoResult.ProviderSessionID != "" {
		t.Fatalf("result.json session id = %q", protoResult.ProviderSessionID)
	}
}

func TestAdapterRunOmitsProviderSessionIDWhenTextContainsJSON(t *testing.T) {
	repo := initCursorRepo(t)
	fake := writeFakeAgent(t, `{"type":"system","subtype":"init","session_id":"should-not-store"}`)

	result, err := New().Run(context.Background(), adapters.RunRequest{
		Prompt:     "task",
		ColonyRoot: repo,
		Workspace:  repo,
		TraceID:    "trace-1",
		AgentID:    "agent-1",
		Params: adapters.RunParams{
			Binary:       fake,
			OutputFormat: "text",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProviderSessionID != "" {
		t.Fatalf("text format must not store session id, got %q", result.ProviderSessionID)
	}
}

func TestAdapterRunCommandOverridePersistsSessionID(t *testing.T) {
	repo := initCursorRepo(t)
	stdout := `{"type":"system","subtype":"init","session_id":"override-uuid"}
{"type":"result","subtype":"success","result":"done","session_id":"override-uuid"}`
	fake := writeFakeAgent(t, stdout)

	result, err := New().Run(context.Background(), adapters.RunRequest{
		Prompt:     "task",
		ColonyRoot: repo,
		Workspace:  repo,
		TraceID:    "trace-1",
		AgentID:    "agent-1",
		Command:    []string{fake, "-p", "--output-format", "stream-json", "task"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProviderSessionID != "override-uuid" {
		t.Fatalf("session id = %q, want override-uuid", result.ProviderSessionID)
	}
}

func initCursorRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")
	return dir
}

func writeFakeAgent(t *testing.T, stdout string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-agent")
	script := "#!/bin/sh\nprintf '%s\\n' " + shellQuote(stdout) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
