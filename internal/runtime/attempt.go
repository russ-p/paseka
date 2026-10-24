package runtime

import (
	"fmt"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
)

func attemptProvenance(a trailconfig.Attempt) protocol.Provenance {
	return protocol.Provenance{
		AttemptID:            a.AttemptID,
		ConfigRef:            a.ConfigRef,
		ConfigRevision:       a.ConfigRevision,
		InitialCodeBaseSHA:   a.InitialCodeBaseSHA,
		InitialCodeBaseDirty: a.InitialCodeBaseDirty,
	}
}

func eventIdentity(ev protocol.Event) string {
	if ev.ID != "" {
		return ev.ID
	}
	return directEventFingerprint(ev)
}

// stampIncomingProvenance replaces any provenance on a received event with the
// owning run's recorded values. A producer cannot claim a different attempt or
// configuration than the run directory that emitted the event.
func (r *Reactor) stampIncomingProvenance(ev *protocol.Event) {
	if ev == nil || r.colony.ColonyRoot == "" || ev.TraceID == "" || ev.AgentID == "" {
		return
	}
	meta, ok, err := runs.FindRun(r.colony.ColonyRoot, ev.TraceID, ev.AgentID)
	if err != nil || !ok {
		return
	}
	ev.Provenance = meta.Provenance
}

// stampRuntimeProvenance attaches the task's owning attempt to an event the
// runtime synthesizes. Runtime events carry no agent run of their own.
func (r *Reactor) stampRuntimeProvenance(ev *protocol.Event) {
	if ev == nil || ev.AttemptID != "" || r.colony.ColonyRoot == "" {
		return
	}
	taskID := protocol.PayloadTaskID(ev.Payload)
	if taskID == "" {
		return
	}
	attempt, ok, err := trailconfig.LatestTaskAttempt(r.colony.ColonyRoot, ev.TraceID, taskID)
	if err != nil || !ok {
		return
	}
	ev.Provenance = attemptProvenance(attempt)
}

// contextForAttempt resolves the execution context at the attempt's recorded
// configuration. Rework, guard, review, and HITL continuations must use the
// original snapshot even after the trail's active version changes.
func (r *Reactor) contextForAttempt(base *trailExecutionContext, traceID string, attempt trailconfig.Attempt) (*trailExecutionContext, error) {
	if base == nil {
		resolved, err := r.executionContext(traceID)
		if err != nil {
			return nil, err
		}
		base = resolved
	}
	if attempt.ConfigRef == "" || attempt.ConfigRef == base.Colony.ConfigRef {
		bound := base.Colony
		if attempt.ConfigRef != "" {
			bound.ConfigRef = attempt.ConfigRef
		}
		if attempt.ConfigRevision > 0 {
			bound.ConfigRevision = attempt.ConfigRevision
		}
		return &trailExecutionContext{
			Colony:   bound,
			Manifest: base.Manifest,
			Registry: base.Registry,
		}, nil
	}

	bound, err := trailconfig.BindRef(base.Colony, traceID, attempt.ConfigRef, attempt.ConfigRevision)
	if err != nil {
		return nil, fmt.Errorf("runtime: bind attempt %s config: %w", attempt.AttemptID, err)
	}
	registry, err := r.registryForConfig(bound)
	if err != nil {
		return nil, err
	}
	manifest, err := colony.LoadColony(bound.ConfigRootPath())
	if err != nil {
		return nil, err
	}
	return &trailExecutionContext{Colony: bound, Manifest: manifest, Registry: registry}, nil
}

func taskAttemptInput(task taskledger.TaskSnapshot) *trailconfig.TaskInputEvidence {
	return trailconfig.NewTaskInputEvidence(
		task.TaskID,
		task.Title,
		task.Body,
		task.Bee,
		task.Sector,
		task.Intent,
		string(task.Review),
		task.CueID,
	)
}

