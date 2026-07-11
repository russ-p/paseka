package cursor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/adapters/systeminject"
	"github.com/paseka/paseka/internal/runs"
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
	if req.InitialPrompt == "" && req.SystemPrompt == "" {
		return adapters.SessionCommand{}, errors.New("cursor: initial prompt or system prompt is required")
	}
	if req.SystemPrompt != "" && (req.ColonyRoot == "" || req.TraceID == "" || req.AgentID == "") {
		return adapters.SessionCommand{}, errors.New("cursor: colony root, traceId, and agentId are required for system prompt injection")
	}

	prompt := req.InitialPrompt
	var pluginDir string
	if req.SystemPrompt != "" {
		runDir := runs.Dir{
			ColonyRoot: req.ColonyRoot,
			TraceID:    req.TraceID,
			AgentID:    req.AgentID,
		}
		dir, err := systeminject.WriteCursorPlugin(runDir, req.SystemPrompt)
		if err != nil {
			return adapters.SessionCommand{}, fmt.Errorf("cursor: write system plugin: %w", err)
		}
		pluginDir = dir
	}

	binary, args := adapters.ResolveExec(req.Command, func() (string, []string) {
		b := req.Params.Binary
		if b == "" {
			b = defaultBinary
		}
		return b, buildInteractiveArgs(req, prompt, pluginDir)
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

func buildInteractiveArgs(req adapters.SessionRequest, prompt, pluginDir string) []string {
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
	if pluginDir != "" {
		args = append(args, "--plugin-dir", pluginDir)
	}
	if prompt != "" {
		args = append(args, prompt)
	}
	return args
}
