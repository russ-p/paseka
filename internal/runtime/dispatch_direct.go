package runtime

import (
	"context"
	"strings"

	"github.com/paseka/paseka/internal/logging"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

func (r *Reactor) dispatchDirect(ctx context.Context, ev protocol.Event, beeRole string) error {
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
	defer endInflight()

	ok, err := r.gateDispatchEnergy(dispatchCtx, ev.TraceID, taskID, "direct.dispatch")
	if err != nil {
		return err
	}
	if !ok {
		logDispatchSkip("honey reserve exhausted", ev.TraceID, taskID, beeRole)
		return nil
	}

	proposalSector, proposalKind := proposalDispatchFields(ev)

	res, err := r.dispatcher.DispatchColonyBee(dispatchCtx, r.colony, ColonyDispatchRequest{
		Bee:          beeRole,
		TraceID:      ev.TraceID,
		TaskID:       taskID,
		Task:         taskBody,
		Sector:       proposalSector,
		ProposalKind: proposalKind,
	}, DispatchModeDirect)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.directProcessed[key] = struct{}{}
	r.mu.Unlock()
	status := "unknown"
	if res.Result != nil {
		status = res.Result.Status
	}
	logDispatchDone(DispatchModeDirect, beeRole, ev.TraceID, taskID, res.AgentID, status)
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
