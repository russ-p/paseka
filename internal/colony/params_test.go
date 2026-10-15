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
			"model":     "gpt-4",
			"plan":      true,
			"binary":    "/usr/bin/pi",
			"provider":  "gemini",
			"reasoning": "high",
			"trust":     false,
			"force":     false,
		},
	}
	p := colony.RunParamsFromBee(bee)
	if p.Model != "gpt-4" {
		t.Fatalf("model = %q", p.Model)
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
	if p.Reasoning != "high" {
		t.Fatalf("reasoning = %q", p.Reasoning)
	}
	if p.Trust {
		t.Fatal("trust = true, want false")
	}
	if p.Force {
		t.Fatal("force = true, want false")
	}
}

func TestMergeRunParamsProviderReasoning(t *testing.T) {
	base := adapters.RunParamsLayer{
		Params:   adapters.RunParams{Model: "base"},
		Declared: map[string]bool{"model": true},
	}
	over := adapters.RunParamsLayer{
		Params:   adapters.RunParams{Provider: "openai", Reasoning: "low"},
		Declared: map[string]bool{"provider": true, "reasoning": true},
	}
	got := colony.MergeRunParamsLayers(base, over)
	if got.Provider != "openai" {
		t.Fatalf("provider = %q", got.Provider)
	}
	if got.Reasoning != "low" {
		t.Fatalf("reasoning = %q", got.Reasoning)
	}
	if got.Model != "base" {
		t.Fatalf("model = %q", got.Model)
	}
}

func TestRunParamsReasoningLegacyAlias(t *testing.T) {
	// thinking is a one-release deprecation alias for reasoning.
	legacy := colony.LayerFromBee(colony.Bee{
		Role:   "builder",
		Params: map[string]any{"thinking": "low"},
	})
	if legacy.Params.Reasoning != "low" {
		t.Fatalf("thinking alias = %q, want low", legacy.Params.Reasoning)
	}
	if !legacy.Declares("reasoning") {
		t.Fatalf("thinking should declare reasoning: %+v", legacy.Declared)
	}

	// reasoning wins when both keys are present.
	both := colony.LayerFromBee(colony.Bee{
		Role:   "builder",
		Params: map[string]any{"reasoning": "high", "thinking": "low"},
	})
	if both.Params.Reasoning != "high" {
		t.Fatalf("reasoning = %q, want high (new key wins)", both.Params.Reasoning)
	}
}

func TestYoloMatchesLegacyTrustForce(t *testing.T) {
	yolo := colony.LayerFromBee(colony.Bee{Role: "builder", Params: map[string]any{"yolo": true}})
	legacy := colony.LayerFromBee(colony.Bee{Role: "builder", Params: map[string]any{"trust": true, "force": true}})
	if yolo.Params.Trust != legacy.Params.Trust || yolo.Params.Force != legacy.Params.Force {
		t.Fatalf("yolo %+v != trust+force %+v", yolo.Params, legacy.Params)
	}
	if !yolo.Params.Trust || !yolo.Params.Force {
		t.Fatalf("yolo should enable trust and force: %+v", yolo.Params)
	}
}

func TestDiagnoseBeeParams(t *testing.T) {
	bees := map[string]colony.Bee{
		"builder": {Role: "builder", Adapter: "cursor", Params: map[string]any{"trust": true, "force": true}},
		"planner": {Role: "planner", Adapter: "cursor", Params: map[string]any{"reasoning": "high"}},
		"scout":   {Role: "scout", Adapter: "pi", Params: map[string]any{"yolo": true}},
		"clean":   {Role: "clean", Adapter: "pi", Params: map[string]any{"plan": false}},
	}
	warnings := colony.DiagnoseBeeParams(bees)
	joined := strings.Join(warnings, "\n")
	for _, want := range []string{
		`bee "builder": params.trust is deprecated; use params.yolo`,
		`bee "builder": params.force is deprecated; use params.yolo`,
		`bee "planner": params.reasoning has no mapping on adapter cursor`,
		`bee "scout": adapter pi has no yolo equivalent`,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing warning %q in:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, `bee "clean"`) {
		t.Fatalf("clean bee should produce no warnings:\n%s", joined)
	}
}

