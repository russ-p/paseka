package launch_test

import (
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/launch"
)

// resolveForAttribution resolves a cursor bee with the given layers so the
// layering matrix can assert one field at a time.
func resolveForAttribution(t *testing.T, bee colony.Bee, extra adapters.RunParamsLayer, aliases map[string]string) launch.LaunchPlan {
	t.Helper()
	plan, err := launch.ResolveLaunch(launch.Request{
		Mode:      launch.AFK,
		Adapter:   "cursor",
		Bee:       bee,
		Extra:     extra,
		Aliases:   aliases,
		Workspace: "/ws",
		Prompt:    "do work",
		RunDir:    testRunDir(),
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return plan
}

func attributionField(t *testing.T, plan launch.LaunchPlan, field string) launch.AttributionField {
	t.Helper()
	for _, f := range plan.Attribution() {
		if f.Field == field {
			return f
		}
	}
	t.Fatalf("field %q missing from attribution: %+v", field, plan.Attribution())
	return launch.AttributionField{}
}

func beeWith(params map[string]any) colony.Bee {
	return colony.Bee{Role: "builder", Adapter: "cursor", Params: params}
}

func extraWith(declared map[string]bool, params adapters.RunParams) adapters.RunParamsLayer {
	return adapters.RunParamsLayer{Params: params, Declared: declared}
}

// TestAttributionNamesTheWinningLayerPerField is the layering matrix: every
// field must name the layer that actually decided its value, including the
// binary asymmetry (committed wins, machine is a fallback), the alias layer
// above the model's declaring layer, and a field no layer declared falling to
// the adapter default.
func TestAttributionNamesTheWinningLayerPerField(t *testing.T) {
	cases := []struct {
		name      string
		bee       colony.Bee
		extra     adapters.RunParamsLayer
		aliases   map[string]string
		field     string
		wantValue string
		wantLayer launch.Layer
	}{
		{
			name:      "committed binary beats the machine file",
			bee:       beeWith(map[string]any{"binary": "/repo/agent"}),
			extra:     extraWith(map[string]bool{"binary": true}, adapters.RunParams{Binary: "/machine/agent"}),
			field:     launch.FieldBinary,
			wantValue: "/repo/agent",
			wantLayer: launch.LayerBee,
		},
		{
			name:      "machine binary is a fallback when the bee is silent",
			bee:       beeWith(nil),
			extra:     extraWith(map[string]bool{"binary": true}, adapters.RunParams{Binary: "/machine/agent"}),
			field:     launch.FieldBinary,
			wantValue: "/machine/agent",
			wantLayer: launch.LayerMachine,
		},
		{
			name:      "absent binary leaves the adapter default",
			bee:       beeWith(nil),
			extra:     extraWith(map[string]bool{}, adapters.RunParams{}),
			field:     launch.FieldBinary,
			wantValue: "agent",
			wantLayer: launch.LayerDefault,
		},
		{
			name:      "alias decides the model above its declaring layer",
			bee:       beeWith(map[string]any{"model": "big"}),
			extra:     extraWith(map[string]bool{}, adapters.RunParams{}),
			aliases:   map[string]string{"big": "openai/gpt-5.5"},
			field:     launch.FieldModel,
			wantValue: "openai/gpt-5.5",
			wantLayer: launch.LayerAlias,
		},
		{
			name:      "bee model without an alias stays the bee's",
			bee:       beeWith(map[string]any{"model": "m1"}),
			extra:     extraWith(map[string]bool{}, adapters.RunParams{}),
			field:     launch.FieldModel,
			wantValue: "m1",
			wantLayer: launch.LayerBee,
		},
		{
			name:      "machine model overlays the bee",
			bee:       beeWith(map[string]any{"model": "m1"}),
			extra:     extraWith(map[string]bool{"model": true}, adapters.RunParams{Model: "m2"}),
			field:     launch.FieldModel,
			wantValue: "m2",
			wantLayer: launch.LayerMachine,
		},
		{
			name:      "undeclared yolo is the adapter default, not a declaration",
			bee:       beeWith(nil),
			extra:     extraWith(map[string]bool{}, adapters.RunParams{}),
			field:     launch.FieldYolo,
			wantValue: "true",
			wantLayer: launch.LayerDefault,
		},
		{
			name:      "bee yolo false is declared by the bee",
			bee:       beeWith(map[string]any{"yolo": false}),
			extra:     extraWith(map[string]bool{}, adapters.RunParams{}),
			field:     launch.FieldYolo,
			wantValue: "false",
			wantLayer: launch.LayerBee,
		},
		{
			name:      "bee reasoning is attributed to the bee",
			bee:       beeWith(map[string]any{"reasoning": "high"}),
			extra:     extraWith(map[string]bool{}, adapters.RunParams{}),
			field:     launch.FieldReasoning,
			wantValue: "high",
			wantLayer: launch.LayerBee,
		},
		{
			name:      "machine timeout is attributed to the machine",
			bee:       beeWith(nil),
			extra:     extraWith(map[string]bool{"timeout": true}, adapters.RunParams{Timeout: 30 * time.Second}),
			field:     launch.FieldTimeout,
			wantValue: "30s",
			wantLayer: launch.LayerMachine,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			plan := resolveForAttribution(t, tc.bee, tc.extra, tc.aliases)
			got := attributionField(t, plan, tc.field)
			if got.Layer != tc.wantLayer {
				t.Fatalf("%s layer = %q, want %q", tc.field, got.Layer, tc.wantLayer)
			}
			if got.Value != tc.wantValue {
				t.Fatalf("%s value = %q, want %q", tc.field, got.Value, tc.wantValue)
			}
		})
	}
}

// TestAttributionReportsEveryFieldWithALayer keeps the surface total: a field a
// UI renders must never come back with an empty layer, which would read as "no
// source".
func TestAttributionReportsEveryFieldWithALayer(t *testing.T) {
	plan := resolveForAttribution(t, beeWith(nil), extraWith(map[string]bool{}, adapters.RunParams{}), nil)
	fields := plan.Attribution()
	if len(fields) == 0 {
		t.Fatal("attribution returned no fields")
	}
	for _, f := range fields {
		if f.Layer == "" {
			t.Fatalf("field %q has no layer", f.Field)
		}
	}
}
