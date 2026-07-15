package runtime_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runtime"
)

type recordingAdapter struct {
	lastReq adapters.RunRequest
	events  []protocol.Event
	result  *adapters.RunResult
	runErr  error
	calls   int
}

func (r *recordingAdapter) Name() string { return "cursor" }

func (r *recordingAdapter) Run(_ context.Context, req adapters.RunRequest) (*adapters.RunResult, error) {
	r.calls++
	r.lastReq = req
	if r.runErr != nil {
		return nil, r.runErr
	}
	if r.result != nil {
		out := *r.result
		if len(r.events) > 0 {
			out.Events = r.events
		}
		return &out, nil
	}
	return &adapters.RunResult{Status: "completed", Output: "ok", Events: r.events}, nil
}

func writeColony(t *testing.T, root string) {
	t.Helper()
	dirs := []string{
		".paseka/bees",
		".paseka/prompts/_partials",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		".paseka/colony.yaml": `defaults:
  prompt_template: default.md
`,
		".paseka/bees/builder.yaml": `role: builder
adapter: cursor
prompt_template: builder.md
params:
  model: composer-2.5
  trust: true
  force: true
`,
		".paseka/prompts/builder.md": `Builder bee.
Task: {{.Task}}
Result: {{.ResultFile}}
Trail: {{.TraceID}}
`,
		".paseka/prompts/default.md": `Default {{.Task}}`,
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDispatchIntentPassthrough(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)

	rec := &recordingAdapter{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)

	_, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-abc",
		Task:       "fix flaky test",
		Intent:     "test-fix",
	})
	if err != nil {
		t.Fatal(err)
	}

	if rec.lastReq.Intent != "test-fix" {
		t.Fatalf("adapter intent = %q", rec.lastReq.Intent)
	}

	requestPath := filepath.Join(root, ".paseka", "runs", "trace-abc", rec.lastReq.AgentID, "request.json")
	data, err := os.ReadFile(requestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"intent": "test-fix"`) {
		t.Fatalf("request.json missing intent: %s", data)
	}
}

func TestDispatchTaskIDPassthrough(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)

	rec := &recordingAdapter{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)

	_, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-abc",
		TaskID:     "task-1",
		Task:       "implement auth",
	})
	if err != nil {
		t.Fatal(err)
	}

	if rec.lastReq.TaskID != "task-1" {
		t.Fatalf("adapter taskId = %q", rec.lastReq.TaskID)
	}

	requestPath := filepath.Join(root, ".paseka", "runs", "trace-abc", rec.lastReq.AgentID, "request.json")
	data, err := os.ReadFile(requestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"taskId": "task-1"`) {
		t.Fatalf("request.json missing taskId: %s", data)
	}
}

func TestDispatchRendersPromptBeforeAdapter(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)

	rec := &recordingAdapter{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)

	_, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-abc",
		Task:       "implement auth",
	})
	if err != nil {
		t.Fatal(err)
	}

	requestPath := filepath.Join(root, ".paseka", "runs", "trace-abc", rec.lastReq.AgentID, "request.json")
	if _, err := os.Stat(requestPath); err != nil {
		t.Fatalf("request.json missing: %v", err)
	}

	prompt := rec.lastReq.Prompt
	if !strings.Contains(prompt, "implement auth") {
		t.Fatalf("prompt missing task: %q", prompt)
	}
	if !strings.Contains(prompt, "trace-abc") {
		t.Fatalf("prompt missing traceId: %q", prompt)
	}
	if !strings.Contains(prompt, filepath.Join(root, ".paseka", "runs", "trace-abc")) {
		t.Fatalf("prompt missing result path: %q", prompt)
	}
	if rec.lastReq.Bee != "builder" {
		t.Fatalf("bee = %q", rec.lastReq.Bee)
	}
	if rec.lastReq.Params.Model != "composer-2.5" {
		t.Fatalf("model = %q", rec.lastReq.Params.Model)
	}
}

func TestDispatchInlinePromptOverridesTemplate(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)

	rec := &recordingAdapter{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)

	_, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot:   root,
		Bee:          "builder",
		TraceID:      "t1",
		InlinePrompt: "direct {{.Task}}",
		Task:         "hotfix",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.lastReq.Prompt != "direct hotfix" {
		t.Fatalf("got prompt %q", rec.lastReq.Prompt)
	}
}

