package runs_test

import (
	"testing"
	"time"

	"github.com/paseka/paseka/internal/runs"
)

func TestSessionAndTranscript(t *testing.T) {
	root := t.TempDir()
	d := runs.Dir{ColonyRoot: root, TraceID: "t1", AgentID: "a1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}

	started := time.Now().UTC().Truncate(time.Second)
	meta := runs.SessionMeta{
		SessionID:  "a1",
		TraceID:    "t1",
		AgentID:    "a1",
		Bee:        "scout",
		Adapter:    "cursor",
		Workspace:  root,
		ColonyRoot: root,
		PID:        4242,
		State:      "active",
		StartedAt:  started,
	}
	if err := d.WriteSession(meta); err != nil {
		t.Fatal(err)
	}
	got, err := d.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	if got.PID != 4242 || got.State != "active" {
		t.Fatalf("session meta = %+v", got)
	}

	if err := d.AppendTranscript(runs.TranscriptEntry{Role: "user", Content: "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := d.AppendTranscript(runs.TranscriptEntry{Role: "agent", Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	entries, err := d.ReadTranscript()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Role != "user" || entries[1].Content != "hi" {
		t.Fatalf("transcript = %+v", entries)
	}
}
