package opencode

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

func TestAdapterName(t *testing.T) {
	if New().Name() != "opencode" {
		t.Fatal("expected adapter name opencode")
	}
}

func TestJoinPrompt(t *testing.T) {
	tests := []struct {
		system, prompt, want string
	}{
		{system: "", prompt: "task", want: "task"},
		{system: "sys", prompt: "", want: "sys"},
		{system: "sys", prompt: "task", want: "sys\ntask"},
	}
	for _, tc := range tests {
		if got := joinPrompt(tc.system, tc.prompt); got != tc.want {
			t.Fatalf("joinPrompt(%q, %q) = %q, want %q", tc.system, tc.prompt, got, tc.want)
		}
	}
}

func TestResolveModel(t *testing.T) {
	tests := []struct {
		name string
		p    adapters.RunParams
		want string
	}{
		{name: "provider slash model", p: adapters.RunParams{Model: "anthropic/claude-sonnet-4"}, want: "anthropic/claude-sonnet-4"},
		{name: "join provider", p: adapters.RunParams{Model: "claude-sonnet-4", Provider: "anthropic"}, want: "anthropic/claude-sonnet-4"},
		{name: "model only", p: adapters.RunParams{Model: "claude-sonnet-4"}, want: "claude-sonnet-4"},
		{name: "slash wins over provider", p: adapters.RunParams{Model: "opencode/gpt-5", Provider: "anthropic"}, want: "opencode/gpt-5"},
		{name: "empty", p: adapters.RunParams{}, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveModel(tc.p); got != tc.want {
				t.Fatalf("resolveModel = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildArgs(t *testing.T) {
	req := adapters.RunRequest{
		Workspace: "/colony/worktree",
		AgentID:   "agent-1",
		Params: adapters.RunParams{
			Model:        "claude-sonnet-4",
			Provider:     "anthropic",
			OutputFormat: "json",
			Trust:        true,
			Force:        true,
			Plan:         true,
			Thinking:     "high",
		},
	}
	args := buildArgs(req, "implement feature")
	want := []string{
		"run", "--format", "json", "--dir", "/colony/worktree",
		"--auto",
		"--title", "agent-1",
		"--agent", "plan",
		"--model", "anthropic/claude-sonnet-4",
		"--variant", "high",
		"--", "implement feature",
	}
	assertArgs(t, args, want)
}

func TestBuildArgsNoAutoWhenUntrusted(t *testing.T) {
	req := adapters.RunRequest{
		Workspace: "/ws",
		AgentID:   "a1",
		Params:    adapters.RunParams{Trust: false, Force: false, OutputFormat: "text"},
	}
	args := buildArgs(req, "task")
	for _, arg := range args {
		if arg == "--auto" {
			t.Fatalf("did not expect --auto: %v", args)
		}
	}
	assertArgPair(t, args, "--format", "default")
}

func TestParseJSONL(t *testing.T) {
	stdout := `{"type":"step_start","sessionID":"ses_abc123","part":{"type":"step-start"}}
{"type":"text","sessionID":"ses_abc123","part":{"type":"text","text":"working"}}
{"type":"step_finish","sessionID":"ses_abc123","part":{"type":"step-finish","tokens":{"input":10,"output":4,"cache":{"read":2,"write":1}}}}
{"type":"text","sessionID":"ses_abc123","part":{"type":"text","text":"final answer"}}
{"type":"step_finish","sessionID":"ses_abc123","part":{"type":"step-finish","tokens":{"input":20,"output":8,"cache":{"read":3,"write":1}}}}
`
	got := parseRunOutput(stdout, "json")
	if got.SessionID != "ses_abc123" {
		t.Fatalf("session = %q", got.SessionID)
	}
	if got.Summary != "working\nfinal answer" {
		t.Fatalf("summary = %q", got.Summary)
	}
	if got.Usage == nil {
		t.Fatal("expected usage")
	}
	if got.Usage.InputTokens != 30 || got.Usage.OutputTokens != 12 {
		t.Fatalf("usage = %+v", got.Usage)
	}
	if got.Usage.CacheReadTokens != 5 || got.Usage.CacheWriteTokens != 2 {
		t.Fatalf("cache usage = %+v", got.Usage)
	}
	if got.Usage.Source != protocol.UsageSourceOpenCodeRunJSON {
		t.Fatalf("source = %q", got.Usage.Source)
	}
}

func TestParseTextFormat(t *testing.T) {
	got := parseRunOutput("plain output", "default")
	if got.Summary != "plain output" {
		t.Fatalf("summary = %q", got.Summary)
	}
	if got.SessionID != "" {
		t.Fatalf("session = %q, want empty", got.SessionID)
	}
}

func TestAdapterRunEndToEnd(t *testing.T) {
	repo := initOpenCodeRepo(t)
	stdout := `{"type":"text","sessionID":"ses_run1","part":{"type":"text","text":"opencode completed"}}
{"type":"step_finish","sessionID":"ses_run1","part":{"type":"step-finish","tokens":{"input":5,"output":2}}}`
	fake := writeFakeOpenCode(t, "trace-oc-1", "agent-oc-1", stdout)

	result, err := New().Run(context.Background(), adapters.RunRequest{
		Bee:          "worker",
		Prompt:       "do the task",
		SystemPrompt: "You are a worker.",
		ColonyRoot:   repo,
		Workspace:    repo,
		TraceID:      "trace-oc-1",
		AgentID:      "agent-oc-1",
		Params: adapters.RunParams{
			Binary:       fake,
			OutputFormat: "json",
			Trust:        true,
			Force:        true,
			Model:        "anthropic/claude-sonnet-4",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusCompleted) {
		t.Fatalf("status = %q", result.Status)
	}
	if result.Summary != "opencode completed" {
		t.Fatalf("summary = %q", result.Summary)
	}
	if result.ProviderSessionID != "ses_run1" {
		t.Fatalf("session = %q", result.ProviderSessionID)
	}
	if len(result.Events) != 0 {
		t.Fatalf("expected no bus events, got %d", len(result.Events))
	}
	if result.Usage == nil || result.Usage.InputTokens != 5 {
		t.Fatalf("usage = %+v", result.Usage)
	}

	runDir := runs.Dir{ColonyRoot: repo, TraceID: "trace-oc-1", AgentID: "agent-oc-1"}
	assertFileContains(t, runDir.PromptPath(), "do the task")
	assertFileContains(t, runDir.SystemPath(), "You are a worker.")
	assertFileExists(t, runDir.MetaPath())
	assertFileExists(t, runDir.ResultJSONPath())

	protoResult, err := runDir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if protoResult.ProviderSessionID != "ses_run1" {
		t.Fatalf("result.json session = %q", protoResult.ProviderSessionID)
	}

	metaData, err := os.ReadFile(runDir.MetaPath())
	if err != nil {
		t.Fatal(err)
	}
	var meta runs.Meta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.ProviderSessionID != "ses_run1" {
		t.Fatalf("meta session = %q", meta.ProviderSessionID)
	}

	logPath := filepath.Join(repo, ".paseka", "runs", "trace-oc-1", "agent-oc-1", "oc-invocation.log")
	log := string(readFile(t, logPath))
	if !strings.Contains(log, "run") || !strings.Contains(log, "--format json") {
		t.Fatalf("invocation log missing run flags: %q", log)
	}
	if !strings.Contains(log, "--auto") || !strings.Contains(log, "--title agent-oc-1") {
		t.Fatalf("invocation log missing auto/title: %q", log)
	}
	if strings.Contains(log, "--api-key") {
		t.Fatalf("must not pass --api-key: %q", log)
	}
	if !strings.Contains(log, "You are a worker.") || !strings.Contains(log, "do the task") {
		t.Fatalf("invocation log missing joined prompt: %q", log)
	}
}

func TestAdapterRunCommandOverrideSkipsMappedFlags(t *testing.T) {
	repo := initOpenCodeRepo(t)
	fake := writeFakeOpenCode(t, "trace-oc-2", "agent-oc-2", `{"type":"text","sessionID":"ses_cmd","part":{"text":"from override"}}`)

	result, err := New().Run(context.Background(), adapters.RunRequest{
		Bee:        "worker",
		Prompt:     "do the task",
		ColonyRoot: repo,
		Workspace:  repo,
		TraceID:    "trace-oc-2",
		AgentID:    "agent-oc-2",
		Command:    []string{fake, "run", "--format", "json", "custom prompt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProviderSessionID != "ses_cmd" {
		t.Fatalf("session = %q", result.ProviderSessionID)
	}

	logPath := filepath.Join(repo, ".paseka", "runs", "trace-oc-2", "agent-oc-2", "oc-invocation.log")
	log := string(readFile(t, logPath))
	if strings.Contains(log, "--auto") || strings.Contains(log, "--title") {
		t.Fatalf("command override must not inject mapped flags: %q", log)
	}
	if !strings.Contains(log, "custom prompt") {
		t.Fatalf("override prompt missing: %q", log)
	}
}

func initOpenCodeRepo(t *testing.T) string {
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
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\nchanged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeFakeOpenCode(t *testing.T, traceID, agentID, stdout string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-opencode")
	logRel := filepath.Join(".paseka", "runs", traceID, agentID, "oc-invocation.log")
	script := "#!/bin/sh\n" +
		"mkdir -p \"$(dirname \"$PWD/" + logRel + "\")\" 2>/dev/null || true\n" +
		"echo \"$@\" >\"$PWD/" + logRel + "\"\n" +
		"printf '%s\\n' " + shellQuote(stdout) + "\n"
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

func assertArgs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d args, want %d: %v vs %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
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

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("missing %s: %v", path, err)
	}
}

func assertFileContains(t *testing.T, path, substr string) {
	t.Helper()
	data := readFile(t, path)
	if !strings.Contains(string(data), substr) {
		t.Fatalf("%s missing %q: %s", path, substr, data)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
