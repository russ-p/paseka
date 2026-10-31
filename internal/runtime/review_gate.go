package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/review"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
)

func (r *Reactor) handleHumanFeedback(ctx context.Context, ev protocol.Event, execCtx *trailExecutionContext) error {
	if ev.Type != protocol.EventInsight || protocol.PayloadKind(ev.Payload) != string(protocol.InsightHumanFeedback) {
		return nil
	}
	var payload protocol.HumanFeedbackPayload
	if err := unmarshalPayload(ev.Payload, &payload); err != nil {
		return nil
	}
	if payload.TaskID == "" {
		return nil
	}
	snap, err := r.ledger.Snapshot(ev.TraceID)
	if err != nil {
		return err
	}
	task, ok := snap.Tasks[payload.TaskID]
	if !ok {
		return nil
	}
	if task.Status != protocol.TaskStatusWaitingReview {
		return nil
	}
	if protocol.NormalizeTaskReviewPolicy(task.Review) != protocol.TaskReviewRequired {
		return nil
	}
	if err := r.setTaskStatus(ctx, ev.TraceID, payload.TaskID, protocol.TaskStatusReady, payload.Message); err != nil {
		return err
	}
	snap, err = r.ledger.Snapshot(ev.TraceID)
	if err != nil {
		return err
	}
	task = snap.Tasks[payload.TaskID]
	if execCtx == nil {
		resolved, err := r.executionContext(ev.TraceID)
		if err != nil {
			return err
		}
		execCtx = resolved
	}
	return r.dispatchReady(ctx, ev.TraceID, task, execCtx, eventIdentity(ev))
}

func (r *Reactor) handleReviewSideEffects(ctx context.Context, ev protocol.Event, execCtx *trailExecutionContext) error {
	return r.handleHumanFeedback(ctx, ev, execCtx)
}

func (r *Reactor) handleVerificationSuccess(ctx context.Context, ev protocol.Event, execCtx *trailExecutionContext) error {
	if ev.Type != protocol.EventVerification || protocol.PayloadKind(ev.Payload) != string(protocol.VerificationSuccess) {
		return nil
	}
	var payload protocol.VerificationPayload
	if err := unmarshalPayload(ev.Payload, &payload); err != nil || payload.TaskID == "" {
		return nil
	}
	snap, err := r.ledger.Snapshot(ev.TraceID)
	if err != nil {
		return err
	}
	task, ok := snap.Tasks[payload.TaskID]
	if !ok {
		return nil
	}
	if task.Status != protocol.TaskStatusRunning {
		return nil
	}
	if protocol.NormalizeTaskReviewPolicy(task.Review) != protocol.TaskReviewRequired {
		return nil
	}
	if execCtx == nil {
		resolved, err := r.executionContext(ev.TraceID)
		if err != nil {
			return err
		}
		execCtx = resolved
	}
	routed, found, err := r.recordedAttemptContext(ev, execCtx)
	if err != nil {
		return err
	}
	if found {
		execCtx = routed
	}
	if !r.taskExpectsRootProposal(task, execCtx) {
		return nil
	}
	summary := strings.TrimSpace(payload.Summary)
	return r.setTaskStatus(ctx, ev.TraceID, payload.TaskID, protocol.TaskStatusWaitingReview, summary)
}

func (r *Reactor) taskExpectsRootProposal(task taskledger.TaskSnapshot, execCtx *trailExecutionContext) bool {
	switch task.ProposalWorkspace {
	case protocol.ProposalWorkspaceRoot:
		return true
	case protocol.ProposalWorkspaceIsolated:
		return false
	}
	bee, ok := execCtx.Registry.Bee(colony.EffectiveTaskBee(task.Bee, execCtx.Manifest.Defaults))
	if !ok {
		return false
	}
	ws, ok := bee.ExpectedProposalWorkspace()
	return ok && ws == protocol.ProposalWorkspaceRoot
}

