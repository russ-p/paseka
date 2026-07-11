package prompts_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/prompts"
)

func TestNormalizeIntent(t *testing.T) {
	known := []string{"general", "feature", "bugfix", "test-fix", "refactor"}
	cases := []struct {
		in   string
		want string
	}{
		{"", "general"},
		{"general", "general"},
		{"FEATURE", "feature"},
		{"bugfix", "bugfix"},
		{"test-fix", "test-fix"},
		{"refactor", "refactor"},
		{"custom-unknown", "general"},
	}
	for _, tc := range cases {
		if got := prompts.NormalizeIntent(tc.in, known, "general"); got != tc.want {
			t.Fatalf("NormalizeIntent(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeIntentDrone(t *testing.T) {
	known := []string{"breakdown", "general", "grilling"}
	cases := []struct {
		in   string
		want string
	}{
		{"", "general"},
		{"grilling", "grilling"},
		{"breakdown", "breakdown"},
		{"unknown", "general"},
	}
	for _, tc := range cases {
		if got := prompts.NormalizeIntent(tc.in, known, "general"); got != tc.want {
			t.Fatalf("NormalizeIntent(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPromptContextPreservesRawIntent(t *testing.T) {
	known := []string{"general", "feature"}
	ctx := prompts.PromptContext(prompts.Context{IntentRaw: "CUSTOM"}, known, "general")
	if ctx.Intent != prompts.IntentGeneral {
		t.Fatalf("Intent = %q, want general", ctx.Intent)
	}
	if ctx.IntentRaw != "CUSTOM" {
		t.Fatalf("IntentRaw = %q", ctx.IntentRaw)
	}
}

func TestDiscoverIntentsFromYAML(t *testing.T) {
	root := t.TempDir()
	bee := colony.Bee{
		Role:          "custom",
		Intents:       []string{"alpha", "BETA", "alpha"},
		DefaultIntent: "beta",
	}
	intents, defaultIntent, err := prompts.DiscoverIntents(root, bee)
	if err != nil {
		t.Fatal(err)
	}
	if len(intents) != 2 || intents[0] != "alpha" || intents[1] != "beta" {
		t.Fatalf("intents = %#v", intents)
	}
	if defaultIntent != "beta" {
		t.Fatalf("defaultIntent = %q", defaultIntent)
	}
}

func TestDiscoverIntentsFromPartials(t *testing.T) {
	root := t.TempDir()
	partialsDir := filepath.Join(root, ".paseka", "prompts", "_partials")
	if err := os.MkdirAll(partialsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"builder-intent-general.md",
		"builder-intent-feature.md",
		"builder-intent-bugfix.md",
		"drone-intent-grilling.md",
	} {
		if err := os.WriteFile(filepath.Join(partialsDir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	bee := colony.Bee{Role: "builder"}
	intents, defaultIntent, err := prompts.DiscoverIntents(root, bee)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"bugfix", "feature", "general"}
	if len(intents) != len(want) {
		t.Fatalf("intents = %#v, want %#v", intents, want)
	}
	for i, intent := range want {
		if intents[i] != intent {
			t.Fatalf("intents[%d] = %q, want %q", i, intents[i], intent)
		}
	}
	if defaultIntent != prompts.IntentGeneral {
		t.Fatalf("defaultIntent = %q", defaultIntent)
	}
}

func TestDiscoverIntentsNoVocabulary(t *testing.T) {
	root := t.TempDir()
	bee := colony.Bee{Role: "scout"}
	intents, defaultIntent, err := prompts.DiscoverIntents(root, bee)
	if err != nil {
		t.Fatal(err)
	}
	if len(intents) != 0 {
		t.Fatalf("intents = %#v", intents)
	}
	if defaultIntent != "" {
		t.Fatalf("defaultIntent = %q", defaultIntent)
	}
}
