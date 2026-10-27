package trailconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/russ-p/paseka/internal/trailconfig"
)

func TestCaptureFollowsFileAndDirectorySymlinks(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")

	external := t.TempDir()
	promptTarget := filepath.Join(external, "shared.md")
	if err := os.WriteFile(promptTarget, []byte("linked prompt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(promptTarget, filepath.Join(root, ".paseka", "prompts", "linked.md")); err != nil {
		t.Fatal(err)
	}

	skillTarget := filepath.Join(external, "skill")
	if err := os.MkdirAll(skillTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillTarget, "SKILL.md"), []byte("linked skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(skillTarget, filepath.Join(root, ".agents", "skills", "linked")); err != nil {
		t.Fatal(err)
	}

	manifest, err := trailconfig.Ensure(testContext(root), "trace-symlink")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(root, ".paseka", "runs", "trace-symlink", "config", "snapshots", manifest.InitialConfigRef, "tree")

	for rel, want := range map[string]string{
		".paseka/prompts/linked.md":      "linked prompt\n",
		".agents/skills/linked/SKILL.md": "linked skill\n",
	} {
		data, err := os.ReadFile(filepath.Join(snapshot, filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("missing captured symlink %s: %v", rel, err)
			continue
		}
		if string(data) != want {
			t.Errorf("captured %s = %q, want %q", rel, data, want)
		}
	}

	if err := os.WriteFile(promptTarget, []byte("changed prompt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := trailconfig.Ensure(testContext(root), "trace-symlink-changed")
	if err != nil {
		t.Fatal(err)
	}
	if changed.InitialConfigRef == manifest.InitialConfigRef {
		t.Fatal("changing a symlink target did not change ConfigRef")
	}
}

func TestCaptureFollowsSymlinkedTreeRoot(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")

	prompts := filepath.Join(root, ".paseka", "prompts")
	if err := os.RemoveAll(prompts); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "builder.md"), []byte("external builder\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(target, "_partials"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "_partials", "shared.md"), []byte("external partial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, prompts); err != nil {
		t.Fatal(err)
	}

	manifest, err := trailconfig.Ensure(testContext(root), "trace-symlink-root")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(root, ".paseka", "runs", "trace-symlink-root", "config", "snapshots", manifest.InitialConfigRef, "tree")
	for rel, want := range map[string]string{
		".paseka/prompts/builder.md":          "external builder\n",
		".paseka/prompts/_partials/shared.md": "external partial\n",
	} {
		data, err := os.ReadFile(filepath.Join(snapshot, filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("missing captured %s: %v", rel, err)
			continue
		}
		if string(data) != want {
			t.Errorf("captured %s = %q, want %q", rel, data, want)
		}
	}
}

func TestCaptureTerminatesOnSymlinkCycle(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")

	if err := os.Symlink(".", filepath.Join(root, ".paseka", "prompts", "self")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("..", filepath.Join(root, ".agents", "skills", "up")); err != nil {
		t.Fatal(err)
	}

	if _, err := trailconfig.Ensure(testContext(root), "trace-symlink-cycle"); err != nil {
		t.Fatalf("capture with a symlink cycle failed: %v", err)
	}
}
