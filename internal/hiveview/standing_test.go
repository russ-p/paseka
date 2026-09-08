package hiveview_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/hiveview"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

func TestLoadStandingTraceIDsFromCues(t *testing.T) {
	root := t.TempDir()
	writeHiveStandingCue(t, root, "daily-triage.yaml", `description: Daily triage
emit: signal
type: SIGNAL
kind: triage.tick
standing:
  trace: trail-daily-triage
  stipend: 4
title: "{{.Title}}"
body: "{{.Body}}"
`)
	ids := hiveview.LoadStandingTraceIDs(root)
	if _, ok := ids["trail-daily-triage"]; !ok {
		t.Fatalf("ids = %#v", ids)
	}
}

func TestListTracesMarksStandingCueIds(t *testing.T) {
	root := t.TempDir()
	writeHiveStandingCue(t, root, "daily-triage.yaml", `description: Daily triage
emit: signal
type: SIGNAL
kind: triage.tick
standing:
  trace: trail-daily-triage
  stipend: 4
title: "{{.Title}}"
body: "{{.Body}}"
`)
	writeHiveTraceRun(t, root, "trail-daily-triage", "watch")
	writeHiveTraceRun(t, root, "trace-bloom", "scout")

	list, err := hiveview.ListTraces(colony.Context{ColonyRoot: root, Slug: "test"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]hiveview.TraceSummaryView{}
	for _, tr := range list {
		byID[tr.TraceID] = tr
	}
	if !byID["trail-daily-triage"].Standing {
		t.Fatalf("standing trail = %+v", byID["trail-daily-triage"])
	}
	if byID["trace-bloom"].Standing {
		t.Fatalf("bloom marked standing: %+v", byID["trace-bloom"])
	}
}

func TestGetTraceStandingFollowsCueYAML(t *testing.T) {
	root := t.TempDir()
	slug := "standing-badge"
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	homeDir := filepath.Join(home, "paseka", slug)
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte("colony_root: "+root+"\nslug: "+slug+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeHiveStandingCue(t, root, "daily-triage.yaml", `description: Daily triage
emit: signal
type: SIGNAL
kind: triage.tick
standing:
  trace: trail-daily-triage
  stipend: 4
title: "{{.Title}}"
body: "{{.Body}}"
`)
	writeHiveTraceRun(t, root, "trail-daily-triage", "watch")

	ctx := colony.Context{ColonyRoot: root, Slug: slug}
	detail, ok, err := hiveview.GetTrace(ctx, "trail-daily-triage")
	if err != nil || !ok {
		t.Fatalf("get = ok=%v err=%v", ok, err)
	}
	if !detail.Standing {
		t.Fatalf("detail = %+v", detail.TraceSummaryView)
	}

	writeHiveTraceRun(t, root, "trace-bloom", "scout")
	bloom, ok, err := hiveview.GetTrace(ctx, "trace-bloom")
	if err != nil || !ok {
		t.Fatalf("bloom get = ok=%v err=%v", ok, err)
	}
	if bloom.Standing {
		t.Fatal("bloom must not be standing")
	}
}

func writeHiveStandingCue(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, ".paseka", "cues")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeHiveTraceRun(t *testing.T, root, traceID, agentID string) {
	t.Helper()
	d := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-time.Minute)
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             agentID,
		Adapter:         "script",
		Workspace:       root,
		ColonyRoot:      root,
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusCompleted,
		StartedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
}
