package colony_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
			"reasoning": "high",
			"yolo":      false,
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
	if p.Reasoning != "high" {
		t.Fatalf("reasoning = %q", p.Reasoning)
	}
	if p.Trust || p.Force {
		t.Fatalf("yolo false should clear trust/force: %+v", p)
	}
}

// TestLegacyTrustForceMapToRunParams pins that the trust/force aliases still
// map onto the internal pair the adapters take, and record their declared keys
// so layering stays tri-state while the window is open.
func TestLegacyTrustForceMapToRunParams(t *testing.T) {
	layer := colony.LayerFromBee(colony.Bee{Role: "builder", Params: map[string]any{"trust": false, "force": true}})
	if layer.Params.Trust {
		t.Fatal("trust = true, want the legacy false preserved")
	}
	if !layer.Params.Force {
		t.Fatal("force = false, want the legacy true preserved")
	}
	if !layer.Declares("trust") || !layer.Declares("force") {
		t.Fatalf("declared = %+v, want trust and force recorded", layer.Declared)
	}
}

// TestYoloWinsOverLegacyTrustForce pins the precedence the review called out:
// yolo is the semantic key, the legacy pair is only a fallback.
func TestYoloWinsOverLegacyTrustForce(t *testing.T) {
	layer := colony.LayerFromBee(colony.Bee{Role: "builder", Params: map[string]any{
		"yolo": false, "trust": true, "force": true,
	}})
	if layer.Params.Trust || layer.Params.Force {
		t.Fatalf("yolo false should beat legacy trust/force: %+v", layer.Params)
	}
}

// TestThinkingAliasesReasoning pins that thinking maps to reasoning only when
// reasoning is unset, and that reasoning wins when both appear.
func TestThinkingAliasesReasoning(t *testing.T) {
	alias := colony.LayerFromBee(colony.Bee{Role: "builder", Params: map[string]any{"thinking": "high"}})
	if alias.Params.Reasoning != "high" || !alias.Declares("reasoning") {
		t.Fatalf("thinking should map to reasoning: %+v declared=%v", alias.Params, alias.Declared)
	}

	both := colony.LayerFromBee(colony.Bee{Role: "builder", Params: map[string]any{"reasoning": "low", "thinking": "high"}})
	if both.Params.Reasoning != "low" {
		t.Fatalf("reasoning = %q, want the semantic key to win", both.Params.Reasoning)
	}
}

// TestProviderMapsToRunParams pins that the deprecated provider key still lands
// on RunParams.Provider for the launch mapping to fold into the model.
func TestProviderMapsToRunParams(t *testing.T) {
	layer := colony.LayerFromBee(colony.Bee{Role: "builder", Params: map[string]any{"provider": "openai"}})
	if layer.Params.Provider != "openai" {
		t.Fatalf("provider = %q, want openai", layer.Params.Provider)
	}
	if !layer.Declares("provider") {
		t.Fatalf("declared = %+v, want provider recorded", layer.Declared)
	}
}

// TestOutputFormatIsAcceptedAndIgnored pins that the runtime-owned
// output_format key loads without contributing a RunParams field or a declared
// key.
func TestOutputFormatIsAcceptedAndIgnored(t *testing.T) {
	layer := colony.LayerFromBee(colony.Bee{Role: "builder", Params: map[string]any{"output_format": "json"}})
	if layer.Declares("output_format") {
		t.Fatalf("output_format must not be declared: %+v", layer.Declared)
	}
	if layer.Params.Provider != "" || layer.Params.Reasoning != "" {
		t.Fatalf("output_format leaked into RunParams: %+v", layer.Params)
	}
}

func TestMergeRunParamsReasoning(t *testing.T) {
	base := adapters.RunParamsLayer{
		Params:   adapters.RunParams{Model: "base"},
		Declared: map[string]bool{"model": true},
	}
	over := adapters.RunParamsLayer{
		Params:   adapters.RunParams{Reasoning: "low"},
		Declared: map[string]bool{"reasoning": true},
	}
	got := colony.MergeRunParamsLayers(base, over)
	if got.Reasoning != "low" {
		t.Fatalf("reasoning = %q", got.Reasoning)
	}
	if got.Model != "base" {
		t.Fatalf("model = %q", got.Model)
	}
}

