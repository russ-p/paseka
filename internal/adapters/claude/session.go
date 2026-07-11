package claude

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/runs"
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
	if req.InitialPrompt == "" && req.SystemPrompt == "" {
		return adapters.SessionCommand{}, errors.New("claude: initial prompt or system prompt is required")
	}
	if req.SystemPrompt != "" && (req.ColonyRoot == "" || req.TraceID == "" || req.AgentID == "") {
		return adapters.SessionCommand{}, errors.New("claude: colony root, traceId, and agentId are required for system prompt injection")
	}

	prompt := req.InitialPrompt
	systemFile := ""
	if req.SystemPrompt != "" {
		runDir := runs.Dir{
			ColonyRoot: req.ColonyRoot,
			TraceID:    req.TraceID,
			AgentID:    req.AgentID,
		}
		systemFile = runDir.SystemPath()
	}

	binary, args := adapters.ResolveExec(req.Command, func() (string, []string) {
		b := req.Params.Binary
		if b == "" {
			b = defaultBinary
		}
		return b, buildInteractiveArgs(req, prompt, systemFile)
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
func buildInteractiveArgs(req adapters.SessionRequest, prompt, systemFile string) []string {
	p := req.Params
	var args []string

	if systemFile != "" {
		args = append(args, "--append-system-prompt-file", systemFile)
	}
	if p.Plan {
		args = append(args, "--permission-mode", "plan")
	}
	if p.Model != "" {
		args = append(args, "--model", p.Model)
	}
	if prompt != "" {
		args = append(args, prompt)
	}
	return args
}
