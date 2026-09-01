package cursor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/logging"
)

const createChatTimeout = 15 * time.Second

var chatUUIDRe = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

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

	prompt := JoinPrompt(req.SystemPrompt, req.InitialPrompt)

	binary, args := adapters.ResolveExec(req.Command, func() (string, []string) {
		b := req.Params.Binary
		if b == "" {
			b = defaultBinary
		}
		return b, buildInteractiveArgs(req, prompt, "")
	})
	if _, err := exec.LookPath(binary); err != nil {
		return adapters.SessionCommand{}, fmt.Errorf("cursor: %q not found in PATH (install Cursor CLI)", binary)
	}

	env := os.Environ()
	if req.Params.APIKey != "" {
		env = append(env, "CURSOR_API_KEY="+req.Params.APIKey)
	}

	providerSessionID := ""
	if len(req.Command) > 0 {
		providerSessionID = strings.TrimSpace(adapters.FlagValue(args, "--resume"))
	} else {
		id, err := runCreateChat(binary, env, req)
		if err != nil {
			logging.Component("adapter").Warn("cursor create-chat failed",
				logging.F("adapter", adapterName),
				logging.F("bee", req.Bee),
				logging.F("trace", req.TraceID),
				logging.F("agent", req.AgentID),
				logging.F("error", err.Error()),
			)
		} else {
			providerSessionID = id
			args = buildInteractiveArgs(req, prompt, id)
		}
	}

	return adapters.SessionCommand{
		Binary:            binary,
		Args:              args,
		Env:               env,
		Dir:               req.Workspace,
		ProviderSessionID: providerSessionID,
	}, nil
}

func buildInteractiveArgs(req adapters.SessionRequest, prompt, resumeID string) []string {
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
	if resumeID != "" {
		args = append(args, "--resume", resumeID)
	}
	if prompt != "" {
		args = append(args, prompt)
	}
	return args
}

func runCreateChat(binary string, env []string, req adapters.SessionRequest) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), createChatTimeout)
	defer cancel()

	args := []string{"create-chat"}
	if req.Params.APIKey != "" {
		args = append(args, "--api-key", req.Params.APIKey)
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("create-chat: %s", msg)
	}
	id := parseCreateChatID(stdout.String())
	if id == "" {
		return "", errors.New("create-chat: empty session id")
	}
	return id, nil
}

func parseCreateChatID(stdout string) string {
	return strings.TrimSpace(chatUUIDRe.FindString(stdout))
}
