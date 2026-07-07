package prompts_test

import (
	"testing"

	"github.com/paseka/paseka/internal/prompts"
)

func TestNormalizeIntent(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", prompts.IntentGeneral},
		{"general", prompts.IntentGeneral},
		{"FEATURE", prompts.IntentFeature},
		{"bugfix", prompts.IntentBugfix},
		{"test-fix", prompts.IntentTestFix},
		{"refactor", prompts.IntentRefactor},
		{"custom-unknown", prompts.IntentGeneral},
	}
	for _, tc := range cases {
		if got := prompts.NormalizeIntent(tc.in); got != tc.want {
			t.Fatalf("NormalizeIntent(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPromptContextPreservesRawIntent(t *testing.T) {
	ctx := prompts.PromptContext(prompts.Context{IntentRaw: "CUSTOM"})
	if ctx.Intent != prompts.IntentGeneral {
		t.Fatalf("Intent = %q, want general", ctx.Intent)
	}
	if ctx.IntentRaw != "CUSTOM" {
		t.Fatalf("IntentRaw = %q", ctx.IntentRaw)
	}
}
