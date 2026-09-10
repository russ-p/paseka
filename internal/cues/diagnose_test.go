package cues_test

import (
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/cues"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

func TestDiagnoseStandingWarnsWorktreeOnlySubscribers(t *testing.T) {
	root := t.TempDir()
	writeCueFile(t, root, "daily-triage.yaml", standingSignalYAML)
	bees := map[string]colony.Bee{
		"watch": {
			Role:     "watch",
			Worktree: true,
			Subscribes: []colony.SubscriptionRule{
				{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "triage.tick"}, Dispatch: colony.DispatchDirect},
			},
		},
	}
	diag := cues.DiagnoseStanding(root, bees)
	if len(diag.Warnings) != 1 || !strings.Contains(diag.Warnings[0], "worktree:true") {
		t.Fatalf("warnings = %v", diag.Warnings)
	}
}

func TestDiagnoseStandingSkipsMixedWorktreeSubscribers(t *testing.T) {
	root := t.TempDir()
	writeCueFile(t, root, "daily-triage.yaml", standingSignalYAML)
	bees := map[string]colony.Bee{
		"watch": {
			Role:     "watch",
			Worktree: false,
			Subscribes: []colony.SubscriptionRule{
				{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "triage.tick"}, Dispatch: colony.DispatchDirect},
			},
		},
		"builder": {
			Role:     "builder",
			Worktree: true,
			Subscribes: []colony.SubscriptionRule{
				{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "triage.tick"}, Dispatch: colony.DispatchDirect},
			},
		},
	}
	diag := cues.DiagnoseStanding(root, bees)
	for _, w := range diag.Warnings {
		if strings.Contains(w, "worktree:true") {
			t.Fatalf("mixed topology should not warn: %v", diag.Warnings)
		}
	}
}

func TestDiagnoseStandingWarnsIsolatedProposalPublish(t *testing.T) {
	root := t.TempDir()
	writeCueFile(t, root, "daily-triage.yaml", standingSignalYAML)
	bees := map[string]colony.Bee{
		"watch": {
			Role:     "watch",
			Worktree: true,
			Subscribes: []colony.SubscriptionRule{
				{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "triage.tick"}, Dispatch: colony.DispatchDirect},
			},
			Publishes: []colony.PublicationRule{
				{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.isolated"}},
			},
		},
	}
	diag := cues.DiagnoseStanding(root, bees)
	found := false
	for _, w := range diag.Warnings {
		if strings.Contains(w, "isolated code.proposal") {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings = %v", diag.Warnings)
	}
}

func TestDiagnoseStandingWarnsIsolatedProposalOnTrail(t *testing.T) {
	root := t.TempDir()
	writeCueFile(t, root, "daily-triage.yaml", standingSignalYAML)
	d := runs.Dir{ColonyRoot: root, TraceID: "trail-daily-triage", AgentID: "watch"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	ev, err := protocol.NewEvent("trail-daily-triage", "watch", 1, protocol.EventMutation, protocol.MutationPayload{
		Kind:      protocol.MutationCodeProposalIsolated,
		TaskID:    "task-1",
		Workspace: protocol.ProposalWorkspaceIsolated,
		Diff:      "+line",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.AppendEvent(ev); err != nil {
		t.Fatal(err)
	}
	diag := cues.DiagnoseStanding(root, map[string]colony.Bee{})
	if len(diag.Warnings) != 1 || !strings.Contains(diag.Warnings[0], "standing trail") {
		t.Fatalf("warnings = %v", diag.Warnings)
	}
}

func TestDiagnoseStandingBloomCueSilent(t *testing.T) {
	root := t.TempDir()
	writeCueFile(t, root, "feature.yaml", `emit: signal
type: SIGNAL
kind: feature.requested
title: "{{.Title}}"
`)
	diag := cues.DiagnoseStanding(root, map[string]colony.Bee{
		"builder": {Role: "builder", Worktree: true},
	})
	if len(diag.Warnings) != 0 {
		t.Fatalf("warnings = %v", diag.Warnings)
	}
}
