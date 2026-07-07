package runs_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/runs"
)

func TestNormalizePTYOutput(t *testing.T) {
	raw := "\x1b[31mhello\x1b[0m world\n\x07"
	got := runs.NormalizePTYOutput([]byte(raw))
	if got != "hello world" {
		t.Fatalf("NormalizePTYOutput = %q, want %q", got, "hello world")
	}
}

func TestReadTranscriptAfter(t *testing.T) {
	root := t.TempDir()
	d := runs.Dir{ColonyRoot: root, TraceID: "t1", AgentID: "a1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	for i, role := range []string{"system", "agent", "agent"} {
		if err := d.AppendTranscript(runs.TranscriptEntry{Role: role, Content: string(rune('a' + i))}); err != nil {
			t.Fatal(err)
		}
	}

	page, next, err := d.ReadTranscriptAfter(0)
	if err != nil || len(page) != 3 || next != 3 {
		t.Fatalf("after 0 = len %d next %d err %v", len(page), next, err)
	}
	page, next, err = d.ReadTranscriptAfter(2)
	if err != nil || len(page) != 1 || next != 3 || page[0].Role != "agent" {
		t.Fatalf("after 2 = %+v next %d err %v", page, next, err)
	}
}

func TestScanRecentSessions(t *testing.T) {
	root := t.TempDir()
	older := time.Now().UTC().Add(-2 * time.Hour)
	newer := time.Now().UTC().Add(-1 * time.Hour)

	writeSession := func(trace, agent string, started time.Time) {
		d := runs.Dir{ColonyRoot: root, TraceID: trace, AgentID: agent}
		if err := d.Prepare(); err != nil {
			t.Fatal(err)
		}
		if err := d.WriteSession(runs.SessionMeta{
			SessionID: agent,
			TraceID:   trace,
			AgentID:   agent,
			Bee:       "scout",
			State:     "completed",
			StartedAt: started,
		}); err != nil {
			t.Fatal(err)
		}
	}

	writeSession("t-old", "a-old", older)
	writeSession("t-new", "a-new", newer)

	list, err := runs.ScanRecentSessions(root, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d", len(list))
	}
	if list[0].AgentID != "a-new" {
		t.Fatalf("expected newest first, got %+v", list)
	}

	found, ok, err := runs.FindSessionMeta(root, "a-old")
	if err != nil || !ok || found.TraceID != "t-old" {
		t.Fatalf("FindSessionMeta = %+v ok=%v err=%v", found, ok, err)
	}
}

func TestScanRecentSessionsEmpty(t *testing.T) {
	root := t.TempDir()
	list, err := runs.ScanRecentSessions(root, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty, got %+v", list)
	}
	if _, err := os.Stat(filepath.Join(root, ".paseka")); os.IsNotExist(err) {
		// fine
	}
}
