package runtime

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/protocol"
)

const maxDirectTaskBody = 8000

// eventDispatchContext builds task text and taskId for direct bee dispatch from a bus event.
func eventDispatchContext(ev protocol.Event) (taskID, taskBody string, err error) {
	kind := protocol.PayloadKind(ev.Payload)
	switch ev.Type {
	case protocol.EventMutation:
		if !protocol.IsCodeProposalKind(kind) {
			return "", "", fmt.Errorf("runtime: unsupported mutation kind %q", kind)
		}
		var payload protocol.MutationPayload
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			return "", "", fmt.Errorf("runtime: parse mutation payload: %w", err)
		}
		taskID = payload.TaskID
		taskBody = formatMutationTask(payload)

	case protocol.EventVerification:
		switch kind {
		case string(protocol.VerificationSuccess):
			var payload protocol.VerificationPayload
			if err := json.Unmarshal(ev.Payload, &payload); err != nil {
				return "", "", fmt.Errorf("runtime: parse verification payload: %w", err)
			}
			taskID = payload.TaskID
			taskBody = formatVerificationSuccessTask(payload)
		case string(protocol.VerificationFailed):
			var payload protocol.VerificationPayload
			if err := json.Unmarshal(ev.Payload, &payload); err != nil {
				return "", "", fmt.Errorf("runtime: parse verification payload: %w", err)
			}
			taskID = payload.TaskID
			taskBody = formatVerificationFailedTask(payload)
		default:
			return "", "", fmt.Errorf("runtime: unsupported verification kind %q for direct dispatch", kind)
		}

	default:
		return "", "", fmt.Errorf("runtime: unsupported direct dispatch event type %s", ev.Type)
	}

	if strings.TrimSpace(taskBody) == "" {
		taskBody = fmt.Sprintf("Handle %s event (kind=%s)", ev.Type, kind)
	}
	return taskID, truncateString(taskBody, maxDirectTaskBody), nil
}

// proposalDispatchFields returns sector and normalized proposal kind for direct proposal dispatch.
func proposalDispatchFields(ev protocol.Event) (sector, proposalKind string) {
	if ev.Type != protocol.EventMutation {
		return "", ""
	}
	kind := protocol.PayloadKind(ev.Payload)
	if !protocol.IsCodeProposalKind(kind) {
		return "", ""
	}
	var payload protocol.MutationPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return "", kind
	}
	if payload.Sector != "" {
		sector = payload.Sector
	}
	if payload.Kind != "" {
		proposalKind = string(protocol.NormalizeCodeProposalKind(payload.Kind))
	} else {
		proposalKind = kind
	}
	return sector, proposalKind
}

func formatMutationTask(p protocol.MutationPayload) string {
	var b strings.Builder
	b.WriteString("Review the following code proposal")
	if p.Summary != "" {
		b.WriteString(": ")
		b.WriteString(p.Summary)
	}
	b.WriteByte('\n')
	if p.Diff != "" {
		b.WriteString("\n```diff\n")
		b.WriteString(p.Diff)
		b.WriteString("\n```\n")
	} else if p.Ref != "" {
		b.WriteString("\nArtifact ref: ")
		b.WriteString(p.Ref)
		b.WriteByte('\n')
	}
	return b.String()
}

func formatVerificationFailedTask(p protocol.VerificationPayload) string {
	var b strings.Builder
	b.WriteString("Address the failed verification")
	if p.Summary != "" {
		b.WriteString(": ")
		b.WriteString(p.Summary)
	}
	return b.String()
}

func formatVerificationSuccessTask(p protocol.VerificationPayload) string {
	var b strings.Builder
	b.WriteString("Verification passed — commit the approved changes")
	if p.Summary != "" {
		b.WriteString(": ")
		b.WriteString(p.Summary)
	}
	return b.String()
}

func truncateString(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n…(truncated)"
}

// directDispatchKey identifies a direct dispatch for deduplication.
// Rework-cycle gates (code.proposal family, verification.failed) key by event identity so
// each builder/guard pass can run again on the same task. Other task-scoped events
// collapse per traceId+taskId+bee+type+kind to ignore mistaken re-emits (e.g. echoed
// verification.success). Events without taskId always use the event fingerprint.
func directDispatchKey(ev protocol.Event, beeRole string) string {
	kind := protocol.PayloadKind(ev.Payload)
	if directDispatchPerEvent(ev.Type, kind) {
		return directEventFingerprint(ev) + "|" + beeRole
	}
	taskID := protocol.PayloadTaskID(ev.Payload)
	if taskID != "" {
		return fmt.Sprintf("%s|%s|%s|%s|%s", ev.TraceID, taskID, beeRole, ev.Type, kind)
	}
	return directEventFingerprint(ev) + "|" + beeRole
}

func directDispatchPerEvent(evType protocol.EventType, kind string) bool {
	switch evType {
	case protocol.EventMutation:
		return protocol.IsCodeProposalKind(kind)
	case protocol.EventVerification:
		return kind == string(protocol.VerificationFailed)
	default:
		return false
	}
}

func directEventFingerprint(ev protocol.Event) string {
	kind := protocol.PayloadKind(ev.Payload)
	if ev.Seq > 0 {
		return fmt.Sprintf("%s|%s|%s|%s|%d", ev.TraceID, ev.Type, kind, ev.AgentID, ev.Seq)
	}
	if !ev.CreatedAt.IsZero() {
		return fmt.Sprintf("%s|%s|%s|%s|%s", ev.TraceID, ev.Type, kind, ev.AgentID, ev.CreatedAt.UTC().Format(time.RFC3339Nano))
	}
	return fmt.Sprintf("%s|%s|%s|%s|%s", ev.TraceID, ev.Type, kind, ev.AgentID, string(ev.Payload))
}
