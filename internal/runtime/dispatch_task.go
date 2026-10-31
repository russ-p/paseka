package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/review"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
)

func (r *Reactor) dispatchReady(ctx context.Context, traceID string, task taskledger.TaskSnapshot, execCtx *trailExecutionContext, sourceEventID string) error {
	defer func() {
		if _, err := r.tryActivateConfig(context.Background(), traceID, true); err != nil {
			runtimeLog.Warn("activate pending configuration failed", logging.F("error", err.Error()))
		}
	}()
	if execCtx == nil {
		resolved, err := r.executionContext(traceID)
		if err != nil {
			return fmt.Errorf("runtime: trail config: %w", err)
		}
		execCtx = resolved
	}
	bee := colony.EffectiveTaskBee(task.Bee, execCtx.Manifest.Defaults)
	key := traceID + ":" + task.TaskID
	r.mu.Lock()
	if _, ok := r.inflight[key]; ok {
		r.mu.Unlock()
		logDispatchSkip("already running", traceID, task.TaskID, bee)
		return nil
	}
	r.mu.Unlock()

	if r.traceKilled(traceID) {
		logDispatchSkip("trace killed", traceID, task.TaskID, bee)
		return nil
	}

	dispatchCtx, endInflight := r.beginTaskInflight(traceID, task.TaskID)
	defer endInflight()

	snap, err := r.ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	taskSnap := snap.Tasks[task.TaskID]
	if taskSnap.TaskID != "" {
		task = taskSnap
	}
	if taskledger.ShouldSkipDispatch(task) {
		logDispatchSkip("final review gate — no AFK dispatch", traceID, task.TaskID, bee)
		return r.setTaskStatus(dispatchCtx, traceID, task.TaskID, protocol.TaskStatusWaitingReview, "Trace ready for human review and merge")
	}

	if held, err := r.taskAdmissionHeld(traceID, task.TaskID); err != nil {
		return err
	} else if held {
		logDispatchSkip("pending configuration change", traceID, task.TaskID, bee)
		return nil
	}

	ok, err := r.gateTaskDispatchEnergy(dispatchCtx, traceID, task.TaskID, "task.dispatch")
	if err != nil {
		return err
	}
	if !ok {
		logDispatchSkip("honey reserve exhausted", traceID, task.TaskID, bee)
		return nil
	}

	attempt, attemptCtx, skip, err := r.prepareTaskAttempt(execCtx, traceID, task, sourceEventID)
	if err != nil {
		return err
	}
	if skip {
		logDispatchSkip("duplicate or closed attempt", traceID, task.TaskID, bee)
		return nil
	}
	execCtx = attemptCtx
	bee = colony.EffectiveTaskBee(task.Bee, execCtx.Manifest.Defaults)

	if err := r.setTaskStatus(dispatchCtx, traceID, task.TaskID, protocol.TaskStatusRunning, ""); err != nil {
		return err
	}

	// A guaranteed closing round owns pr.body, so the last work task stops
	// carrying the must-emit guidance. Without a guarantee the existing
	// fallback stays: a small trail still produces a description.
	isLastWorkTask := taskledger.IsLastWorkTask(snap, task.TaskID)
	suppressPRBody := isLastWorkTask && review.ClosingRoundGuaranteed(execCtx.Manifest.Closing, snap)

	// The closing round reviews the whole trail, so it receives the same
	// structured inventory gate copy renders with. Re-snapshot so its own row
	// reads running instead of ready.
	inventory := colony.TrailInventory{}
	var policyParams map[string]any
	if taskledger.IsClosingTask(task) {
		latest := snap
		if refreshed, snapErr := r.ledger.Snapshot(traceID); snapErr == nil {
			latest = refreshed
		}
		inventory = review.BuildTrailInventory(execCtx.Colony, traceID, latest)
		// The pinned policy's params are what make the wrap-up a stronger
		// reviewer than the bees that built the trail.
		policyParams = execCtx.Manifest.Closing.Task.Params
	}

	body := task.Body
	if body == "" {
		body = task.Title
	}
	if body == "" {
		body = fmt.Sprintf("Execute task %s", task.TaskID)
	}

	startedAt := time.Now().UTC()
	res, err := r.dispatcher.DispatchColonyBee(dispatchCtx, execCtx.Colony, ColonyDispatchRequest{
		Bee:            bee,
		TraceID:        traceID,
		TaskID:         task.TaskID,
		Task:           body,
		Sector:         task.Sector,
		Intent:         task.Intent,
		IsLastWorkTask: isLastWorkTask,
		SuppressPRBody: suppressPRBody,
		TrailInventory: inventory,
		PolicyParams:   policyParams,
		Provenance:     attemptProvenance(attempt),
	}, DispatchModeTask)
	if err != nil {
		// Kill may cancel finalize after a completed adapter run (busRequired); do not
		// publish failed over cancelled (mirrors the non-completed / success gates below).
		if r.traceKilled(traceID) {
			return nil
		}
		if setErr := r.setTaskStatus(dispatchCtx, traceID, task.TaskID, protocol.TaskStatusFailed, taskDispatchFailureSummary(nil, err)); setErr != nil {
			return setErr
		}
		return nil
	}
	if err := trailconfig.RecordAttemptAgent(r.colony.ColonyRoot, traceID, attempt.AttemptID, res.AgentID); err != nil {
		runtimeLog.Warn("attempt agent record failed", logging.F("error", err.Error()))
	}
	r.recordTaskRunStart(traceID, task.TaskID, bee, res, startedAt, attempt)
	if res.Result == nil || res.Result.Status != string(protocol.StatusCompleted) {
		if res.Result != nil {
			r.recordTaskRunFinish(traceID, task.TaskID, res.AgentID, res.Result.Status, time.Now().UTC())
		}
		status := "unknown"
		if res.Result != nil {
			status = res.Result.Status
		}
		logDispatchDone(DispatchModeTask, bee, traceID, task.TaskID, res.AgentID, status)
		if r.traceKilled(traceID) {
			return nil
		}
		if setErr := r.setTaskStatus(dispatchCtx, traceID, task.TaskID, protocol.TaskStatusFailed, taskDispatchFailureSummary(res, nil)); setErr != nil {
			return setErr
		}
		return nil
	}
	logDispatchDone(DispatchModeTask, bee, traceID, task.TaskID, res.AgentID, string(protocol.StatusCompleted))
	finishedAt := time.Now().UTC()
	r.recordTaskRunFinish(traceID, task.TaskID, res.AgentID, string(protocol.StatusCompleted), finishedAt)

	// Kill may land after the adapter returns completed but before finalize; do not
	// overwrite cancelled (mirrors the failure-path gate above).
	if r.traceKilled(traceID) {
		return nil
	}

	summary := strings.TrimSpace(res.Result.Summary)
	if protocol.NormalizeTaskReviewPolicy(task.Review) == protocol.TaskReviewRequired {
		if runOpenedRootProposal(execCtx.Registry, bee, res.Result) {
			return nil
		}
		return r.setTaskStatus(dispatchCtx, traceID, task.TaskID, protocol.TaskStatusWaitingReview, summary)
	}
	if ev, ok := runEmittedTaskCompleted(res.Result, task.TaskID); ok {
		return r.applyTaskCompletedEvent(dispatchCtx, traceID, ev, execCtx)
	}
	if shouldDeferAFKCompletion(execCtx.Registry, bee, res.Result) {
		afkSummary := summary
		if afkSummary == "" {
			afkSummary = "Awaiting AFK verification gate"
		}
		return r.setTaskStatus(dispatchCtx, traceID, task.TaskID, protocol.TaskStatusWaitingReview, afkSummary)
	}
	return r.completeTask(dispatchCtx, traceID, task.TaskID, summary, "", execCtx)
}

