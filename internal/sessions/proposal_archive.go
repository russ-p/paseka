package sessions

import (
	"context"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/patches"
	"github.com/russ-p/paseka/internal/proposal"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

// finalizeSessionProposal archives a code proposal from a finished interactive session.
//
// It runs after the session hooks, so the captured diff describes the workspace the hooks
// left behind, and it uses the same gate, collector, writer, and publisher as an AFK run
// rather than routing through the AFK dispatcher. A session that failed or was cancelled
// never reaches the preparation gate, so partial interactive work is not presented as a
// completed proposal attempt. Resume mints a fresh run id and captures a fresh baseline, so
// continuing a session leaves the earlier session's record intact.
//
// Diagnostics land in the run's status note, which is the surface a Beekeeper already reads
// for a session's outcome. A successful agent process is not described as a successfully
// published proposal when finalization failed.
func (m *Manager) finalizeSessionProposal(active *activeSession, state adapters.SessionState, pub bus.Publisher) {
	if state != adapters.SessionCompleted {
		return
	}
	entry := active.entry
	res, err := proposal.Archive(context.Background(), pub, proposal.ArchiveRequest{
		Provenance:  active.proposal.provenance,
		ColonyRoot:  entry.Handle.ColonyRoot,
		TraceID:     entry.Handle.TraceID,
		AgentID:     entry.Handle.AgentID,
		Bee:         active.proposal.bee,
		TaskID:      active.proposal.taskID,
		Workspace:   entry.Handle.Workspace,
		Mode:        patches.ModeHITL,
		Baseline:    active.proposal.baseline,
		Summary:     active.proposal.task,
		AutoPublish: proposal.ShouldAutoPublish(active.proposal.bee),
		Completed:   true,
		Explicit:    sessionEmittedCodeProposal(entry.RunDir),
	})
	switch {
	case err != nil:
		msg := "runtime: proposal finalization: " + err.Error()
		logging.Component("sessions").Warn(msg,
			logging.F("bee", entry.Handle.Bee),
			logging.F("trace", entry.Handle.TraceID),
			logging.F("agent", entry.Handle.AgentID),
		)
		_ = entry.RunDir.AppendStatusNote(msg)
	case res.Misconfigured != "":
		logging.Component("sessions").Warn("auto mutation skipped",
			logging.F("bee", entry.Handle.Bee),
			logging.F("trace", entry.Handle.TraceID),
			logging.F("agent", entry.Handle.AgentID),
			logging.F("reason", res.Misconfigured),
		)
	}
}

// sessionEmittedCodeProposal reports whether the session already published a code proposal
// itself, live or queued. Such a run is passed through unchanged and gets no automatic patch
// conversion, so adding HITL finalization never dispatches a Guard twice for one proposal.
func sessionEmittedCodeProposal(runDir runs.Dir) bool {
	readers := []func() ([]protocol.Event, error){runDir.ReadEvents, runDir.ReadPending}
	for _, read := range readers {
		events, err := read()
		if err != nil {
			continue
		}
		for _, ev := range events {
			if ev.Type != protocol.EventMutation {
				continue
			}
			if protocol.IsCodeProposalKind(protocol.PayloadKind(ev.Payload)) {
				return true
			}
		}
	}
	return false
}
