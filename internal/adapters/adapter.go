package adapters

import (
	"context"

	"github.com/paseka/paseka/internal/protocol"
)

// Artifact is a normalized output from an adapter run.
type Artifact struct {
	Kind    string // result, diff, stdout, stream-json
	Path    string // optional file path
	Content string
}

// RunRequest is passed to an adapter when a bee is dispatched.
type RunRequest struct {
	Bee        string // role name from bees/*.yaml
	Prompt     string // rendered prompt
	ColonyRoot string // git repo root — .paseka/runs/ always lives here
	Workspace  string // absolute path: repo root or worktree (adapter cwd)
	Sector     string // resolved sector name, if any
	SectorPath string // relative sector path within colony/worktree
	Params     RunParams
	Command    []string // optional full argv; overrides Params-based arg building when set
	TraceID    string
	AgentID    string // unique id per spawned agent invocation
	TaskID     string
	Task       string
	Intent     string
	Insights   []string
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

// RunResult is the normalized adapter output.
type RunResult struct {
	Status    string // completed | failed | cancelled
	Summary   string // normalized run summary (legacy result.txt, stream-json, or empty)
	Output    string // preferred display text (summary or stdout)
	Events    []protocol.Event
	Artifacts []Artifact
	ExitCode  int
	Err       error
	Warnings  []string // advisory runtime notices (e.g. undeclared publishes)
}

// Adapter launches an external agent and returns normalized results.
type Adapter interface {
	Name() string
	Run(ctx context.Context, req RunRequest) (*RunResult, error)
}
