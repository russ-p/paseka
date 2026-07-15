package invites

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/ids"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/sessions"
)

// EventPublisher publishes protocol events (satisfied by bus.Client).
type EventPublisher interface {
	PublishEvent(context.Context, protocol.Event) error
}

// Service coordinates invite persistence, bus publish, and session launch.
type Service struct {
	Colony    colony.Context
	Bus       *bus.Client
	Publisher EventPublisher
	Sessions  *sessions.Manager
}

// AcceptResult summarizes an accepted invite.
type AcceptResult struct {
	Invite    colony.InviteEntry
	SessionID string
	TraceID   string
}

// RecordInput upserts a pending invite from a validated session.invite payload.
type RecordInput struct {
	TraceID string
	Payload protocol.SessionInvitePayload
}

// Accept starts an interactive session for a pending invite.
func (s *Service) Accept(ctx context.Context, inviteID string, attach bool) (*AcceptResult, error) {
	invite, err := colony.FindInvite(s.Colony.Slug, inviteID)
	if err != nil {
		return nil, err
	}
	if invite.Status != colony.InviteStatusPending {
		return nil, fmt.Errorf("invites: invite %q is %s, not pending", inviteID, invite.Status)
	}

	if err := s.consumeSessionEnergy(ctx, invite.TraceID); err != nil {
		return nil, err
	}

	if err := s.publishReady(ctx, invite, protocol.BeekeeperAccept); err != nil {
		return nil, err
	}

	mgr := s.Sessions
	if mgr == nil {
		mgr = sessions.DefaultManager
	}
	runReq := sessions.RunRequest{
		StartDir: s.Colony.ColonyRoot,
		Bee:      invite.Bee,
		TraceID:  invite.TraceID,
		Task:     invite.Task,
		Intent:   invite.Intent,
	}

	var res *sessions.RunResult
	if attach {
		res, err = mgr.RunInteractive(ctx, runReq)
	} else {
		res, err = mgr.StartDetached(ctx, runReq)
	}
	if err != nil {
		return nil, err
	}

	invite.Status = colony.InviteStatusAccepted
	invite.SessionID = res.SessionID
	invite.UpdatedAt = time.Now().UTC()
	if err := colony.UpsertInvite(s.Colony.Slug, invite); err != nil {
		return nil, err
	}

	return &AcceptResult{
		Invite:    invite,
		SessionID: res.SessionID,
		TraceID:   res.TraceID,
	}, nil
}

// Reject cancels or defers a pending invite.
func (s *Service) Reject(ctx context.Context, inviteID string, deferInvite bool) (colony.InviteEntry, error) {
	invite, err := colony.FindInvite(s.Colony.Slug, inviteID)
	if err != nil {
		return colony.InviteEntry{}, err
	}
	if invite.Status != colony.InviteStatusPending {
		return colony.InviteEntry{}, fmt.Errorf("invites: invite %q is %s, not pending", inviteID, invite.Status)
	}

	action := protocol.BeekeeperReject
	status := colony.InviteStatusCancelled
	if deferInvite {
		action = protocol.BeekeeperDefer
		status = colony.InviteStatusDeferred
	}
	if err := s.publishReady(ctx, invite, action); err != nil {
		return colony.InviteEntry{}, err
	}

	invite.Status = status
	invite.UpdatedAt = time.Now().UTC()
	if err := colony.UpsertInvite(s.Colony.Slug, invite); err != nil {
		return colony.InviteEntry{}, err
	}
	return invite, nil
}

// Record upserts a pending invite from validated payload fields.
func (s *Service) Record(in RecordInput) (colony.InviteEntry, error) {
	traceID := strings.TrimSpace(in.TraceID)
	if traceID == "" {
		return colony.InviteEntry{}, fmt.Errorf("invites: traceId is required")
	}
	payload := in.Payload
	payload.Kind = protocol.SignalSessionInvite
	if strings.TrimSpace(payload.InviteID) == "" {
		id, err := ids.Prefixed("inv-")
		if err != nil {
			return colony.InviteEntry{}, err
		}
		payload.InviteID = id
	}
	if payload.Status == "" {
		payload.Status = protocol.InviteStatusPending
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return colony.InviteEntry{}, err
	}
	eventInput := protocol.EventInput{
		TraceID: traceID,
		Type:    protocol.EventSignal,
		Payload: raw,
	}
	if details := eventInput.Validate(); len(details) > 0 {
		return colony.InviteEntry{}, &protocol.ValidationError{
			Code:    "schema_validation_failed",
			Details: details,
		}
	}

	entry := entryFromPayload(traceID, payload)
	if err := colony.UpsertInvite(s.Colony.Slug, entry); err != nil {
		return colony.InviteEntry{}, err
	}
	return entry, nil
}

