package trailconfig_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
)

// TestBlockersReportOnlyLiveExecution pins the cutover rule: an open attempt is
// not execution by itself. It blocks while it was admitted but never
// dispatched, or while it still owns a run that has not reached a terminal
// state. Idle direct-chain and session attempts stay open for routing and must
// not hold configuration activation forever.
func TestBlockersReportOnlyLiveExecution(t *testing.T) {
	traceID := "trace-blockers"
	cases := []struct {
		name string
		// kind is "direct" for an independent chain, "task" for a ledger task
		// attempt, or "none" for a run that has no attempt record at all.
		kind       string
		taskStatus protocol.TaskStatus
		dispatch   bool
		runStates  []protocol.RunStatus
		// linkAgent records the run agent on the attempt. When false the run is
		// still owned through its recorded provenance instead.
		linkAgent bool
		// noStatus creates the agent directory without a readable status.json.
		noStatus    bool
		wantNone    bool
		wantSubstrs []string
	}{
		{
			name:      "dispatched chain with completed run",
			kind:      "direct",
			dispatch:  true,
			runStates: []protocol.RunStatus{protocol.StatusCompleted},
			linkAgent: true,
			wantNone:  true,
		},
		{
			name:      "dispatched chain with failed run",
			kind:      "direct",
			dispatch:  true,
			runStates: []protocol.RunStatus{protocol.StatusFailed},
			linkAgent: true,
			wantNone:  true,
		},
		{
			name:      "dispatched chain with running run",
			kind:      "direct",
			dispatch:  true,
			runStates: []protocol.RunStatus{protocol.StatusRunning},
			linkAgent: true,
			wantSubstrs: []string{
				"%attempt%",
				"which is running",
			},
		},
		{
			name:      "chain with a finished and a live run",
			kind:      "direct",
			dispatch:  true,
			runStates: []protocol.RunStatus{protocol.StatusCompleted, protocol.StatusQueued},
			linkAgent: true,
			wantSubstrs: []string{
				"%attempt%",
				"which is queued",
			},
		},
		{
			name:     "admitted chain before dispatch",
			kind:     "direct",
			dispatch: false,
			wantSubstrs: []string{
				"%attempt%",
				"is open",
			},
		},
		{
			name:      "cli run owned only through provenance",
			kind:      "direct",
			dispatch:  true,
			runStates: []protocol.RunStatus{protocol.StatusRunning},
			linkAgent: false,
			wantSubstrs: []string{
				"%attempt%",
				"which is running",
			},
		},
		{
			name:      "finished interactive session attempt",
			kind:      "direct",
			dispatch:  true,
			runStates: []protocol.RunStatus{protocol.StatusCancelled},
			linkAgent: true,
			wantNone:  true,
		},
		{
			name:      "live interactive session attempt",
			kind:      "direct",
			dispatch:  true,
			runStates: []protocol.RunStatus{protocol.StatusRunning},
			linkAgent: true,
			wantSubstrs: []string{
				"%attempt%",
				"which is running",
			},
		},
		{
			name:       "task attempt with running task",
			kind:       "task",
			taskStatus: protocol.TaskStatusRunning,
			dispatch:   true,
			runStates:  []protocol.RunStatus{protocol.StatusCompleted},
			linkAgent:  true,
			wantSubstrs: []string{
				"task task-1 is running",
			},
		},
		{
			name:       "task attempt waiting for review",
			kind:       "task",
			taskStatus: protocol.TaskStatusWaitingReview,
			dispatch:   true,
			runStates:  []protocol.RunStatus{protocol.StatusCompleted},
			linkAgent:  true,
			wantSubstrs: []string{
				"task task-1 is waiting_review",
			},
		},
		{
			name:       "task attempt with completed task and terminal run",
			kind:       "task",
			taskStatus: protocol.TaskStatusCompleted,
			dispatch:   true,
			runStates:  []protocol.RunStatus{protocol.StatusCompleted},
			linkAgent:  true,
			wantNone:   true,
		},
		{
			name:      "attempt owns an unreadable run",
			kind:      "direct",
			dispatch:  true,
			linkAgent: true,
			noStatus:  true,
			wantSubstrs: []string{
				"%attempt%",
				"with no readable status",
			},
		},
		{
			name:      "run without an attempt record is still live",
			kind:      "none",
			runStates: []protocol.RunStatus{protocol.StatusRunning},
			wantSubstrs: []string{
				"run agent-1 is running",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := writeColony(t, "slug: test\n")
			setupHome(t, "test")
			ctx := testContext(root)
			if _, err := trailconfig.Ensure(ctx, traceID); err != nil {
				t.Fatal(err)
			}

			attemptID := ""
			if tc.kind != "none" {
				var err error
				if tc.taskStatus != "" {
					if err := runs.WriteTaskSnapshot(root, traceID, taskledger.TaskSnapshot{
						TaskID: "task-1", Title: "owned work", Status: tc.taskStatus,
					}); err != nil {
						t.Fatal(err)
					}
				}
				var attempt trailconfig.Attempt
				switch tc.kind {
				case "task":
					input := trailconfig.NewTaskInputEvidence("task-1", "owned work", "body", "builder", "", "feature", "none")
					attempt, err = trailconfig.EnsureTaskAttempt(ctx, traceID, "task-1", "evt-1", input)
				default:
					attempt, _, err = trailconfig.AdmitChainAttempt(ctx, traceID, "chain-1", "", "evt-1")
				}
				if err != nil {
					t.Fatal(err)
				}
				attemptID = attempt.AttemptID
				if tc.dispatch {
					if _, _, err := trailconfig.MarkAttemptDispatched(root, traceID, attempt.AttemptID, "dispatch-1"); err != nil {
						t.Fatal(err)
					}
				}
			}

			for i, state := range tc.runStates {
				agentID := fmt.Sprintf("agent-%d", i+1)
				if tc.linkAgent && attemptID != "" {
					if err := trailconfig.RecordAttemptAgent(root, traceID, attemptID, agentID); err != nil {
						t.Fatal(err)
					}
				}
				writeRunStatus(t, root, traceID, agentID, attemptID, state)
			}
			if tc.noStatus {
				if tc.linkAgent && attemptID != "" {
					if err := trailconfig.RecordAttemptAgent(root, traceID, attemptID, "agent-1"); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.MkdirAll(filepath.Join(root, ".paseka", "runs", traceID, "agent-1"), 0o755); err != nil {
					t.Fatal(err)
				}
			}

			blockers, err := trailconfig.Blockers(root, traceID)
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(blockers, "\n")
			if tc.wantNone {
				if len(blockers) != 0 {
					t.Fatalf("blockers = %v, want none", blockers)
				}
				return
			}
			for _, want := range tc.wantSubstrs {
				want = strings.ReplaceAll(want, "%attempt%", attemptID)
				if !strings.Contains(joined, want) {
					t.Fatalf("blockers = %v, want substring %q", blockers, want)
				}
			}
		})
	}
}

func writeRunStatus(t *testing.T, root, traceID, agentID, attemptID string, state protocol.RunStatus) {
	t.Helper()
	dir := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: agentID}
	if err := os.MkdirAll(dir.Root(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := dir.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           state,
		Provenance:      protocol.Provenance{AttemptID: attemptID},
	}); err != nil {
		t.Fatal(err)
	}
}
