package protocol

import (
	"fmt"
	"strings"
)

// ExecutionPurpose is the runtime-owned reason an execution was admitted.
//
// It is recorded at admission and carried by the run record. It is never
// derived from a Bee's name, a task title, or an agent's free-form output: a
// renamed Bee or an unexpectedly witty summary must not change what a run was
// for.
//
// The vocabulary is closed. Work and closing belong to the trail's product
// work; analysis studies a result without producing it. A record without a
// purpose predates this field and stays unknown: it is neither work nor
// analysis by inference.
type ExecutionPurpose string

const (
	// PurposeWork is ordinary goal work. It is the default for a newly admitted
	// execution that has no other recorded purpose.
	PurposeWork ExecutionPurpose = "work"
	// PurposeClosing is the runtime-seeded closing round. Closing is trail
	// overhead, not goal work.
	PurposeClosing ExecutionPurpose = "closing"
	// PurposeAnalysis studies an already-recorded result. Analysis stays in the
	// source trail but is accounted separately from work.
	PurposeAnalysis ExecutionPurpose = "analysis"
)

// IsKnownExecutionPurpose reports whether p is part of the closed purpose
// vocabulary. The empty value is not known: it means the record carries no
// attribution.
func IsKnownExecutionPurpose(p ExecutionPurpose) bool {
	switch p {
	case PurposeWork, PurposeClosing, PurposeAnalysis:
		return true
	default:
		return false
	}
}

// PurposeForTaskPhase maps a recorded task phase to the purpose of its
// execution. Closing follows the phase; ordinary work and a merge gate are
// work, because neither is a separately accounted round.
func PurposeForTaskPhase(phase TaskPhase) ExecutionPurpose {
	if NormalizeTaskPhase(phase) == TaskPhaseClosing {
		return PurposeClosing
	}
	return PurposeWork
}

// ResolveAdmissionAttribution validates an admission's purpose and the work
// milestone an analysis studies. An empty purpose is ordinary work for a newly
// admitted execution; an analysis without analysisOf is refused rather than
// recorded with an incomplete claim, and analysisOf without analysis is refused
// rather than stored as misleading provenance.
func ResolveAdmissionAttribution(purpose ExecutionPurpose, analysisOf string) (ExecutionPurpose, string, error) {
	if purpose == "" {
		purpose = PurposeWork
	}
	if !IsKnownExecutionPurpose(purpose) {
		return "", "", fmt.Errorf("protocol: unsupported execution purpose %q", purpose)
	}
	analysisOf = strings.TrimSpace(analysisOf)
	switch {
	case purpose == PurposeAnalysis && analysisOf == "":
		return "", "", fmt.Errorf("protocol: analysis purpose requires the studied milestone (analysisOf)")
	case purpose != PurposeAnalysis && analysisOf != "":
		return "", "", fmt.Errorf("protocol: analysisOf is only valid for analysis purpose")
	}
	return purpose, analysisOf, nil
}
