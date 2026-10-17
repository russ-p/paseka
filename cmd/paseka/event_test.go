package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

// TestEventEmitRecordsWithNoBroker is the file-only colony's emit path: NATS is
// not configured, so nothing can be broadcast — and the event must still land in
// the trail's record. Every other publishing site in the colony falls back to the
// local publisher; this one used to be the exception and reported
// nats_not_configured instead.
func TestEventEmitRecordsWithNoBroker(t *testing.T) {
	repo := initTopologyFixtureRepoCLI(t)
	setupCLIHome(t, repo)

	const traceID = "trace-file-only"
	runDir := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: "agent-1"}
	if err := runDir.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := runDir.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         "agent-1",
		ColonyRoot:      repo,
	}); err != nil {
		t.Fatal(err)
	}

	emit := `{"traceId":"` + traceID + `","type":"SIGNAL","payload":{"kind":"feature.requested"}}`
	restore := swapStdin(t, emit)
	defer restore()

	// The command reads the process stdin and writes the process stdout, both
	// directly, so the test supplies both.
	restoreIn := swapStdin(t, emit)
	defer restoreIn()
	restoreOut, capture := captureStdout(t)
	defer restoreOut()

	cmd := newRoot()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"event", "emit", "--stdin", "--agent", "agent-1", "-C", repo})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("emit: %v\n%s", err, capture())
	}

	var result protocol.EventCLIResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(capture())), &result); err != nil {
		t.Fatalf("result %q: %v", capture(), err)
	}
	if !result.OK {
		t.Fatalf("result = %+v, want a recorded emit", result)
	}
	if result.Subject != "" {
		t.Fatalf("subject = %q, want empty — nothing was broadcast", result.Subject)
	}

	recorded, err := runs.ReadTrailEvents(repo, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(recorded) != 1 {
		t.Fatalf("trail record holds %d events, want the emitted one", len(recorded))
	}
	if recorded[0].ID == "" {
		t.Fatalf("recorded event has no identity: %+v", recorded[0])
	}
	// One event in both stores, one row in the merged view.
	runEvents, err := runDir.ReadEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(runEvents) != 1 || runEvents[0].ID != recorded[0].ID {
		t.Fatalf("run log = %+v, trail record = %+v", runEvents, recorded[0])
	}
	merged, err := runs.ReadTraceEvents(repo, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 1 {
		t.Fatalf("merged history = %d events, want 1", len(merged))
	}
	if _, err := os.Stat(filepath.Join(repo, ".paseka", "runs", traceID, runs.TrailEventsFileName)); err != nil {
		t.Fatalf("trail record missing: %v", err)
	}
}

// swapStdin points os.Stdin at the given payload for the duration of a test.
// `paseka event emit` reads the process stdin directly.
func swapStdin(t *testing.T, payload string) func() {
	t.Helper()
	path := filepath.Join(t.TempDir(), "event.json")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	prev := os.Stdin
	os.Stdin = f
	return func() {
		os.Stdin = prev
		_ = f.Close()
	}
}

// captureStdout redirects the process stdout for the duration of a test and
// returns an accessor for what was written.
func captureStdout(t *testing.T) (restore func(), read func() string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	prev := os.Stdout
	os.Stdout = w
	return func() { os.Stdout = prev }, func() string {
		_ = w.Close()
		data, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		_ = r.Close()
		return string(data)
	}
}
