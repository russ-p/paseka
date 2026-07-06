package colony

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/paseka/paseka/internal/gitroot"
	"gopkg.in/yaml.v3"
)

// InitOptions configures paseka init.
type InitOptions struct {
	StartDir string // directory to resolve git root from; default cwd
}

// InitResult summarizes what init did.
type InitResult struct {
	ColonyRoot string
	Slug       string
	HomeDir    string
	Created    []string
	Skipped    []string
}

// Init bootstraps .paseka/ in the repo and home config. Idempotent.
func Init(opts InitOptions) (InitResult, error) {
	start := opts.StartDir
	if start == "" {
		wd, err := os.Getwd()
		if err != nil {
			return InitResult{}, err
		}
		start = wd
	}

	repoRoot, err := gitroot.Find(start)
	if err != nil {
		return InitResult{}, fmt.Errorf("init: %w", err)
	}

	origin, _ := gitroot.OriginURL(repoRoot)

	manifest, err := LoadColony(repoRoot)
	if err != nil {
		return InitResult{}, err
	}

	baseSlug := ResolveSlug(repoRoot, manifest, origin)
	homeBase, err := HomeBase()
	if err != nil {
		return InitResult{}, err
	}
	slug, err := UniqueHomeSlug(baseSlug, repoRoot, homeBase)
	if err != nil {
		return InitResult{}, err
	}

	res := InitResult{ColonyRoot: repoRoot, Slug: slug}

	if err := res.scaffoldProject(slug, manifest); err != nil {
		return InitResult{}, err
	}
	if err := res.scaffoldHome(slug, repoRoot); err != nil {
		return InitResult{}, err
	}

	homeDir, err := HomeDir(slug)
	if err != nil {
		return InitResult{}, err
	}
	res.HomeDir = homeDir
	return res, nil
}

func (r *InitResult) track(created bool, path string, err error) error {
	if err != nil {
		return err
	}
	if created {
		r.Created = append(r.Created, path)
	} else {
		r.Skipped = append(r.Skipped, path)
	}
	return nil
}

func (r *InitResult) scaffoldProject(slug string, manifest Colony) error {
	root := r.ColonyRoot
	for _, d := range []string{
		PasekaPath(root, "bees"),
		PasekaPath(root, "prompts", "_partials"),
	} {
		if err := mkdirAll(d); err != nil {
			return err
		}
	}

	if err := r.writeColonyManifest(root, slug, manifest); err != nil {
		return err
	}

	files := map[string]string{
		PasekaPath(root, ".gitignore"):                                gitignoreContent,
		PasekaPath(root, "bees", "scout.yaml"):                        scoutBeeYAML,
		PasekaPath(root, "bees", "builder.yaml"):                      builderBeeYAML,
		PasekaPath(root, "prompts", "default.md"):                     defaultPrompt,
		PasekaPath(root, "prompts", "scout.md"):                       scoutPrompt,
		PasekaPath(root, "prompts", "builder.md"):                     builderPrompt,
		PasekaPath(root, "prompts", "_partials", "json-events.md"):    jsonEventsPartial,
		PasekaPath(root, "prompts", "_partials", "task-events.md"):    taskEventsPartial,
		PasekaPath(root, "prompts", "_partials", "insight-events.md"): insightEventsPartial,
	}

	for path, content := range files {
		created, err := writeFileIfMissing(path, []byte(content), 0o644)
		if err := r.track(created, relProject(root, path), err); err != nil {
			return err
		}
	}
	return nil
}

func (r *InitResult) writeColonyManifest(root, slug string, manifest Colony) error {
	path := PasekaPath(root, "colony.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		c := Colony{
			Slug:     slug,
			Defaults: Defaults{PromptTemplate: "default.md"},
		}
		data, err := yaml.Marshal(c)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
		r.Created = append(r.Created, relProject(root, path))
		return nil
	}

	if strings.TrimSpace(manifest.Slug) != "" {
		r.Skipped = append(r.Skipped, relProject(root, path))
		return nil
	}

	manifest.Slug = slug
	if manifest.Defaults.PromptTemplate == "" {
		manifest.Defaults.PromptTemplate = "default.md"
	}
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	r.Created = append(r.Created, relProject(root, path)+" (updated slug)")
	return nil
}

func (r *InitResult) scaffoldHome(slug, repoRoot string) error {
	homeDir, err := HomeDir(slug)
	if err != nil {
		return err
	}
	if err := mkdirAll(filepath.Join(homeDir, "adapters")); err != nil {
		return err
	}

	cfgPath := filepath.Join(homeDir, "config.yaml")
	cfgContent := fmt.Sprintf(`colony_root: %q
slug: %q
nats:
  url: nats://127.0.0.1:4222
adapters:
  cursor:
    api_key_env: CURSOR_API_KEY
`, repoRoot, slug)
	created, err := writeFileIfMissing(cfgPath, []byte(cfgContent), 0o600)
	if err := r.track(created, cfgPath, err); err != nil {
		return err
	}

	statePath := filepath.Join(homeDir, "state.json")
	created, err = writeFileIfMissing(statePath, []byte("{}\n"), 0o644)
	if err := r.track(created, statePath, err); err != nil {
		return err
	}

	cursorPath := filepath.Join(homeDir, "adapters", "cursor.yaml")
	created, err = writeFileIfMissing(cursorPath, []byte(cursorAdapterYAML), 0o644)
	return r.track(created, cursorPath, err)
}

