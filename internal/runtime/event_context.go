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
		if kind != string(protocol.MutationCodeProposal) {
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

func truncateString(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n…(truncated)"
}

// directDispatchKey identifies one bus event + bee role for deduplication.
func directDispatchKey(ev protocol.Event, beeRole string) string {
	return directEventFingerprint(ev) + ":" + beeRole
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
