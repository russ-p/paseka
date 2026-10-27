package trailconfig_test

import (
	"os"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/trailconfig"
)

func TestApplyRefusesKilledTrail(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	traceID := "trace-killed"
	initial, err := trailconfig.Ensure(ctx, traceID)
	if err != nil {
		t.Fatal(err)
	}
	appendKill(t, root, traceID)
	writeFile(t, root, ".paseka/prompts/builder.md", "must not apply\n")

	expected := initial.ConfigRevision
	_, err = trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: traceID, Source: trailconfig.SourceColony,
		Actor: "operator", ExpectedRevision: &expected,
	})
	if err == nil || !strings.Contains(err.Error(), "killed") {
		t.Fatalf("err = %v, want killed refusal", err)
	}
	assertBindingUnchanged(t, root, traceID, initial)

	// Cancel and reads stay available on a killed trail.
	if _, err := trailconfig.Read(root, traceID); err != nil {
		t.Fatalf("read on a killed trail: %v", err)
	}
}

func TestCaptureRefusesKilledTrail(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	traceID := "trace-killed-capture"
	appendKill(t, root, traceID)

	zero := 0
	if _, _, err := trailconfig.EnsureInitialExpected(ctx, traceID, &zero); err == nil || !strings.Contains(err.Error(), "killed") {
		t.Fatalf("err = %v, want killed capture refusal", err)
	}
	if _, err := os.Stat(trailconfig.ConfigDir(root, traceID)); !os.IsNotExist(err) {
		t.Fatalf("refused capture left a config directory: %v", err)
	}
}

func TestTraceKilledFindsKillMidLog(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	traceID := "trace-mid-log"
	appendNonKill(t, root, traceID, "before-1")
	appendNonKill(t, root, traceID, "before-2")
	appendKill(t, root, traceID)
	appendNonKill(t, root, traceID, "after")

	killed, err := trailconfig.TraceKilled(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if !killed {
		t.Fatal("kill recorded mid-log was not detected")
	}

	otherRoot := writeColony(t, "slug: test\n")
	if killed, err := trailconfig.TraceKilled(otherRoot, "trace-no-log"); err != nil || killed {
		t.Fatalf("absent log = %v, %v; want false, nil", killed, err)
	}
	appendNonKill(t, otherRoot, "trace-live", "note")
	if killed, err := trailconfig.TraceKilled(otherRoot, "trace-live"); err != nil || killed {
		t.Fatalf("un-killed log = %v, %v; want false, nil", killed, err)
	}
}

func TestTraceKilledUnreadableLogIsError(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	traceID := "trace-unreadable"
	path := runs.Trail{ColonyRoot: root, TraceID: traceID}.EventsPath()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := trailconfig.TraceKilled(root, traceID); err == nil {
		t.Fatal("expected an unreadable trail-event record to fail closed")
	}
	ctx := testContext(root)
	if _, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: traceID, Source: trailconfig.SourceColony, Actor: "operator",
	}); err == nil || !strings.Contains(err.Error(), "trail events") {
		t.Fatalf("apply err = %v, want a trail-event read failure", err)
	}
}

func TestApplyUnkilledTrailStillApplies(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	traceID := "trace-un-killed"
	initial, err := trailconfig.Ensure(ctx, traceID)
	if err != nil {
		t.Fatal(err)
	}
	appendNonKill(t, root, traceID, "context.note")
	writeFile(t, root, ".paseka/prompts/builder.md", "still allowed\n")
	expected := initial.ConfigRevision
	result, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: traceID, Source: trailconfig.SourceColony,
		Actor: "operator", ExpectedRevision: &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Change.Outcome != trailconfig.OutcomeApplied || result.Manifest.ConfigRevision != initial.ConfigRevision+1 {
		t.Fatalf("apply = %+v", result)
	}
}

func appendKill(t *testing.T, root, traceID string) {
	t.Helper()
	event, err := protocol.NewEvent(traceID, "cli", 0, protocol.EventSignal, protocol.SystemKillPayload{
		Kind: protocol.SignalSystemKill, Reason: "stop",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := (runs.Trail{ColonyRoot: root, TraceID: traceID}).AppendEvent(event); err != nil {
		t.Fatal(err)
	}
}

func appendNonKill(t *testing.T, root, traceID, kind string) {
	t.Helper()
	event, err := protocol.NewEvent(traceID, "cli", 0, protocol.EventSignal, map[string]string{"kind": kind})
	if err != nil {
		t.Fatal(err)
	}
	if err := (runs.Trail{ColonyRoot: root, TraceID: traceID}).AppendEvent(event); err != nil {
		t.Fatal(err)
	}
}
