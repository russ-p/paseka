package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadColonyYAMLMissing(t *testing.T) {
	root := t.TempDir()
	snap, err := loadColonyYAML(root)
	if err != nil {
		t.Fatal(err)
	}
	if snap == nil || !snap.Missing {
		t.Fatalf("want missing snapshot, got %+v", snap)
	}
	if snap.Content != "" {
		t.Fatalf("missing colony should have empty content, got %q", snap.Content)
	}
}

func TestLoadColonyYAMLReadError(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".paseka", "colony.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := loadColonyYAML(root)
	if err == nil {
		t.Fatal("expected read error when colony.yaml is a directory")
	}
	if !strings.Contains(err.Error(), "colony.yaml") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadBeeYAMLSkipsInvalidRoles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "secret.yaml"), []byte("leaked: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := loadBeeYAML(root, []string{"../../secret", "scout"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("expected no snapshots, got %+v", out)
	}
}

func TestValidBeeRole(t *testing.T) {
	if !validBeeRole("scout") || validBeeRole("../x") || validBeeRole("a/b") {
		t.Fatal("validBeeRole mismatch")
	}
}
