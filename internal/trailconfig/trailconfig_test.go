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

	captured, _, err := trailconfig.EnsureInitial(ctx, "trace-bind")
	if err != nil {
		t.Fatal(err)
	}
	bound, manifest, err := trailconfig.Bind(ctx, "trace-bind")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.InitialConfigRef != captured.InitialConfigRef {
		t.Fatalf("bound manifest = %+v, want captured %+v", manifest, captured)
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
	if _, _, err := trailconfig.EnsureInitial(ctx, "trace-corrupt"); err != nil {
		t.Fatal(err)
	}
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

func TestForkBindingPinsExecutionRoot(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)

	source, err := trailconfig.Ensure(ctx, "trace-source")
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, ".paseka", "worktrees", "trace-fork")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	fork, err := trailconfig.CreateForkBinding(trailconfig.ForkBindingOptions{
		Context:       ctx,
		SourceTraceID: "trace-source",
		TraceID:       "trace-fork",
		ConfigRef:     source.ActiveConfigRef,
		WorkspaceRoot: workspace,
		RequestID:     "req-fork",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fork.WorkspaceRoot != workspace {
		t.Fatalf("manifest workspace root = %q, want %q", fork.WorkspaceRoot, workspace)
	}

	binding, err := trailconfig.Read(root, "trace-fork")
	if err != nil {
		t.Fatal(err)
	}
	if binding.WorkspaceRoot != workspace {
		t.Fatalf("binding workspace root = %q, want %q", binding.WorkspaceRoot, workspace)
	}

	bound, err := trailconfig.BindRef(ctx, "trace-fork", fork.ActiveConfigRef, 1)
	if err != nil {
		t.Fatal(err)
	}
	if bound.WorkspaceRoot != workspace {
		t.Fatalf("bound workspace root = %q, want %q", bound.WorkspaceRoot, workspace)
	}

	// Editing the live colony and the fork checkout's configuration does not
	// move the pinned execution root or its captured configuration.
	if err := os.WriteFile(filepath.Join(workspace, "colony.yaml"), []byte("slug: other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".paseka", "bees", "builder.yaml"), []byte("role: builder\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rebound, err := trailconfig.BindRef(ctx, "trace-fork", fork.ActiveConfigRef, 1)
	if err != nil {
		t.Fatal(err)
	}
	if rebound.WorkspaceRoot != workspace || rebound.ConfigRoot != bound.ConfigRoot {
		t.Fatalf("rebound = %#v, want root %q config %q", rebound, workspace, bound.ConfigRoot)
	}

	sourceBound, err := trailconfig.BindRef(ctx, "trace-source", source.ActiveConfigRef, 1)
	if err != nil {
		t.Fatal(err)
	}
	if sourceBound.WorkspaceRoot != root {
		t.Fatalf("ordinary source workspace root = %q, want %q", sourceBound.WorkspaceRoot, root)
	}
}

func TestPrivateInputsAreClassifiedRestrictedAndReadable(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)

	manifest, err := trailconfig.Ensure(ctx, "trace-private")
	if err != nil {
		t.Fatal(err)
	}
	inventory, contents, err := trailconfig.ReadSnapshot(root, "trace-private", manifest.InitialConfigRef)
	if err != nil {
		t.Fatal(err)
	}

	private := map[string]bool{}
	for _, entry := range inventory.Entries {
		private[entry.Path] = entry.Private
	}
	for _, rel := range []string{
		"home/skills/private/SKILL.md",
		"home/profiles/pi/adapters/cursor.yaml",
		".paseka/bees/builder.local.yaml",
	} {
		if !private[rel] {
			t.Fatalf("%s was not classified private: %+v", rel, private)
		}
	}
	for _, rel := range []string{
		".paseka/colony.yaml",
		".paseka/bees/builder.yaml",
		".paseka/prompts/builder.md",
		".agents/skills/example/SKILL.md",
	} {
		if private[rel] {
			t.Fatalf("%s was classified private", rel)
		}
	}
	if len(contents) != len(inventory.Entries) {
		t.Fatalf("contents = %d, entries = %d", len(contents), len(inventory.Entries))
	}
	found := false
	for _, file := range contents {
		if file.Path != "home/skills/private/SKILL.md" {
			continue
		}
		found = true
		if string(file.Data) != "private: true\n" {
			t.Fatalf("retained private input = %q", file.Data)
		}
	}
	if !found {
		t.Fatal("retained snapshot did not return the private input")
	}

	// The private subtree is not world-traversable, so classification also
	// carries an access restriction on disk.
	privateDir := filepath.Join(root, ".paseka", "runs", "trace-private", "config", "snapshots",
		manifest.InitialConfigRef, "tree", "home")
	info, err := os.Stat(privateDir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("private subtree mode = %o, want 700", info.Mode().Perm())
	}
}

func TestConfigRefTracksPrivateInputContent(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	home := setupHome(t, "test")
	ctx := testContext(root)
	first, err := trailconfig.Ensure(ctx, "trace-private-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "skills", "private", "SKILL.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := trailconfig.Ensure(ctx, "trace-private-b")
	if err != nil {
		t.Fatal(err)
	}
	if first.InitialConfigRef == second.InitialConfigRef {
		t.Fatal("private input content did not contribute to ConfigRef")
	}
}

func TestBindingAndAttemptOwnershipSurviveRestart(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	ctx := testContext(root)
	if _, err := trailconfig.Ensure(ctx, "trace-restart"); err != nil {
		t.Fatal(err)
	}
	input := trailconfig.NewTaskInputEvidence("task-1", "title", "body", "builder", "", "feature", "none")
	attempt, err := trailconfig.EnsureTaskAttempt(ctx, "trace-restart", "task-1", "evt-1", input)
	if err != nil {
		t.Fatal(err)
	}

	// A fresh process reads the same durable binding and attempt ownership
	// instead of capturing a second version or admitting a duplicate attempt.
	reloaded, err := trailconfig.Load(root, "trace-restart")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ConfigRevision != 1 || reloaded.ActiveConfigRef == "" {
		t.Fatalf("reloaded binding = %+v", reloaded)
	}
	reopened, err := trailconfig.EnsureTaskAttempt(ctx, "trace-restart", "task-1", "evt-2", input)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.AttemptID != attempt.AttemptID {
		t.Fatalf("restart admitted a duplicate attempt: %s -> %s", attempt.AttemptID, reopened.AttemptID)
	}
	attempts, err := trailconfig.Attempts(root, "trace-restart")
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 {
		t.Fatalf("attempt records = %d, want 1", len(attempts))
	}
}
