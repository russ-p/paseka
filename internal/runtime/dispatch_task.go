package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/logging"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/taskledger"
)

func (r *Reactor) dispatchReady(ctx context.Context, traceID string, task taskledger.TaskSnapshot) error {
	key := traceID + ":" + task.TaskID
	r.mu.Lock()
	if _, ok := r.inflight[key]; ok {
		r.mu.Unlock()
		logDispatchSkip("already running", traceID, task.TaskID, taskBeeName(task))
		return nil
	}
	r.mu.Unlock()

	if r.traceKilled(traceID) {
		logDispatchSkip("trace killed", traceID, task.TaskID, taskBeeName(task))
		return nil
	}

	dispatchCtx, endInflight := r.beginTaskInflight(traceID, task.TaskID)
	defer endInflight()

	manifest, err := r.loadColonyManifest()
	if err != nil {
		return err
	}
	bee := colony.EffectiveTaskBee(task.Bee, manifest.Defaults)
	if !r.registry.CanDispatchTaskReady(bee) {
		logDispatchSkip("bee not subscribed to task.ready", traceID, task.TaskID, bee)
		return nil
	}

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

	ok, err := r.gateDispatchEnergy(dispatchCtx, traceID, task.TaskID, "task.dispatch")
	if err != nil {
		return err
	}
	if !ok {
		logDispatchSkip("honey reserve exhausted", traceID, task.TaskID, bee)
		return nil
	}

	if err := r.setTaskStatus(dispatchCtx, traceID, task.TaskID, protocol.TaskStatusRunning, ""); err != nil {
		return err
	}

	body := task.Body
	if body == "" {
		body = task.Title
	}
	if body == "" {
		body = fmt.Sprintf("Execute task %s", task.TaskID)
	}

	startedAt := time.Now().UTC()
	res, err := r.dispatcher.DispatchColonyBee(dispatchCtx, r.colony, ColonyDispatchRequest{
		Bee:            bee,
		TraceID:        traceID,
		TaskID:         task.TaskID,
		Task:           body,
		Sector:         task.Sector,
		Intent:         task.Intent,
		IsLastWorkTask: taskledger.IsLastWorkTask(snap, task.TaskID),
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
	r.recordTaskRunStart(traceID, task.TaskID, bee, res, startedAt)
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
		if runOpenedRootProposal(r.registry, bee, res.Result) {
			return nil
		}
		return r.setTaskStatus(dispatchCtx, traceID, task.TaskID, protocol.TaskStatusWaitingReview, summary)
	}
	if ev, ok := runEmittedTaskCompleted(res.Result, task.TaskID); ok {
		return r.applyTaskCompletedEvent(dispatchCtx, traceID, ev)
	}
	if shouldDeferAFKCompletion(r.registry, bee, res.Result) {
		afkSummary := summary
		if afkSummary == "" {
			afkSummary = "Awaiting AFK verification gate"
		}
		return r.setTaskStatus(dispatchCtx, traceID, task.TaskID, protocol.TaskStatusWaitingReview, afkSummary)
	}
	return r.completeTask(dispatchCtx, traceID, task.TaskID, summary, "")
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

func (r *Reactor) recordTaskRunStart(traceID, taskID, bee string, res *BeeRunResult, startedAt time.Time) {
	if res == nil || taskID == "" {
		return
	}
	if err := runs.AppendTaskRun(r.colony.ColonyRoot, traceID, taskID, runs.TaskRunEntry{
		AgentID:   res.AgentID,
		Bee:       bee,
		RunDir:    res.RunDir,
		StartedAt: startedAt,
		RunStatus: "running",
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
