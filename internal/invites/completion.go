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

// MatchDoneWhen reports whether ev matches an invite completion contract.
func MatchDoneWhen(ev protocol.Event, doneWhen colony.InviteDoneWhen) bool {
	kind := protocol.PayloadKind(ev.Payload)
	if !doneWhen.When.Matches(ev.Type, kind) {
		return false
	}
	if len(doneWhen.Match) == 0 {
		return true
	}
	payload, err := payloadMap(ev.Payload)
	if err != nil {
		return false
	}
	for key, want := range doneWhen.Match {
		got, _ := payloadString(payload, key)
		if strings.TrimSpace(got) != strings.TrimSpace(want) {
			return false
		}
	}
	return true
}

// CompleteFromEvent evaluates persisted invite done_when contracts on the trace.
// Returns the published status event (if any) and true when an invite was updated.
func (s *Service) CompleteFromEvent(ctx context.Context, ev protocol.Event) (protocol.Event, bool, error) {
	traceID := strings.TrimSpace(ev.TraceID)
	if traceID == "" {
		return protocol.Event{}, false, nil
	}

	payload, err := payloadMap(ev.Payload)
	if err != nil {
		return protocol.Event{}, false, nil
	}

	entries, err := colony.ListInvites(s.Colony.Slug, "", traceID)
	if err != nil {
		return protocol.Event{}, false, err
	}

	for _, invite := range entries {
		if invite.DoneWhen == nil {
			continue
		}
		switch invite.Status {
		case colony.InviteStatusAccepted, colony.InviteStatusIncomplete:
		default:
			continue
		}
		if !MatchDoneWhen(ev, *invite.DoneWhen) {
			continue
		}

		refField := strings.TrimSpace(invite.DoneWhen.RequireFile.From)
		ref, _ := payloadString(payload, refField)
		ref = strings.TrimSpace(ref)

		status := colony.InviteStatusIncomplete
		if ref != "" && artifactRefExists(s.Colony.ColonyRoot, traceID, ref) {
			status = colony.InviteStatusCompleted
			if from := strings.TrimSpace(invite.DoneWhen.SetArtifactRef.From); from != "" {
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
		DoneWhen:    doneWhenToProtocol(invite.DoneWhen),
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
