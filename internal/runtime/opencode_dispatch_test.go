package runtime_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/runtime"
)

func TestDispatchOpenCodeAFKEndToEnd(t *testing.T) {
	repo := initMixedAdapterRepo(t)

	fake := filepath.Join(t.TempDir(), "fake-opencode")
	script := "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"text\",\"sessionID\":\"ses_dispatch\",\"part\":{\"text\":\"dispatch ok\"}}'\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	setupOpenCodeDispatchHome(t, repo, fake)

	bee := `role: oc-worker
adapter: opencode
prompt_template: worker.md
worktree: false
params:
  output_format: json
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
