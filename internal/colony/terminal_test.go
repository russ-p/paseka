package colony_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/colony"
)

func TestLoadTerminalConfigDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	slug := "term-test"
	if err := os.MkdirAll(filepath.Join(home, "paseka", slug), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := colony.LoadTerminalConfig(slug)
	if cfg.Terminal != "default" {
		t.Fatalf("terminal = %q", cfg.Terminal)
	}
	if cfg.GhosttyBinary != "ghostty" {
		t.Fatalf("ghostty_binary = %q", cfg.GhosttyBinary)
	}
}

func TestLoadTerminalConfigFromFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	slug := "term-test"
	dir := filepath.Join(home, "paseka", slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "terminal: ghostty\nghostty_binary: /usr/bin/ghostty\n"
	if err := os.WriteFile(filepath.Join(dir, "terminal.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := colony.LoadTerminalConfig(slug)
	if cfg.Terminal != "ghostty" {
		t.Fatalf("terminal = %q", cfg.Terminal)
	}
	if cfg.GhosttyBinary != "/usr/bin/ghostty" {
		t.Fatalf("ghostty_binary = %q", cfg.GhosttyBinary)
	}
}
