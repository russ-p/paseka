package prompts_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/prompts"
)

// The refactor-tick procedure only runs once a day at most, so a broken
// template would surface on the next tick instead of at commit time. Render the
// committed prompt against the colony's real partials and assert the pieces the
// procedure cannot work without.
func TestNurseRefactorTickPrompt(t *testing.T) {
	root := colonyRoot(t)
	loader, err := prompts.NewLoader(root)
	if err != nil {
		t.Fatalf("loader: %v", err)
	}
	trace := "trail-complexity"
	run := filepath.Join(root, ".paseka", "runs", trace, "agent-nurse")
	out, err := loader.Render("nurse.md", prompts.Context{
		Bee:          "nurse",
		TraceID:      trace,
		AgentID:      "agent-nurse",
		ColonyRoot:   root,
		Workspace:    root,
		Task:         "tick 2026-11-17",
		Intent:       "refactor-tick",
		IntentRaw:    "refactor-tick",
		ArtifactsDir: filepath.Join(run, "artifacts"),
		ResultFile:   filepath.Join(run, "summary.md"),
		Adapter:      "cursor",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	for _, want := range []string{
		"scripts/refactor-targets.py",
		"paseka cue run refactor",
		"one commit",
		"checkpoint.json",
		// Included through emit-howto: the standing contract the tick inherits.
		"Standing Trail",
		"observes and records",
		trace,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("nurse prompt lost %q", want)
		}
	}
	if strings.Contains(out, "{{") {
		t.Errorf("nurse prompt rendered with an unexpanded template action")
	}
}
