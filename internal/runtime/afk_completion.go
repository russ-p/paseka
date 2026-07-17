package runtime

import (
	"strings"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
)

func colonyDeclaresTaskCompleted(reg *BeeRegistry) bool {
	if reg == nil {
		return false
	}
	return colony.AnyBeeDeclaresPublish(reg.Bees(), protocol.EventVerification, string(protocol.TaskEventCompleted))
}

func runEmittedTaskCompleted(result *adapters.RunResult, taskID string) (protocol.Event, bool) {
	if result == nil {
		return protocol.Event{}, false
	}
	for _, ev := range result.Events {
		if ev.Type != protocol.EventVerification {
			continue
		}
		if protocol.PayloadKind(ev.Payload) != string(protocol.TaskEventCompleted) {
			continue
		}
		if taskID != "" {
			var payload protocol.TaskCompletedPayload
			if err := unmarshalPayload(ev.Payload, &payload); err == nil && payload.TaskID != "" && payload.TaskID != taskID {
				continue
			}
		}
		return ev, true
	}
	return protocol.Event{}, false
}

func runOpenedCodeProposal(reg *BeeRegistry, beeRole string, result *adapters.RunResult) bool {
	if result == nil {
		return false
	}
	if resultContainsIsolatedCodeProposal(result) {
		return true
	}
	if !hasNonEmptyDiffArtifact(result) {
		return false
	}
	if reg == nil {
		return false
	}
	bee, ok := reg.Bee(beeRole)
	if !ok {
		return false
	}
	return bee.DeclaresCodeProposalIsolated()
}

func resultContainsIsolatedCodeProposal(result *adapters.RunResult) bool {
	for _, ev := range result.Events {
		if ev.Type != protocol.EventMutation {
			continue
		}
		if protocol.CodeProposalKindsMatch(string(protocol.MutationCodeProposalIsolated), protocol.PayloadKind(ev.Payload)) {
			return true
		}
	}
	return false
}

func hasNonEmptyDiffArtifact(result *adapters.RunResult) bool {
	for _, a := range result.Artifacts {
		if a.Kind == "diff" && strings.TrimSpace(a.Content) != "" {
			return true
		}
	}
	return false
}

func shouldDeferAFKCompletion(reg *BeeRegistry, beeRole string, result *adapters.RunResult) bool {
	return colonyDeclaresTaskCompleted(reg) && runOpenedCodeProposal(reg, beeRole, result)
}
