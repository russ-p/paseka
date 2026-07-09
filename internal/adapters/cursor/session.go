package cursor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/paseka/paseka/internal/adapters"
)

// SessionAdapter builds commands for interactive Cursor Agent CLI sessions.
type SessionAdapter struct{}

func NewSession() *SessionAdapter {
	return &SessionAdapter{}
}

func (a *SessionAdapter) Name() string {
	return adapterName
}

// SessionCommand builds an agent invocation for interactive PTY sessions.
// Detached attach (console / PTY hub) still uses the interactive TUI; headless
// -p belongs to Adapter.Run(), not SessionAdapter.
func (a *SessionAdapter) SessionCommand(req adapters.SessionRequest) (adapters.SessionCommand, error) {
	if req.Workspace == "" {
		return adapters.SessionCommand{}, errors.New("cursor: workspace is required")
	}
	if req.InitialPrompt == "" {
		return adapters.SessionCommand{}, errors.New("cursor: initial prompt is required")
	}

	prompt := req.InitialPrompt
	binary, args := adapters.ResolveExec(req.Command, func() (string, []string) {
		b := req.Params.Binary
		if b == "" {
			b = defaultBinary
		}
		return b, buildInteractiveArgs(req, prompt)
	})
	if _, err := exec.LookPath(binary); err != nil {
		return adapters.SessionCommand{}, fmt.Errorf("cursor: %q not found in PATH (install Cursor CLI)", binary)
	}

	env := os.Environ()
	if req.Params.APIKey != "" {
		env = append(env, "CURSOR_API_KEY="+req.Params.APIKey)
	}

	return adapters.SessionCommand{
		Binary: binary,
		Args:   args,
		Env:    env,
		Dir:    req.Workspace,
	}, nil
}

func buildInteractiveArgs(req adapters.SessionRequest, prompt string) []string {
	p := req.Params
	args := []string{
		"--workspace", req.Workspace,
	}

	// --trust is headless-only (-p); interactive sessions prompt in the TUI instead.
	if p.Force {
		args = append(args, "--force")
	}
	if p.Plan {
		args = append(args, "--plan")
	}
	if p.Model != "" {
		args = append(args, "--model", p.Model)
	}
	if p.APIKey != "" {
		args = append(args, "--api-key", p.APIKey)
	}

	args = append(args, prompt)
	return args
}
