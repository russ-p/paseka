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

func TestDispatchPiAFKEndToEnd(t *testing.T) {
	repo := initMixedAdapterRepo(t)

	fakePi := filepath.Join(t.TempDir(), "fake-pi")
	script := "#!/bin/sh\nprintf '%s\\n' '{\"summary\":\"dispatch ok\"}'\n"
	if err := os.WriteFile(fakePi, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	setupPiDispatchHome(t, repo, fakePi)

	piBee := `role: pi-worker
adapter: pi
prompt_template: worker.md
worktree: false
params:
  output_format: json
`
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "bees", "pi-worker.yaml"), []byte(piBee), 0o644); err != nil {
		t.Fatal(err)
	}

	d := runtime.NewDispatcher()
	res, err := d.BeeRun(context.Background(), runtime.BeeRunRequest{
		StartDir: repo,
		Bee:      "pi-worker",
		TraceID:  "trace-pi-dispatch",
		Task:     "run pi adapter",
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
	if len(res.Result.Events) != 0 {
		t.Fatalf("expected no parsed events, got %d", len(res.Result.Events))
	}

	runDir := runs.Dir{
		ColonyRoot: repo,
		TraceID:    "trace-pi-dispatch",
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
	if req.Adapter != "pi" {
		t.Fatalf("request adapter = %q", req.Adapter)
	}
	if !strings.Contains(req.Task, "run pi adapter") {
		t.Fatalf("request task = %q", req.Task)
	}

	status, err := runDir.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.State != protocol.StatusCompleted {
		t.Fatalf("final status = %q", status.State)
	}
}

func setupPiDispatchHome(t *testing.T, repo, fakePi string) {
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
	piYAML := fmt.Sprintf("binary: %s\n", fakePi)
	if err := os.WriteFile(filepath.Join(homeDir, "adapters", "pi.yaml"), []byte(piYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "adapters", "cursor.yaml"), []byte("binary: agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
