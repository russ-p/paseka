package runtime

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
)

// tryActivateConfig advances a pending configuration action when its blockers
// have drained. Activation is serialized with admission by the trailconfig
// binding lock, so a run cannot be admitted against one revision while another
// revision is being activated.
func (r *Reactor) tryActivateConfig(ctx context.Context, traceID string, dispatchReady bool) (bool, error) {
	if r.colony.ColonyRoot == "" || traceID == "" {
		return false, nil
	}
	manifest, err := trailconfig.Load(r.colony.ColonyRoot, traceID)
	if err != nil {
		if errors.Is(err, trailconfig.ErrNoBinding) {
			return false, nil
		}
		return false, err
	}
	if manifest.Pending == nil {
		return false, nil
	}
	if r.traceKilled(traceID) {
		r.cancelPendingConfig(traceID, "trace killed")
		return false, nil
	}
	updated, changed, err := trailconfig.TryActivate(r.colony.ColonyRoot, traceID, nil)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	runtimeLog.Info("configuration activated",
		logging.F("trace", traceID),
		logging.F("revision", fmt.Sprintf("%d", updated.ConfigRevision)),
		logging.F("configRef", updated.ActiveConfigRef),
	)
	if dispatchReady {
		if err := r.dispatchReadyUnderActive(ctx, traceID); err != nil {
			return true, err
		}
	}
	return true, nil
}

// refreshExecutionIfChanged returns the active execution context after an
// activation that happened outside this event's initial resolution.
func (r *Reactor) refreshExecutionIfChanged(execCtx *trailExecutionContext, traceID string) (*trailExecutionContext, bool, error) {
	if r.colony.ColonyRoot == "" || traceID == "" {
		return execCtx, false, nil
	}
	manifest, err := trailconfig.Load(r.colony.ColonyRoot, traceID)
	if err != nil {
		if errors.Is(err, trailconfig.ErrNoBinding) {
			return execCtx, false, nil
		}
		return nil, false, err
	}
	if execCtx != nil && execCtx.Colony.ConfigRef == manifest.ActiveConfigRef {
		return execCtx, false, nil
	}
	resolved, err := r.executionContext(traceID)
	if err != nil {
		return nil, false, err
	}
	return resolved, true, nil
}

func (r *Reactor) dispatchReadyUnderActive(ctx context.Context, traceID string) error {
	if r.ledger == nil || r.traceKilled(traceID) {
		return nil
	}
	execCtx, err := r.executionContext(traceID)
	if err != nil {
		return err
	}
	ready, err := r.readyTasksInLedger(traceID)
	if err != nil {
		return err
	}
	source := "config-activate:" + execCtx.Colony.ConfigRef
	for _, task := range ready {
		if err := r.dispatchReady(ctx, traceID, task, execCtx, source); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reactor) readyTasksInLedger(traceID string) ([]taskledger.TaskSnapshot, error) {
	if r.ledger == nil {
		return nil, nil
	}
	snap, err := r.ledger.Snapshot(traceID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for taskID, task := range snap.Tasks {
		if task.Status == protocol.TaskStatusReady {
			ids = append(ids, taskID)
		}
	}
	sort.Strings(ids)
	tasks := make([]taskledger.TaskSnapshot, 0, len(ids))
	for _, taskID := range ids {
		tasks = append(tasks, snap.Tasks[taskID])
	}
	return tasks, nil
}

// RecoverPendingConfigs restores the admission barrier after a Reactor restart
// and activates any pending transition that is already at a safe boundary. A
// still-blocked action stays pending and continues to hold new admissions.
func (r *Reactor) RecoverPendingConfigs(ctx context.Context) error {
	if r.colony.ColonyRoot == "" {
		return nil
	}
	traceIDs, err := trailconfig.PendingTraceIDs(r.colony.ColonyRoot)
	if err != nil {
		return err
	}
	for _, traceID := range traceIDs {
		if _, err := r.tryActivateConfig(ctx, traceID, true); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reactor) cancelPendingConfig(traceID, reason string) {
	if r.colony.ColonyRoot == "" || traceID == "" {
		return
	}
	_, changed, err := trailconfig.CancelIfPending(r.colony.ColonyRoot, traceID, "runtime", reason)
	if err != nil {
		runtimeLog.Warn("cancel pending configuration failed", logging.F("error", err.Error()))
		return
	}
	if changed {
		runtimeLog.Info("pending configuration cancelled", logging.F("trace", traceID), logging.F("reason", reason))
	}
}

// taskAdmissionHeld reports whether a new task attempt would be rejected by the
// current admission barrier. An attempt that is already open is existing work
// and must keep dispatching under its recorded configuration.
func (r *Reactor) taskAdmissionHeld(traceID, taskID string) (bool, error) {
	if r.colony.ColonyRoot == "" || traceID == "" || taskID == "" {
		return false, nil
	}
	held, err := trailconfig.HasPending(r.colony.ColonyRoot, traceID)
	if err != nil || !held {
		return false, err
	}
	attempt, ok, err := trailconfig.LatestTaskAttempt(r.colony.ColonyRoot, traceID, taskID)
	if err != nil {
		return false, err
	}
	return !ok || !attempt.Open(), nil
}

// directAdmissionHeld reports whether a new independent direct chain would be
// rejected. Events belonging to an existing attempt or chain are allowed.
func (r *Reactor) directAdmissionHeld(ev protocol.Event, beeRole, taskID string) (bool, error) {
	if r.colony.ColonyRoot == "" || ev.TraceID == "" {
		return false, nil
	}
	held, err := trailconfig.HasPending(r.colony.ColonyRoot, ev.TraceID)
	if err != nil || !held {
		return false, err
	}
	if ev.AttemptID != "" {
		return false, nil
	}
	if taskID != "" {
		if _, ok, err := trailconfig.LatestTaskAttempt(r.colony.ColonyRoot, ev.TraceID, taskID); err != nil {
			return false, err
		} else if ok {
			return false, nil
		}
	}
	if _, ok, err := trailconfig.ChainAttempt(r.colony.ColonyRoot, ev.TraceID, directDispatchKey(ev, beeRole)); err != nil {
		return false, err
	} else if ok {
		return false, nil
	}
	return true, nil
}
