package invites

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
)

// MatchAutoInvite reports whether ev matches the colony auto-invite rule.
func MatchAutoInvite(ev protocol.Event, rule colony.AutoInviteRule) bool {
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

// BuildInvite maps a triggering event to a session.invite payload using rule config.
func BuildInvite(ev protocol.Event, rule colony.AutoInviteRule, traceEvents []protocol.Event) (protocol.SessionInvitePayload, error) {
	payload, err := payloadMap(ev.Payload)
	if err != nil {
		return protocol.SessionInvitePayload{}, err
	}
	spec := rule.Invite

	bee := resolveStringField(spec.Bee, payload)
	if bee == "" {
		return protocol.SessionInvitePayload{}, fmt.Errorf("invites: auto_invite bee unresolved")
	}
	intent := resolveStringField(spec.Intent, payload)

	task, err := resolveTaskField(spec.Task, payload, traceEvents)
	if err != nil {
		return protocol.SessionInvitePayload{}, err
	}
	if task == "" {
		return protocol.SessionInvitePayload{}, fmt.Errorf("invites: auto_invite task unresolved")
	}

	status := protocol.InviteStatusPending
	if s := strings.TrimSpace(spec.Status); s != "" {
		status = protocol.InviteStatus(s)
	}

	out := protocol.SessionInvitePayload{
		Kind:   protocol.SignalSessionInvite,
		Bee:    bee,
		Intent: intent,
		Task:   task,
		Status: status,
	}
	if specRef := resolveStringField(spec.SpecRef, payload); specRef != "" {
		out.SpecRef = specRef
	}
	return out, nil
}

// HasPendingDedupe reports whether a pending invite already matches dedupe keys.
func HasPendingDedupe(invites []colony.InviteEntry, payload protocol.SessionInvitePayload, keys []string) bool {
	if len(keys) == 0 {
		return false
	}
	for _, inv := range invites {
		if inv.Status != colony.InviteStatusPending {
			continue
		}
		if inviteMatchesDedupe(inv, payload, keys) {
			return true
		}
	}
	return false
}

func inviteMatchesDedupe(inv colony.InviteEntry, payload protocol.SessionInvitePayload, keys []string) bool {
	for _, key := range keys {
		switch strings.TrimSpace(key) {
		case "bee":
			if strings.TrimSpace(inv.Bee) != strings.TrimSpace(payload.Bee) {
				return false
			}
		case "intent":
			if strings.TrimSpace(inv.Intent) != strings.TrimSpace(payload.Intent) {
				return false
			}
		case "specRef":
			if strings.TrimSpace(inv.SpecRef) != strings.TrimSpace(payload.SpecRef) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func resolveStringField(field colony.InviteStringField, payload map[string]any) string {
	if from := strings.TrimSpace(field.From); from != "" {
		if got, ok := payloadString(payload, from); ok && strings.TrimSpace(got) != "" {
			return strings.TrimSpace(got)
		}
	}
	return strings.TrimSpace(field.Default)
}

func resolveTaskField(field colony.InviteTaskField, payload map[string]any, traceEvents []protocol.Event) (string, error) {
	if kind := strings.TrimSpace(field.FromTraceKind); kind != "" {
		traceField := strings.TrimSpace(field.FromTraceField)
		if traceField == "" {
			return "", fmt.Errorf("invites: from_trace_field required with from_trace_kind")
		}
		if value, ok := latestTraceField(traceEvents, kind, traceField); ok {
			value = strings.TrimSpace(value)
			if value != "" {
				return field.Prefix + value, nil
			}
		}
	}
	if from := strings.TrimSpace(field.From); from != "" {
		if got, ok := payloadString(payload, from); ok && strings.TrimSpace(got) != "" {
			return strings.TrimSpace(got), nil
		}
	}
	if fallback := strings.TrimSpace(field.FallbackFrom); fallback != "" {
		if got, ok := payloadString(payload, fallback); ok && strings.TrimSpace(got) != "" {
			return strings.TrimSpace(got), nil
		}
	}
	return strings.TrimSpace(field.Default), nil
}

func latestTraceField(events []protocol.Event, kind, field string) (string, bool) {
	var value string
	found := false
	for _, ev := range events {
		if ev.Type != protocol.EventSignal || protocol.PayloadKind(ev.Payload) != kind {
			continue
		}
		payload, err := payloadMap(ev.Payload)
		if err != nil {
			continue
		}
		got, ok := payloadString(payload, field)
		if !ok {
			continue
		}
		value = got
		found = true
	}
	return value, found
}

func payloadMap(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("invites: empty payload")
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("invites: parse payload: %w", err)
	}
	return payload, nil
}

func payloadString(payload map[string]any, key string) (string, bool) {
	v, ok := payload[key]
	if !ok || v == nil {
		return "", false
	}
	switch s := v.(type) {
	case string:
		return s, true
	case json.Number:
		return s.String(), true
	default:
		return fmt.Sprint(v), true
	}
}