func TestYoloSetsTrustAndForce(t *testing.T) {
	yolo := colony.LayerFromBee(colony.Bee{Role: "builder", Params: map[string]any{"yolo": true}})
	if !yolo.Params.Trust || !yolo.Params.Force {
		t.Fatalf("yolo should enable trust and force: %+v", yolo.Params)
	}
	if !yolo.Declares("trust") || !yolo.Declares("force") {
		t.Fatalf("yolo should declare trust and force: %+v", yolo.Declared)
	}
}

func TestDiagnoseBeeParams(t *testing.T) {
	bees := map[string]colony.Bee{
		"builder": {Role: "builder", Adapter: "cursor", Params: map[string]any{"trust": true, "force": true}},
		"planner": {Role: "planner", Adapter: "cursor", Params: map[string]any{"reasoning": "high"}},
		"thinker": {Role: "thinker", Adapter: "cursor", Params: map[string]any{"thinking": "high"}},
		"typo":    {Role: "typo", Adapter: "cursor", Params: map[string]any{"modle": "gpt-4"}},
		"scout":   {Role: "scout", Adapter: "pi", Params: map[string]any{"yolo": true}},
		"clean":   {Role: "clean", Adapter: "pi", Params: map[string]any{"plan": false}},
	}
	warnings := colony.DiagnoseBeeParams(bees)
	joined := strings.Join(warnings, "\n")
	for _, want := range []string{
		`bee "builder": params.trust is deprecated; use params.yolo`,
		`bee "builder": params.force is deprecated; use params.yolo`,
		`bee "planner": params.reasoning has no mapping on adapter cursor`,
		`bee "thinker": params.thinking is deprecated; use params.reasoning`,
		`bee "typo": unknown params key "modle"`,
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

// TestDiagnoseBeeParamsFlagsDeprecatedProvider pins the doctor half of the open
// window: params.provider still loads, so doctor must name the migration.
func TestDiagnoseBeeParamsFlagsDeprecatedProvider(t *testing.T) {
	bees := map[string]colony.Bee{
		"builder": {Role: "builder", Adapter: "pi", Params: map[string]any{"model": "gemini/gpt-4", "provider": "gemini"}},
	}
	warnings := colony.DiagnoseBeeParams(bees)
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, `bee "builder": params.provider is deprecated; put the vendor in params.model`) {
		t.Fatalf("missing provider deprecation warning:\n%s", joined)
	}
}

// TestDiagnoseBeeParamsFlagsDeprecatedOutputFormat pins that the runtime-owned
// output_format key warns rather than failing the load.
func TestDiagnoseBeeParamsFlagsDeprecatedOutputFormat(t *testing.T) {
	bees := map[string]colony.Bee{
		"builder": {Role: "builder", Adapter: "cursor", Params: map[string]any{"output_format": "json"}},
	}
	warnings := colony.DiagnoseBeeParams(bees)
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, `bee "builder": params.output_format is deprecated; the runtime owns structured output`) {
		t.Fatalf("missing output_format deprecation warning:\n%s", joined)
	}
}

