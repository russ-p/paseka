package colony_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/colony"
)

func TestResolveSector(t *testing.T) {
	c := colony.Colony{
		Sectors: map[string]colony.Sector{
			"frontend": {Path: "frontend"},
		},
	}

	sector, err := c.ResolveSector("frontend")
	if err != nil {
		t.Fatal(err)
	}
	if sector.Path != "frontend" {
		t.Fatalf("path = %q", sector.Path)
	}

	if _, err := c.ResolveSector("missing"); err == nil {
		t.Fatal("expected error for unknown sector")
	}
}

func TestSectorRelPathRejectsEscape(t *testing.T) {
	c := colony.Colony{
		Sectors: map[string]colony.Sector{
			"bad": {Path: "../outside"},
		},
	}
	if _, err := c.SectorRelPath("bad"); err == nil {
		t.Fatal("expected path escape error")
	}
}

func TestSectorAbsPath(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "backend", "users"), 0o755); err != nil {
		t.Fatal(err)
	}

	c := colony.Colony{
		Sectors: map[string]colony.Sector{
			"users": {Path: "backend/users"},
		},
	}

	got, err := c.SectorAbsPath(root, "users")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "backend", "users")
	if got != want {
		t.Fatalf("abs path = %q, want %q", got, want)
	}
}

func TestJoinSectorPath(t *testing.T) {
	base := t.TempDir()
	sectorDir := filepath.Join(base, "frontend")
	if err := os.MkdirAll(sectorDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := colony.JoinSectorPath(base, "frontend")
	if err != nil {
		t.Fatal(err)
	}
	if got != sectorDir {
		t.Fatalf("joined = %q, want %q", got, sectorDir)
	}
}

func TestEnsureSectorDirExists(t *testing.T) {
	root := t.TempDir()
	if err := colony.EnsureSectorDirExists(root); err != nil {
		t.Fatalf("existing dir: %v", err)
	}
	if err := colony.EnsureSectorDirExists(filepath.Join(root, "missing")); err == nil {
		t.Fatal("expected missing dir error")
	}
}

func TestEffectiveSector(t *testing.T) {
	if got := colony.EffectiveSector("frontend", "backend"); got != "frontend" {
		t.Fatalf("task sector wins: got %q", got)
	}
	if got := colony.EffectiveSector("", "backend"); got != "backend" {
		t.Fatalf("bee default: got %q", got)
	}
}
