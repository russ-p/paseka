package bus

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

// Deferred deny-list kinds are live-only and cannot be queued with --defer.
var deferredDenyKinds = map[string]struct{}{
	"system.kill":     {},
	"energy.add":      {},
	"energy.consume":  {},
	"session.invite":  {},
	"beekeeper.ready": {},
	"task.status":     {},
}

// FlushResult summarizes a pending queue flush or discard.
type FlushResult struct {
	OK        bool                        `json:"ok"`
	TraceID   string                      `json:"traceId,omitempty"`
	AgentID   string                      `json:"agentId,omitempty"`
	Published int                         `json:"published,omitempty"`
	Discarded int                         `json:"discarded,omitempty"`
	Error     string                      `json:"error,omitempty"`
	Details   []protocol.ValidationDetail `json:"details,omitempty"`
}

// PendingInspectResult reports queued event count and kinds for a run.
type PendingInspectResult struct {
	OK      bool     `json:"ok"`
	TraceID string   `json:"traceId"`
	AgentID string   `json:"agentId"`
	Count   int      `json:"count"`
	Kinds   []string `json:"kinds"`
}

// IsDeferDenied reports whether kind cannot be deferred.
func IsDeferDenied(kind string) bool {
	_, ok := deferredDenyKinds[strings.TrimSpace(kind)]
	return ok
}

// InspectPending returns pending count and kinds for a run.
func InspectPending(colonyRoot, traceID, agentID string) (PendingInspectResult, error) {
	runDir := runs.Dir{ColonyRoot: colonyRoot, TraceID: traceID, AgentID: agentID}
	if _, err := os.Stat(runDir.Root()); err != nil {
		return PendingInspectResult{}, fmt.Errorf("run directory not found for trace %s agent %s: %w", traceID, agentID, err)
	}
	summary, err := runDir.PendingSummary()
	if err != nil {
		return PendingInspectResult{}, err
	}
	return PendingInspectResult{
		OK:      true,
		TraceID: traceID,
		AgentID: agentID,
		Count:   summary.Count,
		Kinds:   summary.Kinds,
	}, nil
}

// FlushPending publishes queued events FIFO to the bus and audit log, or discards without publish.
func FlushPending(ctx context.Context, pub Publisher, colonyRoot, traceID, agentID string, discard bool) (FlushResult, error) {
	runDir := runs.Dir{ColonyRoot: colonyRoot, TraceID: traceID, AgentID: agentID}
	if _, err := os.Stat(runDir.Root()); err != nil {
		return FlushResult{
			OK:      false,
			TraceID: traceID,
			AgentID: agentID,
			Error:   "run_not_found",
			Details: []protocol.ValidationDetail{{
				Path:    "",
				Message: fmt.Sprintf("run directory not found for trace %s agent %s", traceID, agentID),
			}},
		}, nil
	}

	pending, err := runDir.ReadPending()
	if err != nil {
		return FlushResult{}, err
	}
	if len(pending) == 0 {
		return FlushResult{OK: true, TraceID: traceID, AgentID: agentID}, nil
	}

	if discard {
		if err := runDir.ClearPending(); err != nil {
			return FlushResult{}, err
		}
		return FlushResult{
			OK:        true,
			TraceID:   traceID,
			AgentID:   agentID,
			Discarded: len(pending),
		}, nil
	}

	if pub == nil {
		return FlushResult{
			OK:      false,
			TraceID: traceID,
			AgentID: agentID,
			Error:   "nats_not_configured",
			Details: []protocol.ValidationDetail{{
				Path:    "",
				Message: "NATS URL is not configured for this colony",
			}},
		}, nil
	}

	published := 0
	for len(pending) > 0 {
		ev := pending[0]
		if err := validateEventFileRefs(colonyRoot, traceID, ev); err != nil {
			return FlushResult{
				OK:        false,
				TraceID:   traceID,
				AgentID:   agentID,
				Published: published,
				Error:     "file_ref_missing",
				Details: []protocol.ValidationDetail{{
					Path:    "",
					Message: err.Error(),
				}},
			}, nil
		}
		if err := pub.PublishEvent(ctx, ev); err != nil {
			return FlushResult{
				OK:        false,
				TraceID:   traceID,
				AgentID:   agentID,
				Published: published,
				Error:     "publish_failed",
				Details: []protocol.ValidationDetail{{
					Path:    "",
					Message: err.Error(),
				}},
			}, nil
		}
		if _, err := appendRunAuditLog(colonyRoot, ev); err != nil {
			return FlushResult{
				OK:        false,
				TraceID:   traceID,
				AgentID:   agentID,
				Published: published,
				Error:     "audit_log_failed",
				Details: []protocol.ValidationDetail{{
					Path:    "",
					Message: err.Error(),
				}},
			}, nil
		}
		published++
		pending = pending[1:]
		if err := runDir.WritePending(pending); err != nil {
			return FlushResult{}, err
		}
	}

	return FlushResult{
		OK:        true,
		TraceID:   traceID,
		AgentID:   agentID,
		Published: published,
	}, nil
}

