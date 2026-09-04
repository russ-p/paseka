package colony_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/colony"
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
