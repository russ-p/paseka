package taskledger

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
)

// ApplyEvent is a pure reducer: given a trace snapshot and one bus event,
// returns the updated snapshot and any tasks that became ready.
// No persistence — suitable for tests and future Ledger implementations.
//
// Payloads the ledger does not own are skipped, not applied, and leave the trace
// untouched. system.kill is sticky: once a trace is killed, a late task.status,
// task.ready, or task.completed no longer mutates it.
func ApplyEvent(trace TraceSnapshot, event protocol.Event) (ApplyResult, error) {
	trace, err := traceForEvent(trace, event)
	if err != nil {
		return ApplyResult{}, err
	}

	step, err := reduceEvent(trace, event, eventStamp(event))
	if err != nil {
		return ApplyResult{}, err
	}
	if step.skipped {
		return ApplyResult{Trace: step.trace}, nil
	}

	trace, dropped := dropStalePendingReady(step.trace)
	if dropped {
		step.changed = true
	}
	return ApplyResult{
		Trace:   trace,
		Ready:   step.ready,
		Changed: step.changed,
	}, nil
}

// reduceEvent routes one bus event to the reducer that owns its payload kind.
func reduceEvent(trace TraceSnapshot, event protocol.Event, now time.Time) (reduceStep, error) {
	switch event.Type {
	case protocol.EventInsight:
		return reduceInsight(trace, event, now)
	case protocol.EventSignal:
		return reduceSignal(trace, event, now)
	case protocol.EventVerification:
		return reduceVerification(trace, event, now)
	case protocol.EventMutation:
		return reduceMutation(trace, event, now)
	default:
		return reduceStep{trace: trace}, nil
	}
}

// reduceStep carries one event's reduction in flight: the working trace, the
// tasks that became ready, and whether the trace changed.
type reduceStep struct {
	trace   TraceSnapshot
	ready   []TaskSnapshot
	changed bool

	// skipped marks a payload this ledger does not own: the trace is returned as
	// it was and the trailing housekeeping is bypassed.
	skipped bool
}

// skipStep ends a reduction for a payload kind this ledger does not own.
func skipStep(trace TraceSnapshot) reduceStep {
	return reduceStep{trace: trace, skipped: true}
}

// traceForEvent checks the event against the snapshot and returns the working
// copy every reducer mutates.
func traceForEvent(trace TraceSnapshot, event protocol.Event) (TraceSnapshot, error) {
	if event.TraceID != "" && trace.TraceID != "" && event.TraceID != trace.TraceID {
		return TraceSnapshot{}, fmt.Errorf("taskledger: trace mismatch: event %q vs snapshot %q", event.TraceID, trace.TraceID)
	}
	if trace.TraceID == "" && event.TraceID != "" {
		trace.TraceID = event.TraceID
	}
	if trace.Tasks == nil {
		trace.Tasks = make(map[string]TaskSnapshot)
	}
	return trace, nil
}

// eventStamp is the event time, or now when the publisher omitted createdAt.
func eventStamp(event protocol.Event) time.Time {
	if event.CreatedAt.IsZero() {
		return time.Now().UTC()
	}
	return event.CreatedAt
}

// decodePayload decodes the raw payload an event carries into T for the reducer
// named label, so a malformed payload reports which kind failed to parse.
func decodePayload[T any](event protocol.Event, label string) (T, error) {
	var payload T
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return payload, fmt.Errorf("taskledger: parse %s: %w", label, err)
	}
	return payload, nil
}

// signalKind reads payload.kind from a SIGNAL event, exactly as published.
func signalKind(event protocol.Event) string {
	if len(event.Payload) == 0 {
		return ""
	}
	var meta struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(event.Payload, &meta)
	return meta.Kind
}

