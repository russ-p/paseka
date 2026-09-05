package cues

import (
	"context"
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
	"github.com/paseka/paseka/internal/tasks"
)

// RunInput describes a cue run request from any channel.
type RunInput struct {
	ColonyRoot string
	CueID      string
	Text       string
	TraceID    string
	Vars       map[string]string
	Source     string
	AgentID    string
}

// RunResult is returned after a successful cue run.
type RunResult struct {
	TraceID   string
	TaskID    string
	EventType string
	Kind      string
}

// Run loads a cue, optionally seeds honey, and publishes its bus event(s).
func Run(ctx context.Context, publisher bus.Publisher, ledger taskledger.Ledger, in RunInput) (RunResult, error) {
	cue, err := Load(in.ColonyRoot, in.CueID)
	if err != nil {
		return RunResult{}, err
	}
	switch cue.Emit {
	case EmitSignal:
		return runSignal(ctx, publisher, ledger, cue, in)
	case EmitTask:
		return runTask(ctx, publisher, ledger, cue, in)
	default:
		return RunResult{}, fmt.Errorf("cue %q: unsupported emit %q", cue.ID, cue.Emit)
	}
}

func runSignal(ctx context.Context, publisher bus.Publisher, ledger taskledger.Ledger, cue Cue, in RunInput) (RunResult, error) {
	if publisher == nil {
		return RunResult{}, fmt.Errorf("nats url not configured (cue run requires NATS)")
	}

	traceID, err := resolveRunTraceID(cue, in.TraceID)
	if err != nil {
		return RunResult{}, err
	}

	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = "cli"
	}
	agentID := strings.TrimSpace(in.AgentID)
	if agentID == "" {
		agentID = source
	}

	renderCtx := NewRenderContext(in.Text, source, traceID, in.Vars)
	payload, err := BuildSignalPayload(cue, renderCtx)
	if err != nil {
		return RunResult{}, err
	}

	if err := seedTrailEnergy(ledger, traceID, in.ColonyRoot, cue); err != nil {
		return RunResult{}, fmt.Errorf("cue %q: %w", cue.ID, err)
	}

	ev, err := bus.NewEventFromCLI(traceID, agentID, cue.SignalType, string(payload))
	if err != nil {
		return RunResult{}, fmt.Errorf("cue %q: %w", cue.ID, err)
	}
	if err := publisher.PublishEvent(ctx, ev); err != nil {
		return RunResult{}, fmt.Errorf("cue %q: publish: %w", cue.ID, err)
	}

	return RunResult{
		TraceID:   traceID,
		EventType: cue.SignalType,
		Kind:      cue.SignalKind,
	}, nil
}

