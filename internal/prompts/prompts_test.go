package prompts_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/prompts"
)

func writePromptTree(t *testing.T, colonyRoot string) {
	t.Helper()
	dirs := []string{
		".paseka/prompts/_partials",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(colonyRoot, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		".paseka/prompts/builder.md": `# Builder

Colony: {{.ColonyRoot}}
Trail: {{.TraceID}}
Task: {{.Task}}
{{template "json-events" .}}
`,
		".paseka/prompts/_partials/json-events.md": `Emit valid JSON events on the bus.`,
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(colonyRoot, path), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRenderWithPartial(t *testing.T) {
	root := t.TempDir()
	writePromptTree(t, root)

	loader, err := prompts.NewLoader(root)
	if err != nil {
		t.Fatal(err)
	}

	got, err := loader.Render("builder.md", prompts.Context{
		ColonyRoot: root,
		TraceID:    "trace-1",
		Task:       "add login",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !containsAll(got, root, "trace-1", "add login", "Emit valid JSON") {
		t.Fatalf("unexpected render:\n%s", got)
	}
}

func TestRenderResolvedInlineWins(t *testing.T) {
	root := t.TempDir()
	writePromptTree(t, root)

	loader, err := prompts.NewLoader(root)
	if err != nil {
		t.Fatal(err)
	}

	got, err := loader.RenderResolved(prompts.ResolveInput{
		InlinePrompt: "one-shot {{.Task}}",
		BeeTemplate:  "builder.md",
	}, prompts.Context{Task: "fix bug"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "one-shot fix bug" {
		t.Fatalf("got %q", got)
	}
}

func TestResolvePrecedence(t *testing.T) {
	cases := []struct {
		name string
		in   prompts.ResolveInput
		file string
		body string
	}{
		{
			name: "inline",
			in:   prompts.ResolveInput{InlinePrompt: "hi", BeeTemplate: "builder.md"},
			body: "hi",
		},
		{
			name: "bee local",
			in:   prompts.ResolveInput{BeeLocalTemplate: "local.md", BeeTemplate: "builder.md"},
			file: "local.md",
		},
		{
			name: "bee",
			in:   prompts.ResolveInput{BeeTemplate: "builder.md"},
			file: "builder.md",
		},
		{
			name: "default",
			in:   prompts.ResolveInput{DefaultTemplate: "default.md"},
			file: "default.md",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, body, err := prompts.Resolve(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if file != tc.file || body != tc.body {
				t.Fatalf("file=%q body=%q, want file=%q body=%q", file, body, tc.file, tc.body)
			}
		})
	}
}

func TestRenderInsights(t *testing.T) {
	root := t.TempDir()
	writePromptTree(t, root)
	if err := os.WriteFile(filepath.Join(root, ".paseka/prompts/scout.md"),
		[]byte("{{range .Insights}}* {{.}}\n{{end}}"), 0o644); err != nil {
		t.Fatal(err)
	}

	loader, err := prompts.NewLoader(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := loader.Render("scout.md", prompts.Context{
		Insights: []string{"a", "b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "* a\n* b\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRejectPathEscape(t *testing.T) {
	root := t.TempDir()
	writePromptTree(t, root)

	loader, err := prompts.NewLoader(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = loader.Render("../outside.md", prompts.Context{})
	if err == nil {
		t.Fatal("expected error for path escape")
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if p != "" && !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
