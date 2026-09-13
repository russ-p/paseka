package adapters

import (
	"io"
	"time"
)

// SessionState is the lifecycle state of an interactive agent session.
type SessionState string

const (
	SessionActive    SessionState = "active"
	SessionDetached  SessionState = "detached"
	SessionCompleted SessionState = "completed"
	SessionFailed    SessionState = "failed"
	SessionCancelled SessionState = "cancelled"
)

// SessionRequest is passed when starting an interactive agent session.
type SessionRequest struct {
	Bee           string
	InitialPrompt string
	SystemPrompt  string
	ColonyRoot    string
	Workspace     string
	Params        RunParams
	Command       []string // optional full argv; overrides Params-based arg building when set
	TraceID       string
	AgentID       string
	TaskID        string
	Task          string
	Intent        string
	Insights      []string
	// ResumeSessionID, when set, asks the adapter to continue an existing
	// provider chat instead of creating a new one. Cursor uses --resume and
	// skips create-chat; InitialPrompt is an optional continue line only.
	ResumeSessionID string
	// Detached is retained for compatibility but ignored by session adapters.
	// StartDetached means "no local terminal attach / use PTY hub", not headless -p.
	Detached bool
}

// SessionPromptDelivery describes an initial prompt that must be delivered to
// a running provider session through the interactive process's HTTP control
// server (used by OpenCode, whose TUI ignores --prompt when --session is set).
type SessionPromptDelivery struct {
	BaseURL       string // control server origin, e.g. http://127.0.0.1:4096
	SessionID     string // provider session id to post to
	Username      string // basic auth username
	Password      string // basic auth password
	Text          string // prompt text
	Agent         string // optional agent name
	Variant       string // optional reasoning variant
	ModelProvider string // optional model provider id
	ModelID       string // optional model id
}

// SessionCommand describes how to launch the external agent process.
type SessionCommand struct {
	Binary            string
	Args              []string
	Env               []string
	Dir               string
	ProviderSessionID string // optional; native provider session id when known before PTY start
	// Prompt, when set, must be delivered to ProviderSessionID through the
	// process's HTTP control server once it becomes reachable.
	Prompt *SessionPromptDelivery
}

// SessionHandle identifies a running or recently finished session.
type SessionHandle struct {
	SessionID         string // equals AgentID for MVP
	TraceID           string
	AgentID           string
	RunDir            string
	Workspace         string
	ColonyRoot        string
	Bee               string
	Adapter           string
	PID               int
	State             SessionState
	StartedAt         time.Time
	ProviderSessionID string
	ResumedFrom       string
	Profile           string
}

// SessionEventKind categorizes normalized session events.
type SessionEventKind string

const (
	SessionEventStarted   SessionEventKind = "started"
	SessionEventOutput    SessionEventKind = "output"
	SessionEventUserInput SessionEventKind = "user_input"
	SessionEventStopped   SessionEventKind = "stopped"
	SessionEventError     SessionEventKind = "error"
)

// SessionEvent is a normalized lifecycle or I/O event from a session.
type SessionEvent struct {
	Kind      SessionEventKind
	SessionID string
	TraceID   string
	AgentID   string
	At        time.Time
	Data      []byte
	Err       error
}

// UserMessage is a message sent to an interactive session.
type UserMessage struct {
	Text string
}

// SessionAdapter knows how to build commands for interactive agent sessions.
// The runtime owns the PTY process; adapters only describe how to invoke the tool.
type SessionAdapter interface {
	Name() string
	SessionCommand(req SessionRequest) (SessionCommand, error)
}

// PTYSession is an active PTY-backed session owned by the runtime.
type PTYSession interface {
	Handle() SessionHandle
	PTY() io.ReadWriteCloser
	Wait() error
	Kill() error
}