// plannedTaskPhase derives the phase a task.plan earns at reduce time. closing
// is a runtime-only capability: a plan from any other publisher — planner,
// builder, reviewer, console, CLI — is coerced to work. merge is derived from
// review: final and is never authored directly, so a plan carrying merge with
// any other review policy reads as work.
func plannedTaskPhase(spec protocol.TaskSpec, publisher string) protocol.TaskPhase {
	phase := protocol.DeriveTaskPhase(spec.Phase, spec.Review)
	if phase == protocol.TaskPhaseClosing && strings.TrimSpace(publisher) != protocol.RuntimeAgentID {
		return protocol.TaskPhaseWork
	}
	return phase
}

// taskOrNew returns the ledger task for taskID, or a bare snapshot when the id
// first shows up on an event other than task.plan.
func taskOrNew(trace TraceSnapshot, taskID string) TaskSnapshot {
	if task, ok := trace.Tasks[taskID]; ok {
		return task
	}
	return TaskSnapshot{TaskID: taskID}
}

// killBlocks reports whether system.kill has frozen this trace. The kill is
// sticky: a late task.status, task.ready, or task.completed must not rewrite a
// cancelled trace.
func killBlocks(trace TraceSnapshot) bool {
	return trace.Killed
}

func reduceInsight(trace TraceSnapshot, event protocol.Event, now time.Time) (reduceStep, error) {
	payload, err := decodePayload[protocol.TaskPlanPayload](event, "task.plan")
	if err != nil {
		return reduceStep{}, err
	}
	if payload.Kind != protocol.TaskEventPlan {
		return skipStep(trace), nil
	}

	step := reduceStep{trace: trace}
	for _, spec := range payload.Tasks {
		if spec.TaskID == "" {
			continue
		}
		if existing, exists := step.trace.Tasks[spec.TaskID]; exists && existing.Status != "" {
			continue
		}
		step.trace.Tasks[spec.TaskID] = TaskSnapshot{
			TaskID:    spec.TaskID,
			Title:     spec.Title,
			Body:      spec.Body,
			Bee:       spec.Bee,
			Sector:    spec.Sector,
			Intent:    spec.Intent,
			CueID:     spec.CueID,
			Review:    protocol.NormalizeTaskReviewPolicy(spec.Review),
			Phase:     plannedTaskPhase(spec, event.AgentID),
			Status:    protocol.TaskStatusPlanned,
			DependsOn: append([]string(nil), spec.DependsOn...),
			CreatedAt: now,
			UpdatedAt: now,
		}
		step.changed = true
	}

	var promoted []TaskSnapshot
	var promotedChanged bool
	step.trace, promoted, promotedChanged = tryPromotePendingReady(step.trace, now)
	if promotedChanged {
		step.ready = append(step.ready, promoted...)
		step.changed = true
	}
	return step, nil
}

func reduceSignal(trace TraceSnapshot, event protocol.Event, now time.Time) (reduceStep, error) {
	kind := signalKind(event)
	switch kind {
	case string(protocol.SignalSystemKill):
		return reduceSystemKill(trace, event, now)
	case string(protocol.TaskEventStatus):
		return reduceTaskStatus(trace, event, now)
	case string(protocol.TaskEventReady):
		return reduceTaskReady(trace, event, now)
	default:
		return reduceEnergy(trace, event, kind)
	}
}

func reduceSystemKill(trace TraceSnapshot, event protocol.Event, now time.Time) (reduceStep, error) {
	payload, err := decodePayload[protocol.SystemKillPayload](event, "system.kill")
	if err != nil {
		return reduceStep{}, err
	}
	if payload.Kind != protocol.SignalSystemKill {
		return skipStep(trace), nil
	}
	killed := trace.Killed
	trace = applySystemKill(trace, payload.Reason, now)
	return reduceStep{trace: trace, changed: !killed}, nil
}

