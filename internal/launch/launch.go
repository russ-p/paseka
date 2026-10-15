// Package launch resolves how a bee's adapter process should be launched.
//
// A single ResolveLaunch call turns the layers that used to be assembled
// separately by the AFK dispatcher and the interactive session manager — bee
// params, the machine-local layer, model aliases, the command template, the
// provider join, the resolved binary, and the child environment — into one
// LaunchPlan. Both call sites consume that plan, so they cannot drift apart.
//
// The resolver is the only place that reads the ambient environment, merges run
// params, or joins a provider prompt. Adapters stay pure argv mappers: they take
// the plan's binary, env, and cwd and add their own flags.
package launch

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/adapters/claude"
	"github.com/russ-p/paseka/internal/adapters/cursor"
	"github.com/russ-p/paseka/internal/adapters/opencode"
	"github.com/russ-p/paseka/internal/adapters/pi"
	"github.com/russ-p/paseka/internal/adapters/script"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/runs"
)

// Mode distinguishes the two launch shapes that share resolution.
type Mode string

const (
	// AFK is a one-shot headless adapter run (Adapter.Run).
	AFK Mode = "afk"
	// Interactive is a PTY-backed session (SessionAdapter.SessionCommand).
	Interactive Mode = "interactive"
)

// Form is the argv strategy the plan selected.
type Form string

const (
	// FormParams means the adapter builds argv from merged params.
	FormParams Form = "params"
	// FormCommand means a bee command template supplied the full argv.
	FormCommand Form = "command"
	// FormScript is FormCommand for the script adapter (which always needs a command).
	FormScript Form = "script"
)

// Request carries everything resolution needs that is not adapter-owned.
type Request struct {
	Mode  Mode
	Bee   colony.Bee
	Extra adapters.RunParamsLayer
	// Adapter is the resolved adapter name; its built-in launch defaults come
	// from defaultSpec.
	Adapter string
	Aliases map[string]string

	Workspace string
	TaskID    string
	// Prompt and SystemPrompt are the already-rendered prompts. On a resume,
	// Prompt is the trimmed continue line.
	Prompt       string
	SystemPrompt string
	// Resume suppresses the provider prompt join and the command template
	// override, matching interactive resume semantics.
	Resume bool
	// Raw is the runtime-owned `--raw` escape hatch: structured output parsing is
	// given up for plain text on this invocation only. Interactive sessions never
	// set it.
	Raw bool

	RunDir runs.Dir
	// EnvBase overrides the inherited environment. Nil means os.Environ().
	EnvBase []string
}

// LaunchPlan is the resolved launch: the single source of truth both call sites use.
type LaunchPlan struct {
	Adapter string
	Mode    Mode
	Form    Form

	// Binary is params.binary or the adapter's built-in default.
	Binary string
	// Params are the fully merged run params (bee + machine-local layers, aliases applied).
	Params adapters.RunParams
	// Command is the rendered bee command argv, set for FormCommand/FormScript.
	Command []string
	// CommandPrompt is the joined prompt used in command-template substitution.
	CommandPrompt string
	// Env is the complete child environment, or nil to inherit.
	Env []string
	// Dir is the resolved working directory; empty means inherit the parent cwd.
	Dir string
	// Resume reports whether the plan continues an existing provider session.
	Resume bool
	// Raw reports whether this launch opted out of structured output.
	Raw bool
}