// TestValidateRunParamKeysAcceptsLegacyAliases pins the schema half of the open
// window: every legacy key still loads (the removal is deferred one release)
// and is advertised in the valid-key list, so a colony from the old surface is
// not rejected.
func TestValidateRunParamKeysAcceptsLegacyAliases(t *testing.T) {
	for _, key := range []string{"trust", "force", "thinking", "provider", "output_format"} {
		if err := colony.ValidateRunParamKeys(map[string]any{key: "x"}); err != nil {
			t.Fatalf("params.%s should be accepted while the window is open, got %v", key, err)
		}
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

func TestOpenCodeAdapterExtraRoutesPinnedNames(t *testing.T) {
	slug := setupOpenCodeAdapterHome(t, `binary: /opt/opencode
version: 2
v1_binary: /opt/opencode-v1
v2_binary: /opt/opencode-v2
`)
	cfg, err := colony.LoadOpenCodeAdapter(slug)
	if err != nil {
		t.Fatal(err)
	}
	ctx := colony.Context{OpenCode: cfg}

	proxy := colony.AdapterExtra(ctx, "opencode")
	if proxy.Binary != "/opt/opencode" || proxy.Options["version"] != "2" {
		t.Fatalf("proxy extra = %+v", proxy)
	}

	v1 := colony.AdapterExtra(ctx, "opencodev1")
	if v1.Binary != "/opt/opencode-v1" || v1.Options["version"] != "1" {
		t.Fatalf("v1 extra = %+v", v1)
	}

	v2 := colony.AdapterExtra(ctx, "opencodev2")
	if v2.Binary != "/opt/opencode-v2" || v2.Options["version"] != "2" {
		t.Fatalf("v2 extra = %+v", v2)
	}
}

func TestBeeOptionsOverlayMachineDefaults(t *testing.T) {
	bee := colony.Bee{
		Role:    "builder",
		Adapter: "opencode",
		Params: map[string]any{"options": map[string]any{
			"version":   "1",
			"v2_binary": "/bee/opencode-v2",
		}},
	}
	over := adapters.RunParamsLayer{
		Params:   adapters.RunParams{Options: map[string]string{"version": "2", "v1_binary": "/machine/v1"}},
		Declared: map[string]bool{"options": true},
	}
	got := colony.MergeRunParamsLayers(colony.LayerFromBee(bee), over)
	if got.Options["version"] != "1" {
		t.Fatalf("version = %q, want the bee value", got.Options["version"])
	}
	if got.Options["v1_binary"] != "/machine/v1" || got.Options["v2_binary"] != "/bee/opencode-v2" {
		t.Fatalf("merged options = %+v", got.Options)
	}
}

func TestValidateRunParamValuesAcceptsOptions(t *testing.T) {
	if err := colony.ValidateRunParamValues(map[string]any{
		"options": map[string]any{"version": 2},
	}); err != nil {
		t.Fatalf("options map should validate: %v", err)
	}
	if err := colony.ValidateRunParamValues(map[string]any{
		"options": "version=2",
	}); err == nil {
		t.Fatal("string options should fail validation")
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

// TestValidateRunParamKeysAcceptsOutputFormat pins that output_format is still
// accepted for one release: the runtime owns structured output, so the key
// loads, but doctor warns (see TestDiagnoseBeeParamsFlagsDeprecatedOutputFormat).
func TestValidateRunParamKeysAcceptsOutputFormat(t *testing.T) {
	err := colony.ValidateRunParamKeys(map[string]any{"output_format": "json"})
	if err != nil {
		t.Fatalf("output_format should be accepted while the window is open: %v", err)
	}
	if !strings.Contains(strings.Join(colony.RunParamKeys, ", "), "output_format") {
		t.Fatalf("output_format should be advertised in the valid-key list: %v", colony.RunParamKeys)
	}
}

// TestLoadBeeAcceptsLegacyParams pins the schema half of the open window: a bee
// that still carries any legacy key loads without error.
func TestLoadBeeAcceptsLegacyParams(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".paseka", "bees")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "role: builder\nadapter: cursor\nparams:\n  trust: true\n  force: true\n  thinking: high\n  provider: openai\n  output_format: json\n"
	if err := os.WriteFile(filepath.Join(dir, "builder.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := colony.LoadBee(root, "builder"); err != nil {
		t.Fatalf("legacy keys should load while the window is open: %v", err)
	}
}

// TestDiagnosisLoaderToleratesLegacyAlias pins the doctor half: the diagnosis
// loader sees a bee with a legacy key and DiagnoseBeeParams names the migration.
func TestDiagnosisLoaderToleratesLegacyAlias(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".paseka", "bees")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "role: builder\nadapter: cursor\nparams:\n  trust: true\n"
	if err := os.WriteFile(filepath.Join(dir, "builder.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	bees, err := colony.LoadAllBeesForDiagnosis(root)
	if err != nil {
		t.Fatalf("diagnosis load should tolerate a legacy key: %v", err)
	}
	warnings := strings.Join(colony.DiagnoseBeeParams(bees), "\n")
	if !strings.Contains(warnings, `params.trust is deprecated; use params.yolo`) {
		t.Fatalf("doctor did not flag the leftover:\n%s", warnings)
	}
}

// TestDiagnosisLoaderRejectsBadParamValue pins the other half of the diagnosis
// gate: doctor relaxes only the key check, so a colony the runtime refuses to
// load on a bad value must still fail the diagnosis loader (and therefore make
// doctor exit non-zero) rather than passing silently.
func TestDiagnosisLoaderRejectsBadParamValue(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".paseka", "bees")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "role: builder\nadapter: cursor\nparams:\n  env_mode: bogus\n"
	if err := os.WriteFile(filepath.Join(dir, "builder.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := colony.LoadAllBeesForDiagnosis(root); err == nil {
		t.Fatal("diagnosis load should reject a bad params.env_mode value")
	} else if !strings.Contains(err.Error(), "params.env_mode must be") {
		t.Fatalf("err = %v, want the env_mode value named", err)
	}

	// The key gate still relaxes: a retired key alone loads for diagnosis.
	if err := os.WriteFile(filepath.Join(dir, "builder.yaml"), []byte("role: builder\nadapter: cursor\nparams:\n  trust: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := colony.LoadAllBeesForDiagnosis(root); err != nil {
		t.Fatalf("diagnosis load should tolerate a retired key: %v", err)
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

// TestValidateRunParamsTimeout pins the opt-in params.timeout: a positive Go
// duration string is accepted, and a missing, non-positive, or unparseable
// value fails at load instead of silently leaving the run unbounded.
func TestValidateRunParamsTimeout(t *testing.T) {
	if err := colony.ValidateRunParams(map[string]any{"timeout": "90s"}); err != nil {
		t.Fatalf("90s should validate: %v", err)
	}
	if err := colony.ValidateRunParams(map[string]any{"timeout": "5m"}); err != nil {
		t.Fatalf("5m should validate: %v", err)
	}

	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"non-string", 30, "duration string"},
		{"unparseable", "soon", "params.timeout"},
		{"zero", "0s", "positive"},
		{"negative", "-1s", "positive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := colony.ValidateRunParams(map[string]any{"timeout": tc.value})
			if err == nil {
				t.Fatalf("timeout %v should be rejected", tc.value)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

// TestLayerFromBeeTimeout pins that params.timeout becomes a try-state declared
// layer key: an absent timeout stays unbounded, a declared one carries both the
// parsed duration and the declared marker used by the layered merge.
func TestLayerFromBeeTimeout(t *testing.T) {
	absent := colony.LayerFromBee(colony.Bee{Role: "builder", Params: map[string]any{"model": "m"}})
	if absent.Params.Timeout != 0 || absent.Declares("timeout") {
		t.Fatalf("absent timeout = %v declared=%v, want unbounded/undeclared", absent.Params.Timeout, absent.Declares("timeout"))
	}

	layer := colony.LayerFromBee(colony.Bee{Role: "builder", Params: map[string]any{"timeout": "45s"}})
	if layer.Params.Timeout != 45*time.Second {
		t.Fatalf("timeout = %v, want 45s", layer.Params.Timeout)
	}
	if !layer.Declares("timeout") {
		t.Fatal("timeout should be recorded as declared")
	}

	merged := colony.MergeRunParamsLayers(absent, layer)
	if merged.Timeout != 45*time.Second {
		t.Fatalf("merged timeout = %v, want 45s", merged.Timeout)
	}
}

// TestCommandBeeMayCarryTimeout pins that timeout is process control, not an
// adapter flag: a command-mode bee may bound its run without tripping the
// command/params exclusivity check.
func TestCommandBeeMayCarryTimeout(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".paseka", "bees")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "role: builder\nadapter: script\ncommand: sleep 30\nrun_summary: disabled\nparams:\n  timeout: 2s\n"
	if err := os.WriteFile(filepath.Join(dir, "builder.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	bee, _, err := colony.LoadBee(root, "builder")
	if err != nil {
		t.Fatalf("command + timeout should load: %v", err)
	}
	if !bee.Command.IsSet() {
		t.Fatal("expected command to be set")
	}
}