func deferEvent(colonyRoot string, ev protocol.Event) (string, error) {
	runDir := runs.Dir{
		ColonyRoot: colonyRoot,
		TraceID:    ev.TraceID,
		AgentID:    ev.AgentID,
	}
	if _, err := os.Stat(runDir.Root()); err != nil {
		return "", fmt.Errorf("run directory not found for trace %s agent %s: %w", ev.TraceID, ev.AgentID, err)
	}
	if err := runDir.AppendPending(ev); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(runDir.PendingPath())
	if err != nil {
		return runDir.PendingPath(), nil
	}
	return abs, nil
}

func validateEventFileRefs(colonyRoot, traceID string, ev protocol.Event) error {
	kind := protocol.PayloadKind(ev.Payload)
	switch kind {
	case "spec.ready":
		ref, err := payloadRef(ev.Payload, "ref")
		if err != nil {
			return err
		}
		if ref == "" {
			return fmt.Errorf("%s requires payload.ref", kind)
		}
		if !eventRefExists(colonyRoot, traceID, ref) {
			return fmt.Errorf("file not found for %s ref %q", kind, ref)
		}
	case "artifact.written":
		refs, err := protocol.ArtifactWrittenRefs(ev.Payload)
		if err != nil {
			return err
		}
		if len(refs) == 0 {
			return fmt.Errorf("artifact.written requires at least one ref")
		}
		for _, ref := range refs {
			if !eventRefExists(colonyRoot, traceID, ref) {
				return fmt.Errorf("file not found for artifact.written ref %q", ref)
			}
		}
	}
	return nil
}

func payloadRef(payload json.RawMessage, field string) (string, error) {
	var meta map[string]json.RawMessage
	if err := json.Unmarshal(payload, &meta); err != nil {
		return "", fmt.Errorf("invalid payload")
	}
	raw, ok := meta[field]
	if !ok {
		return "", nil
	}
	var ref string
	if err := json.Unmarshal(raw, &ref); err != nil {
		return "", fmt.Errorf("invalid payload.%s", field)
	}
	return strings.TrimSpace(ref), nil
}

func eventRefExists(colonyRoot, traceID, ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.TrimSpace(colonyRoot) == "" {
		return false
	}
	candidates := []string{
		filepath.Join(colonyRoot, ref),
		colony.PasekaPath(colonyRoot, "worktrees", traceID, ref),
		colony.PasekaPath(colonyRoot, "runs", traceID, "artifacts", ref),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	// Also accept repo-relative comb refs.
	if strings.Contains(ref, "/artifacts/") {
		if _, err := os.Stat(filepath.Join(colonyRoot, filepath.FromSlash(ref))); err == nil {
			return true
		}
	}
	return false
}

// PendingWarning returns a human-readable warning when a run ends with queued events.
func PendingWarning(colonyRoot, traceID, agentID string) (string, bool) {
	runDir := runs.Dir{ColonyRoot: colonyRoot, TraceID: traceID, AgentID: agentID}
	summary, err := runDir.PendingSummary()
	if err != nil || summary.Count == 0 {
		return "", false
	}
	return fmt.Sprintf("run has %d deferred event(s) not flushed; inspect with `paseka event pending` or flush manually", summary.Count), true
}

// FlushPendingForRun connects to NATS when configured and flushes the pending queue.
func FlushPendingForRun(ctx context.Context, ctxColony colony.Context, traceID, agentID string) error {
	client, err := ConnectColony(ctxColony, false)
	if err != nil {
		return err
	}
	if client != nil {
		defer client.Close()
	}
	var pub Publisher = NopPublisher{}
	if client != nil {
		pub = client
	}
	result, err := FlushPending(ctx, pub, ctxColony.ColonyRoot, traceID, agentID, false)
	if err != nil {
		return err
	}
	if !result.OK {
		if result.Error != "" {
			return fmt.Errorf("flush pending: %s", result.Error)
		}
		return fmt.Errorf("flush pending failed")
	}
	return nil
}
