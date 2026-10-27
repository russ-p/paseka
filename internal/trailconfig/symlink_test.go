package trailconfig_test

import (
	"os"
	"path/filepath"
	"strings"
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

func TestCaptureRefusesUncapturableSymlinks(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, root string)
		want  string
	}{
		{
			name: "dangling file symlink",
			setup: func(t *testing.T, root string) {
				if err := os.Symlink("missing.md", filepath.Join(root, ".paseka", "prompts", "dangling.md")); err != nil {
					t.Fatal(err)
				}
			},
			want: ".paseka/prompts/dangling.md",
		},
		{
			name: "dangling tree root",
			setup: func(t *testing.T, root string) {
				prompts := filepath.Join(root, ".paseka", "prompts")
				if err := os.RemoveAll(prompts); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(root, ".paseka", "prompts-gone"), prompts); err != nil {
					t.Fatal(err)
				}
			},
			want: ".paseka/prompts",
		},
		{
			name: "dangling single file input",
			setup: func(t *testing.T, root string) {
				manifest := filepath.Join(root, ".paseka", "colony.yaml")
				if err := os.Remove(manifest); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(root, ".paseka", "colony-missing.yaml"), manifest); err != nil {
					t.Fatal(err)
				}
			},
			want: ".paseka/colony.yaml",
		},
		{
			name: "non-regular target",
			setup: func(t *testing.T, root string) {
				if err := os.Symlink(os.DevNull, filepath.Join(root, ".paseka", "prompts", "device.md")); err != nil {
					t.Fatal(err)
				}
			},
			want: ".paseka/prompts/device.md",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := writeColony(t, "slug: test\n")
			setupHome(t, "test")
			tc.setup(t, root)

			_, err := trailconfig.Ensure(testContext(root), "trace-uncapturable")
			if err == nil {
				t.Fatal("capture accepted a symlink it cannot capture")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to name %s", err, tc.want)
			}
		})
	}
}

// TestCaptureIgnoresBrokenLinkOutsideTreeFilter keeps the stricter symlink rule
// from failing a capture over a link that the tree filter never selects.
func TestCaptureIgnoresBrokenLinkOutsideTreeFilter(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")

	if err := os.Symlink("missing.txt", filepath.Join(root, ".paseka", "bees", "notes.txt")); err != nil {
		t.Fatal(err)
	}

	if _, err := trailconfig.Ensure(testContext(root), "trace-irrelevant-link"); err != nil {
		t.Fatalf("capture failed on an ignored link: %v", err)
	}
}
