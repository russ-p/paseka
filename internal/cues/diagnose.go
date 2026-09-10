package cues

import (
	"fmt"
	"sort"
	"strings"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

// StandingDiagnosis is doctor findings for Standing Trail wiring and smells.
type StandingDiagnosis struct {
	Warnings []string
}

// DiagnoseStanding reports standing-cue wiring smells (warnings only).
// Isolated code.proposal on a standing trail, and standing SIGNAL kinds whose
// only direct subscribers use worktree: true, are colony smells — not load errors.
func DiagnoseStanding(colonyRoot string, bees map[string]colony.Bee) StandingDiagnosis {
	var diag StandingDiagnosis
	if bees == nil {
		bees = map[string]colony.Bee{}
	}
	summaries, err := List(colonyRoot)
	if err != nil {
		return diag
	}
	seenTrails := map[string]struct{}{}
	for _, sum := range summaries {
		if strings.TrimSpace(sum.StandingTrace) == "" {
			continue
		}
		cue, err := Load(colonyRoot, sum.ID)
		if err != nil {
			continue
		}
		diag.Warnings = append(diag.Warnings, standingWorktreeWarnings(cue, bees)...)
		diag.Warnings = append(diag.Warnings, standingIsolatedPublishWarnings(cue, bees)...)
		if _, ok := seenTrails[cue.StandingTrace]; ok {
			continue
		}
		seenTrails[cue.StandingTrace] = struct{}{}
		if warn, ok := standingTrailIsolatedProposal(colonyRoot, cue.StandingTrace); ok {
			diag.Warnings = append(diag.Warnings, warn)
		}
	}
	sort.Strings(diag.Warnings)
	return diag
}

func standingWorktreeWarnings(cue Cue, bees map[string]colony.Bee) []string {
	if cue.Emit != EmitSignal {
		return nil
	}
	roles := standingDirectSubscribers(cue, bees)
	if len(roles) == 0 {
		return nil
	}
	var isolated []string
	for _, role := range roles {
		bee, ok := bees[role]
		if !ok || !bee.Worktree {
			return nil
		}
		isolated = append(isolated, role)
	}
	sort.Strings(isolated)
	kind := strings.TrimSpace(cue.SignalKind)
	return []string{fmt.Sprintf(
		"cue %q: standing SIGNAL/%s has only worktree:true subscribers (%s); standing ticks should observe on colony root",
		cue.ID, kind, strings.Join(isolated, ", "),
	)}
}

func standingIsolatedPublishWarnings(cue Cue, bees map[string]colony.Bee) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, role := range standingTickBeeRoles(cue, bees) {
		if _, dup := seen[role]; dup {
			continue
		}
		seen[role] = struct{}{}
		bee, ok := bees[role]
		if !ok || !bee.DeclaresCodeProposalIsolated() {
			continue
		}
		out = append(out, fmt.Sprintf(
			"cue %q: standing: bee %q publishes isolated code.proposal; standing identity should stay observational (spawn a bloom trail)",
			cue.ID, role,
		))
	}
	return out
}

func standingTickBeeRoles(cue Cue, bees map[string]colony.Bee) []string {
	if cue.Emit == EmitTask {
		role := strings.TrimSpace(cue.Bee)
		if role == "" {
			return nil
		}
		return []string{role}
	}
	return standingDirectSubscribers(cue, bees)
}

func standingDirectSubscribers(cue Cue, bees map[string]colony.Bee) []string {
	typ := protocol.EventType(strings.TrimSpace(cue.SignalType))
	if typ == "" {
		typ = protocol.EventSignal
	}
	roles := colony.DirectSubscribers(bees, typ, cue.SignalKind)
	sort.Strings(roles)
	return roles
}

func standingTrailIsolatedProposal(colonyRoot, traceID string) (string, bool) {
	events, err := runs.ReadTraceEvents(colonyRoot, traceID)
	if err != nil || len(events) == 0 {
		return "", false
	}
	for _, ev := range events {
		if ev.Type != protocol.EventMutation {
			continue
		}
		kind := protocol.PayloadKind(ev.Payload)
		if protocol.CodeProposalKindsMatch(kind, string(protocol.MutationCodeProposalIsolated)) {
			return fmt.Sprintf(
				"standing trail %q has isolated code.proposal; treat as a colony smell (spawn a bloom trail, do not merge on the procedure)",
				traceID,
			), true
		}
	}
	return "", false
}
