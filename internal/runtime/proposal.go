package runtime

import (
	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/patches"
	"github.com/russ-p/paseka/internal/proposal"
	"github.com/russ-p/paseka/internal/protocol"
)

// proposalArchiveRequestFromRun builds the AFK request for one finished run.
//
// Completed comes from the final status rather than a nil adapter error: a cancelled run,
// a timed-out run, and a run the completion contract downgraded all reach this point having
// executed partly, and none of them is a completed proposal attempt.
func (d *Dispatcher) proposalArchiveRequestFromRun(
	req DispatchRequest,
	bee colony.Bee,
	baseline adapters.WorkspaceBaseline,
	result *adapters.RunResult,
	explicit bool,
) proposal.ArchiveRequest {
	return proposal.ArchiveRequest{
		ColonyRoot:  req.ColonyRoot,
		TraceID:     req.TraceID,
		AgentID:     req.AgentID,
		Bee:         bee,
		TaskID:      req.TaskID,
		Sector:      req.Sector,
		Workspace:   req.Workspace,
		Mode:        patches.ModeAFK,
		Baseline:    baseline,
		Summary:     result.Summary,
		AutoPublish: shouldAutoPublishMutation(req.Registry, bee),
		Completed:   result.Status == string(protocol.StatusCompleted),
		Explicit:    explicit,
	}
}

// runOpenedCodeProposalEvent reports that a run already published a code proposal itself,
// whether it was emitted live or queued for the finalization flush. Synthesis is suppressed
// so one proposal never becomes two, and a Guard is never dispatched for it twice.
func runOpenedCodeProposalEvent(events []protocol.Event) bool {
	for _, ev := range events {
		if ev.Type != protocol.EventMutation {
			continue
		}
		if protocol.IsCodeProposalKind(protocol.PayloadKind(ev.Payload)) {
			return true
		}
	}
	return false
}

// logAutoProposalSkip records why an eligible-looking run produced no proposal record.
func logAutoProposalSkip(bee colony.Bee, reason string) {
	if reason == "" {
		return
	}
	runtimeLog.Warn("auto mutation skipped", logging.F("bee", bee.Role), logging.F("reason", reason))
}
