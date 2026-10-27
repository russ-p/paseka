package opencodev2

import (
	"context"
	"strings"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/protocol"
)

// installHint names the package that provides the selected OpenCode major.
const installHint = "npm install -g opencode-ai@2"

// usageFromRecord maps the session record's token block and cost onto the shared
// usage struct. A session that reports no tokens yields nil, so "no usage" never
// becomes a zero-valued record.
// failsOutcome reports whether the provider's session outcome describes a run
// that did not finish cleanly. Unknown values stay quiet; the exit code is the
// primary signal and this is only a warning.
func failsOutcome(outcome string) bool {
	switch strings.ToLower(strings.TrimSpace(outcome)) {
	case "failed", "failure", "error", "interrupted", "cancelled", "canceled", "aborted":
		return true
	default:
		return false
	}
}

func usageFromRecord(rec sessionRecord) *protocol.Usage {
	usage := usageFromTokens(rec.Tokens, protocol.UsageSourceOpenCodeV2Session)
	if usage == nil {
		return nil
	}
	usage.CostUSD = rec.Cost
	return usage
}

// SessionUsage reads what a finished V2 TUI session spent. The managed server
// died with the TUI, but OpenCode's store is durable, so a throwaway server
// answers for the stored session id.
func (a *SessionAdapter) SessionUsage(ctx context.Context, handle adapters.SessionHandle) *protocol.Usage {
	sessionID := strings.TrimSpace(handle.ProviderSessionID)
	binary := strings.TrimSpace(handle.Binary)
	if sessionID == "" || binary == "" || handle.Workspace == "" {
		return nil
	}

	var usage *protocol.Usage
	err := withServer(binary, handle.Workspace, handle.Env, func(ctx context.Context, c *apiClient) error {
		rec, err := c.Session(ctx, sessionID)
		if err != nil {
			return err
		}
		usage = usageFromRecord(rec)
		return nil
	})
	if err != nil {
		return nil
	}
	return usage
}
