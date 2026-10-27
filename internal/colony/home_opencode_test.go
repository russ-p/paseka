package colony_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
)

func TestLoadOpenCodeAdapterDefaultsWhenMissing(t *testing.T) {
	slug := setupOpenCodeAdapterHome(t, "")

	cfg, err := colony.LoadOpenCodeAdapter(slug)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Binary != "opencode" {
		t.Fatalf("binary = %q, want opencode", cfg.Binary)
	}
}

func TestLoadOpenCodeAdapterFromFile(t *testing.T) {
	slug := setupOpenCodeAdapterHome(t, "binary: /opt/opencode\n")

	cfg, err := colony.LoadOpenCodeAdapter(slug)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Binary != "/opt/opencode" {
		t.Fatalf("binary = %q", cfg.Binary)
	}
}

func TestLoadOpenCodeAdapterDefaultsBinaryWhenEmpty(t *testing.T) {
	slug := setupOpenCodeAdapterHome(t, "{}\n")

	cfg, err := colony.LoadOpenCodeAdapter(slug)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Binary != "opencode" {
		t.Fatalf("binary = %q, want opencode", cfg.Binary)
	}
}

func TestLoadOpenCodeAdapterVersionKeys(t *testing.T) {
	slug := setupOpenCodeAdapterHome(t, `binary: opencode
version: 2
v1_binary: opencode-v1
v2_binary: opencode-v2
detect_timeout: 250ms
`)

	cfg, err := colony.LoadOpenCodeAdapter(slug)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != "2" || cfg.V1Binary != "opencode-v1" || cfg.V2Binary != "opencode-v2" || cfg.DetectTimeout != "250ms" {
		t.Fatalf("config = %+v", cfg)
	}
	opts := cfg.Options()
	want := map[string]string{
		"version":        "2",
		"v1_binary":      "opencode-v1",
		"v2_binary":      "opencode-v2",
		"detect_timeout": "250ms",
	}
	for key, value := range want {
		if opts[key] != value {
			t.Fatalf("option %s = %q, want %q (all %+v)", key, opts[key], value, opts)
		}
	}
}

func TestLoadOpenCodeAdapterVersionDefaultsToAuto(t *testing.T) {
	slug := setupOpenCodeAdapterHome(t, "{}\n")

	cfg, err := colony.LoadOpenCodeAdapter(slug)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != "auto" {
		t.Fatalf("version = %q", cfg.Version)
	}
	if opts := cfg.Options(); opts != nil {
		t.Fatalf("undeclared options = %+v, want nil", opts)
	}
}

func setupOpenCodeAdapterHome(t *testing.T, yaml string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	slug := "opencode-adapter-test"
	homeDir := filepath.Join(home, "paseka", slug)
	if err := os.MkdirAll(filepath.Join(homeDir, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if yaml != "" {
		path := filepath.Join(homeDir, "adapters", "opencode.yaml")
		if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return slug
}