func runTask(ctx context.Context, publisher bus.Publisher, ledger taskledger.Ledger, cue Cue, in RunInput) (RunResult, error) {
	if publisher == nil {
		return RunResult{}, fmt.Errorf("nats url not configured (cue run requires NATS)")
	}

	traceID, err := resolveRunTraceID(cue, in.TraceID)
	if err != nil {
		return RunResult{}, err
	}

	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = "cli"
	}
	agentID := strings.TrimSpace(in.AgentID)
	if agentID == "" {
		agentID = source
	}

	renderCtx := NewRenderContext(in.Text, source, traceID, in.Vars)
	if err := cue.ValidateRunText(renderCtx); err != nil {
		return RunResult{}, err
	}
	title, err := RenderTemplate("title", cue.TitleTemplate, renderCtx)
	if err != nil {
		return RunResult{}, fmt.Errorf("cue %q: %w", cue.ID, err)
	}
	body, err := RenderTemplate("body", cue.BodyTemplate, renderCtx)
	if err != nil {
		return RunResult{}, fmt.Errorf("cue %q: %w", cue.ID, err)
	}
	if err := cue.ValidateRenderedFields(title, body); err != nil {
		return RunResult{}, err
	}

	if err := seedTrailEnergy(ledger, traceID, in.ColonyRoot, cue); err != nil {
		return RunResult{}, fmt.Errorf("cue %q: %w", cue.ID, err)
	}

	manifest, err := colony.LoadColony(in.ColonyRoot)
	if err != nil {
		return RunResult{}, fmt.Errorf("cue %q: %w", cue.ID, err)
	}
	taskID, err := colony.NewTaskID()
	if err != nil {
		return RunResult{}, err
	}

	resolvedTitle := tasks.DeriveTitle(title, body)
	resolvedBody := strings.TrimSpace(body)
	bee := colony.EffectiveTaskBee(cue.Bee, manifest.Defaults)
	spec := protocol.TaskSpec{
		TaskID: taskID,
		Title:  resolvedTitle,
		Body:   resolvedBody,
		Bee:    bee,
		Intent: cue.Intent,
		Review: protocol.TaskReviewPolicy(strings.TrimSpace(cue.Review)),
	}
	bees, err := colony.LoadAllBees(in.ColonyRoot)
	if err != nil {
		return RunResult{}, fmt.Errorf("cue %q: %w", cue.ID, err)
	}
	if err := colony.ValidateTaskReviewPolicy(spec, bees, manifest.Defaults); err != nil {
		return RunResult{}, fmt.Errorf("cue %q: %w", cue.ID, err)
	}

	planEv, err := tasks.PlanEvent(traceID, agentID, spec)
	if err != nil {
		return RunResult{}, fmt.Errorf("cue %q: %w", cue.ID, err)
	}
	if err := publisher.PublishEvent(ctx, planEv); err != nil {
		return RunResult{}, fmt.Errorf("cue %q: publish: %w", cue.ID, err)
	}

	if cue.Autorun {
		readyEv, err := tasks.ReadyEvent(traceID, agentID, taskledger.TaskSnapshot{
			TaskID: taskID,
			Title:  resolvedTitle,
			Body:   resolvedBody,
			Bee:    bee,
			Intent: cue.Intent,
		}, manifest.Defaults)
		if err != nil {
			return RunResult{}, fmt.Errorf("cue %q: %w", cue.ID, err)
		}
		if err := publisher.PublishEvent(ctx, readyEv); err != nil {
			return RunResult{}, fmt.Errorf("cue %q: publish: %w", cue.ID, err)
		}
	}

	return RunResult{
		TraceID:   traceID,
		TaskID:    taskID,
		EventType: string(protocol.EventInsight),
		Kind:      string(protocol.TaskEventPlan),
	}, nil
}

func resolveRunTraceID(cue Cue, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if !cue.IsStanding() {
		if requested != "" {
			return requested, nil
		}
		id, err := colony.NewTraceID()
		if err != nil {
			return "", err
		}
		return id, nil
	}
	if requested == "" {
		return cue.StandingTrace, nil
	}
	if requested != cue.StandingTrace {
		return "", fmt.Errorf("cue %q: trace id %q does not match standing.trace %q", cue.ID, requested, cue.StandingTrace)
	}
	return cue.StandingTrace, nil
}

func seedTrailEnergy(ledger taskledger.Ledger, traceID, colonyRoot string, cue Cue) error {
	if ledger == nil {
		return nil
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	if snap.EnergyBudget > 0 {
		return nil
	}
	if cue.StandingStipend > 0 {
		return ledger.SeedEnergy(traceID, cue.StandingStipend)
	}
	if cue.EnergyBudget > 0 {
		return ledger.SeedEnergy(traceID, cue.EnergyBudget)
	}
	if cue.Emit != EmitTask {
		return nil
	}
	manifest, err := colony.LoadColony(colonyRoot)
	if err != nil {
		return err
	}
	return ledger.SeedEnergy(traceID, manifest.ResolvedEnergyBudget())
}

// ParseSetFlags parses repeated --set key=val arguments.
func ParseSetFlags(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key, val, ok := strings.Cut(value, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid --set %q (expected key=val)", value)
		}
		out[key] = val
	}
	return out, nil
}
