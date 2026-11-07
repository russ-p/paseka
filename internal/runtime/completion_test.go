package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

func contractTestBee() colony.Bee {
	return colony.Bee{
		Role: "guard",
		CompletionContract: colony.CompletionContract{
			Required: []colony.CompletionRule{{
				Type:      "VERIFICATION",
				KindOneOf: []string{"verification.success", "verification.failed"},
				Count:     1,
			}},
		},
	}
}

func newContractTestRunDir(t *testing.T) runs.Dir {
	t.Helper()
	root := t.TempDir()
	dir := runs.Dir{ColonyRoot: root, TraceID: "trace-contract", AgentID: "agent-contract"}
	if err := os.MkdirAll(dir.Root(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := dir.WriteResult(protocol.Result{
		ProtocolVersion: protocol.Version,
		TraceID:         dir.TraceID,
		AgentID:         dir.AgentID,
		Status:          protocol.StatusCompleted,
		Diagnostics:     protocol.Diagnostics{ExitCode: 0},
	}); err != nil {
		t.Fatal(err)
	}
	if err := dir.WriteStatus(protocol.StatusCompleted, "", 0, time.Time{}, time.Time{}, ""); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestEnforceCompletionContractMarksFailed(t *testing.T) {
	d := NewDispatcher()
	dir := newContractTestRunDir(t)
	result := &adapters.RunResult{Status: string(protocol.StatusCompleted)}
	if err := d.enforceCompletionContract(dir, contractTestBee(), "", nil, nil, result); err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusFailed) {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	if result.Reason != protocol.ReasonCompletionContract {
		t.Fatalf("reason = %q, want %q", result.Reason, protocol.ReasonCompletionContract)
	}
	if result.Contract == nil || result.Contract.Status != protocol.ContractFailed {
		t.Fatalf("contract = %+v, want failed", result.Contract)
	}

	snap, err := dir.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if snap.State != protocol.StatusFailed || snap.Reason != protocol.ReasonCompletionContract {
		t.Fatalf("status record = %s/%s, want failed/%s", snap.State, snap.Reason, protocol.ReasonCompletionContract)
	}
	res, err := dir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != protocol.StatusFailed || res.Reason != protocol.ReasonCompletionContract {
		t.Fatalf("result record = %s/%s, want failed/%s", res.Status, res.Reason, protocol.ReasonCompletionContract)
	}
	if res.Diagnostics.Error == "" {
		t.Fatal("result diagnostics.error is empty, want contract message")
	}
}

func TestEnforceCompletionContractPasses(t *testing.T) {
	d := NewDispatcher()
	dir := newContractTestRunDir(t)
	ev, err := protocol.NewEvent("t", "a", 1, protocol.EventVerification, protocol.VerificationPayload{
		Kind: protocol.VerificationSuccess, Summary: "ok",
	})
	if err != nil {
		t.Fatal(err)
	}
	result := &adapters.RunResult{Status: string(protocol.StatusCompleted)}
	if err := d.enforceCompletionContract(dir, contractTestBee(), "", []protocol.Event{ev}, nil, result); err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusCompleted) {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if result.Contract == nil || result.Contract.Status != protocol.ContractPassed {
		t.Fatalf("contract = %+v, want passed", result.Contract)
	}
	res, err := dir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if res.Contract == nil || res.Contract.Status != protocol.ContractPassed {
		t.Fatalf("recorded contract = %+v, want passed", res.Contract)
	}
	if res.Status != protocol.StatusCompleted {
		t.Fatalf("recorded status = %q, want completed", res.Status)
	}
}

func intentContractTestBee() colony.Bee {
	return colony.Bee{
		Role: "archivist",
		CompletionContract: colony.CompletionContract{
			Required: []colony.CompletionRule{{
				Type:      "INSIGHT",
				KindOneOf: []string{"context.note"},
				Count:     1,
			}},
			Intents: map[string]colony.CompletionOverride{
				"skill": {Required: []colony.CompletionRule{{
					Type:      "INSIGHT",
					KindOneOf: []string{"skill.proposed"},
					Count:     1,
				}}},
			},
		},
	}
}

func contractInsightEvent(t *testing.T, kind protocol.InsightKind) protocol.Event {
	t.Helper()
	ev, err := protocol.NewEvent("trace-contract", "agent-contract", 1, protocol.EventInsight, protocol.NarrativeInsightPayload{
		Kind:    kind,
		Summary: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func contractSkillProposedEvent(t *testing.T, name string) protocol.Event {
	t.Helper()
	ev, err := protocol.NewEvent("trace-contract", "agent-contract", 1, protocol.EventInsight, protocol.SkillProposedPayload{
		Kind:          protocol.InsightSkillProposed,
		Name:          name,
		Description:   "Proposed procedure.",
		Reason:        "Evidence from two runs.",
		Bundle:        "skill-proposals/" + name,
		ProcedureHash: "9f2c",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestEnforceCompletionContractUsesDefaultIntent(t *testing.T) {
	d := NewDispatcher()
	dir := newContractTestRunDir(t)
	result := &adapters.RunResult{Status: string(protocol.StatusCompleted)}
	if err := d.enforceCompletionContract(dir, intentContractTestBee(), "task", []protocol.Event{
		contractInsightEvent(t, protocol.InsightContextNote),
	}, nil, result); err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusCompleted) {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if result.Contract == nil || result.Contract.Status != protocol.ContractPassed {
		t.Fatalf("contract = %+v, want passed", result.Contract)
	}
	if result.Contract.Intent != "task" {
		t.Fatalf("contract intent = %q, want task", result.Contract.Intent)
	}
}

func TestEnforceCompletionContractUsesIntentOverride(t *testing.T) {
	d := NewDispatcher()
	dir := newContractTestRunDir(t)
	result := &adapters.RunResult{Status: string(protocol.StatusCompleted)}
	if err := d.enforceCompletionContract(dir, intentContractTestBee(), "skill", []protocol.Event{
		contractInsightEvent(t, protocol.InsightContextNote),
	}, nil, result); err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusFailed) || result.Reason != protocol.ReasonCompletionContract {
		t.Fatalf("result = %s/%s, want failed/%s", result.Status, result.Reason, protocol.ReasonCompletionContract)
	}
	if result.Contract == nil || result.Contract.Status != protocol.ContractFailed {
		t.Fatalf("contract = %+v, want failed", result.Contract)
	}
	if result.Contract.Intent != "skill" {
		t.Fatalf("contract intent = %q, want skill", result.Contract.Intent)
	}
	if len(result.Contract.Rules) != 1 || len(result.Contract.Rules[0].KindOneOf) != 1 || result.Contract.Rules[0].KindOneOf[0] != "skill.proposed" {
		t.Fatalf("contract rules = %+v, want the override's proposal rule", result.Contract.Rules)
	}
	if !strings.Contains(result.Err.Error(), "skill.proposed") {
		t.Fatalf("error = %v, want the override's kind", result.Err)
	}
	persisted, err := dir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Contract == nil || persisted.Contract.Intent != "skill" {
		t.Fatalf("persisted contract = %+v, want intent skill", persisted.Contract)
	}
	if len(persisted.Contract.Rules) != 1 || persisted.Contract.Rules[0].KindOneOf[0] != "skill.proposed" {
		t.Fatalf("persisted contract rules = %+v, want the applied override rule", persisted.Contract.Rules)
	}
}

func TestEnforceCompletionContractIntentOverridePasses(t *testing.T) {
	d := NewDispatcher()
	dir := newContractTestRunDir(t)
	result := &adapters.RunResult{Status: string(protocol.StatusCompleted)}
	if err := d.enforceCompletionContract(dir, intentContractTestBee(), "skill", []protocol.Event{
		contractInsightEvent(t, protocol.InsightKind("skill.proposed")),
	}, nil, result); err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusCompleted) {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if result.Contract == nil || result.Contract.Status != protocol.ContractPassed {
		t.Fatalf("contract = %+v, want passed", result.Contract)
	}
}

func TestEnforceCompletionContractChroniclePublishesNote(t *testing.T) {
	d := NewDispatcher()
	dir := newContractTestRunDir(t)
	result := &adapters.RunResult{Status: string(protocol.StatusCompleted)}
	if err := d.enforceCompletionContract(dir, intentContractTestBee(), "chronicle", []protocol.Event{
		contractInsightEvent(t, protocol.InsightContextNote),
	}, nil, result); err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusCompleted) {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if result.Contract == nil || result.Contract.Status != protocol.ContractPassed {
		t.Fatalf("contract = %+v, want passed", result.Contract)
	}
	if result.Contract.Intent != "chronicle" {
		t.Fatalf("contract intent = %q, want chronicle", result.Contract.Intent)
	}
}

func TestEnforceCompletionContractSkillPublishingNothingFails(t *testing.T) {
	d := NewDispatcher()
	dir := newContractTestRunDir(t)
	result := &adapters.RunResult{Status: string(protocol.StatusCompleted)}
	if err := d.enforceCompletionContract(dir, intentContractTestBee(), "skill", nil, nil, result); err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusFailed) || result.Reason != protocol.ReasonCompletionContract {
		t.Fatalf("result = %s/%s, want failed/%s", result.Status, result.Reason, protocol.ReasonCompletionContract)
	}
	if result.Contract == nil || result.Contract.Status != protocol.ContractFailed {
		t.Fatalf("contract = %+v, want failed", result.Contract)
	}
}

func TestEnforceCompletionContractSkillPublishingTwoProposalsFails(t *testing.T) {
	d := NewDispatcher()
	dir := newContractTestRunDir(t)
	proposal := contractSkillProposedEvent(t, "paseka-release")
	other := contractSkillProposedEvent(t, "paseka-ship")
	result := &adapters.RunResult{Status: string(protocol.StatusCompleted)}
	if err := d.enforceCompletionContract(dir, intentContractTestBee(), "skill", []protocol.Event{
		proposal, other,
	}, nil, result); err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusFailed) || result.Reason != protocol.ReasonCompletionContract {
		t.Fatalf("result = %s/%s, want failed/%s", result.Status, result.Reason, protocol.ReasonCompletionContract)
	}
}

func TestEnforceCompletionContractTaskPublishingProposalFails(t *testing.T) {
	d := NewDispatcher()
	dir := newContractTestRunDir(t)
	result := &adapters.RunResult{Status: string(protocol.StatusCompleted)}
	if err := d.enforceCompletionContract(dir, intentContractTestBee(), "task", []protocol.Event{
		contractSkillProposedEvent(t, "paseka-release"),
	}, nil, result); err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusFailed) || result.Reason != protocol.ReasonCompletionContract {
		t.Fatalf("result = %s/%s, want failed/%s", result.Status, result.Reason, protocol.ReasonCompletionContract)
	}
}

func TestFinalizeUsesNormalizedIntentForContract(t *testing.T) {
	root := t.TempDir()
	writeStageColony(t, root)
	beeYAML := `role: archivist
adapter: cursor
prompt_template: archivist.md
default_intent: task
intents:
  - task
  - skill
completion_contract:
  required:
    - type: INSIGHT
      kind_one_of:
        - context.note
  intents:
    skill:
      required:
        - type: INSIGHT
          kind_one_of:
            - skill.proposed
`
	if err := os.WriteFile(filepath.Join(root, ".paseka", "bees", "archivist.yaml"), []byte(beeYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".paseka", "prompts", "archivist.md"), []byte("Archivist."), 0o644); err != nil {
		t.Fatal(err)
	}

	d := NewDispatcher()
	prepared, err := d.prepareDispatch(context.Background(), DispatchRequest{
		ColonyRoot: root,
		Bee:        "archivist",
		TraceID:    "trace-normalized-intent",
		Intent:     "not-a-known-intent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.intent != "task" {
		t.Fatalf("normalized intent = %q, want the role default task", prepared.intent)
	}

	dir := newContractTestRunDir(t)
	result := &adapters.RunResult{Status: string(protocol.StatusCompleted)}
	if err := d.enforceCompletionContract(dir, prepared.bee, prepared.intent, []protocol.Event{
		contractInsightEvent(t, protocol.InsightKind("skill.proposed")),
	}, nil, result); err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusFailed) {
		t.Fatalf("status = %q, want failed under the default note contract", result.Status)
	}
	if result.Contract == nil || result.Contract.Intent != "task" {
		t.Fatalf("contract = %+v, want the normalized default intent", result.Contract)
	}
	if len(result.Contract.Rules) != 1 || result.Contract.Rules[0].KindOneOf[0] != "context.note" {
		t.Fatalf("contract rules = %+v, want the default note rule", result.Contract.Rules)
	}
}

func TestEnforceCompletionContractUnavailableFailsRun(t *testing.T) {
	d := NewDispatcher()
	dir := newContractTestRunDir(t)
	result := &adapters.RunResult{Status: string(protocol.StatusCompleted)}
	readErr := os.ErrInvalid
	if err := d.enforceCompletionContract(dir, contractTestBee(), "", nil, readErr, result); err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusFailed) || result.Reason != protocol.ReasonCompletionContract {
		t.Fatalf("result = %s/%s, want failed/%s", result.Status, result.Reason, protocol.ReasonCompletionContract)
	}
	if result.Contract == nil || result.Contract.Status != protocol.ContractUnavailable || result.Contract.Error == "" {
		t.Fatalf("contract = %+v, want unavailable with error", result.Contract)
	}
}

func TestEnforceCompletionContractKeepsGateReason(t *testing.T) {
	d := NewDispatcher()
	dir := newContractTestRunDir(t)
	result := &adapters.RunResult{
		Status: string(protocol.StatusFailed),
		Reason: protocol.ReasonChecksFailed,
	}
	if err := d.enforceCompletionContract(dir, contractTestBee(), "", nil, nil, result); err != nil {
		t.Fatal(err)
	}
	if result.Reason != protocol.ReasonChecksFailed {
		t.Fatalf("reason = %q, want %q", result.Reason, protocol.ReasonChecksFailed)
	}
	if result.Contract == nil || result.Contract.Status != protocol.ContractFailed {
		t.Fatalf("contract = %+v, want failed observation recorded", result.Contract)
	}
	snap, err := dir.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Reason == protocol.ReasonCompletionContract {
		t.Fatal("contract relabelled a gate rejection reason")
	}
}

func TestEnforceCompletionContractUnavailableKeepsPrimaryFailure(t *testing.T) {
	d := NewDispatcher()
	dir := newContractTestRunDir(t)
	if err := dir.WriteStatus(protocol.StatusFailed, protocol.ReasonChecksFailed, 0, time.Time{}, time.Time{}, ""); err != nil {
		t.Fatal(err)
	}
	if err := dir.WriteResult(protocol.Result{
		ProtocolVersion: protocol.Version,
		TraceID:         dir.TraceID,
		AgentID:         dir.AgentID,
		Status:          protocol.StatusFailed,
		Reason:          protocol.ReasonChecksFailed,
	}); err != nil {
		t.Fatal(err)
	}
	result := &adapters.RunResult{
		Status: string(protocol.StatusFailed),
		Reason: protocol.ReasonChecksFailed,
	}
	if err := d.enforceCompletionContract(dir, contractTestBee(), "", nil, os.ErrInvalid, result); err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusFailed) || result.Reason != protocol.ReasonChecksFailed {
		t.Fatalf("result = %s/%s, want failed/%s", result.Status, result.Reason, protocol.ReasonChecksFailed)
	}
	if result.Contract == nil || result.Contract.Status != protocol.ContractUnavailable || result.Contract.Error == "" {
		t.Fatalf("contract = %+v, want unavailable with error", result.Contract)
	}
	res, err := dir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != protocol.ReasonChecksFailed {
		t.Fatalf("recorded reason = %q, want %q", res.Reason, protocol.ReasonChecksFailed)
	}
	if res.Contract == nil || res.Contract.Status != protocol.ContractUnavailable {
		t.Fatalf("recorded contract = %+v, want unavailable", res.Contract)
	}
}

func TestEnforceCompletionContractNoContractWithUnreadableEvents(t *testing.T) {
	d := NewDispatcher()
	dir := newContractTestRunDir(t)
	result := &adapters.RunResult{Status: string(protocol.StatusCompleted)}
	if err := d.enforceCompletionContract(dir, colony.Bee{Role: "guard"}, "", nil, os.ErrInvalid, result); err != nil {
		t.Fatal(err)
	}
	if result.Contract != nil {
		t.Fatalf("contract = %+v, want nil", result.Contract)
	}
	if result.Status != string(protocol.StatusCompleted) {
		t.Fatalf("status = %q, want completed even with an unreadable event record", result.Status)
	}
	res, err := dir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if res.Contract != nil {
		t.Fatalf("recorded contract = %+v, want nil", res.Contract)
	}
}

func TestEnforceCompletionContractNoContractRecordsNothing(t *testing.T) {
	d := NewDispatcher()
	dir := newContractTestRunDir(t)
	result := &adapters.RunResult{Status: string(protocol.StatusCompleted)}
	if err := d.enforceCompletionContract(dir, colony.Bee{Role: "guard"}, "", nil, nil, result); err != nil {
		t.Fatal(err)
	}
	if result.Contract != nil {
		t.Fatalf("contract = %+v, want nil", result.Contract)
	}
	res, err := dir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if res.Contract != nil {
		t.Fatalf("recorded contract = %+v, want nil", res.Contract)
	}
}
