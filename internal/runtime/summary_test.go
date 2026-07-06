package runtime

import (
	"encoding/json"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

func TestSynthesizeRunSummaryAppendsEvent(t *testing.T) {
	root := t.TempDir()
	runDir := runs.Dir{ColonyRoot: root, TraceID: "trace-1", AgentID: "agent-1"}
	if err := runDir.Prepare(); err != nil {
		t.Fatal(err)
	}

	d := NewDispatcher()
	bee := colony.Bee{Role: "builder"}
	result := &adapters.RunResult{
		Status:  string(protocol.StatusCompleted),
		Summary: "implemented feature",
	}

	events, synthesized, err := d.synthesizeRunSummary(runDir, bee, "trace-1", "agent-1", "task-1", result, nil)
	if err != nil {
		t.Fatal(err)
	}
	if synthesized == nil {
		t.Fatal("expected synthesized event")
	}
	if len(events) != 1 {
		t.Fatalf("events len = %d", len(events))
	}
	if protocol.PayloadKind(synthesized.Payload) != string(protocol.InsightRunSummary) {
		t.Fatalf("kind = %q", protocol.PayloadKind(synthesized.Payload))
	}

	stored, err := runDir.ReadEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("stored events = %d", len(stored))
	}
}

func TestSynthesizeRunSummarySkipsDuplicate(t *testing.T) {
	root := t.TempDir()
	runDir := runs.Dir{ColonyRoot: root, TraceID: "trace-1", AgentID: "agent-1"}
	if err := runDir.Prepare(); err != nil {
		t.Fatal(err)
	}

	existing, err := protocol.NewEvent("trace-1", "agent-1", 1, protocol.EventInsight, protocol.NarrativeInsightPayload{
		Kind:    protocol.InsightRunSummary,
		Summary: "already there",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runDir.AppendEvent(existing); err != nil {
		t.Fatal(err)
	}

	d := NewDispatcher()
	bee := colony.Bee{Role: "builder"}
	result := &adapters.RunResult{
		Status:  string(protocol.StatusCompleted),
		Summary: "new summary",
	}

	events, synthesized, err := d.synthesizeRunSummary(runDir, bee, "trace-1", "agent-1", "", result, []protocol.Event{existing})
	if err != nil {
		t.Fatal(err)
	}
	if synthesized != nil {
		t.Fatal("expected no synthesized event")
	}
	if len(events) != 1 {
		t.Fatalf("events len = %d", len(events))
	}
}

func TestSynthesizeRunSummaryDisabled(t *testing.T) {
	root := t.TempDir()
	runDir := runs.Dir{ColonyRoot: root, TraceID: "trace-1", AgentID: "agent-1"}
	if err := runDir.Prepare(); err != nil {
		t.Fatal(err)
	}

	d := NewDispatcher()
	bee := colony.Bee{Role: "builder", RunSummary: colony.RunSummaryDisabled}
	result := &adapters.RunResult{
		Status:  string(protocol.StatusCompleted),
		Summary: "implemented feature",
	}

	_, synthesized, err := d.synthesizeRunSummary(runDir, bee, "trace-1", "agent-1", "", result, nil)
	if err != nil {
		t.Fatal(err)
	}
	if synthesized != nil {
		t.Fatal("expected no synthesized event for disabled policy")
	}
}

func TestEnforceRunSummaryRequiredMarksFailed(t *testing.T) {
	d := NewDispatcher()
	bee := colony.Bee{Role: "builder", RunSummary: colony.RunSummaryRequired}
	result := &adapters.RunResult{Status: string(protocol.StatusCompleted)}

	d.enforceRunSummaryRequired(bee, "agent-1", result, nil)
	if result.Status != string(protocol.StatusFailed) {
		t.Fatalf("status = %q", result.Status)
	}
}

func TestShouldAutoPublishRunSummaryRespectsPublishes(t *testing.T) {
	reg := NewBeeRegistryFromBees(map[string]colony.Bee{
		"builder": {
			Role: "builder",
			Publishes: []colony.PublicationRule{{
				EventRule: colony.EventRule{
					Type: string(protocol.EventMutation),
					Kind: string(protocol.MutationCodeProposal),
				},
			}},
		},
	})
	if reg.ShouldAutoPublishRunSummary("builder") {
		t.Fatal("expected false when run.summary not declared")
	}
}

func TestInsightRunSummaryPayloadTaskID(t *testing.T) {
	root := t.TempDir()
	runDir := runs.Dir{ColonyRoot: root, TraceID: "trace-1", AgentID: "agent-1"}
	if err := runDir.Prepare(); err != nil {
		t.Fatal(err)
	}

	d := NewDispatcher()
	_, synthesized, err := d.synthesizeRunSummary(
		runDir,
		colony.Bee{Role: "builder"},
		"trace-1",
		"agent-1",
		"task-42",
		&adapters.RunResult{Status: string(protocol.StatusCompleted), Summary: "done"},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	var payload protocol.NarrativeInsightPayload
	if err := json.Unmarshal(synthesized.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.TaskID != "task-42" {
		t.Fatalf("taskId = %q", payload.TaskID)
	}
}
