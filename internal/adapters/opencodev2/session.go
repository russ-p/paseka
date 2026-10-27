package opencodev2

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/logging"
)

// sessionServer is the Paseka-managed server a V2 HITL session runs against. It
// is an interface so tests can supply a local stub without starting a binary.
type sessionServer interface {
	URL() string
	client() *apiClient
	Close()
	outputText() string
}

// startSessionServer is a seam: tests replace it with a stub server.
var startSessionServer = func(binary, workspace string, env []string) (sessionServer, error) {
	return startServer(binary, workspace, env)
}

// SessionAdapter builds commands for interactive OpenCode 2 TUI sessions.
type SessionAdapter struct{}

func NewSession() *SessionAdapter {
	return &SessionAdapter{}
}

func (a *SessionAdapter) Name() string {
	return adapterName
}

// ResolveImplementation lets a direct pin report itself without detection.
func (a *SessionAdapter) ResolveImplementation(adapters.RunParams) (string, error) {
	return adapterName, nil
}

// SessionCommand builds a V2 TUI invocation. Paseka owns the server, picks the
// provider session id itself, creates the session through the API with the
// workspace as its location, and passes the kickoff prompt to the TUI. V2
// accepts --prompt together with --session, so no control-server delivery or
// prompt-injection loop is needed.
func (a *SessionAdapter) SessionCommand(req adapters.SessionRequest) (adapters.SessionCommand, error) {
	if req.Workspace == "" {
		return adapters.SessionCommand{}, errors.New("opencodev2: workspace is required")
	}
	resumeID := strings.TrimSpace(req.ResumeSessionID)
	if resumeID == "" && req.InitialPrompt == "" && req.SystemPrompt == "" {
		return adapters.SessionCommand{}, errors.New("opencodev2: initial prompt or system prompt is required")
	}
	configuredName, implementation := adapters.AdapterIdentity(req.AdapterName, req.Implementation, adapterName)

	prompt := joinPrompt(req.SystemPrompt, req.InitialPrompt)
	if resumeID != "" {
		prompt = strings.TrimSpace(req.InitialPrompt)
	}

	if len(req.Command) > 0 {
		return commandOverride(req, implementation)
	}

	binary := adapters.ResolveBinary(req.Binary, req.Params.Binary, defaultBinary)
	if _, err := exec.LookPath(binary); err != nil {
		return adapters.SessionCommand{}, fmt.Errorf("opencodev2: %q not found in PATH (install OpenCode 2: %s)", binary, installHint)
	}

	srv, err := startSessionServer(binary, req.Workspace, req.Env)
	if err != nil {
		return adapters.SessionCommand{}, err
	}
	if srv == nil {
		return adapters.SessionCommand{}, errors.New("opencodev2: session server unavailable")
	}

	providerSessionID := resumeID
	if providerSessionID == "" {
		id, err := NewSessionID()
		if err != nil {
			srv.Close()
			return adapters.SessionCommand{}, fmt.Errorf("opencodev2: generate session id: %w", err)
		}
		rec, err := createSessionOnServer(srv, createSessionBody{
			ID:       id,
			Location: req.Workspace,
			Title:    sessionTitle(req),
			Agent:    sessionAgent(req),
			Model:    sessionModelFor(req.Params.Model, req.Params.Reasoning),
		})
		if err != nil {
			srv.Close()
			return adapters.SessionCommand{}, fmt.Errorf("opencodev2: create session: %w%s", err, serveOutput(srv.outputText()))
		}
		providerSessionID = rec.ID
	}

	args := buildInteractiveArgs(srv.URL(), providerSessionID, prompt, true)
	env := append(envWithout(req.Env, "OPENCODE_SERVER_PASSWORD", "OPENCODE_SERVER_USERNAME"),
		"OPENCODE_SERVER_PASSWORD="+srv.client().password,
		"OPENCODE_SERVER_USERNAME="+apiUsername)

	logging.Component("adapter").Info("opencodev2 session launch",
		logging.F("adapter", configuredName),
		logging.F("implementation", implementation),
		logging.F("bee", req.Bee),
		logging.F("trace", req.TraceID),
		logging.F("agent", req.AgentID),
		logging.F("binary", binary),
		logging.F("server", srv.URL()),
		logging.F("provider_session", providerSessionID),
	)

	return adapters.SessionCommand{
		Binary:            binary,
		Args:              args,
		Env:               env,
		Dir:               adapters.ResolveDir(req.Dir, req.Workspace),
		ProviderSessionID: providerSessionID,
		Implementation:    implementation,
		Cleanup:           srv.Close,
	}, nil
}

// commandOverride passes the bee's argv through untouched, records an explicit
// session id, and skips session creation entirely.
func commandOverride(req adapters.SessionRequest, implementation string) (adapters.SessionCommand, error) {
	binary := req.Command[0]
	var args []string
	if len(req.Command) > 1 {
		args = req.Command[1:]
	}
	if _, err := exec.LookPath(binary); err != nil {
		return adapters.SessionCommand{}, fmt.Errorf("opencodev2: %q not found in PATH (install OpenCode 2: %s)", binary, installHint)
	}
	return adapters.SessionCommand{
		Binary:            binary,
		Args:              args,
		Env:               req.Env,
		Dir:               adapters.ResolveDir(req.Dir, req.Workspace),
		ProviderSessionID: firstFlagValue(args, "--session", "-s"),
		Implementation:    implementation,
	}, nil
}

func sessionTitle(req adapters.SessionRequest) string {
	if title := strings.TrimSpace(req.AgentID); title != "" {
		return title
	}
	return strings.TrimSpace(req.Bee)
}

func sessionAgent(req adapters.SessionRequest) string {
	if req.Params.Plan {
		return "plan"
	}
	return ""
}

// createSessionOnServer retries while the freshly started server boots. A
// non-retryable rejection propagates immediately so the caller can fall back to
// a provider-assigned id.
func createSessionOnServer(srv sessionServer, body createSessionBody) (sessionRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), serveTimeout)
	defer cancel()

	var lastErr error
	for {
		attemptCtx, attemptCancel := context.WithTimeout(ctx, serveAttempt)
		rec, err := srv.client().CreateSession(attemptCtx, body)
		attemptCancel()
		if err == nil {
			return rec, nil
		}
		lastErr = err
		if !isRetryable(err) {
			return sessionRecord{}, err
		}
		select {
		case <-ctx.Done():
			return sessionRecord{}, lastErr
		case <-time.After(serveRetry):
		}
	}
}
