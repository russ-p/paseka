package invites

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
)

const colonyKindSpecReady = "spec.ready"

const defaultGrillBee = "drone"

// CompleteFromSpecReady marks a grilling invite completed or incomplete from a spec.ready event.
// Returns the published status event (if any) and true when an invite was updated.
func (s *Service) CompleteFromSpecReady(ctx context.Context, ev protocol.Event) (protocol.Event, bool, error) {
	if ev.Type != protocol.EventSignal || protocol.PayloadKind(ev.Payload) != colonyKindSpecReady {
		return protocol.Event{}, false, nil
	}
	traceID := strings.TrimSpace(ev.TraceID)
	if traceID == "" {
		return protocol.Event{}, false, nil
	}

	ref, _ := payloadStringFromRaw(ev.Payload, "ref")
	ref = strings.TrimSpace(ref)

	invite, ok, err := findGrillingInvite(s.Colony.Slug, traceID)
	if err != nil {
		return protocol.Event{}, false, err
	}
	if !ok {
		return protocol.Event{}, false, nil
	}

	status := colony.InviteStatusIncomplete
	if ref != "" && specRefExists(s.Colony.ColonyRoot, traceID, ref) {
		status = colony.InviteStatusCompleted
		invite.SpecRef = ref
	}
	invite.Status = status
	invite.UpdatedAt = time.Now().UTC()
	if err := colony.UpsertInvite(s.Colony.Slug, invite); err != nil {
		return protocol.Event{}, false, err
	}
	published, err := s.publishInviteStatusEvent(ctx, invite)
	if err != nil {
		return protocol.Event{}, false, err
	}
	return published, true, nil
}

func findGrillingInvite(slug, traceID string) (colony.InviteEntry, bool, error) {
	entries, err := colony.ListInvites(slug, "", traceID)
	if err != nil {
		return colony.InviteEntry{}, false, err
	}
	for _, inv := range entries {
		if !isGrillingInvite(inv) {
			continue
		}
		switch inv.Status {
		case colony.InviteStatusAccepted, colony.InviteStatusIncomplete:
			return inv, true, nil
		}
	}
	return colony.InviteEntry{}, false, nil
}

func isGrillingInvite(inv colony.InviteEntry) bool {
	intent := strings.TrimSpace(inv.Intent)
	if intent == "grilling" {
		return true
	}
	if intent != "" {
		return false
	}
	bee := strings.TrimSpace(inv.Bee)
	if bee == "" {
		bee = defaultGrillBee
	}
	return bee == defaultGrillBee
}

func specRefExists(colonyRoot, traceID, ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.TrimSpace(colonyRoot) == "" {
		return false
	}
	candidates := []string{
		filepath.Join(colonyRoot, ref),
		colony.PasekaPath(colonyRoot, "worktrees", traceID, ref),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func (s *Service) publishInviteStatusEvent(ctx context.Context, invite colony.InviteEntry) (protocol.Event, error) {
	payload := protocol.SessionInvitePayload{
		Kind:     protocol.SignalSessionInvite,
		InviteID: invite.InviteID,
		Bee:      invite.Bee,
		Intent:   invite.Intent,
		Task:     invite.Task,
		Status:   protocol.InviteStatus(invite.Status),
		SpecRef:  invite.SpecRef,
	}
	raw, err := marshalInvitePayload(payload)
	if err != nil {
		return protocol.Event{}, err
	}
	eventInput := protocol.EventInput{
		TraceID: invite.TraceID,
		AgentID: "runtime",
		Type:    protocol.EventSignal,
		Payload: raw,
	}
	if details := eventInput.Validate(); len(details) > 0 {
		return protocol.Event{}, &protocol.ValidationError{Code: "schema_validation_failed", Details: details}
	}
	ev, err := eventInput.ToEvent("runtime")
	if err != nil {
		return protocol.Event{}, err
	}
	pub := s.publisher()
	if pub == nil {
		return protocol.Event{}, nil
	}
	if err := pub.PublishEvent(ctx, ev); err != nil {
		return protocol.Event{}, err
	}
	return ev, nil
}

func payloadStringFromRaw(raw []byte, key string) (string, bool) {
	payload, err := payloadMap(raw)
	if err != nil {
		return "", false
	}
	return payloadString(payload, key)
}

func marshalInvitePayload(payload protocol.SessionInvitePayload) ([]byte, error) {
	payload.Kind = protocol.SignalSessionInvite
	return json.Marshal(payload)
}