func taskDispatchFailureSummary(res *BeeRunResult, dispatchErr error) string {
	if dispatchErr != nil {
		return fmt.Sprintf("dispatch error: %v", dispatchErr)
	}
	if res != nil && res.Result != nil {
		result := res.Result
		if result.Err != nil {
			return fmt.Sprintf("adapter %s: %v", result.Status, result.Err)
		}
		if strings.TrimSpace(result.Summary) != "" {
			return result.Status + ": " + strings.TrimSpace(result.Summary)
		}
		if result.Status != "" {
			return "adapter " + result.Status
		}
	}
	return "adapter failed"
}

func (r *Reactor) recordTaskRunStart(traceID, taskID, bee string, res *BeeRunResult, startedAt time.Time, attempt trailconfig.Attempt) {
	if res == nil || taskID == "" {
		return
	}
	if err := runs.AppendTaskRun(r.colony.ColonyRoot, traceID, taskID, runs.TaskRunEntry{
		AgentID:        res.AgentID,
		Bee:            bee,
		RunDir:         res.RunDir,
		StartedAt:      startedAt,
		RunStatus:      "running",
		AttemptID:      attempt.AttemptID,
		ConfigRef:      attempt.ConfigRef,
		ConfigRevision: attempt.ConfigRevision,
	}); err != nil {
		runtimeLog.Warn("task run projection failed", logging.F("error", err.Error()))
	}
}

func (r *Reactor) recordTaskRunFinish(traceID, taskID, agentID, runStatus string, finishedAt time.Time) {
	if taskID == "" || agentID == "" {
		return
	}
	if err := runs.UpdateTaskRunStatus(r.colony.ColonyRoot, traceID, taskID, agentID, runStatus, finishedAt); err != nil {
		runtimeLog.Warn("task run projection failed", logging.F("error", err.Error()))
	}
}
