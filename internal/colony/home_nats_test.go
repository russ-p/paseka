package colony_test

import (
	"os"
	"os/exec"
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

func TestEnrichFromHomeLoadsNATSURLWhenContextOmitsHome(t *testing.T) {
	repo := t.TempDir()
	runGitHome(t, repo, "init")
	runGitHome(t, repo, "config", "user.email", "test@test.com")
	runGitHome(t, repo, "config", "user.name", "test")
	runGitHome(t, repo, "commit", "--allow-empty", "-m", "init")

	paseka := filepath.Join(repo, ".paseka")
	if err := os.MkdirAll(paseka, 0o755); err != nil {
		t.Fatal(err)
	}
	slug := "nats-enrich-test"
	if err := os.WriteFile(filepath.Join(paseka, "colony.yaml"), []byte("slug: "+slug+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("PASEKA_NATS_URL", "")
	homeDir := filepath.Join(xdg, "paseka", slug)
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "colony_root: " + repo + "\nslug: " + slug + "\nnats:\n  url: nats://from-home:4222\n"
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	partial := colony.Context{ColonyRoot: repo, Slug: slug}
	if got := bus.ConfigFromContext(partial, colony.Colony{}).URL; got != "" {
		t.Fatalf("partial context URL = %q, want empty before enrich", got)
	}
	enriched := colony.EnrichFromHome(partial)
	busCfg := bus.ConfigFromContext(enriched, colony.Colony{})
	if busCfg.URL != "nats://from-home:4222" {
		t.Fatalf("enriched URL = %q, want nats://from-home:4222", busCfg.URL)
	}
}

func runGitHome(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
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
