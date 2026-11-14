package prompts_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/prompts"
)

// colonyRoot locates the repository checkout this package belongs to, so these
// tests exercise the committed Archivist configuration and prompt partials
// rather than a copy of them.
func colonyRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test file")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, ".paseka", "bees", "archivist.yaml")); err != nil {
		t.Fatalf("colony root %s has no archivist bee: %v", root, err)
	}
	return root
}

func TestArchivistDiscoversSkillIntent(t *testing.T) {
	root := colonyRoot(t)
	bee, _, err := colony.LoadBee(root, "archivist")
	if err != nil {
		t.Fatal(err)
	}
	intents, defaultIntent, err := prompts.DiscoverIntents(root, bee)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"chronicle", "skill", "task"}
	if len(intents) != len(want) {
		t.Fatalf("intents = %#v, want %#v", intents, want)
	}
	for i, intent := range want {
		if intents[i] != intent {
			t.Fatalf("intents[%d] = %q, want %q", i, intents[i], intent)
		}
	}
	if defaultIntent != "task" {
		t.Fatalf("defaultIntent = %q, want task", defaultIntent)
	}

	var publishesProposal bool
	for _, pub := range bee.Publishes {
		if pub.Type == "INSIGHT" && pub.Kind == "skill.proposed" {
			publishesProposal = true
		}
	}
	if !publishesProposal {
		t.Fatalf("publishes = %+v, want INSIGHT/skill.proposed", bee.Publishes)
	}

	skillRules := bee.CompletionContract.ForIntent("skill").Required
	if len(skillRules) != 1 || len(skillRules[0].KindOneOf) != 1 || skillRules[0].KindOneOf[0] != "skill.proposed" {
		t.Fatalf("skill contract = %+v, want the proposal rule", skillRules)
	}
	for _, intent := range []string{"task", "chronicle"} {
		rules := bee.CompletionContract.ForIntent(intent).Required
		if len(rules) != 1 || len(rules[0].KindOneOf) != 1 || rules[0].KindOneOf[0] != "context.note" {
			t.Fatalf("%s contract = %+v, want the note rule", intent, rules)
		}
	}
}

func renderArchivist(t *testing.T, root, intent string) string {
	t.Helper()
	loader, err := prompts.NewLoader(root)
	if err != nil {
		t.Fatal(err)
	}
	traceID := "trace-archivist-test"
	artifacts := filepath.Join(root, ".paseka", "runs", traceID, "artifacts")
	got, err := loader.Render("archivist.md", prompts.Context{
		Bee:          "archivist",
		TraceID:      traceID,
		Intent:       intent,
		IntentRaw:    intent,
		ColonyRoot:   root,
		AgentID:      "agent-archivist-test",
		ArtifactsDir: artifacts,
		ResultFile:   filepath.Join(root, ".paseka", "runs", traceID, "agent-archivist-test", "summary.md"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestArchivistSkillIntentRendersSkillGuidance(t *testing.T) {
	root := colonyRoot(t)
	got := renderArchivist(t, root, "skill")

	if !strings.Contains(got, "Propose **one** procedure this finished trail proved the colony needs.") {
		t.Fatalf("skill guidance missing:\n%s", got)
	}
	if !strings.Contains(got, filepath.Join(root, ".paseka", "runs", "trace-archivist-test", "artifacts", "skill-proposals")) {
		t.Fatalf("skill bundle path missing:\n%s", got)
	}
	if !strings.Contains(got, "exactly one `INSIGHT/skill.proposed`") {
		t.Fatalf("proposal emit guidance missing:\n%s", got)
	}
	if strings.Contains(got, "Audit one task of this trail for work that was promised") {
		t.Fatalf("skill intent rendered the task guidance:\n%s", got)
	}
	if strings.Contains(got, "Audit how this trail *ran*") {
		t.Fatalf("skill intent rendered the chronicle guidance:\n%s", got)
	}
	if strings.Contains(got, "Write the draft to") {
		t.Fatalf("skill intent rendered the audit report shape:\n%s", got)
	}
}

func TestArchivistNoteIntentsDoNotRenderSkillGuidance(t *testing.T) {
	root := colonyRoot(t)
	skillMarker := "Propose **one** procedure this finished trail proved the colony needs."

	taskPrompt := renderArchivist(t, root, "task")
	if !strings.Contains(taskPrompt, "Audit one task of this trail for work that was promised") {
		t.Fatalf("task guidance missing:\n%s", taskPrompt)
	}
	if strings.Contains(taskPrompt, skillMarker) {
		t.Fatalf("task intent rendered the skill guidance:\n%s", taskPrompt)
	}
	if !strings.Contains(taskPrompt, "exactly one `INSIGHT/context.note`") {
		t.Fatalf("task intent emit guidance does not publish context.note")
	}

	chroniclePrompt := renderArchivist(t, root, "chronicle")
	if !strings.Contains(chroniclePrompt, "Audit how this trail *ran*") {
		t.Fatalf("chronicle guidance missing:\n%s", chroniclePrompt)
	}
	if strings.Contains(chroniclePrompt, skillMarker) {
		t.Fatalf("chronicle intent rendered the skill guidance:\n%s", chroniclePrompt)
	}
	if !strings.Contains(chroniclePrompt, "exactly one `INSIGHT/context.note`") {
		t.Fatalf("chronicle intent emit guidance does not publish context.note")
	}
}