func TestDispatchBeeLocalTemplate(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)
	if err := os.WriteFile(filepath.Join(root, ".paseka/bees/builder.local.yaml"),
		[]byte("prompt_template: local.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".paseka/prompts/local.md"),
		[]byte("local template {{.Task}}"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := &recordingAdapter{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)

	_, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "t1",
		Task:       "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.lastReq.Prompt != "local template x" {
		t.Fatalf("got %q", rec.lastReq.Prompt)
	}
}

func TestDispatchCustomCommand(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)
	beeYAML := `role: builder
adapter: cursor
prompt_template: builder.md
command: agent -p --yolo $PROMPT
`
	if err := os.WriteFile(filepath.Join(root, ".paseka/bees/builder.yaml"), []byte(beeYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := &recordingAdapter{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)

	_, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-cmd",
		Task:       "ship feature",
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"agent", "-p", "--yolo"}
	got := rec.lastReq.Command
	if len(got) < len(want) {
		t.Fatalf("command = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("command[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
	if !strings.Contains(rec.lastReq.Command[len(rec.lastReq.Command)-1], "ship feature") {
		t.Fatalf("prompt not in command tail: %v", rec.lastReq.Command)
	}
}

func TestDispatchPostExec(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)
	hookOut := filepath.Join(root, "hook.out")
	beeYAML := `role: builder
adapter: cursor
prompt_template: builder.md
post_exec: ["sh", "-c", "echo $RESULT > ` + hookOut + `"]
`
	if err := os.WriteFile(filepath.Join(root, ".paseka/bees/builder.yaml"), []byte(beeYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := &recordingAdapter{
		result: &adapters.RunResult{
			Status:  "completed",
			Summary: "hooked summary",
		},
	}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)

	_, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-hook",
		Task:       "notify me",
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(hookOut)
	if err != nil {
		t.Fatalf("hook output missing: %v", err)
	}
	if strings.TrimSpace(string(data)) != "hooked summary" {
		t.Fatalf("hook output = %q, want %q", data, "hooked summary")
	}
}

func TestDispatchPostExecTaskID(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)
	hookOut := filepath.Join(root, "hook.task")
	beeYAML := `role: builder
adapter: cursor
prompt_template: builder.md
post_exec: ["sh", "-c", "echo $TASK_ID > ` + hookOut + `"]
`
	if err := os.WriteFile(filepath.Join(root, ".paseka/bees/builder.yaml"), []byte(beeYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := &recordingAdapter{
		result: &adapters.RunResult{Status: "completed", Summary: "ok"},
	}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)

	_, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-hook-task",
		TaskID:     "task-hook-1",
		Task:       "notify me",
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(hookOut)
	if err != nil {
		t.Fatalf("hook output missing: %v", err)
	}
	if strings.TrimSpace(string(data)) != "task-hook-1" {
		t.Fatalf("hook task id = %q, want task-hook-1", data)
	}
}

func TestDispatchScriptBee(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".paseka/bees"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".paseka/colony.yaml"), []byte(`defaults:
  prompt_template: default.md
`), 0o644); err != nil {
		t.Fatal(err)
	}

	outFile := filepath.Join(root, "script.out")
	scriptPath := filepath.Join(root, "run.sh")
	scriptBody := "#!/bin/sh\n" +
		"echo \"$PASEKA_TRACE_ID\" > \"" + outFile + "\"\n" +
		"echo \"$PASEKA_TASK_ID\" >> \"" + outFile + "\"\n" +
		"echo ok\n"
	if err := os.WriteFile(scriptPath, []byte(scriptBody), 0o755); err != nil {
		t.Fatal(err)
	}

	beeYAML := "role: oracle-guard\nadapter: script\ncommand: " + scriptPath + "\nrun_summary: disabled\n"
	if err := os.WriteFile(filepath.Join(root, ".paseka/bees/oracle-guard.yaml"), []byte(beeYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	d := runtime.NewDispatcher()
	result, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "oracle-guard",
		TraceID:    "trace-script",
		TaskID:     "task-script",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if result.Summary != "ok" {
		t.Fatalf("summary = %q, want ok", result.Summary)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 || lines[0] != "trace-script" || lines[1] != "task-script" {
		t.Fatalf("script output = %v", lines)
	}
}