func directRoleSubscribed(registry *BeeRegistry, ev protocol.Event, beeRole string) bool {
	if registry == nil {
		return false
	}
	for _, role := range registry.DirectSubscribers(ev) {
		if role == beeRole {
			return true
		}
	}
	return false
}

// prepareTaskAttempt returns the task attempt a ready dispatch should run under,
// or skip=true when this is a duplicate or a replay of a closed attempt.
func (r *Reactor) prepareTaskAttempt(ctx *trailExecutionContext, traceID string, task taskledger.TaskSnapshot, sourceEventID string) (trailconfig.Attempt, *trailExecutionContext, bool, error) {
	if r.colony.ColonyRoot == "" {
		return trailconfig.Attempt{}, nil, true, fmt.Errorf("runtime: colony root is required")
	}
	var beeRole string
	if _, ok, err := trailconfig.LoadManifestIfExists(r.colony.ColonyRoot, traceID); err != nil {
		return trailconfig.Attempt{}, nil, false, err
	} else if !ok {
		beeRole = colony.EffectiveTaskBee(task.Bee, ctx.Manifest.Defaults)
		if ctx.Registry == nil || !ctx.Registry.CanDispatchTaskReady(beeRole) {
			return trailconfig.Attempt{}, ctx, true, nil
		}
		return trailconfig.Attempt{}, ctx, false, nil
	}
	attempt, blocked, err := trailconfig.EnsureTaskAttemptGated(ctx.Colony, traceID, task.TaskID, sourceEventID, taskAttemptInput(task))
	if err != nil {
		return trailconfig.Attempt{}, nil, false, err
	}
	if blocked || !attempt.Open() {
		return attempt, ctx, true, nil
	}
	attemptCtx, err := r.contextForAttempt(ctx, traceID, attempt)
	if err != nil {
		return trailconfig.Attempt{}, nil, false, err
	}
	// The effective role must be resolved against the configuration the
	// attempt was admitted with, not the one used to plan this event.
	beeRole = colony.EffectiveTaskBee(task.Bee, attemptCtx.Manifest.Defaults)
	if attemptCtx.Registry == nil || !attemptCtx.Registry.CanDispatchTaskReady(beeRole) {
		return attempt, attemptCtx, true, nil
	}
	key := attempt.AttemptID + "|" + sourceEventID
	marked, duplicate, err := trailconfig.MarkAttemptDispatched(r.colony.ColonyRoot, traceID, attempt.AttemptID, key)
	if err != nil {
		return trailconfig.Attempt{}, nil, false, err
	}
	if duplicate {
		return marked, attemptCtx, true, nil
	}
	return marked, attemptCtx, false, nil
}

// recordedAttemptContext resolves the execution context recorded by the attempt
// that owns an event. It is used for side effects on attempt-owned events so a
// later active revision cannot re-route them through a registry that no longer
// contains their role.
func (r *Reactor) recordedAttemptContext(ev protocol.Event, active *trailExecutionContext) (*trailExecutionContext, bool, error) {
	root := r.colony.ColonyRoot
	if root == "" {
		return active, false, nil
	}
	var (
		attempt trailconfig.Attempt
		ok      bool
		err     error
	)
	if ev.AttemptID != "" {
		attempt, ok, err = trailconfig.FindAttempt(root, ev.TraceID, ev.AttemptID)
	} else if taskID := protocol.PayloadTaskID(ev.Payload); taskID != "" {
		attempt, ok, err = trailconfig.LatestTaskAttempt(root, ev.TraceID, taskID)
	} else {
		return active, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return active, false, nil
	}
	routed, err := r.contextForAttempt(active, ev.TraceID, attempt)
	if err != nil {
		return nil, false, err
	}
	return routed, true, nil
}

