package bus_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
)

func TestPingUnconfigured(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	slug := "ping-off"
	homeDir := filepath.Join(home, "paseka", slug)
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte("colony_root: "+repo+"\nslug: "+slug+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".paseka"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "colony.yaml"), []byte("name: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := colony.Context{
		ColonyRoot: repo,
		Slug:       slug,
		Home:       colony.HomeConfig{ColonyRoot: repo, Slug: slug},
	}

	result, err := bus.Ping(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Configured || result.Connected {
		t.Fatalf("ping = %+v", result)
	}
}

func TestPingConfiguredDisconnected(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	slug := "ping-down"
	homeDir := filepath.Join(home, "paseka", slug)
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "colony_root: " + repo + "\nslug: " + slug + "\nnats:\n  url: nats://127.0.0.1:59999\n"
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".paseka"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "colony.yaml"), []byte("name: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := colony.Context{
		ColonyRoot: repo,
		Slug:       slug,
		Home: colony.HomeConfig{
			ColonyRoot: repo,
			Slug:       slug,
			NATS:       colony.NATSConfig{URL: "nats://127.0.0.1:59999"},
		},
	}

	result, err := bus.Ping(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Configured || result.Connected {
		t.Fatalf("ping = %+v", result)
	}
}
