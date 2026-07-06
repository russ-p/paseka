package runtime

import (
	"fmt"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
)

// BeeRegistry holds loaded bee configs and routing helpers for the reactor.
type BeeRegistry struct {
	bees map[string]colony.Bee
}

// BuildBeeRegistry loads all bee configs from the colony.
func BuildBeeRegistry(colonyRoot string) (*BeeRegistry, error) {
	bees, err := colony.LoadAllBees(colonyRoot)
	if err != nil {
		return nil, err
	}
	return NewBeeRegistryFromBees(bees), nil
}

// NewBeeRegistryFromBees builds a registry from an in-memory bee map (tests).
func NewBeeRegistryFromBees(bees map[string]colony.Bee) *BeeRegistry {
	return &BeeRegistry{bees: bees}
}

// Bee returns a bee config by role.
func (r *BeeRegistry) Bee(role string) (colony.Bee, bool) {
	b, ok := r.bees[role]
	return b, ok
}

// DirectSubscribers returns bee roles that should react directly to an event.
func (r *BeeRegistry) DirectSubscribers(ev protocol.Event) []string {
	kind := protocol.PayloadKind(ev.Payload)
	return colony.DirectSubscribers(r.bees, ev.Type, kind)
}

// CanDispatchTaskReady reports whether a bee may run task.ready dispatches.
func (r *BeeRegistry) CanDispatchTaskReady(role string) bool {
	bee, ok := r.bees[role]
	if !ok {
		return true
	}
	return bee.CanHandleTaskReady()
}

// ValidatePublish checks advisory publish declarations for a bee role.
func (r *BeeRegistry) ValidatePublish(role string, ev protocol.Event) (warning string, ok bool) {
	bee, ok := r.bees[role]
	if !ok || len(bee.Publishes) == 0 {
		return "", true
	}
	kind := protocol.PayloadKind(ev.Payload)
	if bee.DeclaresPublish(ev.Type, kind) {
		return "", true
	}
	return fmt.Sprintf("bee %q published undeclared event %s/%s", role, ev.Type, kind), false
}

// ShouldAutoPublishMutation reports whether runtime may synthesize MUTATION/code.proposal from git diff.
// Bees with a non-empty publishes list must declare the mutation; empty list keeps backward compatibility.
func (r *BeeRegistry) ShouldAutoPublishMutation(role string) bool {
	if r == nil {
		return true
	}
	bee, ok := r.bees[role]
	if !ok || len(bee.Publishes) == 0 {
		return true
	}
	return bee.DeclaresPublish(protocol.EventMutation, string(protocol.MutationCodeProposal))
}

// ShouldAutoPublishRunSummary reports whether runtime may synthesize INSIGHT/run.summary.
// Bees with run_summary=disabled never auto-publish. Bees with a non-empty publishes list
// must declare the insight kind; empty list keeps backward compatibility.
func (r *BeeRegistry) ShouldAutoPublishRunSummary(role string) bool {
	if r == nil {
		return true
	}
	bee, ok := r.bees[role]
	if !ok {
		return true
	}
	if bee.ResolvedRunSummaryPolicy() == colony.RunSummaryDisabled {
		return false
	}
	if len(bee.Publishes) == 0 {
		return true
	}
	return bee.DeclaresPublish(protocol.EventInsight, string(protocol.InsightRunSummary))
}
