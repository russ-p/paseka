package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/colony"
)

func TestWorkspaceForBeeSectorNoWorktree(t *testing.T) {
	root := t.TempDir()
	sectorDir := filepath.Join(root, "frontend")
	if err := os.MkdirAll(sectorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".paseka"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".paseka", "colony.yaml"), []byte(`sectors:
  frontend:
    path: frontend
`), 0o644); err != nil {
		t.Fatal(err)
	}

	manifest, err := colony.LoadColony(root)
	if err != nil {
		t.Fatal(err)
	}
	ctxColony := colony.Context{ColonyRoot: root, Slug: "test"}
	bee := colony.Bee{Role: "builder", Worktree: false}

	workspace, sectorRel, err := workspaceForDispatch(ctxColony, manifest, bee, "trace-1", "frontend", "")
	if err != nil {
		t.Fatal(err)
	}
	if sectorRel != "frontend" {
		t.Fatalf("sectorRel = %q", sectorRel)
	}
	if workspace != sectorDir {
		t.Fatalf("workspace = %q, want %q", workspace, sectorDir)
	}
}

func TestWorkspaceForBeeUnknownSector(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".paseka"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".paseka", "colony.yaml"), []byte("sectors: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	manifest, err := colony.LoadColony(root)
	if err != nil {
		t.Fatal(err)
	}
	ctxColony := colony.Context{ColonyRoot: root, Slug: "test"}
	bee := colony.Bee{Role: "builder", Worktree: false}

	_, _, err = workspaceForDispatch(ctxColony, manifest, bee, "trace-1", "missing", "")
	if err == nil {
		t.Fatal("expected unknown sector error")
	}
}

func TestWorkspaceForBeeMissingSectorDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".paseka"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".paseka", "colony.yaml"), []byte(`sectors:
  frontend:
    path: frontend
`), 0o644); err != nil {
		t.Fatal(err)
	}

	manifest, err := colony.LoadColony(root)
	if err != nil {
		t.Fatal(err)
	}
	ctxColony := colony.Context{ColonyRoot: root, Slug: "test"}
	bee := colony.Bee{Role: "builder", Worktree: false}

	_, _, err = workspaceForDispatch(ctxColony, manifest, bee, "trace-1", "frontend", "")
	if err == nil {
		t.Fatal("expected missing sector directory error")
	}
}