func (r *Reactor) validateIncomingTaskPlan(ev protocol.Event, execCtx *trailExecutionContext) error {
	if ev.Type != protocol.EventInsight || protocol.PayloadKind(ev.Payload) != string(protocol.TaskEventPlan) {
		return nil
	}
	var payload protocol.TaskPlanPayload
	if err := unmarshalPayload(ev.Payload, &payload); err != nil {
		return nil
	}
	for _, spec := range payload.Tasks {
		if err := colony.ValidateTaskReviewPolicy(spec, execCtx.Registry.Bees(), execCtx.Manifest.Defaults); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reactor) setTaskStatus(ctx context.Context, traceID, taskID string, status protocol.TaskStatus, summary string) error {
	ev, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind:    protocol.TaskEventStatus,
		TaskID:  taskID,
		Status:  status,
		Summary: summary,
	})
	if err != nil {
		return err
	}
	if err := r.applyAndSync(ctx, ev); err != nil {
		return err
	}
	if terminalTaskStatus(status) {
		if err := r.closeLatestTaskAttempt(traceID, taskID, string(status)); err != nil {
			runtimeLog.Warn("close task attempt failed", logging.F("error", err.Error()))
		}
	}
	return nil
}

func terminalTaskStatus(status protocol.TaskStatus) bool {
	switch status {
	case protocol.TaskStatusCompleted, protocol.TaskStatusFailed, protocol.TaskStatusCancelled:
		return true
	default:
		return false
	}
}

// completeTask finishes a task and hands the trail's execution context to the
// gate boundary, so the boundary decides with the configuration this dispatch
// already resolved instead of re-reading it.
func (r *Reactor) completeTask(ctx context.Context, traceID, taskID, summary, commit string, execCtx *trailExecutionContext) error {
	completed, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind:        protocol.TaskEventCompleted,
		TaskID:      taskID,
		Status:      protocol.TaskStatusCompleted,
		Summary:     summary,
		Commit:      commit,
		CompletedAt: time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	return r.applyTaskCompletedEvent(ctx, traceID, completed, execCtx)
}

func (r *Reactor) applyTaskCompletedEvent(ctx context.Context, traceID string, completed protocol.Event, execCtx *trailExecutionContext) error {
	r.stampRuntimeProvenance(&completed)
	stale, err := r.staleTaskCompletion(completed)
	if err != nil {
		return err
	}
	if stale {
		return nil
	}
	res, err := r.ledger.Apply(completed)
	if err != nil {
		return err
	}
	if res.Changed {
		r.syncTaskProjection(res.Trace)
	}
	if taskID := protocol.PayloadTaskID(completed.Payload); taskID != "" {
		if err := r.closeCompletedTaskAttempt(traceID, taskID, completed.AttemptID); err != nil {
			runtimeLog.Warn("close task attempt failed", logging.F("error", err.Error()))
		}
	}
	// Remember before publish so the JetStream echo cannot re-apply.
	r.rememberLocalEvent(completed)
	if bus.PublisherAvailable(r.publisher) {
		if err := r.publisher.PublishEvent(ctx, completed); err != nil {
			return err
		}
	}
	for _, t := range res.Ready {
		if err := r.dispatchReady(ctx, traceID, t, nil, eventIdentity(completed)); err != nil {
			return err
		}
	}
	return r.maybeActivateFinalReview(ctx, traceID, execCtx)
}

// maybeActivateFinalReview is the single gate boundary: it ensures the closing
// round first and then the one merge task. The downstream dispatch of a seeded
// closing round goes through the ordinary AFK scheduler.
func (r *Reactor) maybeActivateFinalReview(ctx context.Context, traceID string, execCtx *trailExecutionContext) error {
	if traceID == "" {
		return nil
	}
	if execCtx == nil {
		// The pinned closing and merge policies decide what this boundary may
		// do, so an unreadable binding is an error, not a default: falling back
		// here would silently disable a wrap-up the trail pinned at start and
		// open the gate over work nobody reviewed.
		resolved, err := r.executionContext(traceID)
		if err != nil {
			return fmt.Errorf("runtime: trail config: %w", err)
		}
		execCtx = resolved
	}
	policies := review.GatePolicies{
		Closing: execCtx.Manifest.Closing,
		Merge:   execCtx.Manifest.Merge,
	}
	res, err := review.EnsureTrailPhases(ctx, r.publisher, r.ledger, r.colony, traceID, policies, review.WriteOptions{
		AfterApply: r.rememberLocalEvent,
	})
	if err != nil {
		return err
	}
	if res.ClosingSeeded != nil {
		if err := r.dispatchReady(ctx, traceID, *res.ClosingSeeded, execCtx, "closing-seed"); err != nil {
			return err
		}
	}
	snap, err := r.ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	r.syncTaskProjection(snap)
	return nil
}

