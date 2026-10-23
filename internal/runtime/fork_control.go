package runtime

import (
	"context"
	"encoding/json"

	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/trailfork"
)

// recoverInterruptedForkCreations finishes approvals that a restart cut short.
//
// Only requests already recorded as creating are resumed: they were approved by
// an operator and their identities were minted before the crash. Creation is
// idempotent under the request identity, so recovery cannot create a second
// trail, reset honey, or duplicate the initial task plan.
func (r *Reactor) recoverInterruptedForkCreations(ctx context.Context) {
	if r.colony.ColonyRoot == "" || r.ledger == nil || !bus.PublisherAvailable(r.publisher) {
		return
	}
	recovered, err := trailfork.RecoverCreating(ctx, trailfork.DecisionOptions{
		Context:   r.colony,
		Actor:     "runtime",
		Reason:    "recovered interrupted creation after restart",
		Publisher: r.publisher,
		Ledger:    r.ledger,
	})
	if err != nil {
		runtimeLog.Warn("recover interrupted fork creation failed", logging.F("error", err.Error()))
	}
	for _, result := range recovered {
		runtimeLog.Info("recovered fork creation",
			logging.F("trace", result.Plan.TraceID),
			logging.F("request", result.Plan.RequestID),
		)
	}
}

// handleForkControl applies platform trail-fork signals before routing.
//
// An agent-originated trail.fork is eligible only when its sender resolves to a
// recorded bee run and the canonical colony policy grants that bee
// SIGNAL/trail.fork publication. The resulting pending request allocates no
// trail, snapshot, worktree, honey, or task plan.
//
// approve/reject/cancel are operator control-path actions. A sender that
// resolves to a recorded bee run is refused so an ordinary agent cannot admit
// its own request through the bus.
func (r *Reactor) handleForkControl(ctx context.Context, ev protocol.Event) (bool, error) {
	kind := protocol.ForkEventKind(protocol.PayloadKind(ev.Payload))
	switch kind {
	case protocol.SignalTrailFork:
		var payload protocol.ForkRequestPayload
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			runtimeLog.Warn("invalid trail.fork payload", logging.F("error", err.Error()))
			return true, nil
		}
		payload.Kind = protocol.SignalTrailFork
		req, err := trailfork.SubmitAgentRequest(trailfork.RequestOptions{
			Context: r.colony,
			Sender: trailfork.Sender{
				TraceID: ev.TraceID,
				AgentID: ev.AgentID,
			},
			Payload: payload,
		})
		if err != nil {
			runtimeLog.Warn("trail.fork refused",
				logging.F("traceId", ev.TraceID),
				logging.F("agentId", ev.AgentID),
				logging.F("requestId", payload.RequestID),
				logging.F("error", err.Error()))
			return true, nil
		}
		runtimeLog.Info("trail.fork pending",
			logging.F("requestId", req.RequestID),
			logging.F("sourceTrace", req.Evidence.SourceTrace))
		return true, nil

	case protocol.SignalTrailForkApprove, protocol.SignalTrailForkReject, protocol.SignalTrailForkCancel:
		var payload protocol.ForkDecisionPayload
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			runtimeLog.Warn("invalid trail.fork decision payload", logging.F("error", err.Error()))
			return true, nil
		}
		if _, recorded, err := runs.FindRun(r.colony.ColonyRoot, ev.TraceID, ev.AgentID); err != nil || recorded {
			runtimeLog.Warn("trail.fork decision from an agent run refused",
				logging.F("traceId", ev.TraceID),
				logging.F("agentId", ev.AgentID),
				logging.F("requestId", payload.RequestID))
			return true, nil
		}
		actor := ev.AgentID
		if actor == "" {
			actor = "operator"
		}
		switch kind {
		case protocol.SignalTrailForkApprove:
			_, err := trailfork.ApproveRequest(ctx, trailfork.DecisionOptions{
				Context:   r.colony,
				RequestID: payload.RequestID,
				Actor:     actor,
				Reason:    payload.Reason,
				Publisher: r.publisher,
				Ledger:    r.ledger,
			})
			if err != nil {
				runtimeLog.Warn("trail.fork approve failed",
					logging.F("requestId", payload.RequestID),
					logging.F("error", err.Error()))
			}
		case protocol.SignalTrailForkReject:
			if _, err := trailfork.RejectRequest(trailfork.DecisionOptions{
				Context:   r.colony,
				RequestID: payload.RequestID,
				Actor:     actor,
				Reason:    payload.Reason,
			}); err != nil {
				runtimeLog.Warn("trail.fork reject failed",
					logging.F("requestId", payload.RequestID),
					logging.F("error", err.Error()))
			}
		case protocol.SignalTrailForkCancel:
			if _, err := trailfork.CancelRequest(trailfork.DecisionOptions{
				Context:   r.colony,
				RequestID: payload.RequestID,
				Actor:     actor,
				Reason:    payload.Reason,
			}); err != nil {
				runtimeLog.Warn("trail.fork cancel failed",
					logging.F("requestId", payload.RequestID),
					logging.F("error", err.Error()))
			}
		}
		return true, nil
	}
	return false, nil
}
