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
// projection reads: the task recorded on it, its outcome pair, when it started
// and finished, and the admission-time purpose and contract verdict that shape
// its participation.
type attentionRunFixture struct {
	agentID  string
	taskID   string
	state    protocol.RunStatus
	started  time.Time
	finished time.Time
	// purpose is the admission-time execution purpose. Empty is a legacy
	// record that never recorded one.
	purpose    protocol.ExecutionPurpose
	analysisOf string
	// contract is the completion-contract verdict. Empty means no declared
	// contract, which the run projection reads as unknown.
	contract string
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
		Purpose:         run.purpose,
		AnalysisOf:      run.analysisOf,
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
		result := protocol.Result{
			ProtocolVersion: protocol.Version,
			TraceID:         traceID,
			AgentID:         run.agentID,
			Status:          run.state,
			FinishedAt:      run.finished,
		}
		if run.contract != "" {
			result.Contract = &protocol.ContractRecord{Status: run.contract}
		}
		if err := d.WriteResult(result); err != nil {
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
			name: "a cancelled run is an open failure read as unfinished work",
			runs: []attentionRunFixture{
				{agentID: "agent-1", taskID: "task-a", state: protocol.StatusCancelled, started: at(0), finished: at(1), purpose: protocol.PurposeWork},
			},
			wantHasFailures: true,
			wantOpen:        1,
			wantNewest:      at(1),
		},
		{
			name: "a later success of the same task clears a cancelled run",
			runs: []attentionRunFixture{
				{agentID: "agent-1", taskID: "task-a", state: protocol.StatusCancelled, started: at(0), finished: at(1), purpose: protocol.PurposeWork},
				{agentID: "agent-2", taskID: "task-a", state: protocol.StatusCompleted, started: at(2), finished: at(3), purpose: protocol.PurposeWork},
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
			// The cancelled attempt is itself the newest open failure; it
			// supersedes nothing, it only adds a fresh unfinished attempt.
			wantNewest: at(3),
		},
		{
			name: "a contract-failed run is an open failure",
			runs: []attentionRunFixture{
				{agentID: "agent-1", taskID: "task-a", state: protocol.StatusCompleted, started: at(0), finished: at(1), contract: protocol.ContractFailed},
			},
			wantHasFailures: true,
			wantOpen:        1,
			wantNewest:      at(1),
		},
		{
			name: "a later success of the same task clears a contract failure",
			runs: []attentionRunFixture{
				{agentID: "agent-1", taskID: "task-a", state: protocol.StatusCompleted, started: at(0), finished: at(1), contract: protocol.ContractFailed},
				{agentID: "agent-2", taskID: "task-a", state: protocol.StatusCompleted, started: at(2), finished: at(3), contract: protocol.ContractPassed},
			},
			wantHasFailures: true,
		},
		{
			name: "a contract-unavailable run is an open failure",
			runs: []attentionRunFixture{
				{agentID: "agent-1", taskID: "task-a", state: protocol.StatusCompleted, started: at(0), finished: at(1), contract: protocol.ContractUnavailable},
			},
			wantHasFailures: true,
			wantOpen:        1,
			wantNewest:      at(1),
		},
		{
			name: "a later success of the same task clears a contract-unavailable run",
			runs: []attentionRunFixture{
				{agentID: "agent-1", taskID: "task-a", state: protocol.StatusCompleted, started: at(0), finished: at(1), contract: protocol.ContractUnavailable},
				{agentID: "agent-2", taskID: "task-a", state: protocol.StatusCompleted, started: at(2), finished: at(3), contract: protocol.ContractPassed},
			},
			wantHasFailures: true,
		},
		{
			name: "a failed analysis run does not raise the trail",
			runs: []attentionRunFixture{
				{agentID: "agent-1", taskID: "task-a", state: protocol.StatusFailed, started: at(0), finished: at(1), purpose: protocol.PurposeAnalysis, analysisOf: "completion-r1"},
			},
			// The historical flag still records the analysis failure; only the
			// attention projection excludes it.
			wantHasFailures: true,
		},
		{
			name: "a successful analysis run does not clear a work failure",
			runs: []attentionRunFixture{
				{agentID: "agent-1", taskID: "task-a", state: protocol.StatusFailed, started: at(0), finished: at(1), purpose: protocol.PurposeWork},
				{agentID: "agent-2", taskID: "task-a", state: protocol.StatusCompleted, started: at(2), finished: at(3), purpose: protocol.PurposeAnalysis, analysisOf: "completion-r1"},
			},
			wantHasFailures: true,
			wantOpen:        1,
			wantNewest:      at(1),
		},
		{
			name: "a failed analysis run does not resolve a work failure",
			runs: []attentionRunFixture{
				{agentID: "agent-1", taskID: "task-a", state: protocol.StatusFailed, started: at(0), finished: at(1), purpose: protocol.PurposeWork},
				{agentID: "agent-2", taskID: "task-a", state: protocol.StatusFailed, started: at(2), finished: at(3), purpose: protocol.PurposeAnalysis, analysisOf: "completion-r1"},
			},
			wantHasFailures: true,
			wantOpen:        1,
			wantNewest:      at(1),
		},
		{
			name: "a successful analysis run alone leaves the trail quiet",
			runs: []attentionRunFixture{
				{agentID: "agent-1", taskID: "task-a", state: protocol.StatusCompleted, started: at(0), finished: at(1), purpose: protocol.PurposeAnalysis, analysisOf: "completion-r1"},
			},
		},
		{
			name: "a closing success does not clear a work failure",
			runs: []attentionRunFixture{
				{agentID: "agent-1", taskID: "task-a", state: protocol.StatusFailed, started: at(0), finished: at(1), purpose: protocol.PurposeWork},
				{agentID: "agent-2", taskID: "closing-1", state: protocol.StatusCompleted, started: at(2), finished: at(3), purpose: protocol.PurposeClosing},
			},
			wantHasFailures: true,
			wantOpen:        1,
			wantNewest:      at(1),
		},
		{
			name: "direct runs share one trail-level group and a later success heals it",
			runs: []attentionRunFixture{
				{agentID: "agent-1", state: protocol.StatusFailed, started: at(0), finished: at(1), purpose: protocol.PurposeWork},
				{agentID: "agent-2", state: protocol.StatusCompleted, started: at(2), finished: at(3), purpose: protocol.PurposeWork},
			},
			wantHasFailures: true,
		},
		{
			name: "a direct failure is not cleared by a task success",
			runs: []attentionRunFixture{
				{agentID: "agent-1", state: protocol.StatusFailed, started: at(0), finished: at(1), purpose: protocol.PurposeWork},
				{agentID: "agent-2", taskID: "task-a", state: protocol.StatusCompleted, started: at(2), finished: at(3), purpose: protocol.PurposeWork},
			},
			wantHasFailures: true,
			wantOpen:        1,
			wantNewest:      at(1),
		},
		{
			name: "a legacy run with no recorded purpose participates as work-like",
			runs: []attentionRunFixture{
				{agentID: "agent-1", taskID: "task-a", state: protocol.StatusFailed, started: at(0), finished: at(1)},
				{agentID: "agent-2", taskID: "task-a", state: protocol.StatusCompleted, started: at(2), finished: at(3)},
			},
			wantHasFailures: true,
		},
		{
			name: "a lone legacy failure is open",
			runs: []attentionRunFixture{
				{agentID: "agent-1", taskID: "task-a", state: protocol.StatusFailed, started: at(0), finished: at(1)},
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
