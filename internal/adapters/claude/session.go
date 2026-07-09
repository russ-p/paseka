package claude

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/paseka/paseka/internal/adapters"
)

// SessionAdapter builds commands for interactive Claude Code CLI sessions.
type SessionAdapter struct{}

func NewSession() *SessionAdapter {
	return &SessionAdapter{}
}

func (a *SessionAdapter) Name() string {
	return adapterName
}

// SessionCommand builds a claude invocation for interactive PTY sessions.
// Detached attach still uses the interactive TUI; headless -p belongs to Adapter.Run().
func (a *SessionAdapter) SessionCommand(req adapters.SessionRequest) (adapters.SessionCommand, error) {
	if req.Workspace == "" {
		return adapters.SessionCommand{}, errors.New("claude: workspace is required")
	}
	if req.InitialPrompt == "" {
		return adapters.SessionCommand{}, errors.New("claude: initial prompt is required")
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
		return adapters.SessionCommand{}, fmt.Errorf("claude: %q not found in PATH (install Claude Code CLI)", binary)
	}

	env := os.Environ()
	if req.Params.APIKey != "" {
		env = append(env, "ANTHROPIC_API_KEY="+req.Params.APIKey)
	}

	return adapters.SessionCommand{
		Binary: binary,
		Args:   args,
		Env:    env,
		Dir:    req.Workspace,
	}, nil
}

// buildInteractiveArgs launches the Claude Code TUI seeded with an initial
// prompt. Permission prompts are handled in the TUI, so --permission-mode is
// only forwarded for plan mode.
func buildInteractiveArgs(req adapters.SessionRequest, prompt string) []string {
	p := req.Params
	var args []string

	if p.Plan {
		args = append(args, "--permission-mode", "plan")
	}
	if p.Model != "" {
		args = append(args, "--model", p.Model)
	}

	args = append(args, prompt)
	return args
}
