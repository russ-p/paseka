package pi

import (
	"context"
	"encoding/json"
	"errors"
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
	if New().Name() != "pi" {
		t.Fatal("expected adapter name pi")
	}
}

func TestBuildArgs(t *testing.T) {
	req := adapters.RunRequest{
		Params: adapters.RunParams{
			Model:        "gpt-4",
			OutputFormat: "json",
			Provider:     "gemini",
			Thinking:     "high",
			Plan:         true,
			APIKey:       "secret",
			Trust:        true,
			Force:        true,
		},
	}
	args := buildArgs(req, "implement feature", "json")

	want := []string{
		"-p", "--mode", "json",
		"--model", "gpt-4",
		"--provider", "gemini",
		"--thinking", "high",
		"--plan",
		"--api-key", "secret",
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

func TestPiModeDefaultsInvalidToJSON(t *testing.T) {
	if got := piMode(""); got != "json" {
		t.Fatalf("piMode(\"\") = %q, want json", got)
	}
	if got := piMode("stream-json"); got != "json" {
		t.Fatalf("piMode(stream-json) = %q, want json", got)
	}
	if got := piMode("text"); got != "text" {
		t.Fatalf("piMode(text) = %q", got)
	}
}

func TestParseJSONSummary(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "summary field", in: `{"summary":"done"}`, want: "done"},
		{name: "output field", in: `{"output":"hello"}`, want: "hello"},
		{name: "nested response", in: `{"response":{"text":"nested"}}`, want: "nested"},
		{name: "ndjson last line", in: "{\"progress\":1}\n{\"result\":\"final\"}", want: "final"},
		{name: "plain text mode", in: "plain output", want: "plain output"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mode := "json"
			if tc.name == "plain text mode" {
				mode = "text"
			}
			got := extractSummary(tc.in, mode)
			if got != tc.want {
				t.Fatalf("extractSummary() = %q, want %q", got, tc.want)
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

func TestAdapterRunEndToEnd(t *testing.T) {
	repo := initPiRepo(t)
	fakePi := writeFakePi(t, `{"summary":"pi completed"}`)

	adapter := New()
	result, err := adapter.Run(context.Background(), adapters.RunRequest{
		Bee:        "worker",
		Prompt:     "do the task",
		ColonyRoot: repo,
		Workspace:  repo,
		TraceID:    "trace-pi-1",
		AgentID:    "agent-pi-1",
		Params: adapters.RunParams{
			Binary:       fakePi,
			OutputFormat: "json",
			Model:        "test-model",
			Provider:     "gemini",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusCompleted) {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if result.Summary != "pi completed" {
		t.Fatalf("summary = %q", result.Summary)
	}
	if len(result.Events) != 0 {
		t.Fatalf("expected no bus events from pi stdout, got %d", len(result.Events))
	}

	runDir := runs.Dir{ColonyRoot: repo, TraceID: "trace-pi-1", AgentID: "agent-pi-1"}
	assertFileContains(t, runDir.PromptPath(), "do the task")
	assertFileExists(t, runDir.MetaPath())
	assertFileExists(t, runDir.StatusPath())
	assertFileExists(t, runDir.ResultJSONPath())

	status, err := runDir.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.State != protocol.StatusCompleted {
		t.Fatalf("status snapshot = %q", status.State)
	}

	protoResult, err := runDir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if protoResult.Summary != "pi completed" {
		t.Fatalf("result summary = %q", protoResult.Summary)
	}

	var kinds []string
	for _, art := range result.Artifacts {
		kinds = append(kinds, art.Kind)
	}
	if !containsAll(kinds, "stdout", "diff") {
		t.Fatalf("artifact kinds = %v", kinds)
	}

	logPath := filepath.Join(repo, ".paseka", "runs", "trace-pi-1", "agent-pi-1", "pi-invocation.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	if !strings.Contains(log, "-p") || !strings.Contains(log, "--mode json") {
		t.Fatalf("invocation log missing expected args: %q", log)
	}
	if !strings.Contains(log, "do the task") {
		t.Fatalf("invocation log missing prompt: %q", log)
	}
}

func initPiRepo(t *testing.T) string {
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

func writeFakePi(t *testing.T, stdout string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-pi")
	script := "#!/bin/sh\n" +
		"echo \"$@\" >\"$PWD/.paseka/runs/trace-pi-1/agent-pi-1/pi-invocation.log\" 2>/dev/null || " +
		"echo \"$@\" >\"$PWD/pi-invocation.log\"\n" +
		"printf '%s\\n' " + shellQuote(stdout) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"\''"'"`) + "'"
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %s: %v", path, err)
	}
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("%s: want substring %q in %q", path, want, string(data))
	}
}

func containsAll(have []string, want ...string) bool {
	set := make(map[string]struct{}, len(have))
	for _, item := range have {
		set[item] = struct{}{}
	}
	for _, item := range want {
		if _, ok := set[item]; !ok {
			return false
		}
	}
	return true
}

func TestAdapterRunStderrArtifact(t *testing.T) {
	repo := initPiRepo(t)
	fakePi := filepath.Join(t.TempDir(), "fake-pi-stderr")
	script := "#!/bin/sh\nprintf '%s\\n' '{\"summary\":\"ok\"}'\nprintf 'warn\\n' >&2\n"
	if err := os.WriteFile(fakePi, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := New().Run(context.Background(), adapters.RunRequest{
		Prompt:     "task",
		ColonyRoot: repo,
		Workspace:  repo,
		TraceID:    "trace-pi-2",
		AgentID:    "agent-pi-2",
		Params:     adapters.RunParams{Binary: fakePi},
	})
	if err != nil {
		t.Fatal(err)
	}
	var stderrFound bool
	for _, art := range result.Artifacts {
		if art.Kind == "stderr" && strings.Contains(art.Content, "warn") {
			stderrFound = true
		}
	}
	if !stderrFound {
		t.Fatalf("stderr artifact missing: %+v", result.Artifacts)
	}
}

func TestAdapterRunDoesNotParseEventsFromStdout(t *testing.T) {
	repo := initPiRepo(t)
	// stdout looks like a domain event but must not be converted.
	eventJSON, _ := json.Marshal(map[string]any{
		"type":    "VERIFICATION",
		"payload": map[string]any{"kind": "verification.success"},
	})
	fakePi := writeFakePi(t, string(eventJSON))

	result, err := New().Run(context.Background(), adapters.RunRequest{
		Prompt:     "task",
		ColonyRoot: repo,
		Workspace:  repo,
		TraceID:    "trace-pi-1",
		AgentID:    "agent-pi-1",
		Params:     adapters.RunParams{Binary: fakePi},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 0 {
		t.Fatalf("expected no parsed events, got %+v", result.Events)
	}
}
