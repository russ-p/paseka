package export

import (
	"context"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/adapters/cursor"
	"github.com/russ-p/paseka/internal/adapters/pi"
	"github.com/russ-p/paseka/internal/hiveview"
)

const (
	maxAgentLogToolCalls = 100
	maxAgentLogArgsRunes = 200
	// maxAgentLogToolInputRunes leaves the transcript's generic tool-input cap
	// out of the Agent log's way: the shipped table picked path/url/command out
	// of the whole input before applying its own 200-rune summary cap, so a
	// summary key past the 2k default must still be found. 8 MiB is the Cursor
	// JSONL line ceiling, so no parsed input can exceed it.
	maxAgentLogToolInputRunes = 8 << 20
	// defaultTranscriptTimeout bounds one provider resolution. A wedged
	// resolver is cut off and reported as an omission instead of stalling the
	// export.
	defaultTranscriptTimeout = 15 * time.Second
)

// TranscriptLookup returns an optional provider-transcript resolver for an
// adapter name.
type TranscriptLookup func(adapterName string) adapters.TranscriptResolver

// AgentLogExport is the per-run Agent log view for HTML/Markdown. It is the
// tool-call projection over the resolved transcript.
type AgentLogExport struct {
	AgentID   string
	Omitted   string
	ToolCalls []adapters.ToolCall
	Truncated bool
}

func defaultTranscriptLookup(adapterName string) adapters.TranscriptResolver {
	switch adapterName {
	case "cursor":
		return cursor.New()
	case "pi":
		return pi.New()
	default:
		return nil
	}
}

func resolveAgentLogs(runs []hiveview.RunView, lookup TranscriptLookup, timeout time.Duration) []AgentLogExport {
	if lookup == nil {
		lookup = defaultTranscriptLookup
	}
	out := make([]AgentLogExport, 0, len(runs))
	for _, run := range runs {
		out = append(out, resolveAgentLog(run, lookup, timeout))
	}
	return out
}

func resolveAgentLog(run hiveview.RunView, lookup TranscriptLookup, timeout time.Duration) AgentLogExport {
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
	if timeout <= 0 {
		timeout = defaultTranscriptTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	limits := adapters.DefaultTranscriptLimits()
	limits.ToolInput = maxAgentLogToolInputRunes
	transcript := adapters.ResolveTranscript(ctx, resolver, adapters.TranscriptRequest{
		ProviderSessionID: run.ProviderSessionID,
		Adapter:           run.Adapter,
		Workspace:         run.Workspace,
		ColonyRoot:        run.ColonyRoot,
		RunDir:            run.RunDir,
		AgentID:           run.AgentID,
		Include:           adapters.PresetToolCall(),
		Limits:            limits,
	})
	if transcript.Omitted != "" {
		entry.Omitted = transcript.Omitted
		return entry
	}

	calls := adapters.ProjectToolCalls(transcript)
	if len(calls) > maxAgentLogToolCalls {
		calls = calls[:maxAgentLogToolCalls]
		entry.Truncated = true
	}
	trimmed := make([]adapters.ToolCall, len(calls))
	for i, call := range calls {
		call.Args, _ = adapters.TruncateRunes(call.Args, maxAgentLogArgsRunes)
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
