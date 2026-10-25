package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

// deferDirectDispatch records a direct reaction that could not start because
// the honey reserve was exhausted. The task lifecycle status is untouched: the
// reaction belongs to an event, not to the task's next state.
func (r *Reactor) deferDirectDispatch(ev protocol.Event, beeRole, taskID string) error {
	dispatch := taskledger.PendingDispatch{
		ID:            directDispatchKey(ev, beeRole),
		Kind:          taskledger.PendingDispatchDirect,
		TaskID:        taskID,
		Bee:           beeRole,
		SourceEventID: eventIdentity(ev),
		Reason:        protocol.HoneyReserveExhaustedSummary,
		Status:        taskledger.PendingDispatchPending,
		CreatedAt:     time.Now().UTC(),
	}
	added, err := r.ledger.DeferPendingDispatch(ev.TraceID, dispatch)
	if err != nil {
		return err
	}
	if added {
		runtimeLog.Info("direct dispatch deferred",
			logging.F("trace", ev.TraceID),
			logging.F("task", taskID),
			logging.F("bee", beeRole),
		)
	}
	return nil
}

// resumePendingDispatches drains deferred work after energy.add. It resumes only
// direct dispatches in this slice; task dispatch unblocking stays in the
// existing task-status path.
func (r *Reactor) resumePendingDispatches(ctx context.Context, traceID string) error {
	snap, err := r.ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	if snap.Killed || snap.EnergyRemaining <= 0 {
		return nil
	}
	pending := taskledger.ActivePendingDispatches(snap)
	if !hasPendingDirect(pending) {
		return nil
	}
	if !bus.ReplayerAvailable(r.replayer) {
		runtimeLog.Warn("pending direct dispatch cannot resume without an event replayer",
			logging.F("trace", traceID),
		)
		return nil
	}
	events, err := r.replayer.ReplayTrace(traceID)
	if err != nil {
		return fmt.Errorf("runtime: replay pending direct dispatches: %w", err)
	}
	byID := make(map[string]protocol.Event, len(events))
	for _, ev := range events {
		byID[eventIdentity(ev)] = ev
	}
	for _, item := range pending {
		if item.Kind != taskledger.PendingDispatchDirect {
			continue
		}
		ev, ok := byID[item.SourceEventID]
		if !ok {
			runtimeLog.Warn("pending direct dispatch source event missing",
				logging.F("trace", traceID),
				logging.F("task", item.TaskID),
				logging.F("bee", item.Bee),
				logging.F("sourceEvent", item.SourceEventID),
			)
			continue
		}
		r.stampIncomingProvenance(&ev)
		if err := r.dispatchDirect(ctx, ev, item.Bee, nil); err != nil {
			return err
		}
		if r.directProcessedRecorded(item.ID) {
			if err := r.ledger.ResolvePendingDispatch(traceID, item.ID, taskledger.PendingDispatchConsumed); err != nil {
				return err
			}
		}
	}
	return nil
}

func hasPendingDirect(pending []taskledger.PendingDispatch) bool {
	for _, item := range pending {
		if item.Kind == taskledger.PendingDispatchDirect {
			return true
		}
	}
	return false
}

func (r *Reactor) directProcessedRecorded(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.directProcessed[key]
	return ok
}
