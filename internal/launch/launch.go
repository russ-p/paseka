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
	"sort"
	"strings"
	"time"

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
	// RedactedEnv names the plan env keys whose values must not appear in logs:
	// the injected credential and the declared params.env pass-through keys.
	RedactedEnv []string
	// Dir is the resolved working directory; empty means inherit the parent cwd.
	Dir string
	// Resume reports whether the plan continues an existing provider session.
	Resume bool
	// Raw reports whether this launch opted out of structured output.
	Raw bool
	// Timeout bounds an AFK run (params.timeout), measured from launch. Zero
	// means unbounded. Interactive plans always carry zero: the human is the
	// timeout.
	Timeout time.Duration
	// Warnings are operator-facing notices raised during resolution.
	Warnings []string

	// attribution records which layer set each field, so `doctor --explain` and
	// the Console can describe a launch without reading Go. Use Attribution()
	// for the display-ready, ordered view.
	attribution map[string]Layer
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
	beeLayer := colony.LayerFromBee(req.Bee)
	params := colony.MergeRunParamsLayers(beeLayer, req.Extra)
	// Record whether an alias decided the model before it is substituted, so the
	// attribution can name the alias layer rather than the layer that declared
	// the alias key.
	_, aliasApplied := colony.ResolveModel(params.Model, req.Aliases)
	colony.ApplyModelAliases(&params, req.Aliases)

	// The one model-id mapping: split the vendor off for the adapters that need
	// it. The vendor lives in the model id, so the mapping is values-in,
	// values-out with no deprecated key to fold.
	mapping := MapModel(req.Adapter, params.Model)
	params.Model = mapping.Model
	params.Provider = mapping.Provider

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
		attribution:   attributeFields(beeLayer, req.Extra, aliasApplied),
	}
	if plan.Binary == "" {
		plan.Binary = spec.DefaultBinary
	}
	// params.timeout bounds AFK runs only. Interactive sessions keep the human
	// as the timeout, so the plan never carries one for them.
	if req.Mode == AFK {
		plan.Timeout = params.Timeout
	}

	// Resolve env before the command so a command template can reference the
	// same plan env the child receives, not only the fixed CommandVars.
	plan.Env, plan.RedactedEnv = resolveEnv(req, params, spec)

	command, form, err := resolveCommand(req, plan)
	if err != nil {
		return LaunchPlan{}, err
	}
	plan.Command = command
	plan.Form = form

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

func resolveCommand(req Request, plan LaunchPlan) ([]string, Form, error) {
	if req.Bee.Command.IsSet() && !req.Resume {
		command, err := req.Bee.Command.RenderCommand(commandVars(req, plan))
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

// commandVars derives the command-template substitution values from the plan,
// so command mode and params mode share the same paths and env conventions.
func commandVars(req Request, plan LaunchPlan) colony.CommandVars {
	resultFile := req.RunDir.ResultPath()
	if abs, err := filepath.Abs(resultFile); err == nil {
		resultFile = abs
	}
	return colony.CommandVars{
		Prompt:       plan.CommandPrompt,
		SystemPrompt: req.SystemPrompt,
		SystemFile:   req.RunDir.SystemPath(),
		Workspace:    req.Workspace,
		TraceID:      req.RunDir.TraceID,
		AgentID:      req.RunDir.AgentID,
		TaskID:       req.TaskID,
		ColonyRoot:   req.RunDir.ColonyRoot,
		ResultFile:   resultFile,
		RunDir:       req.RunDir.Root(),
		Env:          adapters.EnvMap(plan.Env),
	}
}

// resolveEnv builds the complete child environment and records the plan env keys
// whose values must be redacted in logs.
//
// By default the inherited environment is kept whole (undeclared vendor keys
// keep flowing) and declared params.env pass-through plus the adapter's resolved
// credential are layered on top. In EnvModeStrict the inherited environment is
// reduced to non-secret process essentials, so an undeclared secret no longer
// reaches the child.
func resolveEnv(req Request, params adapters.RunParams, spec adapters.LaunchSpec) ([]string, []string) {
	base := req.EnvBase
	if base == nil {
		base = os.Environ()
	}

	// Expand `$HOST_VAR` references against the inherited environment before any
	// strict filtering, so a declared reference still resolves.
	declared := expandDeclaredEnv(params.Env, base)

	env := base
	if params.EnvMode == adapters.EnvModeStrict {
		env = filterStrictEnv(base)
	}
	for _, name := range sortedEnvKeys(declared) {
		env = setEnv(env, name, declared[name])
	}

	redacted := make([]string, 0, len(declared)+1)
	for name := range declared {
		redacted = append(redacted, name)
	}
	if spec.CredentialEnv != "" && params.APIKey != "" {
		env = setEnv(env, spec.CredentialEnv, params.APIKey)
		redacted = append(redacted, spec.CredentialEnv)
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
	sort.Strings(redacted)
	return env, dedupeStrings(redacted)
}

// expandDeclaredEnv resolves each declared params.env value: a `$HOST_VAR` or
// `${HOST_VAR}` reference reads the inherited environment, anything else is a
// literal.
func expandDeclaredEnv(declared map[string]string, host []string) map[string]string {
	if len(declared) == 0 {
		return nil
	}
	hostVals := adapters.EnvMap(host)
	out := make(map[string]string, len(declared))
	for name, val := range declared {
		out[name] = expandHostRef(val, hostVals)
	}
	return out
}

func expandHostRef(val string, host map[string]string) string {
	v := strings.TrimSpace(val)
	if !strings.HasPrefix(v, "$") {
		return val
	}
	ref := strings.TrimPrefix(v, "$")
	if strings.HasPrefix(ref, "{") && strings.HasSuffix(ref, "}") {
		ref = ref[1 : len(ref)-1]
	}
	if ref == "" {
		return val
	}
	return host[ref]
}

// strictEnvAllow is the small set of non-secret process essentials a strict
// launch keeps from the inherited environment. Everything else must be declared
// through params.env or injected as the resolved credential.
var strictEnvAllow = map[string]bool{
	"PATH":    true,
	"HOME":    true,
	"USER":    true,
	"LOGNAME": true,
	"SHELL":   true,
	"TERM":    true,
	"TMPDIR":  true,
	"TZ":      true,
	"LANG":    true,
	"PWD":     true,
}

func filterStrictEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if strictEnvAllow[key] || strings.HasPrefix(key, "LC_") || strings.HasPrefix(key, "PASEKA_") {
			out = append(out, entry)
		}
	}
	return out
}

func setEnv(env []string, key, val string) []string {
	prefix := key + "="
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = prefix + val
			return env
		}
	}
	return append(env, prefix+val)
}

func sortedEnvKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func dedupeStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := in[:1]
	for _, s := range in[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
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
