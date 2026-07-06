package runtime

import (
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

func hasInsightKind(events []protocol.Event, agentID string, kind protocol.InsightKind) bool {
	for _, ev := range events {
		if ev.AgentID != agentID {
			continue
		}
		if ev.Type != protocol.EventInsight {
			continue
		}
		if protocol.PayloadKind(ev.Payload) == string(kind) {
			return true
		}
	}
	return false
}

// synthesizeRunSummary appends a default INSIGHT/run.summary when policy allows and none exists yet.
// Returns updated run events and an optional synthesized event for bus publish.
func (d *Dispatcher) synthesizeRunSummary(
	runDir runs.Dir,
	bee colony.Bee,
	traceID, agentID, taskID string,
	result *adapters.RunResult,
	runEvents []protocol.Event,
) ([]protocol.Event, *protocol.Event, error) {
	if result == nil || result.Status != string(protocol.StatusCompleted) {
		return runEvents, nil, nil
	}

	policy := bee.ResolvedRunSummaryPolicy()
	if policy == colony.RunSummaryDisabled {
		return runEvents, nil, nil
	}
	if hasInsightKind(runEvents, agentID, protocol.InsightRunSummary) {
		return runEvents, nil, nil
	}
	if !d.shouldAutoPublishRunSummary(bee.Role) {
		return runEvents, nil, nil
	}

	summary := strings.TrimSpace(result.Summary)
	if summary == "" {
		return runEvents, nil, nil
	}

	payload := protocol.NarrativeInsightPayload{
		Kind:    protocol.InsightRunSummary,
		Summary: summary,
	}
	if taskID != "" {
		payload.TaskID = taskID
	}
	ev, err := protocol.NewEvent(traceID, agentID, 0, protocol.EventInsight, payload)
	if err != nil {
		return nil, nil, err
	}
	if err := runDir.AppendEvent(ev); err != nil {
		return nil, nil, fmt.Errorf("runtime: append run.summary: %w", err)
	}
	runEvents = append(runEvents, ev)
	return runEvents, &ev, nil
}

func (d *Dispatcher) enforceRunSummaryRequired(bee colony.Bee, agentID string, result *adapters.RunResult, runEvents []protocol.Event) {
	if result == nil || bee.ResolvedRunSummaryPolicy() != colony.RunSummaryRequired {
		return
	}
	if hasInsightKind(runEvents, agentID, protocol.InsightRunSummary) {
		return
	}
	msg := "runtime: required INSIGHT/run.summary missing"
	result.Warnings = append(result.Warnings, msg)
	result.Status = string(protocol.StatusFailed)
	if result.Err == nil {
		result.Err = fmt.Errorf("%s", msg)
	}
}

func (d *Dispatcher) shouldAutoPublishRunSummary(beeRole string) bool {
	if d.registry == nil {
		return true
	}
	return d.registry.ShouldAutoPublishRunSummary(beeRole)
}
