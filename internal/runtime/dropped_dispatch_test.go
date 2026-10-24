package runtime_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runtime"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailfork"
)

func TestBeeRunRefusesDroppedFork(t *testing.T) {
	repo := initBeeRunRepo(t)
	setupBeeRunHome(t, repo)
	traceID := "trace-dropped-run"
	lineage := trailfork.Lineage{TraceID: traceID, SourceTrace: "trace-source", State: trailfork.StateDropped}
	data, err := json.Marshal(lineage)
	if err != nil {
		t.Fatal(err)
	}
	path := trailfork.LineagePath(repo, traceID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	rec := &recordingAdapter{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)
	_, err = d.BeeRun(context.Background(), runtime.BeeRunRequest{
		StartDir: repo,
		Bee:      "scout",
		TraceID:  traceID,
		Task:     "should not run",
	})
	if err == nil || !strings.Contains(err.Error(), "dropped") {
		t.Fatalf("err = %v, want dropped refusal", err)
	}
	if rec.calls != 0 {
		t.Fatalf("adapter launched %d times", rec.calls)
	}
}

func TestBeeRunRefusesKilledTrail(t *testing.T) {
	repo := initBeeRunRepo(t)
	setupBeeRunHome(t, repo)
	traceID := "trace-killed-run"
	ledger := taskledger.NewMemoryLedger()
	kill, err := protocol.NewEvent(traceID, "cli", 0, protocol.EventSignal, protocol.SystemKillPayload{
		Kind:   protocol.SignalSystemKill,
		Reason: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(kill); err != nil {
		t.Fatal(err)
	}

	rec := &recordingAdapter{}
	d := runtime.NewDispatcher()
	d.SetLedger(ledger)
	d.RegisterAdapter("cursor", rec)
	_, err = d.BeeRun(context.Background(), runtime.BeeRunRequest{
		StartDir: repo,
		Bee:      "scout",
		TraceID:  traceID,
		Task:     "should not run",
	})
	if err == nil || !strings.Contains(err.Error(), "killed") {
		t.Fatalf("err = %v, want killed refusal", err)
	}
	if rec.calls != 0 {
		t.Fatalf("adapter launched %d times", rec.calls)
	}
}
