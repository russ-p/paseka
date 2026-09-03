package adapters

import (
	"context"
	"time"
)

// Stable omit reasons for provider session log resolution (export Agent log).
const (
	SessionLogOmittedNoProviderSessionID = "no providerSessionId"
	SessionLogOmittedUnsupported         = "unsupported"
	SessionLogOmittedNotImplemented      = "not implemented"
	SessionLogOmittedStoreNotFound       = "store not found"
	SessionLogOmittedParseError          = "parse error"
	SessionLogOmittedResolveError        = "resolve error"
)

// SessionLogRequest is the read-time lookup for a provider session log.
type SessionLogRequest struct {
	ProviderSessionID string
	Adapter           string
	Workspace         string
	ColonyRoot        string
	RunDir            string
	AgentID           string
}

// ToolCall is one MVP Agent log row (name, short args, optional call id).
type ToolCall struct {
	Name      string
	CallID    string
	Args      string
	Timestamp time.Time
}

// SessionLog is a best-effort tool-call summary for one provider session.
// Omitted is a stable reason when ToolCalls is empty.
type SessionLog struct {
	ToolCalls []ToolCall
	Omitted   string
}

// SessionLogResolver is an optional adapter capability to resolve provider logs by id.
type SessionLogResolver interface {
	ResolveSessionLog(ctx context.Context, req SessionLogRequest) (SessionLog, error)
}
