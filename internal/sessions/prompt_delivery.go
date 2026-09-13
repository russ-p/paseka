package sessions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/logging"
)

const (
	sessionPromptReadyTimeout = 30 * time.Second
	sessionPromptReadyAttempt = 3 * time.Second
	sessionPromptPollInterval = 250 * time.Millisecond
	sessionPromptPostTimeout  = 15 * time.Second
)

// sessionPromptClient talks to the interactive process's loopback control
// server. Proxy is disabled so an inherited HTTP(S)_PROXY cannot intercept it.
var sessionPromptClient = &http.Client{Transport: &http.Transport{Proxy: nil}}

// deliverSessionPrompt waits for the interactive process's control server and
// posts the initial prompt into its provider session. The TUI is already
// running, so failures are advisory: the user can still type the prompt.
func deliverSessionPrompt(ctx context.Context, d adapters.SessionPromptDelivery) {
	if strings.TrimSpace(d.BaseURL) == "" || strings.TrimSpace(d.SessionID) == "" {
		return
	}
	readyCtx, cancel := context.WithTimeout(ctx, sessionPromptReadyTimeout)
	defer cancel()
	if err := waitForControlServer(readyCtx, d); err != nil {
		logging.Component("sessions").Warn("session prompt delivery: control server not ready",
			logging.F("session", d.SessionID),
			logging.F("error", err.Error()),
		)
		return
	}
	if err := postSessionPrompt(d); err != nil {
		logging.Component("sessions").Warn("session prompt delivery failed",
			logging.F("session", d.SessionID),
			logging.F("error", err.Error()),
		)
	}
}

func waitForControlServer(ctx context.Context, d adapters.SessionPromptDelivery) error {
	base := strings.TrimRight(d.BaseURL, "/")
	var lastErr error
	for {
		attemptCtx, cancel := context.WithTimeout(ctx, sessionPromptReadyAttempt)
		err := probeControlServer(attemptCtx, base, d)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return lastErr
			}
			return ctx.Err()
		case <-time.After(sessionPromptPollInterval):
		}
	}
}

func probeControlServer(ctx context.Context, base string, d adapters.SessionPromptDelivery) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/session", nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(d.Username, d.Password)
	resp, err := sessionPromptClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("control server returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func postSessionPrompt(d adapters.SessionPromptDelivery) error {
	payload := map[string]any{
		"parts": []map[string]string{{"type": "text", "text": d.Text}},
	}
	if d.Agent != "" {
		payload["agent"] = d.Agent
	}
	if d.Variant != "" {
		payload["variant"] = d.Variant
	}
	if d.ModelProvider != "" && d.ModelID != "" {
		payload["model"] = map[string]string{"providerID": d.ModelProvider, "modelID": d.ModelID}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), sessionPromptPostTimeout)
	defer cancel()
	base := strings.TrimRight(d.BaseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/session/"+d.SessionID+"/prompt_async", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(d.Username, d.Password)
	resp, err := sessionPromptClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("prompt_async returned HTTP %d", resp.StatusCode)
	}
	return nil
}
