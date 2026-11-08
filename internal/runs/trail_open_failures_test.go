package runs_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

// attentionRunFixture is one run directory described by what the attention
// projection reads: the task recorded on it, its outcome pair, and when it
// started and finished.
type attentionRunFixture struct {
	agentID  string
	taskID   string
	state    protocol.RunStatus
	started  time.Time
	finished time.Time
}

func writeAttentionRun(t *testing.T, root, traceID string, run attentionRunFixture) {
	t.Helper()
	d := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: run.agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         run.agentID,
		Bee:             "builder",
		Adapter:         "cursor",
		Workspace:       root,
		ColonyRoot:      root,
		TaskID:          run.taskID,
		CreatedAt:       run.started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           run.state,
		StartedAt:       run.started,
		FinishedAt:      run.finished,
	}); err != nil {
		t.Fatal(err)
	}
	switch run.state {
	case protocol.StatusCompleted, protocol.StatusFailed, protocol.StatusCancelled:
		// A terminal run needs both halves of the outcome pair; otherwise the
		// reader reports it as an incomplete record rather than an outcome.
		if err := d.WriteResult(protocol.Result{
			ProtocolVersion: protocol.Version,
			TraceID:         traceID,
			AgentID:         run.agentID,
			Status:          run.state,
			FinishedAt:      run.finished,
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// The trail summary's attention input is current: a failed run stays open only
// until a later success of the same task supersedes it. The historical flag is
// beside it, unchanged.
func TestLoadTraceSummaryProjectsOpenFailures(t *testing.T) {
	base := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	at := func(minutes int) time.Time { return base.Add(time.Duration(minutes) * time.Minute) }

	tests := []struct {
		name            string
		runs            []attentionRunFixture
		wantHasFailures bool
		wantOpen        int
		wantNewest      time.Time
		wantActive      bool
	}{
		{
			name: "failure then completion of the same task is healed",
			runs: []attentionRunFixture{
				{agentID: "agent-1", taskID: "task-a", state: protocol.StatusFailed, started: at(0), finished: at(1)},
				{agentID: "agent-2", taskID: "task-a", state: protocol.StatusCompleted, started: at(2), finished: at(3)},
			},
			wantHasFailures: true,
		},
		{
			name: "completion then failure is open",
			runs: []attentionRunFixture{
				{agentID: "agent-1", taskID: "task-a", state: protocol.StatusCompleted, started: at(0), finished: at(1)},
				{agentID: "agent-2", taskID: "task-a", state: protocol.StatusFailed, started: at(2), finished: at(3)},
			},
			wantHasFailures: true,
			wantOpen:        1,
			wantNewest:      at(3),
		},
		{
			name: "two failures on one task then one success resolve both",
			runs: []attentionRunFixture{
				{agentID: "agent-1", taskID: "task-a", state: protocol.StatusFailed, started: at(0), finished: at(1)},
				{agentID: "agent-2", taskID: "task-a", state: protocol.StatusFailed, started: at(2), finished: at(3)},
				{agentID: "agent-3", taskID: "task-a", state: protocol.StatusCompleted, started: at(4), finished: at(5)},
			},
			wantHasFailures: true,
		},
		{
			name: "a success on another task does not clear the failure",
			runs: []attentionRunFixture{
				{agentID: "agent-1", taskID: "task-a", state: protocol.StatusFailed, started: at(0), finished: at(1)},
				{agentID: "agent-2", taskID: "task-b", state: protocol.StatusCompleted, started: at(2), finished: at(3)},
			},
			wantHasFailures: true,
			wantOpen:        1,
			wantNewest:      at(1),
		},
		{
			name: "a queued retry keeps the failure open and marks the trail active",
			runs: []attentionRunFixture{
				{agentID: "agent-1", taskID: "task-a", state: protocol.StatusFailed, started: at(0), finished: at(1)},
				{agentID: "agent-2", taskID: "task-a", state: protocol.StatusQueued, started: at(2)},
			},
			wantHasFailures: true,
			wantOpen:        1,
			wantNewest:      at(1),
			wantActive:      true,
		},
		{
			name: "a running run is active and neither resolves nor adds",
			runs: []attentionRunFixture{
				{agentID: "agent-1", taskID: "task-a", state: protocol.StatusFailed, started: at(0), finished: at(1)},
				{agentID: "agent-2", taskID: "task-a", state: protocol.StatusRunning, started: at(2)},
			},
			wantHasFailures: true,
			wantOpen:        1,
			wantNewest:      at(1),
			wantActive:      true,
		},
		{
			name: "a cancelled run is not an open failure",
			runs: []attentionRunFixture{
				{agentID: "agent-1", taskID: "task-a", state: protocol.StatusCancelled, started: at(0), finished: at(1)},
			},
			wantHasFailures: true,
		},
		{
			name: "a cancelled run does not resolve an earlier failure",
			runs: []attentionRunFixture{
				{agentID: "agent-1", taskID: "task-a", state: protocol.StatusFailed, started: at(0), finished: at(1)},
				{agentID: "agent-2", taskID: "task-a", state: protocol.StatusCancelled, started: at(2), finished: at(3)},
			},
			wantHasFailures: true,
			wantOpen:        1,
			wantNewest:      at(1),
		},
		{
			name: "a finish-less failure orders by its start instant",
			runs: []attentionRunFixture{
				{agentID: "agent-1", taskID: "task-a", state: protocol.StatusCompleted, started: at(0), finished: at(1)},
				// No finish instant anywhere: the fallback is the start.
				{agentID: "agent-2", taskID: "task-a", state: protocol.StatusFailed, started: at(2)},
			},
			wantHasFailures: true,
			wantOpen:        1,
			wantNewest:      at(2),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			traceID := "trace-attention"
			for _, run := range tc.runs {
				writeAttentionRun(t, root, traceID, run)
			}

			summary, err := runs.LoadTraceSummary(root, traceID)
			if err != nil {
				t.Fatal(err)
			}
			if summary.HasFailures != tc.wantHasFailures {
				t.Fatalf("hasFailures = %v, want %v", summary.HasFailures, tc.wantHasFailures)
			}
			if summary.OpenFailures != tc.wantOpen {
				t.Fatalf("openFailures = %d, want %d", summary.OpenFailures, tc.wantOpen)
			}
			if summary.HasActive != tc.wantActive {
				t.Fatalf("hasActive = %v, want %v", summary.HasActive, tc.wantActive)
			}
			if tc.wantNewest.IsZero() {
				if summary.NewestOpenFailureAt != nil {
					t.Fatalf("newestOpenFailureAt = %s, want absent", summary.NewestOpenFailureAt)
				}
				return
			}
			if summary.NewestOpenFailureAt == nil || !summary.NewestOpenFailureAt.Equal(tc.wantNewest) {
				t.Fatalf("newestOpenFailureAt = %v, want %s", summary.NewestOpenFailureAt, tc.wantNewest)
			}
		})
	}
}

// A trail whose only history is events never flew a run, so it is idle rather
// than failed.
func TestLoadTraceSummaryEventsOnlyTrailStaysIdle(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-events-only"
	ev, err := protocol.NewEvent(traceID, "cli", 1, protocol.EventInsight, protocol.NarrativeInsightPayload{
		Kind:    protocol.InsightContextNote,
		Summary: "intake request",
	})
	if err != nil {
		t.Fatal(err)
	}
	trail := runs.Trail{ColonyRoot: root, TraceID: traceID}
	if err := trail.AppendEvent(ev); err != nil {
		t.Fatal(err)
	}

	summary, err := runs.LoadTraceSummary(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.EventCount != 1 || summary.RunCount != 0 {
		t.Fatalf("summary = %+v, want one event and no runs", summary)
	}
	if summary.HasFailures || summary.HasActive || summary.OpenFailures != 0 {
		t.Fatalf("summary = %+v, want an idle trail", summary)
	}
	if summary.NewestOpenFailureAt != nil {
		t.Fatalf("newestOpenFailureAt = %s, want absent", summary.NewestOpenFailureAt)
	}
}

// A run directory that cannot be read is skipped exactly as before: one damaged
// directory costs its own run, not the trail's projection.
func TestLoadTraceSummarySkipsUnreadableRunDirectory(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-broken-run"
	broken := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: "agent-broken"}
	if err := broken.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(broken.RequestPath(), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeAttentionRun(t, root, traceID, attentionRunFixture{
		agentID:  "agent-good",
		taskID:   "task-a",
		state:    protocol.StatusCompleted,
		started:  time.Now().UTC().Add(-time.Minute),
		finished: time.Now().UTC(),
	})

	summary, err := runs.LoadTraceSummary(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.RunCount != 1 {
		t.Fatalf("runCount = %d, want the readable run only", summary.RunCount)
	}
	if summary.HasFailures || summary.HasActive || summary.OpenFailures != 0 {
		t.Fatalf("summary = %+v, want a quiet trail", summary)
	}
}

// The historical flag keeps its meaning and the new fields are additive: a
// healed trail is hasFailures true with nothing open, and a client that does not
// know the new keys still decodes the payload.
func TestTraceSummaryAttentionFieldsAreAdditiveOnTheWire(t *testing.T) {
	quiet, err := json.Marshal(runs.TraceSummary{TraceID: "trace-quiet"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(quiet), "openFailures") || strings.Contains(string(quiet), "newestOpenFailureAt") {
		t.Fatalf("payload = %s, want the new fields absent when nothing is open", quiet)
	}

	newest := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(runs.TraceSummary{
		TraceID:             "trace-open",
		HasFailures:         true,
		OpenFailures:        2,
		NewestOpenFailureAt: &newest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `"openFailures":2`) ||
		!strings.Contains(string(payload), `"newestOpenFailureAt":"2026-09-25T12:00:00Z"`) {
		t.Fatalf("payload = %s, want the open-failure fields present", payload)
	}

	var older struct {
		HasFailures bool `json:"hasFailures"`
	}
	if err := json.Unmarshal(payload, &older); err != nil {
		t.Fatalf("older client failed to decode the additive payload: %v", err)
	}
	if !older.HasFailures {
		t.Fatal("older client lost the historical flag")
	}
}
