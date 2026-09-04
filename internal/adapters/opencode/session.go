package opencode

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/paseka/paseka/internal/adapters"
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
func (a *SessionAdapter) SessionCommand(req adapters.SessionRequest) (adapters.SessionCommand, error) {
	if req.Workspace == "" {
		return adapters.SessionCommand{}, errors.New("opencode: workspace is required")
	}
	if req.InitialPrompt == "" && req.SystemPrompt == "" {
		return adapters.SessionCommand{}, errors.New("opencode: initial prompt or system prompt is required")
	}

	prompt := joinPrompt(req.SystemPrompt, req.InitialPrompt)
	binary, args := adapters.ResolveExec(req.Command, func() (string, []string) {
		b := req.Params.Binary
		if b == "" {
			b = defaultBinary
		}
		return b, buildInteractiveArgs(req, prompt)
	})
	if _, err := exec.LookPath(binary); err != nil {
		return adapters.SessionCommand{}, fmt.Errorf("opencode: %q not found in PATH (install OpenCode CLI)", binary)
	}

	return adapters.SessionCommand{
		Binary: binary,
		Args:   args,
		Env:    os.Environ(),
		Dir:    req.Workspace,
	}, nil
}
