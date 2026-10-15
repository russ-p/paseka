package colony_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
)

func TestMergedModelAliasesHomeWins(t *testing.T) {
	colonyMap := map[string]string{"high": "vendor-a", "low": "vendor-low"}
	homeMap := map[string]string{"high": "vendor-b"}
	got := colony.MergedModelAliases(colonyMap, homeMap)
	if got["high"] != "vendor-b" {
		t.Fatalf("high = %q, want vendor-b", got["high"])
	}
	if got["low"] != "vendor-low" {
		t.Fatalf("low = %q, want vendor-low", got["low"])
	}
}

func TestResolveModelAliasAndPassThrough(t *testing.T) {
	aliases := map[string]string{"high": "cursor-grok-4.6-high-fast"}
	resolved, ok := colony.ResolveModel("high", aliases)
	if !ok || resolved != "cursor-grok-4.6-high-fast" {
		t.Fatalf("alias resolve = %q ok=%v", resolved, ok)
	}
	resolved, ok = colony.ResolveModel("composer-2.5", aliases)
	if ok || resolved != "composer-2.5" {
		t.Fatalf("pass-through = %q ok=%v", resolved, ok)
	}
}

func TestResolveModelTrimsName(t *testing.T) {
	aliases := map[string]string{"high": "vendor-id"}
	resolved, ok := colony.ResolveModel("  high  ", aliases)
	if !ok || resolved != "vendor-id" {
		t.Fatalf("trimmed resolve = %q ok=%v", resolved, ok)
	}
}

func TestValidateModelAliasesRejectsEmptyKey(t *testing.T) {
	err := colony.ValidateModelAliases(map[string]string{"": "x"})
	if err == nil || !strings.Contains(err.Error(), "empty alias key") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateModelAliasesRejectsEmptyValue(t *testing.T) {
	err := colony.ValidateModelAliases(map[string]string{"high": ""})
	if err == nil || !strings.Contains(err.Error(), "empty value") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateModelAliasesRejectsAliasChain(t *testing.T) {
	err := colony.ValidateModelAliases(map[string]string{
		"high":   "medium",
		"medium": "composer-2.5",
	})
	if err == nil || !strings.Contains(err.Error(), "not another alias") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadColonyModelAliases(t *testing.T) {
	dir := t.TempDir()
	pasekaDir := filepath.Join(dir, ".paseka")
	if err := os.MkdirAll(pasekaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pasekaDir, "colony.yaml"), []byte(`slug: test
model_aliases:
  high: composer-2.5
`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := colony.LoadColony(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.ModelAliases["high"] != "composer-2.5" {
		t.Fatalf("aliases = %#v", c.ModelAliases)
	}
}

func TestLoadColonyRejectsAliasChain(t *testing.T) {
	dir := t.TempDir()
	pasekaDir := filepath.Join(dir, ".paseka")
	if err := os.MkdirAll(pasekaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pasekaDir, "colony.yaml"), []byte(`slug: test
model_aliases:
  high: medium
  medium: composer-2.5
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := colony.LoadColony(dir); err == nil {
		t.Fatal("expected alias-chain error")
	}
}

func TestApplyModelAliases(t *testing.T) {
	params := adapters.RunParams{Model: "high"}
	colony.ApplyModelAliases(&params, map[string]string{"high": "vendor-id"})
	if params.Model != "vendor-id" {
		t.Fatalf("model = %q", params.Model)
	}
}

func TestApplyModelAliasesSkipsEmpty(t *testing.T) {
	params := adapters.RunParams{}
	colony.ApplyModelAliases(&params, map[string]string{"high": "vendor-id"})
	if params.Model != "" {
		t.Fatalf("model = %q, want empty", params.Model)
	}
}

func TestValidateModelIDAcceptsVendorAndBareIDs(t *testing.T) {
	tests := []string{
		"anthropic/claude-sonnet-4",
		"openrouter/deepseek/deepseek-r1:free",
		"composer-2.5",
		"gpt-4o:extended",
	}
	for _, id := range tests {
		if err := colony.ValidateModelID(id); err != nil {
			t.Fatalf("ValidateModelID(%q) = %v, want nil", id, err)
		}
	}
	if err := colony.ValidateModelID(""); err == nil {
		t.Fatal("empty id should be rejected")
	}
	if err := colony.ValidateModelID("/model"); err == nil {
		t.Fatal("empty vendor should be rejected")
	}
	if err := colony.ValidateModelID("vendor/"); err == nil {
		t.Fatal("empty model should be rejected")
	}
}

func TestValidateModelAliasesAcceptsVendorIDs(t *testing.T) {
	err := colony.ValidateModelAliases(map[string]string{
		"high":   "anthropic/claude-sonnet-4",
		"small":  "openai/gpt-5.5",
		"legacy": "composer-2.5",
	})
	if err != nil {
		t.Fatalf("ValidateModelAliases = %v, want nil", err)
	}
}

func TestValidateModelAliasesRejectsMalformedID(t *testing.T) {
	err := colony.ValidateModelAliases(map[string]string{"high": "/claude-sonnet-4"})
	if err == nil || !strings.Contains(err.Error(), "empty vendor") {
		t.Fatalf("err = %v", err)
	}
}

func TestProfileAliasOverlaySwitchesVendorForEveryBee(t *testing.T) {
	repo, slug := setupProfileColony(t)
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "colony.yaml"),
		[]byte("slug: "+slug+"\nmodel_aliases:\n  high: anthropic/claude-sonnet-4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustWriteProfile(t, filepath.Join(repo, ".paseka", "bees", "scout.yaml"),
		"role: scout\nadapter: cursor\nparams:\n  model: high\n")
	mustWriteProfile(t, filepath.Join(repo, ".paseka", "bees", "builder.yaml"),
		"role: builder\nadapter: cursor\nparams:\n  model: high\n")

	writeColonyProfile(t, repo, "ollama", "model_aliases:\n  high: ollama/qwen3\n")
	resetProcessProfile(t)
	colony.SetProcessProfile(colony.ProfileSelection{Name: "ollama", FlagSet: true})

	ctx, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.ModelAliases["high"] != "ollama/qwen3" {
		t.Fatalf("merged aliases = %+v", ctx.ModelAliases)
	}
	bees, err := ctx.LoadAllBees()
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"builder", "scout"} {
		params := colony.RunParamsFromBee(bees[role])
		colony.ApplyModelAliases(&params, ctx.ModelAliases)
		if params.Model != "ollama/qwen3" {
			t.Fatalf("%s resolved model = %q, want ollama/qwen3", role, params.Model)
		}
	}
}
