package colony_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
)

func writeColonyManifest(t *testing.T, manifest string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".paseka")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "colony.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	writeColonyBee(t, root, "builder", "")
	writeColonyBee(t, root, "reviewer", "")
	writeColonyBee(t, root, "chattering", "subscribes:\n  - type: SIGNAL\n    kind: review.needed\n    dispatch: direct\n")
	return root
}

func writeColonyBee(t *testing.T, root, role, extra string) {
	t.Helper()
	dir := filepath.Join(root, ".paseka", "bees")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "role: " + role + "\nadapter: script\nprompt_template: " + role + ".md\ncommand: [echo]\n" + extra
	if err := os.WriteFile(filepath.Join(dir, role+".yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadColonyWithoutClosingBlockIsDefaultDisabled(t *testing.T) {
	root := writeColonyManifest(t, "slug: test\n")

	c, err := colony.LoadColony(root)
	if err != nil {
		t.Fatal(err)
	}
	if c.Closing.Enabled {
		t.Fatal("closing is enabled by default")
	}
	if got := c.Closing.ResolvedMaxRounds(); got != colony.DefaultClosingMaxRounds {
		t.Fatalf("ResolvedMaxRounds() = %d, want %d", got, colony.DefaultClosingMaxRounds)
	}
	if c.Merge.Configured() {
		t.Fatal("merge copy is configured by default")
	}
	if c.Closing.Matches(0, false) != true {
		t.Fatal("a default policy must match an empty trail")
	}
}

func TestLoadColonyClosingPolicy(t *testing.T) {
	root := writeColonyManifest(t, `slug: test
closing:
  enabled: true
  when:
    min_work_tasks: 2
    has_isolated_proposal: true
  task:
    bee: reviewer
    intent: closing
    title: Review the trail and prepare the pull request
    review: none
    params:
      model: strong
  max_rounds: 2
merge:
  task:
    title: "Review and merge: {{.TraceTitle}}"
    body: |
      ## What this trail did
      {{range .Tasks}}- {{.TaskID}} ({{.Intent}}) — {{.Summary}}
      {{end}}
`)

	c, err := colony.LoadColony(root)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Closing.Enabled {
		t.Fatal("closing is not enabled")
	}
	if c.Closing.When.MinWorkTasks != 2 || !c.Closing.When.HasIsolatedProposal {
		t.Fatalf("when = %+v", c.Closing.When)
	}
	if c.Closing.Task.Bee != "reviewer" || c.Closing.Task.Intent != "closing" {
		t.Fatalf("task = %+v", c.Closing.Task)
	}
	if got := c.Closing.ResolvedMaxRounds(); got != 2 {
		t.Fatalf("ResolvedMaxRounds() = %d", got)
	}
	if !c.Closing.Matches(2, true) || c.Closing.Matches(1, true) || c.Closing.Matches(2, false) {
		t.Fatal("predicates did not evaluate as declared")
	}
	if !c.Merge.Configured() {
		t.Fatal("merge copy was not loaded")
	}
	if title, err := c.Merge.ResolvedTitle(colony.TrailInventory{}); err != nil || title != "Review and merge: " {
		t.Fatalf("rendered empty-inventory title = %q, %v", title, err)
	}
}

func TestLoadColonyRejectsInvalidClosingPolicies(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		want     []string
	}{
		{
			name: "unknown predicate",
			manifest: `closing:
  enabled: true
  when:
    min_proposals: 2
  task:
    bee: builder
`,
			want: []string{"closing.when", "unknown predicate", "min_proposals"},
		},
		{
			name: "negative minimum",
			manifest: `closing:
  enabled: false
  when:
    min_work_tasks: -1
`,
			want: []string{"closing.when.min_work_tasks", "negative"},
		},
		{
			name: "missing bee",
			manifest: `closing:
  enabled: true
  task:
    bee: ghost
`,
			want: []string{"closing.task.bee", `"ghost"`, "not defined"},
		},
		{
			name: "non task bee",
			manifest: `closing:
  enabled: true
  task:
    bee: chattering
`,
			want: []string{"closing.task.bee", `"chattering"`, "task.ready"},
		},
		{
			name: "final review",
			manifest: `closing:
  enabled: true
  task:
    bee: builder
    review: final
`,
			want: []string{"closing.task.review", `"final"`, "not allowed"},
		},
		{
			name: "unknown review",
			manifest: `closing:
  enabled: true
  task:
    bee: builder
    review: sometimes
`,
			want: []string{"closing.task.review", "unknown review policy"},
		},
		{
			name: "authored phase",
			manifest: `closing:
  enabled: true
  task:
    bee: builder
    phase: closing
`,
			want: []string{"closing.task.phase", "runtime assigns closing"},
		},
		{
			name: "zero max rounds",
			manifest: `closing:
  enabled: true
  task:
    bee: builder
  max_rounds: 0
`,
			want: []string{"closing.max_rounds", "at least 1"},
		},
		{
			name: "negative max rounds",
			manifest: `closing:
  enabled: true
  task:
    bee: builder
  max_rounds: -2
`,
			want: []string{"closing.max_rounds", "at least 1"},
		},
		{
			name: "unknown adapter param",
			manifest: `closing:
  enabled: true
  task:
    bee: builder
    params:
      modle: strong
`,
			want: []string{"closing.task.params", "modle"},
		},
		{
			name: "malformed merge title",
			manifest: `merge:
  task:
    title: "{{.TraceTitle"
`,
			want: []string{"merge.task.title", "parse template"},
		},
		{
			name: "malformed merge body",
			manifest: `merge:
  task:
    body: "{{range .Tasks}}"
`,
			want: []string{"merge.task.body", "parse template"},
		},
		{
			name: "merge title fails empty inventory",
			manifest: `merge:
  task:
    title: "{{index .Tasks 0}}"
`,
			want: []string{"merge.task.title", "render template"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeColonyManifest(t, "slug: test\n"+tc.manifest)
			_, err := colony.LoadColony(root)
			if err == nil {
				t.Fatal("LoadColony() succeeded, want an error")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q missing %q", err, want)
				}
			}
		})
	}
}

