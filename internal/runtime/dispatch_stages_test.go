package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
)

func TestPrepareDispatchRequiresColonyRoot(t *testing.T) {
	d := NewDispatcher()
	_, err := d.prepareDispatch(context.Background(), DispatchRequest{
		Bee:     "builder",
		TraceID: "trace-1",
	})
	if err == nil || !strings.Contains(err.Error(), "colony root") {
		t.Fatalf("err = %v", err)
	}
}

func TestPrepareDispatchRequiresBee(t *testing.T) {
	d := NewDispatcher()
	_, err := d.prepareDispatch(context.Background(), DispatchRequest{
		ColonyRoot: t.TempDir(),
		TraceID:    "trace-1",
	})
	if err == nil || !strings.Contains(err.Error(), "bee role") {
		t.Fatalf("err = %v", err)
	}
}

func TestDispatchOrchestratesStages(t *testing.T) {
	root := t.TempDir()
	writeStageColony(t, root)

	rec := &stageRecordingAdapter{}
	d := NewDispatcher()
	d.RegisterAdapter("cursor", rec)

	result, err := d.Dispatch(context.Background(), DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-1",
		Task:       "ship it",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.calls != 1 {
		t.Fatalf("adapter calls = %d", rec.calls)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q", result.Status)
	}
}

type stageRecordingAdapter struct {
	calls int
}

func (s *stageRecordingAdapter) Name() string { return "cursor" }

func (s *stageRecordingAdapter) Run(_ context.Context, _ adapters.RunRequest) (*adapters.RunResult, error) {
	s.calls++
	return &adapters.RunResult{Status: "completed", Output: "ok"}, nil
}

func writeStageColony(t *testing.T, root string) {
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
`,
		".paseka/prompts/builder.md": `Builder bee.
Task: {{.Task}}
`,
		".paseka/prompts/default.md": `Default {{.Task}}`,
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
