package opencode

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
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
// A fresh session is pre-created via the OpenCode server (the create-chat analog); an explicit
// ResumeSessionID skips pre-create and continues that session directly. Because the TUI ignores
// --prompt whenever --session is set, any kickoff/continue prompt is delivered out-of-band through
// the TUI's own loopback HTTP server (--port), which keeps a deterministic provider session id.
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

	env := os.Environ()
	var controlArgs []string
	delivery := (*adapters.SessionPromptDelivery)(nil)
	promptArg := prompt
	if !commandOverride && providerSessionID != "" && prompt != "" {
		d, ctrlArgs, envErr := buildPromptDelivery(req, providerSessionID, prompt)
		if envErr != nil {
			logging.Component("adapter").Warn("opencode prompt delivery unavailable",
				logging.F("adapter", adapterName),
				logging.F("bee", req.Bee),
				logging.F("trace", req.TraceID),
				logging.F("agent", req.AgentID),
				logging.F("error", envErr.Error()),
			)
		} else {
			delivery = d
			controlArgs = ctrlArgs
			env = append(envWithout(env, "OPENCODE_SERVER_PASSWORD", "OPENCODE_SERVER_USERNAME"),
				"OPENCODE_SERVER_PASSWORD="+d.Password,
				"OPENCODE_SERVER_USERNAME="+d.Username)
			promptArg = ""
		}
	}

	if !commandOverride {
		args = buildInteractiveArgs(req, promptArg, providerSessionID)
		args = append(args, controlArgs...)
	}

	return adapters.SessionCommand{
		Binary:            binary,
		Args:              args,
		Env:               env,
		Dir:               req.Workspace,
		ProviderSessionID: providerSessionID,
		Prompt:            delivery,
	}, nil
}

// buildPromptDelivery reserves a loopback control port for the TUI and describes
// how to POST the initial prompt to the pre-created session through it.
func buildPromptDelivery(req adapters.SessionRequest, providerSessionID, prompt string) (*adapters.SessionPromptDelivery, []string, error) {
	port, err := freeLoopbackPort()
	if err != nil {
		return nil, nil, err
	}
	password, err := randomToken()
	if err != nil {
		return nil, nil, err
	}
	provider, modelID := splitModel(resolveModel(req.Params))
	agent := ""
	if req.Params.Plan {
		agent = "plan"
	}
	delivery := &adapters.SessionPromptDelivery{
		BaseURL:       fmt.Sprintf("http://127.0.0.1:%d", port),
		SessionID:     providerSessionID,
		Username:      "opencode",
		Password:      password,
		Text:          prompt,
		Agent:         agent,
		Variant:       strings.TrimSpace(req.Params.Thinking),
		ModelProvider: provider,
		ModelID:       modelID,
	}
	controlArgs := []string{"--port", strconv.Itoa(port), "--hostname", "127.0.0.1"}
	return delivery, controlArgs, nil
}

// splitModel turns a resolved "provider/model" (or bare "model") into its parts.
func splitModel(model string) (provider, id string) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "", ""
	}
	if i := strings.IndexByte(model, '/'); i >= 0 {
		return model[:i], model[i+1:]
	}
	return "", model
}

// envWithout returns env with any entries for the given keys removed, so a
// freshly generated control-server secret always wins over an inherited one.
func envWithout(env []string, keys ...string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		skip := false
		for _, k := range keys {
			if strings.HasPrefix(e, k+"=") {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, e)
		}
	}
	return out
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
