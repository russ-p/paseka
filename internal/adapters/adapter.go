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
}

// RunParams holds adapter CLI flags and shared bee params.
type RunParams struct {
	Model        string
	OutputFormat string // text | json | stream-json | rpc
	Trust        bool
	Force        bool
	Plan         bool
	Binary       string // adapter binary override
	APIKey       string // optional; from machine-local api_key_env when set
	Provider     string
	Thinking     string
}

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
