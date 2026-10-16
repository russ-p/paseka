package runtime_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runtime"
)

// sleepingAgentScript is a fake agent binary that outlives any reasonable test.
// It replaces the recording fake so the runtime's params.timeout is the only
// thing that can end the run. It traps SIGTERM and records it, so the test can
// prove the runtime stops the process gracefully rather than SIGKILLing it
// outright.
const sleepingAgentScript = `#!/bin/sh
trap 'echo term > "$PASEKA_TERM_FILE"; exit 0' TERM
sleep 30 >/dev/null 2>&1 &
wait
`

// TestAFKTimeoutKillsSleepingAgent pins spec 038 Phase 6: an opt-in
// params.timeout bounds an AFK run. A fake agent that sleeps is brought down
// (SIGTERM, then SIGKILL after the grace) and the terminal status.json carries
// state cancelled with reason timeout, so a post-mortem can tell a slow run
// from a stuck one.
func TestAFKTimeoutKillsSleepingAgent(t *testing.T) {
	f := newLaunchFixture(t)

	sleeper := filepath.Join(t.TempDir(), "sleepy-agent")
	if err := os.WriteFile(sleeper, []byte(sleepingAgentScript), 0o755); err != nil {
		t.Fatal(err)
	}
	termFile := filepath.Join(t.TempDir(), "terminated")
	t.Setenv("PASEKA_TERM_FILE", termFile)
	homeDir, err := colony.HomeDir(launchSlug)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "adapters", "cursor.yaml"),
		[]byte("binary: "+sleeper+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Opt the cursor bee into a one-second bound.
	beeYAML := "role: cursor\nadapter: cursor\nprompt_template: prompt.md\nworktree: false\n" +
		"params:\n  model: " + launchModel + "\n  timeout: 1s\n"
	if err := os.WriteFile(filepath.Join(f.repo, ".paseka", "bees", "cursor.yaml"), []byte(beeYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	d := runtime.NewDispatcher()
	start := time.Now()
	res, err := d.BeeRun(context.Background(), runtime.BeeRunRequest{
		StartDir: f.repo, Bee: "cursor", TraceID: f.trace, Task: f.task, NoBus: true,
	})
	if err != nil {
		t.Fatalf("BeeRun: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Fatalf("run took %v; the sleeping agent was not killed by params.timeout", elapsed)
	}
	if res.Result == nil || res.Result.Status != string(protocol.StatusCancelled) {
		t.Fatalf("run status = %+v, want cancelled", res.Result)
	}
	if _, err := os.Stat(termFile); err != nil {
		t.Fatalf("agent was not asked to stop with SIGTERM: %v", err)
	}

	snap := readStatusSnapshot(t, res.RunDir)
	if snap.State != protocol.StatusCancelled {
		t.Fatalf("status.json state = %q, want cancelled", snap.State)
	}
	if snap.Reason != protocol.ReasonTimeout {
		t.Fatalf("status.json reason = %q, want %q", snap.Reason, protocol.ReasonTimeout)
	}
}

// TestAFKWithoutTimeoutStaysUnbounded pins the default: a bee that never sets
// params.timeout still runs to completion. The recording fake exits at once, so
// the run is bounded by the agent itself, not the runtime.
func TestAFKWithoutTimeoutStaysUnbounded(t *testing.T) {
	f := newLaunchFixture(t)
	capPath := filepath.Join(t.TempDir(), "capture.txt")
	t.Setenv("PASEKA_CAPTURE_FILE", capPath)

	d := runtime.NewDispatcher()
	res, err := d.BeeRun(context.Background(), runtime.BeeRunRequest{
		StartDir: f.repo, Bee: "cursor", TraceID: f.trace, Task: f.task, NoBus: true,
	})
	if err != nil {
		t.Fatalf("BeeRun: %v", err)
	}
	if res.Result == nil || res.Result.Status != string(protocol.StatusCompleted) {
		t.Fatalf("run status = %+v, want completed", res.Result)
	}
	snap := readStatusSnapshot(t, res.RunDir)
	if snap.Reason != "" {
		t.Fatalf("status.json reason = %q, want empty for a clean run", snap.Reason)
	}
}

func readStatusSnapshot(t *testing.T, runDir string) protocol.StatusSnapshot {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(runDir, "status.json"))
	if err != nil {
		t.Fatal(err)
	}
	var snap protocol.StatusSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatal(err)
	}
	return snap
}
