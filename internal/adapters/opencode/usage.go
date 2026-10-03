package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/protocol"
)

// SessionUsage reads what a finished TUI session spent. The TUI keeps no
// machine-readable stdout and its control server dies with the process, but
// OpenCode accounts every session in its own store, so a throwaway
// `opencode serve` answers for the session id the TUI was launched with.
// Reported tokens are cumulative for that provider session, so a resumed session
// includes the turns from before the resume.
func (a *SessionAdapter) SessionUsage(ctx context.Context, handle adapters.SessionHandle) *protocol.Usage {
	sessionID := strings.TrimSpace(handle.ProviderSessionID)
	binary := strings.TrimSpace(handle.Binary)
	if sessionID == "" || binary == "" || handle.Workspace == "" {
		return nil
	}

	var usage *protocol.Usage
	err := withServer(binary, handle.Workspace, os.Environ(), func(ctx context.Context, port int, password string) error {
		fetched, err := getSessionUsage(ctx, port, password, sessionID)
		if err != nil {
			return err
		}
		usage = fetched
		return nil
	})
	if err != nil {
		return nil
	}
	return usage
}

// getSessionUsage reads the cumulative token totals OpenCode keeps per session.
// Any answer from the server is final — a session that never spent tokens, is
// unknown to this OpenCode, or answers with an unexpected body all mean "no
// usage", so only a transport failure (server still booting) is worth retrying.
func getSessionUsage(parent context.Context, port int, password, sessionID string) (*protocol.Usage, error) {
	ctx, cancel := context.WithTimeout(parent, serveAttempt)
	defer cancel()

	url := fmt.Sprintf("http://127.0.0.1:%d/session/%s", port, sessionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("opencode", password)

	resp, err := serveClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, nil
	}

	var out struct {
		Tokens map[string]json.RawMessage `json:"tokens"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, nil
	}
	return usageFromTokens(out.Tokens, protocol.UsageSourceOpenCodeServer), nil
}