func relProject(root, path string) string {
	if rp, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rp, "..") {
		return rp
	}
	return path
}

const (
	gitignoreContent = `worktrees/
runs/
*.local.yaml
cache/
`
	scoutBeeYAML = `role: scout
adapter: cursor
prompt_template: scout.md
params:
  output_format: stream-json
  trust: true
  force: true
  plan: true
worktree: false
publishes:
  - type: INSIGHT
    kind: task.plan
`
	builderBeeYAML = `role: builder
adapter: cursor
prompt_template: builder.md
params:
  model: composer-2.5
  output_format: stream-json
  trust: true
  force: true
worktree: true
subscribes:
  - type: SIGNAL
    kind: task.ready
    dispatch: task
  - type: VERIFICATION
    kind: verification.failed
    dispatch: direct
publishes:
  - type: MUTATION
    kind: code.proposal
  - type: VERIFICATION
    kind: task.completed
`
	defaultPrompt = `You are a Worker Bee in colony {{.ColonyRoot}}.

Flight trail: {{.TraceID}}

## Task
{{.Task}}
`
	scoutPrompt = `You are Scout Bee. Analyze and plan — do not edit files unless necessary.

Colony: {{.ColonyRoot}}
Flight trail: {{.TraceID}}

## Task
{{.Task}}

## Prior discoveries
{{range .Insights}}- {{.}}
{{end}}

{{template "json-events" .}}
{{template "task-events" .}}
`
	builderPrompt = `You are Builder Bee. Implement the task in the workspace.

Colony: {{.ColonyRoot}}
Flight trail: {{.TraceID}}
Workspace: {{.Workspace}}

## Task
{{.Task}}

## Prior discoveries
{{range .Insights}}- {{.}}
{{end}}

{{template "json-events" .}}
{{template "insight-events" .}}

Runtime persists a human-readable run log at {{.ResultFile}}. If you do not emit run.summary, runtime will synthesize one from the normalized run outcome when possible.
`
	jsonEventsPartial = `When you need to publish a bus event during a run:

1. Build one valid JSON object for the event.
2. Validate and publish it with Paseka CLI via stdin.
3. If validation fails, inspect the returned JSON error, fix the event, and retry once.
4. After successful publish, continue with a normal human-readable summary.

Do not print raw event JSON in the final answer.
Do not write event JSON directly to files.

Use this command form:

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"VERIFICATION","payload":{"kind":"verification.success","summary":"All requirements met"}}
EOF

Each event JSON object must include:
- traceId — current flight trail id ({{.TraceID}})
- agentId — current agent run id ({{.AgentID}})
- type — one of SIGNAL, INSIGHT, MUTATION, VERIFICATION
- payload — event-specific object with required payload.kind

Routing vs narrative:
- VERIFICATION — gate outcomes that drive workflow routing
- INSIGHT — narrative context for audit and prompt memory

If the command returns "ok": false, treat it as a failed publish and correct the payload before continuing.`
	insightEventsPartial = `## Narrative INSIGHT events

Use INSIGHT for context and audit. INSIGHT does not drive workflow routing — use VERIFICATION for gate decisions.

Runtime projects run.summary, review.note, context.note, and human.feedback into {{.Insights}} for subsequent bees. Runtime may auto-synthesize run.summary after successful AFK runs when the bee policy allows.

Examples:

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"run.summary","summary":"Implemented the requested change","taskId":"{{.TaskID}}"}}
EOF

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"review.note","summary":"Missing error handling in token refresh","taskId":"{{.TaskID}}","severity":"medium"}}
EOF`
	taskEventsPartial = `## Task lifecycle events

Use these payload.kind values when publishing task queue events:

- task.plan — INSIGHT: publish a breakdown of tasks
- task.ready — SIGNAL: mark a task ready to run
- task.completed — VERIFICATION: report that a task passed review/commit gate

Examples:

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"task.plan","tasks":[{"taskId":"task-1","title":"Add endpoint","bee":"builder"}]}}
EOF

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"task.ready","taskId":"task-1","title":"Add endpoint","bee":"builder"}}
EOF

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"VERIFICATION","payload":{"kind":"task.completed","taskId":"task-1","status":"completed","summary":"Endpoint implemented and committed"}}
EOF`
	cursorAdapterYAML = `binary: agent
api_key_env: CURSOR_API_KEY
`
)
