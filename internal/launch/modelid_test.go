package launch_test

import (
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/launch"
)

func TestMapModelPerAdapter(t *testing.T) {
	tests := []struct {
		name         string
		adapter      string
		model        string
		provider     string
		wantModel    string
		wantProvider string
		wantWarning  bool
	}{
		{
			name:      "cursor passes vendor id through",
			adapter:   "cursor",
			model:     "anthropic/claude-sonnet-4",
			wantModel: "anthropic/claude-sonnet-4",
		},
		{
			name:      "cursor passes bare id through",
			adapter:   "cursor",
			model:     "composer-2.5",
			wantModel: "composer-2.5",
		},
		{
			name:      "claude passes vendor id through",
			adapter:   "claude",
			model:     "anthropic/claude-sonnet-4",
			wantModel: "anthropic/claude-sonnet-4",
		},
		{
			name:      "opencode consumes provider slash model natively",
			adapter:   "opencode",
			model:     "anthropic/claude-sonnet-4",
			wantModel: "anthropic/claude-sonnet-4",
		},
		{
			name:      "opencode bare id passes through",
			adapter:   "opencode",
			model:     "big-pickle",
			wantModel: "big-pickle",
		},
		{
			name:         "pi splits first slash and keeps inner slashes and suffix",
			adapter:      "pi",
			model:        "openrouter/deepseek/deepseek-r1:free",
			wantProvider: "openrouter",
			wantModel:    "deepseek/deepseek-r1:free",
		},
		{
			name:         "pi keeps colon suffix when there is no inner slash",
			adapter:      "pi",
			model:        "openrouter/deepseek-r1:free",
			wantProvider: "openrouter",
			wantModel:    "deepseek-r1:free",
		},
		{
			name:      "pi passes bare id through",
			adapter:   "pi",
			model:     "composer-2.5",
			wantModel: "composer-2.5",
		},
		{
			name:      "pi keeps a colon suffix on a bare id",
			adapter:   "pi",
			model:     "gpt-4o:extended",
			wantModel: "gpt-4o:extended",
		},
		{
			name:         "deprecated provider folds into pi provider and model",
			adapter:      "pi",
			model:        "gpt-4",
			provider:     "gemini",
			wantProvider: "gemini",
			wantModel:    "gpt-4",
			wantWarning:  true,
		},
		{
			name:        "deprecated provider folds into opencode model",
			adapter:     "opencode",
			model:       "gpt-4",
			provider:    "openai",
			wantModel:   "openai/gpt-4",
			wantWarning: true,
		},
		{
			name:        "deprecated provider is ignored when model already has a vendor",
			adapter:     "opencode",
			model:       "anthropic/claude-sonnet-4",
			provider:    "openai",
			wantModel:   "anthropic/claude-sonnet-4",
			wantWarning: true,
		},
		{
			name:        "deprecated provider folds into cursor model",
			adapter:     "cursor",
			model:       "gpt-4",
			provider:    "openai",
			wantModel:   "openai/gpt-4",
			wantWarning: true,
		},
		{
			name:      "empty model stays empty",
			adapter:   "pi",
			model:     "",
			wantModel: "",
		},
		{
			name:         "pi keeps a provider-only model for one release",
			adapter:      "pi",
			model:        "",
			provider:     "gemini",
			wantProvider: "gemini",
			wantWarning:  true,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := launch.MapModel(tc.adapter, tc.model, tc.provider)
			if got.Model != tc.wantModel {
				t.Fatalf("model = %q, want %q", got.Model, tc.wantModel)
			}
			if got.Provider != tc.wantProvider {
				t.Fatalf("provider = %q, want %q", got.Provider, tc.wantProvider)
			}
			if tc.wantWarning && got.Warning == "" {
				t.Fatalf("expected a deprecation warning")
			}
			if !tc.wantWarning && got.Warning != "" {
				t.Fatalf("unexpected warning %q", got.Warning)
			}
			if tc.provider != "" && !strings.Contains(got.Warning, "params.provider") {
				t.Fatalf("warning %q should name params.provider", got.Warning)
			}
		})
	}
}

func TestResolveLaunchMapsModelAndWarnsOnDeprecatedProvider(t *testing.T) {
	req := baseRequest(t, "pi", launch.AFK)
	req.Bee.Params = map[string]any{"model": "gpt-4", "provider": "gemini"}

	plan, err := launch.ResolveLaunch(req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Params.Model != "gpt-4" || plan.Params.Provider != "gemini" {
		t.Fatalf("params = %+v, want provider gemini and model gpt-4", plan.Params)
	}
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "params.provider") {
		t.Fatalf("warnings = %v", plan.Warnings)
	}
}

func TestResolveLaunchSplitsPiAliasVendor(t *testing.T) {
	req := baseRequest(t, "pi", launch.AFK)
	req.Bee.Params = map[string]any{"model": "high"}
	req.Aliases = map[string]string{"high": "openrouter/deepseek/deepseek-r1:free"}

	plan, err := launch.ResolveLaunch(req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Params.Provider != "openrouter" {
		t.Fatalf("provider = %q, want openrouter", plan.Params.Provider)
	}
	if plan.Params.Model != "deepseek/deepseek-r1:free" {
		t.Fatalf("model = %q, want deepseek/deepseek-r1:free", plan.Params.Model)
	}
	if len(plan.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", plan.Warnings)
	}
}
