package colony_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/colony"
)

func TestLoadPiAdapterDefaultsWhenMissing(t *testing.T) {
	slug := setupPiAdapterHome(t, "")

	cfg, err := colony.LoadPiAdapter(slug)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Binary != "pi" {
		t.Fatalf("binary = %q, want pi", cfg.Binary)
	}
	if cfg.APIKeyEnv != "" {
		t.Fatalf("api_key_env = %q, want empty", cfg.APIKeyEnv)
	}
}

func TestLoadPiAdapterFromFile(t *testing.T) {
	slug := setupPiAdapterHome(t, "binary: /opt/pi\napi_key_env: GEMINI_API_KEY\n")

	cfg, err := colony.LoadPiAdapter(slug)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Binary != "/opt/pi" {
		t.Fatalf("binary = %q", cfg.Binary)
	}
	if cfg.APIKeyEnv != "GEMINI_API_KEY" {
		t.Fatalf("api_key_env = %q", cfg.APIKeyEnv)
	}
}

func TestLoadPiAdapterDefaultsBinaryWhenEmpty(t *testing.T) {
	slug := setupPiAdapterHome(t, "api_key_env: GEMINI_API_KEY\n")

	cfg, err := colony.LoadPiAdapter(slug)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Binary != "pi" {
		t.Fatalf("binary = %q, want pi", cfg.Binary)
	}
}

func TestPiAdapterConfigAPIKeyMissingEnv(t *testing.T) {
	cfg := colony.PiAdapterConfig{APIKeyEnv: "GEMINI_API_KEY"}
	t.Setenv("GEMINI_API_KEY", "")
	if got := cfg.APIKey(); got != "" {
		t.Fatalf("APIKey = %q, want empty when env unset", got)
	}
}

func TestPiAdapterConfigAPIKeyWhenEnvSet(t *testing.T) {
	cfg := colony.PiAdapterConfig{APIKeyEnv: "GEMINI_API_KEY"}
	t.Setenv("GEMINI_API_KEY", "secret-key")
	if got := cfg.APIKey(); got != "secret-key" {
		t.Fatalf("APIKey = %q, want secret-key", got)
	}
}

func TestPiAdapterConfigAPIKeyWithoutEnvName(t *testing.T) {
	cfg := colony.PiAdapterConfig{}
	t.Setenv("GEMINI_API_KEY", "secret-key")
	if got := cfg.APIKey(); got != "" {
		t.Fatalf("APIKey = %q, want empty when api_key_env unset", got)
	}
}

func setupPiAdapterHome(t *testing.T, piYAML string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	slug := "pi-adapter-test"
	homeDir := filepath.Join(home, "paseka", slug)
	if err := os.MkdirAll(filepath.Join(homeDir, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if piYAML != "" {
		path := filepath.Join(homeDir, "adapters", "pi.yaml")
		if err := os.WriteFile(path, []byte(piYAML), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return slug
}