func reduceEnergy(trace TraceSnapshot, event protocol.Event, kind string) (reduceStep, error) {
	switch protocol.EnergyEventKind(kind) {
	case protocol.SignalEnergyAdd:
		payload, err := decodePayload[protocol.EnergyAddPayload](event, "energy.add")
		if err != nil {
			return reduceStep{}, err
		}
		if payload.Amount <= 0 {
			return reduceStep{}, fmt.Errorf("taskledger: energy.add amount must be positive")
		}
		trace.EnergyRemaining += payload.Amount
		// Leave EnergyBudget untouched when still 0 so SeedEnergy can apply
		// colony defaults.energy_budget (not a hardcoded DefaultEnergyBudget).
		if trace.EnergyBudget > 0 {
			trace.EnergyAdded += payload.Amount
		}
		return reduceStep{trace: trace, changed: true}, nil

	case protocol.SignalEnergyConsume:
		payload, err := decodePayload[protocol.EnergyConsumePayload](event, "energy.consume")
		if err != nil {
			return reduceStep{}, err
		}
		if payload.Amount <= 0 {
			return reduceStep{}, fmt.Errorf("taskledger: energy.consume amount must be positive")
		}
		if trace.EnergyRemaining < payload.Amount {
			return reduceStep{}, fmt.Errorf("taskledger: insufficient honey reserve")
		}
		trace.EnergyRemaining -= payload.Amount
		return reduceStep{trace: trace, changed: true}, nil

	case protocol.SignalEnergyStipend:
		payload, err := decodePayload[protocol.EnergyStipendPayload](event, "energy.stipend")
		if err != nil {
			return reduceStep{}, err
		}
		if payload.Amount <= 0 {
			return reduceStep{}, fmt.Errorf("taskledger: energy.stipend amount must be positive")
		}
		trace.EnergyRemaining = payload.Amount
		return reduceStep{trace: trace, changed: true}, nil

	default:
		// A SIGNAL kind this ledger does not own is skipped, but a payload that
		// is not valid JSON still fails to parse.
		if _, err := decodePayload[protocol.TaskReadyPayload](event, "task.ready"); err != nil {
			return reduceStep{}, err
		}
		return skipStep(trace), nil
	}
}

func reduceVerification(trace TraceSnapshot, event protocol.Event, now time.Time) (reduceStep, error) {
	payload, err := decodePayload[protocol.TaskCompletedPayload](event, "task.completed")
	if err != nil {
		return reduceStep{}, err
	}
	if payload.Kind != protocol.TaskEventCompleted {
		return skipStep(trace), nil
	}
	if payload.TaskID == "" {
		return reduceStep{}, fmt.Errorf("taskledger: task.completed missing taskId")
	}
	if killBlocks(trace) {
		return skipStep(trace), nil
	}

	task := taskOrNew(trace, payload.TaskID)
	task = withCreatedAt(task, now)
	status := payload.Status
	if status == "" {
		status = protocol.TaskStatusCompleted
	}
	task.Status = status
	if payload.Summary != "" {
		task.Summary = payload.Summary
	}
	if payload.Commit != "" {
		task.Commit = payload.Commit
	}
	task.UpdatedAt = now
	if !payload.CompletedAt.IsZero() {
		task.UpdatedAt = payload.CompletedAt
	}
	trace.Tasks[payload.TaskID] = task
	step := reduceStep{trace: trace, changed: true}

	var promoted []TaskSnapshot
	var promotedChanged bool
	step.trace, promoted, promotedChanged = tryPromotePendingReady(step.trace, now)
	if promotedChanged {
		step.ready = append(step.ready, promoted...)
		step.changed = true
	}

	// Promote at most one eligible planned task to ready.
	var candidate TaskSnapshot
	var promotedOK bool
	step.trace, candidate, promotedOK = PromoteFirstEligible(step.trace, now)
	if promotedOK {
		step.ready = append(step.ready, candidate)
		step.changed = true
	}
	return step, nil
}

func reduceMutation(trace TraceSnapshot, event protocol.Event, now time.Time) (reduceStep, error) {
	payload, err := decodePayload[protocol.MutationPayload](event, "mutation")
	if err != nil {
		return reduceStep{}, err
	}
	if !protocol.IsCodeProposalKind(string(payload.Kind)) || payload.TaskID == "" {
		return skipStep(trace), nil
	}

	task := taskOrNew(trace, payload.TaskID)
	task = withCreatedAt(task, now)
	switch protocol.NormalizeCodeProposalKind(payload.Kind) {
	case protocol.MutationCodeProposalIsolated:
		task.ProposalWorkspace = protocol.ProposalWorkspaceIsolated
	case protocol.MutationCodeProposalRoot:
		task.ProposalWorkspace = protocol.ProposalWorkspaceRoot
	}
	if payload.Workspace != "" {
		task.ProposalWorkspace = payload.Workspace
	}
	task.UpdatedAt = now
	trace.Tasks[payload.TaskID] = task
	return reduceStep{trace: trace, changed: true}, nil
}

