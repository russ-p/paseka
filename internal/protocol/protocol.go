package protocol

import (
	"encoding/json"
	"time"
)

const Version = "agent-runtime.v1"

// EventType is a domain or lifecycle event emitted during an agent run.
type EventType string

const (
	EventSignal        EventType = "SIGNAL"
	EventInsight       EventType = "INSIGHT"
	EventMutation      EventType = "MUTATION"
	EventVerification  EventType = "VERIFICATION"
	EventLog           EventType = "LOG"
	EventProgress      EventType = "PROGRESS"
	EventToolCall      EventType = "TOOL_CALL"
	EventAssistantText EventType = "ASSISTANT_TEXT"
)

// RunStatus is the lifecycle state of one agent invocation.
type RunStatus string

const (
	StatusQueued    RunStatus = "queued"
	StatusRunning   RunStatus = "running"
	StatusCompleted RunStatus = "completed"
	StatusFailed    RunStatus = "failed"
	StatusCancelled RunStatus = "cancelled"
)

// Event is one NDJSON line in events.ndjson.
type Event struct {
	ProtocolVersion string          `json:"protocolVersion"`
	TraceID         string          `json:"traceId"`
	AgentID         string          `json:"agentId"`
	Seq             int             `json:"seq"`
	Type            EventType       `json:"type"`
	CreatedAt       time.Time       `json:"createdAt"`
	Payload         json.RawMessage `json:"payload,omitempty"`
}

// Request is written to request.json before the agent starts.
type Request struct {
	ProtocolVersion string    `json:"protocolVersion"`
	TraceID         string    `json:"traceId"`
	AgentID         string    `json:"agentId"`
	Bee             string    `json:"bee"`
	Adapter         string    `json:"adapter"`
	Workspace       string    `json:"workspace"`
	ColonyRoot      string    `json:"colonyRoot"`
	TaskID          string    `json:"taskId,omitempty"`
	Task            string    `json:"task,omitempty"`
	Intent          string    `json:"intent,omitempty"`
	Insights        []string  `json:"insights,omitempty"`
	ResultPath      string    `json:"resultPath"`
	EventLogPath    string    `json:"eventLogPath"`
	CreatedAt       time.Time `json:"createdAt"`
}

// ArtifactRef points to a normalized output artifact.
type ArtifactRef struct {
	Kind string `json:"kind"`
	Path string `json:"path,omitempty"`
}

// Diagnostics captures process-level outcome details.
type Diagnostics struct {
	ExitCode int    `json:"exitCode"`
	Error    string `json:"error,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
}

// Result is the final envelope written to result.json.
type Result struct {
	ProtocolVersion string        `json:"protocolVersion"`
	TraceID         string        `json:"traceId"`
	AgentID         string        `json:"agentId"`
	Status          RunStatus     `json:"status"`
	Summary         string        `json:"summary"`
	Artifacts       []ArtifactRef `json:"artifacts,omitempty"`
	Diagnostics     Diagnostics   `json:"diagnostics"`
	FinishedAt      time.Time     `json:"finishedAt"`
}

// StatusSnapshot is written to status.json at lifecycle transitions.
type StatusSnapshot struct {
	ProtocolVersion string    `json:"protocolVersion"`
	State           RunStatus `json:"state"`
	PID             int       `json:"pid,omitempty"`
	ExitCode        int       `json:"exitCode,omitempty"`
	StartedAt       time.Time `json:"startedAt,omitempty"`
	FinishedAt      time.Time `json:"finishedAt,omitempty"`
	Error           string    `json:"error,omitempty"`
}

// BusEvent is the minimal JSON shape agents send to `paseka event emit --stdin`.
type BusEvent struct {
	TraceID string          `json:"traceId"`
	TaskID  string          `json:"taskId,omitempty"`
	Type    EventType       `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// NewEvent builds a protocol event with defaults filled in.
func NewEvent(traceID, agentID string, seq int, typ EventType, payload any) (Event, error) {
	var raw json.RawMessage
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return Event{}, err
		}
		raw = data
	}
	return Event{
		ProtocolVersion: Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Seq:             seq,
		Type:            typ,
		CreatedAt:       time.Now().UTC(),
		Payload:         raw,
	}, nil
}
