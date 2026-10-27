package runtime_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/runtime"
)

func TestDispatchOpenCodeAFKEndToEnd(t *testing.T) {
	repo := initMixedAdapterRepo(t)

	fake := filepath.Join(t.TempDir(), "fake-opencode")
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 1.0.0; exit 0; fi\nprintf '%s\\n' '{\"type\":\"text\",\"sessionID\":\"ses_dispatch\",\"part\":{\"text\":\"dispatch ok\"}}'\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	setupOpenCodeDispatchHome(t, repo, fake)

	// `model: fast` is an alias key, so the record has to hold the resolved id and
	// the key that resolved it — not the key alone, which is all the bee config says.
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "colony.yaml"), []byte(`slug: test-colony
defaults:
  prompt_template: default.md
model_aliases:
  fast: opencode/space-bunny-free
`), 0o644); err != nil {
		t.Fatal(err)
	}

	bee := `role: oc-worker
adapter: opencode
prompt_template: worker.md
worktree: false
params:
  model: fast
`
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "bees", "oc-worker.yaml"), []byte(bee), 0o644); err != nil {
		t.Fatal(err)
	}

	d := runtime.NewDispatcher()
	res, err := d.BeeRun(context.Background(), runtime.BeeRunRequest{
		StartDir: repo,
		Bee:      "oc-worker",
		TraceID:  "trace-oc-dispatch",
		Task:     "run opencode adapter",
		NoBus:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Result.Status != string(protocol.StatusCompleted) {
		t.Fatalf("status = %q", res.Result.Status)
	}
	if res.Result.Summary != "dispatch ok" {
		t.Fatalf("summary = %q", res.Result.Summary)
	}
	if res.Result.ProviderSessionID != "ses_dispatch" {
		t.Fatalf("session = %q", res.Result.ProviderSessionID)
	}
	if len(res.Result.Events) != 0 {
		t.Fatalf("expected no parsed events, got %d", len(res.Result.Events))
	}

	runDir := runs.Dir{
		ColonyRoot: repo,
		TraceID:    "trace-oc-dispatch",
		AgentID:    res.AgentID,
	}
	for _, path := range []string{
		runDir.PromptPath(),
		runDir.MetaPath(),
		runDir.RequestPath(),
		runDir.StatusPath(),
		runDir.ResultJSONPath(),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing artifact %s: %v", path, err)
		}
	}

	req, err := runDir.ReadRequest()
	if err != nil {
		t.Fatal(err)
	}
	if req.Adapter != "opencode" {
		t.Fatalf("request adapter = %q", req.Adapter)
	}
	if !strings.Contains(req.Task, "run opencode adapter") {
		t.Fatalf("request task = %q", req.Task)
	}
	if req.Model != "opencode/space-bunny-free" {
		t.Fatalf("request model = %q, want the resolved id", req.Model)
	}
	if req.ModelAlias != "fast" {
		t.Fatalf("request modelAlias = %q, want the declared key", req.ModelAlias)
	}
	if req.LaunchForm != "params" {
		t.Fatalf("request launchForm = %q", req.LaunchForm)
	}

	// The run projection reads the model back off disk, which is the whole point:
	// no bee config or alias table is consulted to answer it.
	meta, err := runs.LoadRunMeta(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Model != "opencode/space-bunny-free" {
		t.Fatalf("run meta model = %q", meta.Model)
	}
}

func setupOpenCodeDispatchHome(t *testing.T, repo, fake string) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	slug := "test-colony"

	homeDir, err := colony.HomeDir(slug)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(homeDir, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf("colony_root: %q\nslug: %q\n", repo, slug)
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "adapters", "opencode.yaml"), []byte(fmt.Sprintf("binary: %s\n", fake)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "adapters", "cursor.yaml"), []byte("binary: agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
