package trailview

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/trailfork"
)

// Scope is the experimental-scope verdict a side-effecting consumer resolves
// before acting on an event.
//
// Fork events are ordinary trail events in v1: they use the same subjects and
// envelope as any other trail event and are identified only by traceId. A
// consumer that needs to know whether an event belongs to an experiment calls
// ResolveScope with that identity instead of looking for a fork-specific field
// or subject prefix.
//
// The verdict comes from the trail's retained configuration policy and fork
// lineage. It never reads the source trail's current state, so a later change to
// the source cannot reclassify the experiment.
type Scope struct {
	TraceID string `json:"traceId"`
	// ForkOf and ForkID are the recorded source trail and experiment identity.
	ForkOf string `json:"forkOf,omitempty"`
	ForkID string `json:"forkId,omitempty"`
	// SourceTask, SourceAttempt, and SourceConfigRef name the source evidence
	// the experiment inherited.
	SourceTask      string `json:"sourceTask,omitempty"`
	SourceAttempt   string `json:"sourceAttempt,omitempty"`
	SourceConfigRef string `json:"sourceConfigRef,omitempty"`
	// ConfigRef is the pinned candidate configuration when the trail is bound.
	ConfigRef string `json:"configRef,omitempty"`
	// Policy is the trail's configuration policy: editable or pinned.
	Policy string `json:"policy,omitempty"`
	// WorkspaceRoot is the experiment's execution checkout when recorded.
	WorkspaceRoot string `json:"workspaceRoot,omitempty"`
	// AdmissionState mirrors the shared collection: ordinary, runnable, pinned,
	// or a fork lineage state such as creating or dropped.
	AdmissionState string `json:"admissionState"`
	// Experimental is true for a pinned trail or a trail with fork lineage.
	// Work-producing and side-effecting built-ins must apply their
	// experimental-policy checks when this is true.
	Experimental bool `json:"experimental"`
	// Evidence names an inconsistency the consumer must not paper over, such
	// as a pinned trail whose lineage or configuration binding is missing.
	Evidence []string `json:"evidence,omitempty"`
}

// ResolveScope resolves one event's trail lineage for a built-in consumer.
//
// A missing lineage record is not an error: the trail is ordinary unless its
// immutable configuration policy says it is pinned. An unreadable or malformed
// manifest or lineage record is an error, because a consumer that cannot resolve
// scope must not guess whether an experiment is safe to act on.
func ResolveScope(ctx colony.Context, traceID string) (Scope, error) {
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return Scope{}, fmt.Errorf("trailview: trace id is required")
	}
	scope := Scope{TraceID: traceID, AdmissionState: AdmissionOrdinary}
	// An absent colony root cannot resolve lineage. Test fixtures and
	// read-only callers construct partial contexts; production composition
	// roots always carry the root.
	if strings.TrimSpace(ctx.ColonyRoot) == "" {
		return scope, nil
	}

	manifest, bound, err := trailconfig.LoadManifestIfExists(ctx.ColonyRoot, traceID)
	switch {
	case err != nil:
		return Scope{}, fmt.Errorf("trailview: inspect trail policy: %w", err)
	case bound:
		scope.ConfigRef = manifest.ActiveConfigRef
		scope.Policy = manifest.Policy
		scope.WorkspaceRoot = manifest.WorkspaceRoot
		scope.Experimental = manifest.Policy == trailconfig.PolicyPinned
	}

	lineage, err := trailfork.LoadLineage(ctx.ColonyRoot, traceID)
	switch {
	case err == nil:
		scope.Experimental = true
		scope.ForkOf = lineage.SourceTrace
		scope.ForkID = lineage.ForkID
		scope.SourceTask = lineage.SourceTask
		scope.SourceAttempt = lineage.SourceAttempt
		scope.SourceConfigRef = lineage.SourceConfigRef
		scope.AdmissionState = lineage.State
		if strings.TrimSpace(scope.AdmissionState) == "" {
			scope.AdmissionState = AdmissionRunnable
		}
		if !bound {
			scope.Evidence = append(scope.Evidence,
				"fork lineage exists but the experiment has no configuration binding")
		} else if lineage.ConfigRef != "" && lineage.ConfigRef != scope.ConfigRef {
			scope.Evidence = append(scope.Evidence,
				fmt.Sprintf("fork lineage candidate %s does not match active configuration %s",
					lineage.ConfigRef, scope.ConfigRef))
		}
	case errors.Is(err, os.ErrNotExist):
		if scope.Experimental {
			scope.AdmissionState = AdmissionPinned
			scope.Evidence = append(scope.Evidence,
				"pinned trail has no fork lineage record; source comparison is unavailable")
		}
	default:
		return Scope{}, fmt.Errorf("trailview: inspect fork lineage: %w", err)
	}
	return scope, nil
}