// directRoutingContext selects the configuration a direct event should be
// routed under before subscriber lookup. A closed attempt produces no
// configuration: its late events must not be evaluated against the active one.
func (r *Reactor) directRoutingContext(ev protocol.Event, active *trailExecutionContext) (*trailExecutionContext, bool, error) {
	root := r.colony.ColonyRoot
	if root == "" {
		return active, false, nil
	}
	if ev.AttemptID != "" {
		attempt, ok, err := trailconfig.FindAttempt(root, ev.TraceID, ev.AttemptID)
		if err != nil {
			return nil, false, err
		}
		if !ok || !attempt.Open() {
			return nil, true, nil
		}
		routed, err := r.contextForAttempt(active, ev.TraceID, attempt)
		return routed, false, err
	}
	taskID := protocol.PayloadTaskID(ev.Payload)
	if taskID == "" {
		return active, false, nil
	}
	attempt, ok, err := trailconfig.LatestTaskAttempt(root, ev.TraceID, taskID)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return active, false, nil
	}
	if !attempt.Open() {
		return nil, true, nil
	}
	routed, err := r.contextForAttempt(active, ev.TraceID, attempt)
	if err != nil {
		return nil, false, err
	}
	return routed, false, nil
}

// prepareDirectAttempt resolves and marks one direct dispatch for a bee. The
// dispatch key includes the bee so two gates can share a task attempt while a
// replayed event still deduplicates per bee.
func (r *Reactor) prepareDirectAttempt(ev protocol.Event, beeRole string, active *trailExecutionContext) (trailconfig.Attempt, *trailExecutionContext, bool, error) {
	root := r.colony.ColonyRoot
	key := directDispatchKey(ev, beeRole)
	taskID := protocol.PayloadTaskID(ev.Payload)

	if _, ok, err := trailconfig.LoadManifestIfExists(root, ev.TraceID); err != nil {
		return trailconfig.Attempt{}, nil, false, err
	} else if !ok {
		if !directRoleSubscribed(active.Registry, ev, beeRole) {
			return trailconfig.Attempt{}, active, true, nil
		}
		return trailconfig.Attempt{}, active, false, nil
	}

	var attempt trailconfig.Attempt
	switch {
	case ev.AttemptID != "":
		found, ok, err := trailconfig.FindAttempt(root, ev.TraceID, ev.AttemptID)
		if err != nil {
			return trailconfig.Attempt{}, nil, false, err
		}
		if !ok || !found.Open() {
			return found, active, true, nil
		}
		attempt = found
	case taskID != "":
		found, ok, err := trailconfig.LatestTaskAttempt(root, ev.TraceID, taskID)
		if err != nil {
			return trailconfig.Attempt{}, nil, false, err
		}
		if ok {
			if !found.Open() {
				return found, active, true, nil
			}
			attempt = found
		} else {
			admitted, _, blocked, err := trailconfig.AdmitChainAttemptGated(active.Colony, ev.TraceID, key, taskID, ev.ID)
			if err != nil {
				return trailconfig.Attempt{}, nil, false, err
			}
			if blocked || !admitted.Open() {
				return admitted, active, true, nil
			}
			attempt = admitted
		}
	default:
		admitted, _, blocked, err := trailconfig.AdmitChainAttemptGated(active.Colony, ev.TraceID, key, "", ev.ID)
		if err != nil {
			return trailconfig.Attempt{}, nil, false, err
		}
		if blocked || !admitted.Open() {
			return admitted, active, true, nil
		}
		attempt = admitted
	}

	attemptCtx, err := r.contextForAttempt(active, ev.TraceID, attempt)
	if err != nil {
		return trailconfig.Attempt{}, nil, false, err
	}
	if !directRoleSubscribed(attemptCtx.Registry, ev, beeRole) {
		return attempt, attemptCtx, true, nil
	}
	marked, duplicate, err := trailconfig.MarkAttemptDispatched(root, ev.TraceID, attempt.AttemptID, key)
	if err != nil {
		return trailconfig.Attempt{}, nil, false, err
	}
	if duplicate {
		return marked, attemptCtx, true, nil
	}
	return marked, attemptCtx, false, nil
}
