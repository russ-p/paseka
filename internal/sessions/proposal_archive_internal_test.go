package sessions

import (
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

// A session that already emitted or queued a code proposal must not be synthesized again:
// the explicit proposal is passed through, and a Guard is never dispatched twice for one
// proposal. Both the run's own audit log and its pending queue are consulted.
func TestSessionEmittedCodeProposalReadsEventsAndPending(t *testing.T) {
	root := t.TempDir()
	runDir := runs.Dir{ColonyRoot: root, TraceID: "trace-1", AgentID: "agent-1"}
	if err := runDir.Prepare(); err != nil {
		t.Fatal(err)
	}

	if sessionEmittedCodeProposal(runDir) {
		t.Fatal("an empty run has emitted nothing")
	}

	insight, err := protocol.NewEvent("trace-1", "agent-1", 0, protocol.EventInsight, map[string]any{"kind": "task.plan"})
	if err != nil {
		t.Fatal(err)
	}
	if err := runDir.AppendEvent(insight); err != nil {
		t.Fatal(err)
	}
	if sessionEmittedCodeProposal(runDir) {
		t.Fatal("a non-proposal event must not suppress synthesis")
	}

	pending, err := protocol.NewEvent("trace-1", "agent-1", 0, protocol.EventMutation, protocol.MutationPayload{
		Kind: protocol.MutationCodeProposalIsolated,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runDir.AppendPending(pending); err != nil {
		t.Fatal(err)
	}
	if !sessionEmittedCodeProposal(runDir) {
		t.Fatal("a queued code proposal must suppress synthesis")
	}
}