// applyAndSync applies locally first, then publishes. Publish-before-apply would
// leave a bad stream event when CAS/apply fails, and the reactor's own
// subscription would double-apply non-idempotent events (e.g. energy.consume).
// staleTaskCompletion drops a terminal task event owned by an older attempt once
// a newer attempt is open. A replay must not overwrite the work in progress.
func (r *Reactor) staleTaskCompletion(ev protocol.Event) (bool, error) {
	if ev.Type != protocol.EventVerification || protocol.PayloadKind(ev.Payload) != string(protocol.TaskEventCompleted) {
		return false, nil
	}
	taskID := protocol.PayloadTaskID(ev.Payload)
	if ev.AttemptID == "" || taskID == "" {
		return false, nil
	}
	latest, ok, err := trailconfig.LatestTaskAttempt(r.colony.ColonyRoot, ev.TraceID, taskID)
	if err != nil {
		return false, err
	}
	if ok && latest.Open() && latest.AttemptID != ev.AttemptID {
		logDispatchSkip("stale attempt completion", ev.TraceID, taskID, "")
		return true, nil
	}
	return false, nil
}

func (r *Reactor) closeLatestTaskAttempt(traceID, taskID, reason string) error {
	latest, ok, err := trailconfig.LatestTaskAttempt(r.colony.ColonyRoot, traceID, taskID)
	if err != nil || !ok || !latest.Open() {
		return err
	}
	return trailconfig.CloseAttempt(r.colony.ColonyRoot, traceID, latest.AttemptID, reason)
}

func (r *Reactor) closeCompletedTaskAttempt(traceID, taskID, attemptID string) error {
	if attemptID == "" {
		return r.closeLatestTaskAttempt(traceID, taskID, "completed")
	}
	attempt, ok, err := trailconfig.FindAttempt(r.colony.ColonyRoot, traceID, attemptID)
	if err != nil || !ok || !attempt.Open() {
		return err
	}
	return trailconfig.CloseAttempt(r.colony.ColonyRoot, traceID, attemptID, "completed")
}

func (r *Reactor) applyAndSync(ctx context.Context, ev protocol.Event) error {
	r.stampRuntimeProvenance(&ev)
	stale, err := r.staleTaskCompletion(ev)
	if err != nil {
		return err
	}
	if stale {
		return nil
	}
	res, err := r.ledger.Apply(ev)
	if err != nil {
		return err
	}
	if res.Changed {
		r.syncTaskProjection(res.Trace)
	}
	if taskID := protocol.PayloadTaskID(ev.Payload); taskID != "" {
		if task, ok := res.Trace.Tasks[taskID]; ok && terminalTaskStatus(task.Status) {
			if err := r.closeCompletedTaskAttempt(ev.TraceID, taskID, ev.AttemptID); err != nil {
				runtimeLog.Warn("close task attempt failed", logging.F("error", err.Error()))
			}
		}
	}
	r.rememberLocalEvent(ev)
	if bus.PublisherAvailable(r.publisher) {
		if err := r.publisher.PublishEvent(ctx, ev); err != nil {
			return err
		}
	}
	return nil
}

func unmarshalPayload(raw []byte, out any) error {
	if len(raw) == 0 {
		return fmt.Errorf("empty payload")
	}
	return json.Unmarshal(raw, out)
}
