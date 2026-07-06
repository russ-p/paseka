package bus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

// ProcessEventInput validates stdin JSON and optionally publishes to NATS.
// When publish is true and colonyRoot is set, the event is also appended to
// .paseka/runs/<traceId>/<agentId>/events.ndjson for the emitting agent run.
func ProcessEventInput(ctx context.Context, client *Client, raw []byte, defaultAgentID string, publish bool, colonyRoot string) (protocol.EventCLIResult, error) {
	in, err := protocol.ParseEventInput(raw)
	if err != nil {
		var verr *protocol.ValidationError
		if ok := asValidationError(err, &verr); ok {
			return protocol.EventCLIResult{
				OK:      false,
				Error:   verr.Code,
				Details: verr.Details,
			}, nil
		}
		return protocol.EventCLIResult{}, err
	}

	if details := in.Validate(); len(details) > 0 {
		return protocol.EventCLIResult{
			OK:      false,
			Error:   "schema_validation_failed",
			Details: details,
		}, nil
	}

	ev, err := in.ToEvent(defaultAgentID)
	if err != nil {
		var verr *protocol.ValidationError
		if ok := asValidationError(err, &verr); ok {
			return protocol.EventCLIResult{
				OK:      false,
				Error:   verr.Code,
				Details: verr.Details,
			}, nil
		}
		return protocol.EventCLIResult{}, err
	}

	kind := protocol.PayloadKind(ev.Payload)
	subject := ""
	if client != nil {
		subject = EventSubject(client.Config().SubjectPrefix, ev)
	}

	if !publish {
		return protocol.EventCLIResult{
			OK:      true,
			TraceID: ev.TraceID,
			Type:    ev.Type,
			Kind:    kind,
			Subject: subject,
		}, nil
	}

	if client == nil {
		return protocol.EventCLIResult{
			OK:    false,
			Error: "nats_not_configured",
			Details: []protocol.ValidationDetail{{
				Path:    "",
				Message: "NATS URL is not configured for this colony",
			}},
		}, nil
	}

	if err := client.PublishEvent(ctx, ev); err != nil {
		return protocol.EventCLIResult{
			OK:    false,
			Error: "publish_failed",
			Details: []protocol.ValidationDetail{{
				Path:    "",
				Message: err.Error(),
			}},
		}, nil
	}

	eventLogPath, err := appendRunAuditLog(colonyRoot, ev)
	if err != nil {
		return protocol.EventCLIResult{
			OK:      false,
			Error:   "audit_log_failed",
			TraceID: ev.TraceID,
			Type:    ev.Type,
			Kind:    kind,
			Subject: subject,
			Details: []protocol.ValidationDetail{{
				Path:    "",
				Message: err.Error(),
			}},
		}, nil
	}

	return protocol.EventCLIResult{
		OK:           true,
		TraceID:      ev.TraceID,
		Type:         ev.Type,
		Kind:         kind,
		Subject:      subject,
		EventLogPath: eventLogPath,
	}, nil
}

// ReadEventInput reads one JSON object from stdin.
func ReadEventInput(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("bus: read stdin: %w", err)
	}
	if len(data) == 0 {
		return nil, &protocol.ValidationError{
			Code: "invalid_json",
			Details: []protocol.ValidationDetail{{
				Path:    "",
				Message: "stdin is empty; expected one JSON object",
			}},
		}
	}
	return data, nil
}

// WriteEventCLIResult prints a machine-readable result as JSON.
func WriteEventCLIResult(w io.Writer, result protocol.EventCLIResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

func asValidationError(err error, target **protocol.ValidationError) bool {
	if err == nil {
		return false
	}
	verr, ok := err.(*protocol.ValidationError)
	if !ok {
		return false
	}
	*target = verr
	return true
}

func appendRunAuditLog(colonyRoot string, ev protocol.Event) (string, error) {
	if colonyRoot == "" {
		return "", fmt.Errorf("colony root is required to write run audit log")
	}
	if ev.TraceID == "" || ev.AgentID == "" {
		return "", fmt.Errorf("traceId and agentId are required to write run audit log")
	}

	runDir := runs.Dir{
		ColonyRoot: colonyRoot,
		TraceID:    ev.TraceID,
		AgentID:    ev.AgentID,
	}
	if _, err := os.Stat(runDir.Root()); err != nil {
		return "", fmt.Errorf("run directory not found for trace %s agent %s: %w", ev.TraceID, ev.AgentID, err)
	}
	if err := runDir.AppendEvent(ev); err != nil {
		return "", fmt.Errorf("append events.ndjson: %w", err)
	}
	abs, err := filepath.Abs(runDir.EventsPath())
	if err != nil {
		return runDir.EventsPath(), nil
	}
	return abs, nil
}