func TestLoadColonyAllowsDisabledClosingWithoutABee(t *testing.T) {
	root := writeColonyManifest(t, `slug: test
closing:
  enabled: false
  when:
    min_work_tasks: 3
  task:
    intent: closing
`)

	if _, err := colony.LoadColony(root); err != nil {
		t.Fatalf("disabled policy failed to load: %v", err)
	}
}

func TestMergePolicyFallsBackToLiteralCopy(t *testing.T) {
	var unset colony.MergePolicy
	title, err := unset.ResolvedTitle(colony.TrailInventory{})
	if err != nil || title != colony.DefaultMergeTaskTitle {
		t.Fatalf("default title = %q, %v", title, err)
	}
	body, err := unset.ResolvedBody(colony.TrailInventory{})
	if err != nil || body != colony.DefaultMergeTaskBody {
		t.Fatalf("default body = %q, %v", body, err)
	}

	authored := colony.MergePolicy{Task: colony.MergeTask{
		Title: "Review and merge: {{.TraceTitle}}",
		Body:  "{{range .Tasks}}{{.TaskID}} {{end}}",
	}}
	title, err = authored.ResolvedTitle(colony.TrailInventory{TraceTitle: "Ship it", Tasks: []colony.TrailTask{{TaskID: "task-1"}}})
	if err != nil || title != "Review and merge: Ship it" {
		t.Fatalf("authored title = %q, %v", title, err)
	}
	body, err = authored.ResolvedBody(colony.TrailInventory{Tasks: []colony.TrailTask{{TaskID: "task-1"}}})
	if err != nil || body != "task-1 " {
		t.Fatalf("authored body = %q, %v", body, err)
	}
}