// PublishPending validates, persists, and publishes a pending session.invite.
func (s *Service) PublishPending(ctx context.Context, traceID string, payload protocol.SessionInvitePayload) (protocol.Event, error) {
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return protocol.Event{}, fmt.Errorf("invites: traceId is required")
	}
	payload.Kind = protocol.SignalSessionInvite
	if strings.TrimSpace(payload.InviteID) == "" {
		id, err := ids.Prefixed("inv-")
		if err != nil {
			return protocol.Event{}, err
		}
		payload.InviteID = id
	}
	if payload.Status == "" {
		payload.Status = protocol.InviteStatusPending
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return protocol.Event{}, err
	}
	eventInput := protocol.EventInput{
		TraceID: traceID,
		AgentID: "runtime",
		Type:    protocol.EventSignal,
		Payload: raw,
	}
	if details := eventInput.Validate(); len(details) > 0 {
		return protocol.Event{}, &protocol.ValidationError{
			Code:    "schema_validation_failed",
			Details: details,
		}
	}
	ev, err := eventInput.ToEvent("runtime")
	if err != nil {
		return protocol.Event{}, err
	}

	entry := entryFromPayload(traceID, payload)
	if err := colony.UpsertInvite(s.Colony.Slug, entry); err != nil {
		return protocol.Event{}, err
	}

	pub := s.publisher()
	if pub == nil {
		return protocol.Event{}, fmt.Errorf("invites: nats url not configured")
	}
	if err := pub.PublishEvent(ctx, ev); err != nil {
		return protocol.Event{}, err
	}
	return ev, nil
}

// AutoInviteFromEvent evaluates colony auto_invite rules and publishes the first matching invite.
// Returns the published event and true when an invite was created.
func (s *Service) AutoInviteFromEvent(ctx context.Context, ev protocol.Event, rules []colony.AutoInviteRule, traceEvents []protocol.Event) (protocol.Event, bool, error) {
	if len(rules) == 0 {
		return protocol.Event{}, false, nil
	}
	traceID := strings.TrimSpace(ev.TraceID)
	if traceID == "" {
		return protocol.Event{}, false, nil
	}

	for _, rule := range rules {
		if !MatchAutoInvite(ev, rule) {
			continue
		}
		payload, err := BuildInvite(ev, rule, traceEvents)
		if err != nil {
			return protocol.Event{}, false, err
		}
		pending, err := colony.ListInvites(s.Colony.Slug, colony.InviteStatusPending, traceID)
		if err != nil {
			return protocol.Event{}, false, err
		}
		if HasPendingDedupe(pending, payload, rule.Dedupe) {
			return protocol.Event{}, false, nil
		}
		published, err := s.PublishPending(ctx, traceID, payload)
		if err != nil {
			return protocol.Event{}, false, err
		}
		return published, true, nil
	}
	return protocol.Event{}, false, nil
}

// ProjectEvent upserts invite state from a bus session.invite event.
func (s *Service) ProjectEvent(ev protocol.Event) error {
	if ev.Type != protocol.EventSignal || protocol.PayloadKind(ev.Payload) != string(protocol.SignalSessionInvite) {
		return nil
	}
	var payload protocol.SessionInvitePayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return fmt.Errorf("invites: parse session.invite: %w", err)
	}
	entry := entryFromPayload(ev.TraceID, payload)
	return colony.UpsertInvite(s.Colony.Slug, entry)
}

func (s *Service) publishReady(ctx context.Context, invite colony.InviteEntry, action protocol.BeekeeperAction) error {
	payload := protocol.BeekeeperReadyPayload{
		Kind:     protocol.SignalBeekeeperReady,
		InviteID: invite.InviteID,
		Action:   action,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	eventInput := protocol.EventInput{
		TraceID: invite.TraceID,
		AgentID: "beekeeper",
		Type:    protocol.EventSignal,
		Payload: raw,
	}
	if details := eventInput.Validate(); len(details) > 0 {
		return &protocol.ValidationError{Code: "schema_validation_failed", Details: details}
	}
	ev, err := eventInput.ToEvent("beekeeper")
	if err != nil {
		return err
	}

	pub := s.publisher()
	if pub == nil {
		var err error
		client, err := bus.ConnectColony(s.Colony, false)
		if err != nil {
			return err
		}
		if client == nil {
			return fmt.Errorf("invites: nats url not configured")
		}
		defer client.Close()
		pub = client
	}
	return pub.PublishEvent(ctx, ev)
}

func (s *Service) publisher() EventPublisher {
	if s.Publisher != nil {
		return s.Publisher
	}
	if s.Bus != nil {
		return s.Bus
	}
	return nil
}

func entryFromPayload(traceID string, payload protocol.SessionInvitePayload) colony.InviteEntry {
	status := string(payload.Status)
	if status == "" {
		status = colony.InviteStatusPending
	}
	return colony.InviteEntry{
		InviteID:    payload.InviteID,
		TraceID:     traceID,
		Bee:         payload.Bee,
		Intent:      payload.Intent,
		Task:        payload.Task,
		Status:      status,
		ArtifactRef: payload.ArtifactRef,
		DoneWhen:    doneWhenToColony(payload.DoneWhen),
	}
}
