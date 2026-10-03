package pi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

// SessionAdapter builds commands for interactive Pi CLI sessions.
type SessionAdapter struct{}

func NewSession() *SessionAdapter {
	return &SessionAdapter{}
}

func (a *SessionAdapter) Name() string {
	return adapterName
}

// SessionCommand builds a Pi invocation for interactive PTY sessions.
// Detached attach still uses the interactive TUI; headless -p belongs to Adapter.Run().
func (a *SessionAdapter) SessionCommand(req adapters.SessionRequest) (adapters.SessionCommand, error) {
	if req.Workspace == "" {
		return adapters.SessionCommand{}, errors.New("pi: workspace is required")
	}
	if req.InitialPrompt == "" && req.SystemPrompt == "" {
		return adapters.SessionCommand{}, errors.New("pi: initial prompt or system prompt is required")
	}
	if req.ColonyRoot == "" || req.TraceID == "" || req.AgentID == "" {
		return adapters.SessionCommand{}, errors.New("pi: colony root, traceId, and agentId are required")
	}

	prompt := req.InitialPrompt
	runDir := runs.Dir{
		ColonyRoot: req.ColonyRoot,
		TraceID:    req.TraceID,
		AgentID:    req.AgentID,
	}
	sessionDir := sessionDirFor(req.ColonyRoot, req.TraceID, req.AgentID)
	systemFile := ""
	if req.SystemPrompt != "" {
		systemFile = runDir.SystemPath()
	}

	binary, args := adapters.ResolveExec(req.Command, func() (string, []string) {
		b := req.Params.Binary
		if b == "" {
			b = defaultBinary
		}
		return b, buildInteractiveArgs(req, sessionDir, prompt, systemFile)
	})
	if _, err := exec.LookPath(binary); err != nil {
		return adapters.SessionCommand{}, fmt.Errorf("pi: %q not found in PATH (install Pi CLI)", binary)
	}

	providerSessionID := ""
	if len(req.Command) > 0 {
		providerSessionID = strings.TrimSpace(adapters.FlagValue(args, "--session-id"))
	} else {
		providerSessionID = strings.TrimSpace(req.AgentID)
	}

	return adapters.SessionCommand{
		Binary:            binary,
		Args:              args,
		Env:               os.Environ(),
		Dir:               req.Workspace,
		ProviderSessionID: providerSessionID,
	}, nil
}

// SessionUsage sums the TUI's session file after exit. The TUI writes no
// machine-readable stdout, but it appends to the same run-scoped session dir the
// AFK path reads, so token totals are recoverable for both.
func (a *SessionAdapter) SessionUsage(_ context.Context, handle adapters.SessionHandle) *protocol.Usage {
	if handle.ColonyRoot == "" || handle.TraceID == "" || handle.AgentID == "" {
		return nil
	}
	return usageFromSessionDir(sessionDirFor(handle.ColonyRoot, handle.TraceID, handle.AgentID))
}

func buildInteractiveArgs(req adapters.SessionRequest, sessionDir, prompt, systemFile string) []string {
	p := req.Params
	args := []string{
		"--session-dir", sessionDir,
		"--session-id", req.AgentID,
	}
	if systemFile != "" {
		args = append(args, "--append-system-prompt", systemFile)
	}
	if p.Model != "" {
		args = append(args, "--model", p.Model)
	}
	if p.Provider != "" {
		args = append(args, "--provider", p.Provider)
	}
	if p.Thinking != "" {
		args = append(args, "--thinking", p.Thinking)
	}
	if p.Plan {
		args = append(args, "--plan")
	}
	if p.APIKey != "" {
		args = append(args, "--api-key", p.APIKey)
	}
	if prompt != "" {
		args = append(args, "--", prompt)
	}
	return args
}
