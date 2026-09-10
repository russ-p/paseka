package colony_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
)

func TestLoadColonyDeliveryDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".paseka"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".paseka", "colony.yaml"), []byte("slug: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := colony.LoadColony(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Defaults.ResolvedDelivery() != colony.DeliveryLocalMerge {
		t.Fatalf("delivery = %q", c.Defaults.ResolvedDelivery())
	}
}

func TestLoadColonyDeliveryPullRequest(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".paseka"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".paseka", "colony.yaml"), []byte("slug: test\ndefaults:\n  delivery: pull_request\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := colony.LoadColony(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Defaults.ResolvedDelivery() != colony.DeliveryPullRequest {
		t.Fatalf("delivery = %q", c.Defaults.ResolvedDelivery())
	}
}

func TestLoadColonyRejectsUnknownDelivery(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".paseka"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".paseka", "colony.yaml"), []byte("slug: test\ndefaults:\n  delivery: both\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := colony.LoadColony(dir)
	if err == nil || !strings.Contains(err.Error(), "defaults.delivery") {
		t.Fatalf("err = %v", err)
	}
}

func TestDiagnoseDeliveryForgeCommandNotExecutable(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".paseka"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".paseka", "colony.yaml"), []byte("slug: test\ndefaults:\n  delivery: pull_request\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest, err := colony.LoadColony(dir)
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "no-such-forge")
	warnings := colony.DiagnoseDelivery(colony.Context{
		ColonyRoot: dir,
		Home:       colony.HomeConfig{Forge: colony.ForgeConfig{Command: []string{missing}}},
	}, manifest)
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "not executable") {
		t.Fatalf("warnings = %q", joined)
	}
}