func TestDiagnoseBeeParamsWarnsProviderDeprecation(t *testing.T) {
	bees := map[string]colony.Bee{
		"builder": {Role: "builder", Adapter: "pi", Params: map[string]any{"model": "gpt-4", "provider": "gemini"}},
	}
	warnings := colony.DiagnoseBeeParams(bees)
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, `bee "builder": params.provider is deprecated`) {
		t.Fatalf("missing provider deprecation warning:\n%s", joined)
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

// TestValidateRunParamKeysRejectsRemovedOutputFormat pins that output_format is
// no longer a bee-visible key: the runtime owns structured output, so a leftover
// key fails at load instead of silently degrading parsing.
func TestValidateRunParamKeysRejectsRemovedOutputFormat(t *testing.T) {
	err := colony.ValidateRunParamKeys(map[string]any{"output_format": "json"})
	if err == nil {
		t.Fatal("expected output_format to be rejected as a removed key")
	}
	msg := err.Error()
	if !strings.Contains(msg, `unknown params key "output_format"`) {
		t.Fatalf("err = %v, want output_format named as unknown", err)
	}
	_, valid, ok := strings.Cut(msg, "valid keys:")
	if !ok {
		t.Fatalf("err = %v, want the valid-key list", err)
	}
	if strings.Contains(valid, "output_format") {
		t.Fatalf("output_format must not be advertised as valid: %v", err)
	}
}

func TestLoadBeeRejectsRemovedOutputFormat(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".paseka", "bees")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "role: scout\nadapter: cursor\nparams:\n  output_format: json\n"
	if err := os.WriteFile(filepath.Join(dir, "scout.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := colony.LoadBee(root, "scout")
	if err == nil {
		t.Fatal("expected load to fail on the removed output_format key")
	}
	if !strings.Contains(err.Error(), `unknown params key "output_format"`) {
		t.Fatalf("err = %v", err)
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

func TestLayerFromBeeParsesEnvAndMode(t *testing.T) {
	bee := colony.Bee{
		Role: "builder",
		Params: map[string]any{
			"env":      map[string]any{"GITHUB_TOKEN": "$HOST_GITHUB_TOKEN", "PLAIN": "value"},
			"env_mode": "strict",
		},
	}
	layer := colony.LayerFromBee(bee)
	if !layer.Declares("env") || !layer.Declares("env_mode") {
		t.Fatalf("declared = %+v, want env and env_mode", layer.Declared)
	}
	if layer.Params.Env["GITHUB_TOKEN"] != "$HOST_GITHUB_TOKEN" {
		t.Fatalf("env not parsed: %+v", layer.Params.Env)
	}
	if layer.Params.EnvMode != "strict" {
		t.Fatalf("env_mode = %q, want strict", layer.Params.EnvMode)
	}
}

func TestMergeRunParamsLayersEnvOverlays(t *testing.T) {
	base := adapters.RunParamsLayer{
		Params:   adapters.RunParams{Env: map[string]string{"A": "1", "B": "keep"}},
		Declared: map[string]bool{"env": true},
	}
	over := adapters.RunParamsLayer{
		Params:   adapters.RunParams{Env: map[string]string{"A": "2", "C": "3"}, EnvMode: "strict"},
		Declared: map[string]bool{"env": true, "env_mode": true},
	}
	got := colony.MergeRunParamsLayers(base, over)
	want := map[string]string{"A": "2", "B": "keep", "C": "3"}
	if strings.Join([]string{got.Env["A"], got.Env["B"], got.Env["C"]}, ",") != "2,keep,3" {
		t.Fatalf("env = %v, want %v", got.Env, want)
	}
	if got.EnvMode != "strict" {
		t.Fatalf("env_mode = %q, want strict", got.EnvMode)
	}
	if base.Params.Env["A"] != "1" {
		t.Fatalf("merge mutated the base layer: %v", base.Params.Env)
	}
}

func TestMergeRunParamsLayersEnvPreservedWhenUndeclared(t *testing.T) {
	base := adapters.RunParamsLayer{
		Params:   adapters.RunParams{Env: map[string]string{"A": "1"}},
		Declared: map[string]bool{"env": true},
	}
	over := adapters.RunParamsLayer{Declared: map[string]bool{"model": true}}
	got := colony.MergeRunParamsLayers(base, over)
	if got.Env["A"] != "1" {
		t.Fatalf("undeclared env should keep base: %v", got.Env)
	}
}

func TestValidateRunParamsRejectsBadEnvMode(t *testing.T) {
	err := colony.ValidateRunParams(map[string]any{"env_mode": "deny"})
	if err == nil || !strings.Contains(err.Error(), "params.env_mode must be") {
		t.Fatalf("err = %v, want env_mode rejection", err)
	}
	if err := colony.ValidateRunParams(map[string]any{"env_mode": "strict"}); err != nil {
		t.Fatalf("strict env_mode should validate, got %v", err)
	}
	if err := colony.ValidateRunParams(map[string]any{"env": map[string]any{"A": "1"}}); err != nil {
		t.Fatalf("env map should validate, got %v", err)
	}
	if err := colony.ValidateRunParams(map[string]any{"env": "not-a-map"}); err == nil {
		t.Fatal("env as a scalar should be rejected")
	}
}

func TestDiagnoseAdapterCredentialsWarnsUnknownTarget(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	slug := "demo"
	adapterDir := filepath.Join(xdg, "paseka", slug, "adapters")
	if err := os.MkdirAll(adapterDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "binary: agent\napi_key_env: PASEKA_TEST_MISSING_KEY\n"
	if err := os.WriteFile(filepath.Join(adapterDir, "cursor.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("PASEKA_TEST_MISSING_KEY")

	ctx := colony.Context{
		Slug:   slug,
		Cursor: colony.CursorAdapterConfig{APIKeyEnv: "PASEKA_TEST_MISSING_KEY"},
	}
	warnings := colony.DiagnoseAdapterCredentials(ctx)
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, `api_key_env "PASEKA_TEST_MISSING_KEY" is not set`) {
		t.Fatalf("missing unset api_key_env warning:\n%s", joined)
	}

	t.Setenv("PASEKA_TEST_MISSING_KEY", "present")
	if warnings := colony.DiagnoseAdapterCredentials(ctx); len(warnings) != 0 {
		t.Fatalf("set api_key_env should not warn: %v", warnings)
	}
}
