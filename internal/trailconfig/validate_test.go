package trailconfig_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
)

// TestApplyRefusesInvalidCandidateProcedure pins preparation validation to the
// complete procedure: templates, skills, adapters, policy, and every remaining
// task must be usable before a revision is recorded or a snapshot is retained.
func TestApplyRefusesInvalidCandidateProcedure(t *testing.T) {
	cases := []struct {
		name        string
		tasks       []taskledger.TaskSnapshot
		mutate      func(t *testing.T, root string)
		wantErr     string
		wantOutcome string
	}{
		{
			name: "valid candidate is applied",
			mutate: func(t *testing.T, root string) {
				writeFile(t, root, ".paseka/prompts/builder.md", "updated instructions\n")
			},
			wantOutcome: trailconfig.OutcomeApplied,
		},
		{
			name: "missing prompt template",
			mutate: func(t *testing.T, root string) {
				writeFile(t, root, ".paseka/bees/builder.yaml", "role: builder\nadapter: script\nprompt_template: missing.md\ncommand: [echo]\n")
				writeFile(t, root, ".paseka/bees/builder.local.yaml", "prompt_template: missing.md\n")
			},
			wantErr: `prompt template "missing.md" is missing`,
		},
		{
			name: "missing system template",
			mutate: func(t *testing.T, root string) {
				writeFile(t, root, ".paseka/bees/builder.yaml", "role: builder\nadapter: script\nprompt_template: builder.md\nsystem_template: missing-system.md\ncommand: [echo]\n")
			},
			wantErr: `prompt template "missing-system.md" is missing`,
		},
		{
			name: "missing skill SKILL.md",
			mutate: func(t *testing.T, root string) {
				writeFile(t, root, ".agents/skills/broken/notes.txt", "no manifest here\n")
			},
			wantErr: `skill "broken" is missing SKILL.md`,
		},
		{
			name: "invalid bee adapter",
			mutate: func(t *testing.T, root string) {
				writeFile(t, root, ".paseka/bees/builder.yaml", "role: builder\nadapter: telepathy\nprompt_template: builder.md\n")
			},
			wantErr: "unknown adapter",
		},
		{
			name: "removed bee for blocked task",
			tasks: []taskledger.TaskSnapshot{{
				TaskID: "task-1", Title: "blocked work", Bee: "ghost", Status: protocol.TaskStatusBlocked,
			}},
			mutate:  func(t *testing.T, root string) {},
			wantErr: `removes bee "ghost"`,
		},
		{
			name: "removed bee for failed task",
			tasks: []taskledger.TaskSnapshot{{
				TaskID: "task-1", Title: "failed work", Bee: "ghost", Status: protocol.TaskStatusFailed,
			}},
			mutate:  func(t *testing.T, root string) {},
			wantErr: `removes bee "ghost"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := writeColony(t, "slug: test\n")
			setupHome(t, "test")
			ctx := testContext(root)
			traceID := "trace-validate"
			initial, err := trailconfig.Ensure(ctx, traceID)
			if err != nil {
				t.Fatal(err)
			}
			for _, task := range tc.tasks {
				writeRemainingTask(t, root, traceID, task)
			}
			tc.mutate(t, root)

			expected := initial.ConfigRevision
			result, err := trailconfig.Apply(trailconfig.ApplyOptions{
				Context: ctx, TraceID: traceID, Source: trailconfig.SourceColony,
				Actor: "operator", ExpectedRevision: &expected,
			})
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				assertBindingUnchanged(t, root, traceID, initial)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.Change.Outcome != tc.wantOutcome {
				t.Fatalf("outcome = %q, want %q", result.Change.Outcome, tc.wantOutcome)
			}
		})
	}
}

func TestValidateProcedureRefusesUnresolvableReviewPolicy(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	writeFile(t, root, ".paseka/bees/reviewer.yaml",
		"role: reviewer\nadapter: script\ncommand: [echo]\npublishes:\n  - type: MUTATION\n    kind: code.proposal.root\n")
	policy, err := colony.LoadColony(root)
	if err != nil {
		t.Fatal(err)
	}
	err = trailconfig.ValidateProcedure(root, policy, []trailconfig.ProcedureWork{{
		TaskID: "task-1",
		Bee:    "reviewer",
		Review: protocol.TaskReviewFinal,
	}})
	if err == nil || !strings.Contains(err.Error(), "review:final") {
		t.Fatalf("err = %v, want review:final refusal", err)
	}
}

func TestValidateProcedureChecksEveryBeeAdapter(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	writeFile(t, root, ".paseka/bees/broken.yaml", "role: broken\nadapter: telepathy\n")
	policy, err := colony.LoadColony(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := trailconfig.ValidateProcedure(root, policy, nil); err == nil || !strings.Contains(err.Error(), "unknown adapter") {
		t.Fatalf("err = %v, want unknown adapter refusal", err)
	}
}

func TestTryActivateRejectsCandidateLosingCapturedTemplate(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	traceID := "trace-late-template"
	initial, err := trailconfig.Ensure(ctx, traceID)
	if err != nil {
		t.Fatal(err)
	}
	writeRemainingTask(t, root, traceID, taskledger.TaskSnapshot{
		TaskID: "task-1", Title: "running work", Bee: "builder", Status: protocol.TaskStatusRunning,
	})
	writeFile(t, root, ".paseka/prompts/builder.md", "prepared while running\n")
	expected := initial.ConfigRevision
	pending, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: traceID, Source: trailconfig.SourceColony,
		Actor: "operator", ExpectedRevision: &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pending.Change.Outcome != trailconfig.OutcomePending {
		t.Fatalf("outcome = %q, want pending", pending.Change.Outcome)
	}

	// The prepared content is durable, but its files can still disappear or be
	// damaged between preparation and the safe boundary.
	prompt := filepath.Join(root, ".paseka", "runs", traceID, "config", "snapshots",
		pending.NewRef, "tree", ".paseka", "prompts", "builder.md")
	if err := os.Remove(prompt); err != nil {
		t.Fatal(err)
	}
	writeRemainingTask(t, root, traceID, taskledger.TaskSnapshot{
		TaskID: "task-1", Title: "running work", Bee: "builder", Status: protocol.TaskStatusCompleted,
	})

	after, changed, err := trailconfig.TryActivate(root, traceID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if changed || after.Pending != nil {
		t.Fatalf("invalid candidate activated: %+v", after)
	}
	if after.ActiveConfigRef != initial.ActiveConfigRef || after.ConfigRevision != initial.ConfigRevision {
		t.Fatalf("rejection changed the active binding: %+v", after)
	}
	last := after.History[len(after.History)-1]
	if last.Outcome != trailconfig.OutcomeRejected || !strings.Contains(last.Detail, "prompt template") {
		t.Fatalf("rejection not recorded: %+v", last)
	}
}

func TestForkCandidateValidationRejectsBrokenTemplate(t *testing.T) {
	base := writeColony(t, "slug: test\n")
	candidate := writeColony(t, "slug: test\n")
	if err := os.Remove(filepath.Join(candidate, ".paseka", "prompts", "builder.md")); err != nil {
		t.Fatal(err)
	}
	err := trailconfig.ValidateForkCandidateTree(base, candidate, "", nil)
	if err == nil || !strings.Contains(err.Error(), "prompt template") {
		t.Fatalf("err = %v, want a broken-template refusal", err)
	}
}

func assertBindingUnchanged(t *testing.T, root, traceID string, initial trailconfig.Manifest) {
	t.Helper()
	reloaded, err := trailconfig.Load(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ConfigRevision != initial.ConfigRevision || reloaded.ActiveConfigRef != initial.ActiveConfigRef {
		t.Fatalf("refused candidate changed the binding: %+v", reloaded)
	}
	if len(reloaded.History) != len(initial.History) {
		t.Fatalf("refused candidate wrote history: %d -> %d", len(initial.History), len(reloaded.History))
	}
	entries, err := os.ReadDir(filepath.Join(root, ".paseka", "runs", traceID, "config", "snapshots"))
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}
	if len(dirs) != 1 || dirs[0] != initial.ActiveConfigRef {
		t.Fatalf("refused candidate retained snapshots: %v", dirs)
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
