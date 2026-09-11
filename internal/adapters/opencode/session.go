package opencode

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/logging"
)

// SessionAdapter builds commands for interactive OpenCode TUI sessions.
type SessionAdapter struct{}

func NewSession() *SessionAdapter {
	return &SessionAdapter{}
}

func (a *SessionAdapter) Name() string {
	return adapterName
}

// SessionCommand builds an OpenCode TUI invocation. Headless `run` belongs to Adapter.Run().
// A fresh session is pre-created via the OpenCode server (the create-chat analog);
// an explicit ResumeSessionID skips pre-create and continues that session directly.
func (a *SessionAdapter) SessionCommand(req adapters.SessionRequest) (adapters.SessionCommand, error) {
	if req.Workspace == "" {
		return adapters.SessionCommand{}, errors.New("opencode: workspace is required")
	}
	resumeID := strings.TrimSpace(req.ResumeSessionID)
	if resumeID == "" && req.InitialPrompt == "" && req.SystemPrompt == "" {
		return adapters.SessionCommand{}, errors.New("opencode: initial prompt or system prompt is required")
	}

	prompt := joinPrompt(req.SystemPrompt, req.InitialPrompt)
	if resumeID != "" {
		prompt = strings.TrimSpace(req.InitialPrompt)
	}

	commandOverride := len(req.Command) > 0

	binary := req.Params.Binary
	if binary == "" {
		binary = defaultBinary
	}
	var args []string
	if commandOverride {
		binary = req.Command[0]
		if len(req.Command) > 1 {
			args = req.Command[1:]
		}
	}
	if _, err := exec.LookPath(binary); err != nil {
		return adapters.SessionCommand{}, fmt.Errorf("opencode: %q not found in PATH (install OpenCode CLI)", binary)
	}

	providerSessionID := ""
	switch {
	case resumeID != "":
		providerSessionID = resumeID
	case commandOverride:
		providerSessionID = firstFlagValue(args, "--session", "-s")
	default:
		id, err := createSessionFunc(binary, req.Workspace, os.Environ())
		if err != nil {
			logging.Component("adapter").Warn("opencode create-session failed",
				logging.F("adapter", adapterName),
				logging.F("bee", req.Bee),
				logging.F("trace", req.TraceID),
				logging.F("agent", req.AgentID),
				logging.F("error", err.Error()),
			)
		} else {
			providerSessionID = id
		}
	}

	if !commandOverride {
		args = buildInteractiveArgs(req, prompt, providerSessionID)
	}

	return adapters.SessionCommand{
		Binary:            binary,
		Args:              args,
		Env:               os.Environ(),
		Dir:               req.Workspace,
		ProviderSessionID: providerSessionID,
	}, nil
}

// firstFlagValue returns the first value found for any of the given flags.
func firstFlagValue(argv []string, flags ...string) string {
	for _, flag := range flags {
		if v := adapters.FlagValue(argv, flag); v != "" {
			return v
		}
	}
	return ""
}
