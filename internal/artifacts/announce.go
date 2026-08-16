package artifacts

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

const (
	ProducerConsole = "console"
)

// WriteFile writes content to a comb file (creates parent dirs).
func WriteFile(colonyRoot, traceID, combRel string, content []byte) (string, error) {
	if _, err := EnsureDir(colonyRoot, traceID); err != nil {
		return "", err
	}
	abs, err := combWritePath(colonyRoot, traceID, combRel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if err := writeRegularFile(abs, Root(colonyRoot, traceID), content); err != nil {
		return "", err
	}
	rel, err := filepath.Rel(colonyRoot, abs)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func combWritePath(colonyRoot, traceID, combRel string) (string, error) {
	combRel = strings.TrimSpace(combRel)
	if combRel == "" {
		return "", fmt.Errorf("artifacts: ref is required")
	}
	combRoot, err := filepath.Abs(Root(colonyRoot, traceID))
	if err != nil {
		return "", err
	}
	norm := filepath.ToSlash(strings.TrimPrefix(combRel, "/"))
	prefix := ".paseka/runs/" + traceID + "/artifacts/"
	var rel string
	switch {
	case strings.HasPrefix(norm, prefix):
		rel = strings.TrimPrefix(norm, prefix)
	case strings.HasPrefix(norm, ".paseka/"):
		abs := filepath.Join(colonyRoot, filepath.FromSlash(norm))
		rel, err = filepath.Rel(combRoot, abs)
		if err != nil {
			return "", fmt.Errorf("artifacts: ref %q escapes comb", combRel)
		}
	default:
		rel = norm
	}
	rel = filepath.Clean(filepath.FromSlash(rel))
	if pathEscapes(combRoot, filepath.Join(combRoot, rel)) {
		return "", fmt.Errorf("artifacts: ref %q escapes comb", combRel)
	}
	return filepath.Join(combRoot, rel), nil
}

// BuildWrittenPayload constructs a normalized artifact.written payload from items.
func BuildWrittenPayload(items []Item) (json.RawMessage, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("artifacts: at least one item required")
	}
	artifacts := make([]protocol.ArtifactWrittenItem, 0, len(items))
	for _, item := range items {
		ref := strings.TrimSpace(item.Ref)
		if ref == "" {
			continue
		}
		kind := strings.TrimSpace(item.ArtifactKind)
		if kind == "" {
			kind = ArtifactKindFromRef(ref)
		}
		entry := protocol.ArtifactWrittenItem{
			Ref:          ref,
			ArtifactKind: kind,
		}
		if title := strings.TrimSpace(item.Title); title != "" {
			entry.Title = title
		}
		artifacts = append(artifacts, entry)
	}
	if len(artifacts) == 0 {
		return nil, fmt.Errorf("artifacts: at least one ref required")
	}
	payload := protocol.ArtifactWrittenPayload{
		Kind:      protocol.SignalArtifactWritten,
		Artifacts: artifacts,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// PublishWritten publishes SIGNAL/artifact.written and appends to the audit log when possible.
func PublishWritten(ctx context.Context, pub bus.Publisher, colonyRoot, subjectPrefix, traceID, agentID string, payload json.RawMessage) error {
	in := protocol.EventInput{
		TraceID: traceID,
		AgentID: agentID,
		Type:    protocol.EventSignal,
		Payload: payload,
	}
	if details := in.Validate(); len(details) > 0 {
		return fmt.Errorf("artifacts: publish: schema_validation_failed")
	}
	ev, err := in.ToEvent(agentID)
	if err != nil {
		return err
	}
	if pub == nil {
		pub = bus.NopPublisher{}
	}
	if err := pub.PublishEvent(ctx, ev); err != nil {
		return fmt.Errorf("artifacts: publish: %w", err)
	}
	if err := appendArtifactAudit(colonyRoot, traceID, agentID, ev); err != nil {
		return fmt.Errorf("artifacts: publish: audit_log_failed: %w", err)
	}
	_ = subjectPrefix
	return nil
}

func appendArtifactAudit(colonyRoot, traceID, agentID string, ev protocol.Event) error {
	runDir := runs.Dir{ColonyRoot: colonyRoot, TraceID: traceID, AgentID: agentID}
	if _, err := os.Stat(runDir.RequestPath()); err == nil {
		return runDir.AppendEvent(ev)
	}
	auditAgent := agentID
	if auditAgent == "" {
		auditAgent = ProducerConsole
	}
	humanDir := runs.Dir{ColonyRoot: colonyRoot, TraceID: traceID, AgentID: auditAgent}
	if err := os.MkdirAll(humanDir.Root(), 0o755); err != nil {
		return err
	}
	return humanDir.AppendEvent(ev)
}

// WriteAndAnnounce writes a comb file and immediately publishes artifact.written.
func WriteAndAnnounce(ctx context.Context, pub bus.Publisher, colonyRoot, subjectPrefix, traceID, combRel, producer string, content []byte) error {
	if strings.TrimSpace(producer) == "" {
		producer = ProducerConsole
	}
	ref, err := WriteFile(colonyRoot, traceID, combRel, content)
	if err != nil {
		return err
	}
	item, err := ItemFromFile(colonyRoot, traceID, combRel)
	if err != nil {
		return err
	}
	item.Ref = ref
	payload, err := BuildWrittenPayload([]Item{item})
	if err != nil {
		return err
	}
	return PublishWritten(ctx, pub, colonyRoot, subjectPrefix, traceID, producer, payload)
}

// FlushRunDelta scans the comb vs the run baseline and publishes one batched event.
func FlushRunDelta(ctx context.Context, pub bus.Publisher, colonyRoot, subjectPrefix, traceID, agentID string) error {
	baseline, ok, err := LoadBaseline(colonyRoot, traceID, agentID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	delta, err := ComputeDelta(colonyRoot, traceID, baseline)
	if err != nil {
		return err
	}
	items := append(delta.Added, delta.Changed...)
	if len(items) == 0 {
		return nil
	}
	payload, err := BuildWrittenPayload(items)
	if err != nil {
		return err
	}
	return PublishWritten(ctx, pub, colonyRoot, subjectPrefix, traceID, agentID, payload)
}

// RunHasArtifactWritten reports pending or flushed artifact.written for a run.
func RunHasArtifactWritten(colonyRoot, traceID, agentID string) (bool, error) {
	runDir := runs.Dir{ColonyRoot: colonyRoot, TraceID: traceID, AgentID: agentID}
	pending, err := runDir.ReadPending()
	if err != nil {
		return false, err
	}
	for _, ev := range pending {
		if ev.Type == protocol.EventSignal && protocol.PayloadKind(ev.Payload) == string(protocol.SignalArtifactWritten) {
			return true, nil
		}
	}
	events, err := runDir.ReadEvents()
	if err != nil {
		return false, err
	}
	for _, ev := range events {
		if ev.Type == protocol.EventSignal && protocol.PayloadKind(ev.Payload) == string(protocol.SignalArtifactWritten) {
			return true, nil
		}
	}
	return false, nil
}

// MergeAnnounced overlays bus announcement metadata onto filesystem items.
func MergeAnnounced(items []Item, events []protocol.Event) []Item {
	announced := map[string]struct {
		producer string
		at       int64
	}{}
	for _, ev := range events {
		if ev.Type != protocol.EventSignal || protocol.PayloadKind(ev.Payload) != string(protocol.SignalArtifactWritten) {
			continue
		}
		refs, err := protocol.ArtifactWrittenRefs(ev.Payload)
		if err != nil {
			continue
		}
		ts := ev.CreatedAt.UTC().Unix()
		for _, ref := range refs {
			announced[ref] = struct {
				producer string
				at       int64
			}{producer: ev.AgentID, at: ts}
		}
	}
	out := make([]Item, len(items))
	for i, item := range items {
		out[i] = item
		if meta, ok := announced[item.Ref]; ok {
			out[i].Announced = true
			out[i].Producer = meta.producer
			if meta.at > 0 {
				out[i].Updated = meta.at
			}
		}
	}
	return out
}
