package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/review"
	"github.com/paseka/paseka/internal/taskledger"
)

func (r *Reactor) handleHumanFeedback(ctx context.Context, ev protocol.Event) error {
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
	return r.dispatchReady(ctx, ev.TraceID, task)
}

func (r *Reactor) handleReviewSideEffects(ctx context.Context, ev protocol.Event) error {
	return r.handleHumanFeedback(ctx, ev)
}

func (r *Reactor) handleVerificationSuccess(ctx context.Context, ev protocol.Event) error {
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
	if !r.taskExpectsRootProposal(task) {
		return nil
	}
	summary := strings.TrimSpace(payload.Summary)
	return r.setTaskStatus(ctx, ev.TraceID, payload.TaskID, protocol.TaskStatusWaitingReview, summary)
}

func (r *Reactor) taskExpectsRootProposal(task taskledger.TaskSnapshot) bool {
	switch task.ProposalWorkspace {
	case protocol.ProposalWorkspaceRoot:
		return true
	case protocol.ProposalWorkspaceIsolated:
		return false
	}
	beeName := task.Bee
	if beeName == "" {
		beeName = "builder"
	}
	bee, ok := r.registry.Bee(beeName)
	if !ok {
		return false
	}
	ws, ok := bee.ExpectedProposalWorkspace()
	return ok && ws == protocol.ProposalWorkspaceRoot
}

func (r *Reactor) validateIncomingTaskPlan(ev protocol.Event) error {
	if ev.Type != protocol.EventInsight || protocol.PayloadKind(ev.Payload) != string(protocol.TaskEventPlan) {
		return nil
	}
	var payload protocol.TaskPlanPayload
	if err := unmarshalPayload(ev.Payload, &payload); err != nil {
		return nil
	}
	for _, spec := range payload.Tasks {
		if err := colony.ValidateTaskReviewPolicy(spec, r.registry.Bees()); err != nil {
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
	return r.applyAndSync(ctx, ev)
}

func (r *Reactor) completeTask(ctx context.Context, traceID, taskID, summary, commit string) error {
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
	return r.applyTaskCompletedEvent(ctx, traceID, completed)
}

func (r *Reactor) applyTaskCompletedEvent(ctx context.Context, traceID string, completed protocol.Event) error {
	res, err := r.ledger.Apply(completed)
	if err != nil {
		return err
	}
	if res.Changed {
		r.syncTaskProjection(res.Trace)
	}
	// Remember before publish so the JetStream echo cannot re-apply.
	r.rememberLocalEvent(completed)
	if r.bus != nil {
		if err := r.bus.PublishEvent(ctx, completed); err != nil {
			return err
		}
	}
	for _, t := range res.Ready {
		if err := r.dispatchReady(ctx, traceID, t); err != nil {
			return err
		}
	}
	return r.maybeActivateFinalReview(ctx, traceID)
}

func (r *Reactor) maybeActivateFinalReview(ctx context.Context, traceID string) error {
	if err := review.ActivateFinalReviewGate(ctx, r.bus, r.ledger, traceID); err != nil {
		return err
	}
	if traceID == "" {
		return nil
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
func (r *Reactor) applyAndSync(ctx context.Context, ev protocol.Event) error {
	res, err := r.ledger.Apply(ev)
	if err != nil {
		return err
	}
	if res.Changed {
		r.syncTaskProjection(res.Trace)
	}
	r.rememberLocalEvent(ev)
	if r.bus != nil {
		if err := r.bus.PublishEvent(ctx, ev); err != nil {
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
