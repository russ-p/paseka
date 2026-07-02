package runtime_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/runtime"
)

type recordingAdapter struct {
	lastReq adapters.RunRequest
}

func (r *recordingAdapter) Name() string { return "cursor" }

func (r *recordingAdapter) Run(_ context.Context, req adapters.RunRequest) (*adapters.RunResult, error) {
	r.lastReq = req
	return &adapters.RunResult{Status: "completed", Output: "ok"}, nil
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
