package colony_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
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
