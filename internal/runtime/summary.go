package runtime

import (
	"fmt"
	"strings"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
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
	registry *BeeRegistry,
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
	if !shouldAutoPublishRunSummary(registry, bee.Role) {
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

// enforceRunSummaryRequired applies the bee's required run.summary policy.
//
// A required summary is a runtime check in its own right, separate from any
// declared completion contract: a missing summary fails the run with reason
// run_summary_missing and never with reason completion_contract. Only an
// otherwise completed run is downgraded; a run the adapter, the check gate, or
// another runtime check already failed keeps its own status and reason, and the
// missing summary is recorded only as a warning.
//
// It returns the message and true when the missing summary downgraded the run,
// so finalization can persist the correction on both outcome records.
func (d *Dispatcher) enforceRunSummaryRequired(bee colony.Bee, agentID string, result *adapters.RunResult, runEvents []protocol.Event) (string, bool) {
	if result == nil || bee.ResolvedRunSummaryPolicy() != colony.RunSummaryRequired {
		return "", false
	}
	if hasInsightKind(runEvents, agentID, protocol.InsightRunSummary) {
		return "", false
	}
	msg := "runtime: required INSIGHT/run.summary missing"
	result.Warnings = append(result.Warnings, msg)
	if result.Status != string(protocol.StatusCompleted) {
		return "", false
	}
	result.Status = string(protocol.StatusFailed)
	result.Reason = protocol.ReasonRunSummaryMissing
	if result.Err == nil {
		result.Err = fmt.Errorf("%s", msg)
	}
	return msg, true
}

func shouldAutoPublishRunSummary(registry *BeeRegistry, beeRole string) bool {
	if registry == nil {
		return true
	}
	return registry.ShouldAutoPublishRunSummary(beeRole)
}
