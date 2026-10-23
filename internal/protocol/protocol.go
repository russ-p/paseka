package protocol

import (
	"encoding/json"
	"time"

	"github.com/russ-p/paseka/internal/ids"
)

const Version = "agent-runtime.v1"

// eventIDPrefix namespaces minted event identities on disk and on the bus.
const eventIDPrefix = "evt-"

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

// Status reasons classify why a terminal run status was reached, so a
// post-mortem can tell "slow" from "stuck". A cancelled run carries
// ReasonTimeout when params.timeout elapsed; a human/system cancel carries no
// reason.
const (
	ReasonTimeout = "timeout"
)

// Provenance identifies the durable execution ownership and configuration a
// record belongs to. It is flattened into envelopes and sidecars so consumers do
// not need a second lookup to know which attempt produced the record.
type Provenance struct {
	AttemptID            string `json:"attemptId,omitempty"`
	ConfigRef            string `json:"configRef,omitempty"`
	ConfigRevision       int    `json:"configRevision,omitempty"`
	InitialCodeBaseSHA   string `json:"initialCodeBaseSha,omitempty"`
	InitialCodeBaseDirty bool   `json:"initialCodeBaseDirty,omitempty"`
}

// Event is one NDJSON line in events.ndjson.
type Event struct {
	ProtocolVersion string `json:"protocolVersion"`
	// ID is the event's stable identity, minted where the event is created.
	// It is colony-internal: no producer supplies it, no bee sees it, and it
	// is absent from the agent-facing input shape. Storing the same event in a
	// per-run audit log and in a trail log is safe because both copies carry
	// it. Absent on records written before identities existed.
	ID        string          `json:"id,omitempty"`
	TraceID   string          `json:"traceId"`
	AgentID   string          `json:"agentId"`
	Seq       int             `json:"seq"`
	Type      EventType       `json:"type"`
	CreatedAt time.Time       `json:"createdAt"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Provenance
}

// Request is written to request.json before the agent starts.
type Request struct {
	ProtocolVersion      string    `json:"protocolVersion"`
	TraceID              string    `json:"traceId"`
	AgentID              string    `json:"agentId"`
	Bee                  string    `json:"bee"`
	Adapter              string    `json:"adapter"`
	Workspace            string    `json:"workspace"`
	ColonyRoot           string    `json:"colonyRoot"`
	TaskID               string    `json:"taskId,omitempty"`
	Task                 string    `json:"task,omitempty"`
	Intent               string    `json:"intent,omitempty"`
	Insights             []string  `json:"insights,omitempty"`
	Model                string    `json:"model,omitempty"`
	ModelAlias           string    `json:"modelAlias,omitempty"`
	Profile              string    `json:"profile,omitempty"`
	AttemptID            string    `json:"attemptId,omitempty"`
	ConfigRef            string    `json:"configRef,omitempty"`
	ConfigRevision       int       `json:"configRevision,omitempty"`
	InitialCodeBaseSHA   string    `json:"initialCodeBaseSha,omitempty"`
	InitialCodeBaseDirty bool      `json:"initialCodeBaseDirty,omitempty"`
	LaunchForm           string    `json:"launchForm,omitempty"`
	ResultPath           string    `json:"resultPath"`
	EventLogPath         string    `json:"eventLogPath"`
	CreatedAt            time.Time `json:"createdAt"`
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

// UsageSource identifies where per-run LLM token counts came from.
const (
	UsageSourceCursorStreamJSON = "cursor.stream-json"
	UsageSourceOpenCodeRunJSON  = "opencode.run-json"
	UsageSourceOpenCodeServer   = "opencode.server-json"
	UsageSourcePiPrintJSON      = "pi.print-json"
	UsageSourcePiSessionJSONL   = "pi.session-jsonl"
	UsageSourceUnavailable      = "unavailable"
)

// Usage is optional LLM token accounting from an adapter when available.
// Adapters that cannot report usage omit this field; projections must tolerate nil.
type Usage struct {
	InputTokens      int64  `json:"inputTokens"`
	OutputTokens     int64  `json:"outputTokens"`
	CacheReadTokens  int64  `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int64  `json:"cacheWriteTokens,omitempty"`
	DurationMs       int64  `json:"durationMs,omitempty"`
	Source           string `json:"source,omitempty"`
}

// Result is the final envelope written to result.json.
type Result struct {
	ProtocolVersion   string        `json:"protocolVersion"`
	TraceID           string        `json:"traceId"`
	AgentID           string        `json:"agentId"`
	Status            RunStatus     `json:"status"`
	Summary           string        `json:"summary"`
	Artifacts         []ArtifactRef `json:"artifacts,omitempty"`
	Diagnostics       Diagnostics   `json:"diagnostics"`
	Usage             *Usage        `json:"usage,omitempty"`
	ProviderSessionID string        `json:"providerSessionId,omitempty"`
	FinishedAt        time.Time     `json:"finishedAt"`
	Provenance
}

// StatusSnapshot is written to status.json at lifecycle transitions.
type StatusSnapshot struct {
	ProtocolVersion string    `json:"protocolVersion"`
	State           RunStatus `json:"state"`
	// Reason is a machine-readable classifier for a terminal state, e.g.
	// "timeout" for a cancelled run that exceeded params.timeout.
	Reason     string    `json:"reason,omitempty"`
	PID        int       `json:"pid,omitempty"`
	ExitCode   int       `json:"exitCode,omitempty"`
	StartedAt  time.Time `json:"startedAt,omitempty"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
	Error      string    `json:"error,omitempty"`
	Provenance
}

// BusEvent is the minimal JSON shape agents send to `paseka event emit --stdin`.
type BusEvent struct {
	TraceID string          `json:"traceId"`
	TaskID  string          `json:"taskId,omitempty"`
	Type    EventType       `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// NewEventID mints a stable identity for one event.
//
// It carries no timestamp and no counter, so nothing about an id reveals when
// or how often it was minted, and the same event found in a per-run log and in a
// trail log is recognizably the same event. The body is 128 random bits rather
// than a mini-ULID: a millisecond-prefixed id gives two events minted in the
// same millisecond a real chance of colliding, and a collision is silent — the
// colony would deduplicate one event away and never know.
func NewEventID() (string, error) {
	return ids.PrefixedRandom(eventIDPrefix)
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
	id, err := NewEventID()
	if err != nil {
		return Event{}, err
	}
	return Event{
		ProtocolVersion: Version,
		ID:              id,
		TraceID:         traceID,
		AgentID:         agentID,
		Seq:             seq,
		Type:            typ,
		CreatedAt:       time.Now().UTC(),
		Payload:         raw,
	}, nil
}
