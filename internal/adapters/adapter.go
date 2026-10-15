package adapters

import (
	"context"

	"github.com/russ-p/paseka/internal/protocol"
)

// Artifact is a normalized output from an adapter run.
type Artifact struct {
	Kind    string // result, diff, stdout, stream-json
	Path    string // optional file path
	Content string
}

// RunRequest is passed to an adapter when a bee is dispatched.
type RunRequest struct {
	Bee          string // role name from bees/*.yaml
	Prompt       string // rendered user/task prompt
	SystemPrompt string // rendered system/role context
	ColonyRoot   string // git repo root — .paseka/runs/ always lives here
	Workspace    string // absolute path: repo root or worktree (adapter cwd)
	Sector       string // resolved sector name, if any
	SectorPath   string // relative sector path within colony/worktree
	Params       RunParams
	Command      []string // optional full argv; overrides Params-based arg building when set
	TraceID      string
	AgentID      string // unique id per spawned agent invocation
	TaskID       string
	Task         string
	Intent       string
	Insights     []string
	Profile      string
	// Raw is the runtime-owned `--raw` escape hatch: the adapter requests plain
	// text instead of its structured mode, so usage and providerSessionId
	// accounting degrade for this invocation only.
	Raw bool
	// Binary, Env, and Dir are resolved by the launch resolver. When set they
	// are authoritative: the adapter must not re-read machine config or the
	// parent environment. An empty Dir means "inherit the parent cwd" and a nil
	// Env means "inherit the parent environment".
	Binary string
	Env    []string
	Dir    string
	// RedactedEnv names plan env keys whose values must never appear in logs:
	// the injected credential and the declared pass-through keys.
	RedactedEnv []string
}

// RunParams holds adapter CLI flags and shared bee params. Output format is
// intentionally absent: the runtime always requests the adapter's structured
// mode (see RunRequest.Raw for the plain-text escape hatch).
type RunParams struct {
	Model     string
	Trust     bool
	Force     bool
	Plan      bool
	Binary    string // adapter binary override
	APIKey    string // optional; from machine-local api_key_env when set
	Provider  string
	Reasoning string // low | medium | high | passthrough
	// Env is the declared pass-through environment from params.env. Values are
	// literals or `$HOST_VAR` references resolved against the inherited
	// environment at launch.
	Env map[string]string
	// EnvMode selects the child environment policy. The empty string inherits
	// the parent environment (undeclared keys keep flowing); EnvModeStrict strips
	// every undeclared key.
	EnvMode string
}

// EnvModeStrict is the opt-in params.env_mode value: the child receives only the
// declared pass-through, the injected credential, and a small allow-list of
// non-secret process essentials (PATH, HOME, locale, and PASEKA_*).
const EnvModeStrict = "strict"

// RunParamsLayer is a RunParams together with the set of keys its source
// actually declared. RunParams uses plain strings and bools, so a zero value
// cannot say whether it was set: "trust unset" and "trust: false" both read as
// Trust == false. Recording the declared keys restores that distinction so the
// layered merge can be presence-aware and tri-state (an explicit false beats a
// lower layer's true). The keys are the bee params names plus api_key.
type RunParamsLayer struct {
	Params   RunParams
	Declared map[string]bool
}

// Declares reports whether the source set the given params key.
func (l RunParamsLayer) Declares(key string) bool {
	return l.Declared[key]
}

// RunResult is the normalized adapter output.
type RunResult struct {
	Status            string // completed | failed | cancelled
	Summary           string // normalized run summary (summary.md, legacy result.txt, stream-json, or empty)
	Output            string // preferred display text (summary or stdout)
	Events            []protocol.Event
	Artifacts         []Artifact
	Usage             *protocol.Usage // optional; set when the adapter can report LLM tokens
	ProviderSessionID string          // optional; native provider session id when resolved
	ExitCode          int
	Err               error
	Warnings          []string // advisory runtime notices (e.g. undeclared publishes)
}

// Adapter launches an external agent and returns normalized results.
type Adapter interface {
	Name() string
	Run(ctx context.Context, req RunRequest) (*RunResult, error)
}
