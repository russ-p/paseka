package trailconfig_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/trailconfig"
)

func TestEnsureCapturesCompleteBinding(t *testing.T) {
	root := writeColony(t, "slug: test\nmodel_aliases:\n  builder: vendor/builder\n")
	home := setupHome(t, "test")
	ctx := testContext(root)

	manifest, err := trailconfig.Ensure(ctx, "trace-one")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.InitialConfigRef == "" || manifest.ActiveConfigRef != manifest.InitialConfigRef {
		t.Fatalf("binding = %+v", manifest)
	}
	if manifest.ConfigRevision != 1 || manifest.Policy != trailconfig.PolicyEditable {
		t.Fatalf("revision/policy = %d/%q", manifest.ConfigRevision, manifest.Policy)
	}
	if len(manifest.Snapshots) != 1 || len(manifest.History) != 1 {
		t.Fatalf("snapshots/history = %d/%d", len(manifest.Snapshots), len(manifest.History))
	}
	if manifest.Snapshots[0].ConfigRef != manifest.InitialConfigRef {
		t.Fatalf("snapshot ref = %q", manifest.Snapshots[0].ConfigRef)
	}

	binding, err := trailconfig.Read(root, "trace-one")
	if err != nil {
		t.Fatal(err)
	}
	if binding.InitialConfigRef != manifest.InitialConfigRef ||
		binding.ActiveConfigRef != manifest.ActiveConfigRef ||
		binding.ConfigRevision != 1 {
		t.Fatalf("binding = %+v", binding)
	}

	snapshot := filepath.Join(root, ".paseka", "runs", "trace-one", "config", "snapshots", manifest.InitialConfigRef)
	tree := filepath.Join(snapshot, "tree")
	for _, rel := range []string{
		".paseka/colony.yaml",
		".paseka/bees/builder.yaml",
		".paseka/bees/builder.local.yaml",
		".paseka/prompts/builder.md",
		".paseka/prompts/_partials/shared.md",
		".paseka/cues/feature.yaml",
		".paseka/profiles/pi.yaml",
		".agents/skills/example/SKILL.md",
		"home/profiles/pi/adapters/cursor.yaml",
		"home/skills/private/SKILL.md",
	} {
		if _, err := os.Stat(filepath.Join(tree, filepath.FromSlash(rel))); err != nil {
			t.Errorf("missing captured %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "config.yaml")); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(snapshot, trailconfig.ResolvedFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "vendor/builder") || !strings.Contains(string(data), `"profile": "pi"`) {
		t.Fatalf("resolved settings = %s", data)
	}
}

func TestConfigRefIgnoresLocationAndTrailIdentity(t *testing.T) {
	rootA := writeColony(t, "slug: first\nmodel_aliases:\n  builder: vendor/builder\n")
	rootB := writeColony(t, "slug: first\nmodel_aliases:\n  builder: vendor/builder\n")

	first, err := trailconfig.Ensure(testContext(rootA), "trace-a")
	if err != nil {
		t.Fatal(err)
	}
	sameRoot, err := trailconfig.Ensure(testContext(rootA), "trace-b")
	if err != nil {
		t.Fatal(err)
	}
	otherRoot, err := trailconfig.Ensure(testContext(rootB), "trace-c")
	if err != nil {
		t.Fatal(err)
	}
	if first.InitialConfigRef != sameRoot.InitialConfigRef || first.InitialConfigRef != otherRoot.InitialConfigRef {
		t.Fatalf("equal content refs differ: %s %s %s", first.InitialConfigRef, sameRoot.InitialConfigRef, otherRoot.InitialConfigRef)
	}

	if err := os.WriteFile(filepath.Join(rootB, ".paseka", "prompts", "builder.md"), []byte("changed prompt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := trailconfig.Ensure(testContext(rootB), "trace-d")
	if err != nil {
		t.Fatal(err)
	}
	if changed.InitialConfigRef == first.InitialConfigRef {
		t.Fatal("content change did not change ConfigRef")
	}
}

func TestConfigRefTracksExecutableModes(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	first, err := trailconfig.Ensure(testContext(root), "trace-mode-a")
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, ".paseka", "prompts", "tool.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := trailconfig.Ensure(testContext(root), "trace-mode-b")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	third, err := trailconfig.Ensure(testContext(root), "trace-mode-c")
	if err != nil {
		t.Fatal(err)
	}
	if second.InitialConfigRef == third.InitialConfigRef {
		t.Fatal("mode change did not change ConfigRef")
	}
	if first.InitialConfigRef == second.InitialConfigRef {
		t.Fatal("new file did not change ConfigRef")
	}
}

func TestSnapshotExcludesSecretsAndRuntimeData(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	home := setupHome(t, "test")
	secret := "super-secret-token"
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("nats:\n  url: nats://user:"+secret+"@localhost\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		".paseka/runs/other/agent/secret.txt",
		".paseka/worktrees/other/code.go",
		".paseka/cache/blob",
		".paseka/runs/trace-secret/trail-events.ndjson",
	} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(secret), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	manifest, err := trailconfig.Ensure(testContext(root), "trace-secret")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(root, ".paseka", "runs", "trace-secret", "config", "snapshots", manifest.InitialConfigRef, "tree")
	err = filepath.WalkDir(snapshot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(snapshot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		for _, forbidden := range []string{"runs/", "worktrees/", "cache/", "home/config.yaml", "home/adapters/"} {
			if strings.Contains(rel, forbidden) {
				t.Errorf("snapshot contains excluded %s", rel)
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), secret) {
			t.Errorf("snapshot %s contains a secret", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentEnsureConvergesOnOneBinding(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	ctx := testContext(root)
	const workers = 24
	var wg sync.WaitGroup
	refs := make([]string, workers)
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			manifest, err := trailconfig.Ensure(ctx, "trace-concurrent")
			if err != nil {
				errs[i] = err
				return
			}
			refs[i] = manifest.InitialConfigRef
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
	}
	for i := 1; i < workers; i++ {
		if refs[i] != refs[0] {
			t.Fatalf("refs diverged: %q vs %q", refs[0], refs[i])
		}
	}

	manifest, err := trailconfig.Load(root, "trace-concurrent")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ConfigRevision != 1 || len(manifest.History) != 1 || len(manifest.Snapshots) != 1 {
		t.Fatalf("duplicate revisions: %+v", manifest)
	}
	entries, err := os.ReadDir(filepath.Join(root, ".paseka", "runs", "trace-concurrent", "config", "snapshots"))
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}
	if len(dirs) != 1 || dirs[0] != manifest.InitialConfigRef {
		t.Fatalf("snapshots = %v", dirs)
	}
}

func TestLoadWithoutBinding(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	if _, err := trailconfig.Read(root, "trace-missing"); !errors.Is(err, trailconfig.ErrNoBinding) {
		t.Fatalf("err = %v", err)
	}
}

func testContext(root string) colony.Context {
	return colony.Context{
		ColonyRoot:    root,
		Slug:          "test",
		Profile:       "pi",
		ProfileLayers: colony.ProfileLayers{Colony: true, Home: true},
		ModelAliases:  map[string]string{"builder": "vendor/builder"},
	}
}

func setupHome(t *testing.T, slug string) string {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	home := filepath.Join(base, "paseka", slug)
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("profile: pi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"profiles/pi/adapters/cursor.yaml",
		"skills/private/SKILL.md",
	} {
		path := filepath.Join(home, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		content := "private: true\n"
		if strings.HasSuffix(rel, "cursor.yaml") {
			content = "binary: agent\napi_key_env: CURSOR_API_KEY\n"
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func writeColony(t *testing.T, manifest string) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		".paseka/colony.yaml":                 manifest,
		".paseka/bees/builder.yaml":           "role: builder\nadapter: script\nprompt_template: builder.md\ncommand: [echo]\n",
		".paseka/bees/builder.local.yaml":     "prompt_template: builder.md\n",
		".paseka/prompts/builder.md":          "build the thing\n",
		".paseka/prompts/_partials/shared.md": "shared partial\n",
		".paseka/cues/feature.yaml":           "emit: signal\ntype: SIGNAL\nkind: feature.requested\n",
		".paseka/profiles/pi.yaml":            "adapter: pi\n",
		".agents/skills/example/SKILL.md":     "---\nname: example\ndescription: example skill\n---\nbody\n",
	}
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestBindReturnsImmutableExecutionContext(t *testing.T) {
	root := writeColony(t, "slug: test\nmodel_aliases:\n  builder: vendor/builder\n")
	setupHome(t, "test")
	ctx := testContext(root)

	bound, manifest, err := trailconfig.Bind(ctx, "trace-bind")
	if err != nil {
		t.Fatal(err)
	}
	if !bound.BoundConfig() {
		t.Fatal("context was not bound")
	}
	if bound.ConfigRef != manifest.InitialConfigRef || bound.ConfigRevision != 1 {
		t.Fatalf("binding = %q rev %d, want %q rev 1", bound.ConfigRef, bound.ConfigRevision, manifest.InitialConfigRef)
	}
	if want := filepath.Join(root, ".paseka", "runs", "trace-bind", "config", "snapshots", bound.ConfigRef, "tree"); bound.ConfigRoot != want {
		t.Fatalf("config root = %q, want %q", bound.ConfigRoot, want)
	}
	if bound.WorkspaceRoot != root {
		t.Fatalf("workspace root = %q, want %q", bound.WorkspaceRoot, root)
	}
	if bound.ModelAliases["builder"] != "vendor/builder" || bound.Profile != "pi" {
		t.Fatalf("resolved settings = %+v", bound)
	}
	bee, _, err := bound.LoadBee("builder")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(bee.Command.Argv(), " "); got != "echo" {
		t.Fatalf("bound command = %q, want echo", got)
	}
	if bee.Adapter != "script" {
		t.Fatalf("bound adapter = %q, want script", bee.Adapter)
	}

	live := filepath.Join(root, ".paseka", "bees", "builder.yaml")
	if err := os.WriteFile(live, []byte("role: builder\nadapter: cursor\nprompt_template: builder.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stillBound, _, err := trailconfig.Bind(testContext(root), "trace-bind")
	if err != nil {
		t.Fatal(err)
	}
	bee, _, err = stillBound.LoadBee("builder")
	if err != nil {
		t.Fatal(err)
	}
	if bee.Adapter != "script" {
		t.Fatalf("bound adapter changed with live edit: %q", bee.Adapter)
	}
}

func TestBindBlocksMissingSnapshotContent(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	bound, _, err := trailconfig.Bind(ctx, "trace-corrupt")
	if err != nil {
		t.Fatal(err)
	}
	prompt := filepath.Join(bound.ConfigRoot, ".paseka", "prompts", "builder.md")
	if err := os.Remove(prompt); err != nil {
		t.Fatal(err)
	}
	if _, _, err := trailconfig.Bind(ctx, "trace-corrupt"); err == nil {
		t.Fatal("expected binding to fail when snapshot content is missing")
	}
}
