package export

import (
	"context"
	"unicode/utf8"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/adapters/cursor"
	"github.com/paseka/paseka/internal/adapters/pi"
	"github.com/paseka/paseka/internal/hiveview"
)

const (
	maxAgentLogToolCalls = 100
	maxAgentLogArgsRunes = 200
)

// SessionLogLookup returns an optional provider-log resolver for an adapter name.
type SessionLogLookup func(adapterName string) adapters.SessionLogResolver

// AgentLogExport is the per-run Agent log view for HTML/Markdown.
type AgentLogExport struct {
	AgentID   string
	Omitted   string
	ToolCalls []adapters.ToolCall
	Truncated bool
}

func defaultSessionLogLookup(adapterName string) adapters.SessionLogResolver {
	switch adapterName {
	case "cursor":
		return cursor.New()
	case "pi":
		return pi.New()
	default:
		return nil
	}
}

func resolveAgentLogs(runs []hiveview.RunView, lookup SessionLogLookup) []AgentLogExport {
	if lookup == nil {
		lookup = defaultSessionLogLookup
	}
	out := make([]AgentLogExport, 0, len(runs))
	for _, run := range runs {
		out = append(out, resolveAgentLog(run, lookup))
	}
	return out
}

func resolveAgentLog(run hiveview.RunView, lookup SessionLogLookup) AgentLogExport {
	entry := AgentLogExport{AgentID: run.AgentID}
	if run.ProviderSessionID == "" {
		entry.Omitted = adapters.SessionLogOmittedNoProviderSessionID
		return entry
	}
	resolver := lookup(run.Adapter)
	if resolver == nil {
		entry.Omitted = adapters.SessionLogOmittedUnsupported
		return entry
	}
	log, err := resolver.ResolveSessionLog(context.Background(), adapters.SessionLogRequest{
		ProviderSessionID: run.ProviderSessionID,
		Adapter:           run.Adapter,
		Workspace:         run.Workspace,
		ColonyRoot:        run.ColonyRoot,
		RunDir:            run.RunDir,
		AgentID:           run.AgentID,
	})
	if err != nil {
		entry.Omitted = adapters.SessionLogOmittedResolveError
		return entry
	}
	if log.Omitted != "" {
		entry.Omitted = log.Omitted
		return entry
	}
	calls := log.ToolCalls
	if len(calls) > maxAgentLogToolCalls {
		calls = calls[:maxAgentLogToolCalls]
		entry.Truncated = true
	}
	trimmed := make([]adapters.ToolCall, len(calls))
	for i, call := range calls {
		call.Args = truncateRunes(call.Args, maxAgentLogArgsRunes)
		trimmed[i] = call
	}
	entry.ToolCalls = trimmed
	return entry
}

func agentLogByAgentID(logs []AgentLogExport, agentID string) AgentLogExport {
	for _, log := range logs {
		if log.AgentID == agentID {
			return log
		}
	}
	return AgentLogExport{AgentID: agentID}
}

func truncateRunes(s string, limit int) string {
	if limit <= 0 || s == "" {
		return s
	}
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return string(runes[:limit]) + "…"
}