func reduceTaskStatus(trace TraceSnapshot, event protocol.Event, now time.Time) (reduceStep, error) {
	payload, err := decodePayload[protocol.TaskStatusPayload](event, "task.status")
	if err != nil {
		return reduceStep{}, err
	}
	if payload.Kind != protocol.TaskEventStatus {
		return skipStep(trace), nil
	}
	if payload.TaskID == "" {
		return reduceStep{}, fmt.Errorf("taskledger: task.status missing taskId")
	}
	if payload.Status == "" {
		return reduceStep{}, fmt.Errorf("taskledger: task.status missing status")
	}
	if killBlocks(trace) {
		return skipStep(trace), nil
	}

	task := taskOrNew(trace, payload.TaskID)
	task = withCreatedAt(task, now)
	task.Status = payload.Status
	// Always assign summary (including "") so unblock/clear transitions
	// can drop stale reasons like "Honey reserve exhausted".
	task.Summary = payload.Summary
	task.UpdatedAt = now
	trace.Tasks[payload.TaskID] = task
	return reduceStep{trace: trace, changed: true}, nil
}

func reduceTaskReady(trace TraceSnapshot, event protocol.Event, now time.Time) (reduceStep, error) {
	payload, err := decodePayload[protocol.TaskReadyPayload](event, "task.ready")
	if err != nil {
		return reduceStep{}, err
	}
	if payload.Kind != protocol.TaskEventReady {
		return skipStep(trace), nil
	}
	if killBlocks(trace) {
		return skipStep(trace), nil
	}
	if payload.TaskID == "" {
		return reduceStep{}, fmt.Errorf("taskledger: task.ready missing taskId")
	}

	task, known := trace.Tasks[payload.TaskID]
	if !known || task.Status == "" {
		// No plan yet: park the kick until task.plan registers the id.
		trace.PendingReady = pendingReadyFromPayload(payload)
		if known {
			delete(trace.Tasks, payload.TaskID)
		}
		return reduceStep{trace: trace, changed: true}, nil
	}

	step := reduceStep{trace: trace}
	task = applyReadyOverlays(task, payload)
	switch {
	case task.Status == protocol.TaskStatusReady:
		// Already ready: keep the status, only refresh the kick's overlays.
	case task.Status == protocol.TaskStatusFailed || task.Status == protocol.TaskStatusRunning:
		task.Status = protocol.TaskStatusReady
		task.Summary = ""
		step.ready = append(step.ready, task)
		step.changed = true
	case !HasReadyTask(trace):
		if first, eligible := FirstEligiblePlanned(trace); eligible && first.TaskID == payload.TaskID {
			task.Status = protocol.TaskStatusReady
			step.ready = append(step.ready, task)
			step.changed = true
		}
	}
	task.UpdatedAt = now
	step.trace.Tasks[payload.TaskID] = task
	return step, nil
}

func allDepsCompleted(trace TraceSnapshot, deps []string) bool {
	for _, dep := range deps {
		t, ok := trace.Tasks[dep]
		if !ok || t.Status != protocol.TaskStatusCompleted {
			return false
		}
	}
	return true
}

// ApplyEvents folds a sequence of events over an initial trace snapshot.
func ApplyEvents(trace TraceSnapshot, events []protocol.Event) (TraceSnapshot, error) {
	for _, ev := range events {
		res, err := ApplyEvent(trace, ev)
		if err != nil {
			return TraceSnapshot{}, err
		}
		trace = res.Trace
	}
	return trace, nil
}