// ResolveLaunch merges every launch layer into one plan.
func ResolveLaunch(req Request) (LaunchPlan, error) {
	if req.Adapter == "" {
		return LaunchPlan{}, fmt.Errorf("launch: adapter is required")
	}
	if req.Workspace == "" {
		return LaunchPlan{}, fmt.Errorf("launch: workspace is required")
	}

	spec := defaultSpec(req.Adapter)

	// The one params merge. Everything downstream reads plan.Params.
	params := colony.MergeRunParamsLayers(colony.LayerFromBee(req.Bee), req.Extra)
	colony.ApplyModelAliases(&params, req.Aliases)

	// The one provider prompt join. Cursor and OpenCode take a single positional
	// prompt built from system + task; a resume passes the continue line alone.
	commandPrompt := req.Prompt
	if !req.Resume && (req.Adapter == "cursor" || req.Adapter == "opencode") {
		commandPrompt = cursor.JoinPrompt(req.SystemPrompt, req.Prompt)
	}

	plan := LaunchPlan{
		Adapter:       req.Adapter,
		Mode:          req.Mode,
		Params:        params,
		CommandPrompt: commandPrompt,
		Resume:        req.Resume,
		Raw:           req.Raw,
		Dir:           resolveDir(req),
		Binary:        params.Binary,
	}
	if plan.Binary == "" {
		plan.Binary = spec.DefaultBinary
	}

	command, form, err := resolveCommand(req, commandPrompt)
	if err != nil {
		return LaunchPlan{}, err
	}
	plan.Command = command
	plan.Form = form

	plan.Env = resolveEnv(req, params, spec)
	return plan, nil
}

// resolveDir picks the child cwd. Cursor's AFK run historically inherits the
// dispatcher cwd (there is no --workspace-less chdir), which the Phase 0
// characterization pins; every other launch chdirs into the workspace.
func resolveDir(req Request) string {
	if req.Mode == AFK && req.Adapter == "cursor" {
		return ""
	}
	return req.Workspace
}

func resolveCommand(req Request, commandPrompt string) ([]string, Form, error) {
	if req.Bee.Command.IsSet() && !req.Resume {
		command, err := req.Bee.Command.RenderCommand(commandVars(req, commandPrompt))
		if err != nil {
			return nil, "", fmt.Errorf("launch: render command: %w", err)
		}
		if req.Adapter == "script" {
			return command, FormScript, nil
		}
		return command, FormCommand, nil
	}
	if req.Adapter == "script" {
		return nil, "", fmt.Errorf("launch: bee %q: adapter script requires command", req.Bee.Role)
	}
	return nil, FormParams, nil
}

func commandVars(req Request, commandPrompt string) colony.CommandVars {
	resultFile := req.RunDir.ResultPath()
	if abs, err := filepath.Abs(resultFile); err == nil {
		resultFile = abs
	}
	return colony.CommandVars{
		Prompt:       commandPrompt,
		SystemPrompt: req.SystemPrompt,
		SystemFile:   req.RunDir.SystemPath(),
		Workspace:    req.Workspace,
		TraceID:      req.RunDir.TraceID,
		AgentID:      req.RunDir.AgentID,
		TaskID:       req.TaskID,
		ColonyRoot:   req.RunDir.ColonyRoot,
		ResultFile:   resultFile,
		RunDir:       req.RunDir.Root(),
	}
}

// resolveEnv builds the complete child environment: the inherited base, the
// adapter's resolved credential (when it carries one in the environment), and
// the PASEKA_* contract for script bees.
func resolveEnv(req Request, params adapters.RunParams, spec adapters.LaunchSpec) []string {
	env := req.EnvBase
	if env == nil {
		env = os.Environ()
	}
	if spec.CredentialEnv != "" && params.APIKey != "" {
		env = append(env, spec.CredentialEnv+"="+params.APIKey)
	}
	if req.Adapter == "script" {
		env = adapters.ScriptEnv(env, adapters.RunRequest{
			Bee:        req.Bee.Role,
			ColonyRoot: req.RunDir.ColonyRoot,
			Workspace:  req.Workspace,
			TraceID:    req.RunDir.TraceID,
			AgentID:    req.RunDir.AgentID,
			TaskID:     req.TaskID,
		}, req.RunDir)
	}
	return env
}

// defaultSpec reports the built-in launch defaults for an adapter name. It
// instantiates the stateless adapter only to read its declared spec.
func defaultSpec(adapter string) adapters.LaunchSpec {
	switch adapter {
	case "cursor":
		return cursor.New().LaunchSpec()
	case "pi":
		return pi.New().LaunchSpec()
	case "claude":
		return claude.New().LaunchSpec()
	case "opencode":
		return opencode.New().LaunchSpec()
	case "script":
		return script.New().LaunchSpec()
	default:
		return adapters.LaunchSpec{}
	}
}
