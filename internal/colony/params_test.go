package colony_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
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

func TestResolveAdapterOpenCode(t *testing.T) {
	bee := colony.Bee{Role: "builder", Adapter: "opencode"}
	name, err := bee.ResolveAdapter()
	if err != nil {
		t.Fatal(err)
	}
	if name != "opencode" {
		t.Fatalf("got %q, want opencode", name)
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
	base := adapters.RunParamsLayer{
		Params:   adapters.RunParams{Model: "base"},
		Declared: map[string]bool{"model": true},
	}
	over := adapters.RunParamsLayer{
		Params:   adapters.RunParams{Provider: "openai", Thinking: "low"},
		Declared: map[string]bool{"provider": true, "thinking": true},
	}
	got := colony.MergeRunParamsLayers(base, over)
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

func TestRunParamsLayerDeclaresExplicitFalse(t *testing.T) {
	bee := colony.Bee{
		Role: "builder",
		Params: map[string]any{
			"yolo": false,
			"plan": false,
		},
	}
	layer := colony.LayerFromBee(bee)
	if !layer.Declares("trust") || !layer.Declares("force") || !layer.Declares("plan") {
		t.Fatalf("declared = %+v, want trust, force and plan", layer.Declared)
	}
	if layer.Declares("model") {
		t.Fatalf("model should not be declared: %+v", layer.Declared)
	}
	if layer.Params.Trust || layer.Params.Force {
		t.Fatalf("yolo false should clear trust/force: %+v", layer.Params)
	}
	if layer.Params.Plan {
		t.Fatal("plan false should be preserved, got true")
	}
}

func TestMergeRunParamsLayersTriStateFalseBeatsTrue(t *testing.T) {
	base := adapters.RunParamsLayer{
		Params:   adapters.RunParams{Trust: true, Force: true},
		Declared: map[string]bool{"trust": true, "force": true},
	}
	over := adapters.RunParamsLayer{
		Params:   adapters.RunParams{Trust: false},
		Declared: map[string]bool{"trust": true},
	}
	got := colony.MergeRunParamsLayers(base, over)
	if got.Trust {
		t.Fatal("trust = true, want false (explicit false must beat true)")
	}
	if !got.Force {
		t.Fatal("force = false, want true (undeclared force keeps base)")
	}
}

func TestMergeRunParamsLayersBinaryCommittedWins(t *testing.T) {
	base := adapters.RunParamsLayer{
		Params:   adapters.RunParams{Binary: "/repo/agent"},
		Declared: map[string]bool{"binary": true},
	}
	over := adapters.RunParamsLayer{
		Params:   adapters.RunParams{Binary: "/machine/agent"},
		Declared: map[string]bool{"binary": true},
	}
	if got := colony.MergeRunParamsLayers(base, over); got.Binary != "/repo/agent" {
		t.Fatalf("binary = %q, want committed /repo/agent", got.Binary)
	}
	// A committed binary wins even when the machine layer declares none.
	emptyOver := adapters.RunParamsLayer{Declared: map[string]bool{}}
	if got := colony.MergeRunParamsLayers(base, emptyOver); got.Binary != "/repo/agent" {
		t.Fatalf("binary = %q, want committed /repo/agent", got.Binary)
	}
}

func TestMergeRunParamsLayersBinaryFallbackOnlyWhenBaseUnset(t *testing.T) {
	base := adapters.RunParamsLayer{Declared: map[string]bool{}}
	over := adapters.RunParamsLayer{
		Params:   adapters.RunParams{Binary: "/machine/agent"},
		Declared: map[string]bool{"binary": true},
	}
	if got := colony.MergeRunParamsLayers(base, over); got.Binary != "/machine/agent" {
		t.Fatalf("binary = %q, want machine fallback /machine/agent", got.Binary)
	}
	// No machine declaration and no committed binary leaves the adapter default
	// in charge.
	over = adapters.RunParamsLayer{Declared: map[string]bool{}}
	if got := colony.MergeRunParamsLayers(base, over); got.Binary != "" {
		t.Fatalf("binary = %q, want empty so the adapter default applies", got.Binary)
	}
}

func TestAdapterExtraLayerIsPresenceAware(t *testing.T) {
	declared := colony.Context{
		Cursor:   colony.CursorAdapterConfig{Binary: "agent"},
		Pi:       colony.PiAdapterConfig{Binary: "pi"},
		OpenCode: colony.OpenCodeAdapterConfig{Binary: "/opt/opencode"},
	}
	// Context literals cannot reach the unexported declared flag, so a binary
	// set here reads as undeclared: the layer must not claim it.
	if layer := colony.AdapterExtraLayer(declared, "cursor"); layer.Declares("binary") {
		t.Fatalf("cursor binary should be undeclared for a literal context: %+v", layer.Declared)
	}
}

func TestAdapterExtraRoutesByAdapter(t *testing.T) {
	ctx := colony.Context{
		Cursor:   colony.CursorAdapterConfig{Binary: "agent"},
		Pi:       colony.PiAdapterConfig{Binary: "pi"},
		OpenCode: colony.OpenCodeAdapterConfig{Binary: "/opt/opencode"},
	}

	cursorExtra := colony.AdapterExtra(ctx, "cursor")
	if cursorExtra.Binary != "agent" {
		t.Fatalf("cursor binary = %q", cursorExtra.Binary)
	}

	piExtra := colony.AdapterExtra(ctx, "pi")
	if piExtra.Binary != "pi" {
		t.Fatalf("pi binary = %q", piExtra.Binary)
	}

	ocExtra := colony.AdapterExtra(ctx, "opencode")
	if ocExtra.Binary != "/opt/opencode" {
		t.Fatalf("opencode binary = %q", ocExtra.Binary)
	}
}

func TestValidateRunParamKeysRejectsUnknown(t *testing.T) {
	err := colony.ValidateRunParamKeys(map[string]any{"modle": "gpt-4"})
	if err == nil {
		t.Fatal("expected unknown param key error")
	}
	msg := err.Error()
	if !strings.Contains(msg, `unknown params key "modle"`) {
		t.Fatalf("err = %v, want the offending key named", err)
	}
	if !strings.Contains(msg, "valid keys:") || !strings.Contains(msg, "model") {
		t.Fatalf("err = %v, want the valid-key list", err)
	}

	if err := colony.ValidateRunParamKeys(map[string]any{"model": "x", "plan": true}); err != nil {
		t.Fatalf("known keys should validate, got %v", err)
	}
}

func TestLoadBeeRejectsUnknownParamsKey(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".paseka", "bees")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "role: scout\nadapter: cursor\nparams:\n  plan_mode: true\n"
	if err := os.WriteFile(filepath.Join(dir, "scout.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := colony.LoadBee(root, "scout")
	if err == nil {
		t.Fatal("expected load to fail on unknown params key")
	}
	if !strings.Contains(err.Error(), `unknown params key "plan_mode"`) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "bee \"scout\"") {
		t.Fatalf("err = %v, want the bee named", err)
	}
}

func TestLoadColonyProfileRejectsUnknownParamsKey(t *testing.T) {
	repo, _ := setupProfileColony(t)
	writeColonyProfile(t, repo, "typo", "adapter: pi\nparams:\n  modle: gpt\n")
	resetProcessProfile(t)
	colony.SetProcessProfile(colony.ProfileSelection{Name: "typo", FlagSet: true})
	_, err := colony.ResolveContext(repo)
	if err == nil || !strings.Contains(err.Error(), `unknown params key "modle"`) {
		t.Fatalf("err = %v, want the profile params typo rejected", err)
	}
}

// TestResolvedBinaryPrecedence pins the ladder: a committed params.binary wins
// over a declared machine file, a machine file is a fallback when the repo is
// silent, and an absent machine file leaves the adapter default in charge.
func TestResolvedBinaryPrecedence(t *testing.T) {
	repo, slug := setupProfileColony(t)
	homeDir, err := colony.HomeDir(slug)
	if err != nil {
		t.Fatal(err)
	}
	machineAdapter := filepath.Join(homeDir, "adapters", "cursor.yaml")

	resolve := func(t *testing.T, beeBody string) (colony.Context, colony.Bee) {
		t.Helper()
		mustWriteProfile(t, filepath.Join(repo, ".paseka", "bees", "scout.yaml"), beeBody)
		ctx, err := colony.ResolveContext(repo)
		if err != nil {
			t.Fatal(err)
		}
		bee, _, err := ctx.LoadBee("scout")
		if err != nil {
			t.Fatal(err)
		}
		return ctx, bee
	}
	merge := func(ctx colony.Context, bee colony.Bee) adapters.RunParams {
		return colony.MergeRunParamsLayers(colony.LayerFromBee(bee), colony.AdapterExtraLayer(ctx, "cursor"))
	}

	// A declared machine binary is only a fallback: the committed binary wins.
	if err := os.WriteFile(machineAdapter, []byte("binary: /machine/agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, bee := resolve(t, "role: scout\nadapter: cursor\nparams:\n  binary: /repo/agent\n")
	if got := merge(ctx, bee); got.Binary != "/repo/agent" {
		t.Fatalf("binary = %q, want the committed /repo/agent", got.Binary)
	}

	// With no committed binary the declared machine binary is the fallback.
	ctx, bee = resolve(t, "role: scout\nadapter: cursor\n")
	if got := merge(ctx, bee); got.Binary != "/machine/agent" {
		t.Fatalf("binary = %q, want the machine fallback /machine/agent", got.Binary)
	}

	// An absent machine file declares nothing, so the adapter default stands.
	if err := os.Remove(machineAdapter); err != nil {
		t.Fatal(err)
	}
	ctx, bee = resolve(t, "role: scout\nadapter: cursor\n")
	if got := merge(ctx, bee); got.Binary != "" {
		t.Fatalf("binary = %q, want empty so the adapter default applies", got.Binary)
	}
	if got := colony.AdapterExtra(ctx, "cursor").Binary; got != "agent" {
		t.Fatalf("effective binary = %q, want the agent default", got)
	}
}
