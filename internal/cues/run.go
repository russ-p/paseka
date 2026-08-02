package cues

import (
	"context"
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/taskledger"
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
		return RunResult{}, fmt.Errorf("cue %q: emit task is not supported yet", cue.ID)
	default:
		return RunResult{}, fmt.Errorf("cue %q: unsupported emit %q", cue.ID, cue.Emit)
	}
}

func runSignal(ctx context.Context, publisher bus.Publisher, ledger taskledger.Ledger, cue Cue, in RunInput) (RunResult, error) {
	if publisher == nil {
		return RunResult{}, fmt.Errorf("nats url not configured (cue run requires NATS)")
	}

	traceID := strings.TrimSpace(in.TraceID)
	if traceID == "" {
		id, err := colony.NewTraceID()
		if err != nil {
			return RunResult{}, err
		}
		traceID = id
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

	if err := seedCueEnergy(ledger, traceID, cue); err != nil {
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

func seedCueEnergy(ledger taskledger.Ledger, traceID string, cue Cue) error {
	if ledger == nil || cue.EnergyBudget <= 0 {
		return nil
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	if snap.EnergyBudget > 0 {
		return nil
	}
	return ledger.SeedEnergy(traceID, cue.EnergyBudget)
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
