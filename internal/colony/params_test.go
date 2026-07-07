package colony_test

import (
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
)

func TestResolveAdapterPi(t *testing.T) {
	bee := colony.Bee{Role: "builder", Adapter: "pi"}
	name, err := bee.ResolveAdapter()
	if err != nil {
		t.Fatal(err)
	}
	if name != "pi" {
		t.Fatalf("got %q, want pi", name)
	}
}

func TestResolveAdapterCursorDefault(t *testing.T) {
	bee := colony.Bee{Role: "builder"}
	name, err := bee.ResolveAdapter()
	if err != nil {
		t.Fatal(err)
	}
	if name != "cursor" {
		t.Fatalf("got %q, want cursor", name)
	}

	bee.Adapter = "cursor"
	name, err = bee.ResolveAdapter()
	if err != nil {
		t.Fatal(err)
	}
	if name != "cursor" {
		t.Fatalf("got %q, want cursor", name)
	}
}

func TestResolveAdapterUnknown(t *testing.T) {
	bee := colony.Bee{Role: "builder", Adapter: "unknown"}
	_, err := bee.ResolveAdapter()
	if err == nil {
		t.Fatal("expected error for unknown adapter")
	}
}

func TestRunParamsFromBeeParsesSharedParams(t *testing.T) {
	bee := colony.Bee{
		Role: "builder",
		Params: map[string]any{
			"model":         "gpt-4",
			"output_format": "json",
			"plan":          true,
			"binary":        "/usr/bin/pi",
			"provider":      "gemini",
			"thinking":      "high",
			"trust":         false,
			"force":         false,
		},
	}
	p := colony.RunParamsFromBee(bee)
	if p.Model != "gpt-4" {
		t.Fatalf("model = %q", p.Model)
	}
	if p.OutputFormat != "json" {
		t.Fatalf("output_format = %q", p.OutputFormat)
	}
	if !p.Plan {
		t.Fatal("plan = false, want true")
	}
	if p.Binary != "/usr/bin/pi" {
		t.Fatalf("binary = %q", p.Binary)
	}
	if p.Provider != "gemini" {
		t.Fatalf("provider = %q", p.Provider)
	}
	if p.Thinking != "high" {
		t.Fatalf("thinking = %q", p.Thinking)
	}
	if p.Trust {
		t.Fatal("trust = true, want false")
	}
	if p.Force {
		t.Fatal("force = true, want false")
	}
}

func TestMergeRunParamsProviderThinking(t *testing.T) {
	base := adapters.RunParams{Model: "base"}
	over := adapters.RunParams{Provider: "openai", Thinking: "low"}
	got := colony.MergeRunParams(base, over)
	if got.Provider != "openai" {
		t.Fatalf("provider = %q", got.Provider)
	}
	if got.Thinking != "low" {
		t.Fatalf("thinking = %q", got.Thinking)
	}
	if got.Model != "base" {
		t.Fatalf("model = %q", got.Model)
	}
}

func TestAdapterExtraRoutesByAdapter(t *testing.T) {
	ctx := colony.Context{
		Cursor: colony.CursorAdapterConfig{Binary: "agent"},
		Pi:     colony.PiAdapterConfig{Binary: "pi"},
	}

	cursorExtra := colony.AdapterExtra(ctx, "cursor")
	if cursorExtra.Binary != "agent" {
		t.Fatalf("cursor binary = %q", cursorExtra.Binary)
	}

	piExtra := colony.AdapterExtra(ctx, "pi")
	if piExtra.Binary != "pi" {
		t.Fatalf("pi binary = %q", piExtra.Binary)
	}
}
