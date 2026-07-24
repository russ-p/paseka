package colony_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
)

func TestNATSConfigEffectiveURLFromConfig(t *testing.T) {
	cfg := colony.NATSConfig{URL: "nats://config:4222"}
	t.Setenv("PASEKA_NATS_URL", "")
	if got := cfg.EffectiveURL(); got != "nats://config:4222" {
		t.Fatalf("EffectiveURL = %q, want config URL", got)
	}
}

func TestNATSConfigEffectiveURLFromEnv(t *testing.T) {
	cfg := colony.NATSConfig{URL: "nats://config:4222"}
	t.Setenv("PASEKA_NATS_URL", "nats://env:4222")
	if got := cfg.EffectiveURL(); got != "nats://env:4222" {
		t.Fatalf("EffectiveURL = %q, want env URL", got)
	}
}

func TestNATSConfigEffectiveURLIgnoresEmptyEnv(t *testing.T) {
	cfg := colony.NATSConfig{URL: "nats://config:4222"}
	t.Setenv("PASEKA_NATS_URL", "   ")
	if got := cfg.EffectiveURL(); got != "nats://config:4222" {
		t.Fatalf("EffectiveURL = %q, want config URL when env is blank", got)
	}
}

func TestLoadHomeConfigNATSURLUsesEnvOverride(t *testing.T) {
	slug := setupHomeConfigNATS(t, "nats:\n  url: nats://config:4222\n")
	t.Setenv("PASEKA_NATS_URL", "nats://env:4222")

	home, err := colony.LoadHomeConfig(slug)
	if err != nil {
		t.Fatal(err)
	}
	busCfg := bus.ConfigFromContext(colony.Context{Slug: slug, Home: home}, colony.Colony{})
	if got := busCfg.URL; got != "nats://env:4222" {
		t.Fatalf("bus URL = %q, want env override", got)
	}
}

func TestLoadHomeConfigNATSURLFallsBackToConfig(t *testing.T) {
	slug := setupHomeConfigNATS(t, "nats:\n  url: nats://config:4222\n")
	t.Setenv("PASEKA_NATS_URL", "")

	home, err := colony.LoadHomeConfig(slug)
	if err != nil {
		t.Fatal(err)
	}
	busCfg := bus.ConfigFromContext(colony.Context{Slug: slug, Home: home}, colony.Colony{})
	if got := busCfg.URL; got != "nats://config:4222" {
		t.Fatalf("bus URL = %q, want config URL", got)
	}
}

func setupHomeConfigNATS(t *testing.T, configYAML string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	slug := "nats-url-test"
	homeDir := filepath.Join(home, "paseka", slug)
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(homeDir, "config.yaml")
	if err := os.WriteFile(path, []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	return slug
}
