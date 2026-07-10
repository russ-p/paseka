package colony_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/colony"
)

func TestNormalizeInitAdapter(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "cursor"},
		{"cursor", "cursor"},
		{"pi", "pi"},
		{"PI", "pi"},
		{" claude ", "cursor"},
		{"unknown", "cursor"},
	}
	for _, tc := range tests {
		if got := colony.NormalizeInitAdapter(tc.in); got != tc.want {
			t.Errorf("NormalizeInitAdapter(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestInitScaffoldWithPiAdapter(t *testing.T) {
	repo := initTestRepo(t)
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	res, err := colony.Init(colony.InitOptions{StartDir: repo, Adapter: "pi"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Adapter != "pi" {
		t.Fatalf("adapter = %q, want pi", res.Adapter)
	}

	scout, err := os.ReadFile(filepath.Join(repo, ".paseka", "bees", "scout.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(scout), "adapter: pi") {
		t.Fatalf("scout bee should use pi adapter:\n%s", scout)
	}
	if !strings.Contains(string(scout), "output_format: json") {
		t.Fatalf("scout bee should use json output:\n%s", scout)
	}

	builder, err := os.ReadFile(filepath.Join(repo, ".paseka", "bees", "builder.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(builder), "adapter: pi") {
		t.Fatalf("builder bee should use pi adapter:\n%s", builder)
	}

	piYAML := filepath.Join(res.HomeDir, "adapters", "pi.yaml")
	if _, err := os.Stat(piYAML); err != nil {
		t.Fatalf("missing pi adapter config: %v", err)
	}

	cfg, err := os.ReadFile(filepath.Join(res.HomeDir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfg), "pi: {}") {
		t.Fatalf("home config should reference pi adapter:\n%s", cfg)
	}
}

func TestInitUnsupportedAdapterFallsBackToCursor(t *testing.T) {
	repo := initTestRepo(t)
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	res, err := colony.Init(colony.InitOptions{StartDir: repo, Adapter: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Adapter != "cursor" {
		t.Fatalf("adapter = %q, want cursor", res.Adapter)
	}

	scout, err := os.ReadFile(filepath.Join(repo, ".paseka", "bees", "scout.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(scout), "adapter: cursor") {
		t.Fatalf("scout bee should fall back to cursor:\n%s", scout)
	}

	if _, err := os.Stat(filepath.Join(res.HomeDir, "adapters", "pi.yaml")); !os.IsNotExist(err) {
		t.Fatalf("pi.yaml should not be scaffolded for cursor init: %v", err)
	}
}
