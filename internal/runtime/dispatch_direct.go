package runtime

import (
	"context"
	"strings"

	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/trailconfig"
)

// dispatchDirect handles an ordinary direct reaction. Ordinary work defaults to
// purpose work; an explicitly attributed analysis reaction enters through
// dispatchDirectWithPurpose and keeps its studied milestone.
func (r *Reactor) dispatchDirect(ctx context.Context, ev protocol.Event, beeRole string, execCtx *trailExecutionContext) error {
	return r.dispatchDirectWithPurpose(ctx, ev, beeRole, execCtx, protocol.PurposeWork, "")
}

func (r *Reactor) dispatchDirectWithPurpose(ctx context.Context, ev protocol.Event, beeRole string, execCtx *trailExecutionContext, purpose protocol.ExecutionPurpose, analysisOf string) error {
	defer func() {
		if _, err := r.tryActivateConfig(context.Background(), ev.TraceID, true); err != nil {
			runtimeLog.Warn("activate pending configuration failed", logging.F("error", err.Error()))
		}
	}()
	if execCtx == nil {
		resolved, err := r.executionContext(ev.TraceID)
		if err != nil {
			return err
		}
		execCtx = resolved
	}
	if r.traceKilled(ev.TraceID) {
		logDispatchSkip("trace killed", ev.TraceID, "", beeRole)
		return nil
	}

	taskID, taskBody, err := eventDispatchContext(ev)
	if err != nil {
		runtimeLog.Warn("direct dispatch skipped",
			logging.F("bee", beeRole),
			logging.F("error", err.Error()),
		)
		return nil
	}

	if publisherBee := r.publisherBee(ev); publisherBee != "" && publisherBee == beeRole {
		logDispatchSkip("publisher is same bee role", ev.TraceID, taskID, beeRole)
		return nil
	}

	key := directDispatchKey(ev, beeRole)
	r.mu.Lock()
	if _, ok := r.directProcessed[key]; ok {
		r.mu.Unlock()
		logDispatchSkip("already processed", ev.TraceID, taskID, beeRole)
		return nil
	}
	if _, ok := r.directInflight[key]; ok {
		r.mu.Unlock()
		logDispatchSkip("already running", ev.TraceID, taskID, beeRole)
		return nil
	}
	r.mu.Unlock()

	dispatchCtx, endInflight := r.beginDirectInflight(key)
	r.noteDirectPurpose(key, purpose)
	defer func() {
		endInflight()
		r.clearDirectPurpose(key)
		// A direct work reaction is an invocation too: the milestone may only
		// be certified once it has left the in-flight set with its result and
		// usage persisted. Analysis is recorded but never holds completion.
		if purpose != protocol.PurposeAnalysis {
			if err := r.maybeCompleteWork(context.Background(), ev.TraceID); err != nil {
				runtimeLog.Warn("work completion check failed",
					logging.F("trace", ev.TraceID),
					logging.F("error", err.Error()),
				)
			}
		}
	}()

	if held, err := r.directAdmissionHeld(ev, beeRole, taskID); err != nil {
		return err
	} else if held {
		if err := r.deferDirectDispatch(dispatchCtx, ev, beeRole, taskID, protocol.ConfigBarrierHoldReason, purpose, analysisOf); err != nil {
			return err
		}
		r.markBarrierHeldDeferral(ev.TraceID)
		logDispatchSkip("pending configuration change — direct dispatch held", ev.TraceID, taskID, beeRole)
		return nil
	}

	ok, err := r.gateDirectDispatchEnergy(dispatchCtx, ev.TraceID, taskID, "direct.dispatch", purpose, analysisOf)
	if err != nil {
		return err
	}
	if !ok {
		if err := r.deferDirectDispatch(dispatchCtx, ev, beeRole, taskID, protocol.HoneyReserveExhaustedSummary, purpose, analysisOf); err != nil {
			return err
		}
		logDispatchSkip("honey reserve exhausted — direct dispatch deferred", ev.TraceID, taskID, beeRole)
		return nil
	}

	attempt, attemptCtx, skip, err := r.prepareDirectAttempt(ev, beeRole, execCtx)
	if err != nil {
		return err
	}
	if skip {
		logDispatchSkip("duplicate or closed attempt", ev.TraceID, taskID, beeRole)
		return nil
	}
	execCtx = attemptCtx

	proposalSector, proposalKind := proposalDispatchFields(ev)

	res, err := r.dispatcher.DispatchColonyBee(dispatchCtx, execCtx.Colony, ColonyDispatchRequest{
		Bee:          beeRole,
		TraceID:      ev.TraceID,
		TaskID:       taskID,
		Task:         taskBody,
		Sector:       proposalSector,
		ProposalKind: proposalKind,
		Provenance:   attemptProvenance(attempt),
		Purpose:      purpose,
		AnalysisOf:   analysisOf,
	}, DispatchModeDirect)
	if err != nil {
		return err
	}
	if res != nil {
		if recErr := trailconfig.RecordAttemptAgent(r.colony.ColonyRoot, ev.TraceID, attempt.AttemptID, res.AgentID); recErr != nil {
			runtimeLog.Warn("attempt agent record failed", logging.F("error", recErr.Error()))
		}
	}
	r.mu.Lock()
	r.directProcessed[key] = struct{}{}
	r.mu.Unlock()
	status := "unknown"
	if res.Result != nil {
		status = res.Result.Status
	}
	logDispatchDone(DispatchModeDirect, beeRole, ev.TraceID, taskID, res.AgentID, status)
	// A gate rejection on a rework pass follows the same rule as any other
	// non-completed run: the task keeps its status and the next rejection event
	// drives the next repair turn. Only an exhausted rejection cap fails the
	// task, with the gate's one-line verdict and no further dispatch.
	if gate := failedCheckGate(res.Result); gate != nil && res.Result.GateReworkSuppressed && taskID != "" {
		if r.traceKilled(ev.TraceID) {
			return nil
		}
		if err := r.setTaskStatus(dispatchCtx, ev.TraceID, taskID, protocol.TaskStatusFailed, gate.Summary); err != nil {
			return err
		}
	}
	return nil
}

// publisherBee resolves the bee role that emitted an event from its run metadata.
func (r *Reactor) publisherBee(ev protocol.Event) string {
	if r.colony.ColonyRoot == "" || ev.TraceID == "" || ev.AgentID == "" {
		return ""
	}
	meta, ok, err := runs.FindRun(r.colony.ColonyRoot, ev.TraceID, ev.AgentID)
	if err != nil || !ok {
		return ""
	}
	return strings.TrimSpace(meta.Bee)
}
