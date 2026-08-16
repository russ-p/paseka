package runtime_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/runtime"
)

type combWritingAdapter struct {
	result *adapters.RunResult
}

func (a *combWritingAdapter) Name() string { return "cursor" }

func (a *combWritingAdapter) Run(_ context.Context, req adapters.RunRequest) (*adapters.RunResult, error) {
	combDir := filepath.Join(req.ColonyRoot, ".paseka", "runs", req.TraceID, "artifacts")
	if err := os.MkdirAll(combDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(combDir, "research.md"), []byte("# Research\nnotes"), 0o644); err != nil {
		return nil, err
	}
	if a.result != nil {
		return a.result, nil
	}
	return &adapters.RunResult{Status: string(protocol.StatusCompleted), Output: "ok"}, nil
}

func TestDispatchFlushesArtifactsOnSuccess(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)

	pub := &recordingPublisher{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", &combWritingAdapter{
		result: &adapters.RunResult{Status: string(protocol.StatusCompleted), Summary: "done"},
	})
	d.SetPublisher(pub, false)

	_, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-art",
		AgentID:    "agent-art",
		Task:       "write comb",
	})
	if err != nil {
		t.Fatal(err)
	}

	var found bool
	for _, ev := range pub.events {
		if ev.Type == protocol.EventSignal && protocol.PayloadKind(ev.Payload) == string(protocol.SignalArtifactWritten) {
			found = true
			if ev.AgentID != "agent-art" {
				t.Fatalf("producer = %q", ev.AgentID)
			}
		}
	}
	if !found {
		t.Fatalf("expected artifact.written, got %#v", pub.events)
	}

	runDir := runs.Dir{ColonyRoot: root, TraceID: "trace-art", AgentID: "agent-art"}
	events, err := runDir.ReadEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("expected audit log entry")
	}
}

func TestDispatchSkipsArtifactsFlushWhenDeferredArtifactWritten(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)

	first, err := protocol.NewEvent("trace-skip", "agent-skip", 0, protocol.EventSignal, map[string]any{
		"kind": "artifact.written",
		"artifacts": []map[string]any{{
			"ref":          ".paseka/runs/trace-skip/artifacts/deferred.md",
			"artifactKind": "deferred",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	combDir := filepath.Join(root, ".paseka", "runs", "trace-skip", "artifacts")
	if err := os.MkdirAll(combDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(combDir, "deferred.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(combDir, "research.md"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}

	adapter := &deferredSeedingAdapter{
		result:  &adapters.RunResult{Status: string(protocol.StatusCompleted), Summary: "done"},
		pending: []protocol.Event{first},
	}
	pub := &recordingPublisher{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", adapter)
	d.SetPublisher(pub, false)

	_, err = d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-skip",
		AgentID:    "agent-skip",
		Task:       "defer artifact",
	})
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for _, ev := range pub.events {
		if ev.Type == protocol.EventSignal && protocol.PayloadKind(ev.Payload) == string(protocol.SignalArtifactWritten) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one artifact.written, got %d", count)
	}
}

func TestDispatchNoArtifactsFlushOnFailedRun(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)

	pub := &recordingPublisher{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", &combWritingAdapter{
		result: &adapters.RunResult{Status: string(protocol.StatusFailed), Summary: "fail"},
	})
	d.SetPublisher(pub, false)

	_, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-fail-art",
		AgentID:    "agent-fail-art",
		Task:       "fail",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range pub.events {
		if protocol.PayloadKind(ev.Payload) == string(protocol.SignalArtifactWritten) {
			t.Fatal("unexpected artifact.written on failed run")
		}
	}
}

func TestDispatchNoArtifactsFlushWhenUnchangedComb(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)
	combDir := filepath.Join(root, ".paseka", "runs", "trace-unchanged", "artifacts")
	if err := os.MkdirAll(combDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(combDir, "old.md"), []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}

	pub := &recordingPublisher{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", &combWritingAdapter{
		result: &adapters.RunResult{Status: string(protocol.StatusCompleted), Summary: "done"},
	})
	d.SetPublisher(pub, false)

	_, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-unchanged",
		AgentID:    "agent-unchanged",
		Task:       "noop",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range pub.events {
		if protocol.PayloadKind(ev.Payload) == string(protocol.SignalArtifactWritten) {
			var payload struct {
				Artifacts []struct {
					Ref string `json:"ref"`
				} `json:"artifacts"`
			}
			_ = json.Unmarshal(ev.Payload, &payload)
			for _, item := range payload.Artifacts {
				if strings.Contains(item.Ref, "old.md") {
					t.Fatal("should not announce unchanged comb file")
				}
			}
		}
	}
}
