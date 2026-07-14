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

// MatchInviteCompletion reports whether ev matches the colony invite-completion rule.
func MatchInviteCompletion(ev protocol.Event, rule colony.InviteCompletionRule) bool {
	kind := protocol.PayloadKind(ev.Payload)
	if !rule.When.Matches(ev.Type, kind) {
		return false
	}
	if len(rule.Match) == 0 {
		return true
	}
	payload, err := payloadMap(ev.Payload)
	if err != nil {
		return false
	}
	for key, want := range rule.Match {
		got, _ := payloadString(payload, key)
		if strings.TrimSpace(got) != strings.TrimSpace(want) {
			return false
		}
	}
	return true
}

// CompleteFromEvent applies matching invite_completion rules to update invite status.
// Returns the published status event (if any) and true when an invite was updated.
func (s *Service) CompleteFromEvent(ctx context.Context, ev protocol.Event, rules []colony.InviteCompletionRule) (protocol.Event, bool, error) {
	traceID := strings.TrimSpace(ev.TraceID)
	if traceID == "" || len(rules) == 0 {
		return protocol.Event{}, false, nil
	}

	payload, err := payloadMap(ev.Payload)
	if err != nil {
		return protocol.Event{}, false, nil
	}

	for _, rule := range rules {
		if !MatchInviteCompletion(ev, rule) {
			continue
		}
		refField := strings.TrimSpace(rule.RequireFile.From)
		ref, _ := payloadString(payload, refField)
		ref = strings.TrimSpace(ref)

		invite, ok, err := findMatchingInvite(s.Colony.Slug, traceID, rule.MatchInvite)
		if err != nil {
			return protocol.Event{}, false, err
		}
		if !ok {
			continue
		}

		status := colony.InviteStatusIncomplete
		if ref != "" && artifactRefExists(s.Colony.ColonyRoot, traceID, ref) {
			status = colony.InviteStatusCompleted
			if from := strings.TrimSpace(rule.SetArtifactRef.From); from != "" {
				if artifactRef, ok := payloadString(payload, from); ok {
					invite.ArtifactRef = strings.TrimSpace(artifactRef)
				}
			}
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
	return protocol.Event{}, false, nil
}

func findMatchingInvite(slug, traceID string, match colony.InviteMatchSpec) (colony.InviteEntry, bool, error) {
	entries, err := colony.ListInvites(slug, "", traceID)
	if err != nil {
		return colony.InviteEntry{}, false, err
	}
	wantIntent := strings.TrimSpace(match.Intent)
	wantBee := strings.TrimSpace(match.Bee)
	for _, inv := range entries {
		switch inv.Status {
		case colony.InviteStatusAccepted, colony.InviteStatusIncomplete:
		default:
			continue
		}
		if wantIntent != "" && strings.TrimSpace(inv.Intent) != wantIntent {
			continue
		}
		if wantBee != "" && strings.TrimSpace(inv.Bee) != wantBee {
			continue
		}
		return inv, true, nil
	}
	return colony.InviteEntry{}, false, nil
}

func artifactRefExists(colonyRoot, traceID, ref string) bool {
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
		Kind:        protocol.SignalSessionInvite,
		InviteID:    invite.InviteID,
		Bee:         invite.Bee,
		Intent:      invite.Intent,
		Task:        invite.Task,
		Status:      protocol.InviteStatus(invite.Status),
		ArtifactRef: invite.ArtifactRef,
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

func marshalInvitePayload(payload protocol.SessionInvitePayload) ([]byte, error) {
	payload.Kind = protocol.SignalSessionInvite
	return json.Marshal(payload)
}
