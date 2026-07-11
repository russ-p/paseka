package colony

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/paseka/paseka/internal/gitroot"
	"github.com/paseka/paseka/internal/protocol"
	"gopkg.in/yaml.v3"
)

// InitOptions configures paseka init.
type InitOptions struct {
	StartDir string // directory to resolve git root from; default cwd
	Adapter  string // scaffold bees and home config for this adapter (default: cursor)
}

// InitResult summarizes what init did.
type InitResult struct {
	ColonyRoot string
	Slug       string
	Adapter    string
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

	adapter := NormalizeInitAdapter(opts.Adapter)
	res := InitResult{ColonyRoot: repoRoot, Slug: slug, Adapter: adapter}

	if err := res.scaffoldProject(slug, manifest, adapter); err != nil {
		return InitResult{}, err
	}
	if err := res.scaffoldHome(slug, repoRoot, adapter); err != nil {
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

func (r *InitResult) scaffoldProject(slug string, manifest Colony, adapter string) error {
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
		PasekaPath(root, ".gitignore"):                                         gitignoreContent,
		PasekaPath(root, "bees", "scout.yaml"):                                 scoutBeeYAMLFor(adapter),
		PasekaPath(root, "bees", "builder.yaml"):                               builderBeeYAMLFor(adapter),
		PasekaPath(root, "bees", "hivewright.yaml"):                            hivewrightBeeYAMLFor(adapter),
		PasekaPath(root, "prompts", "default.md"):                              defaultPrompt,
		PasekaPath(root, "prompts", "scout.md"):                                scoutPrompt,
		PasekaPath(root, "prompts", "builder.md"):                              builderPrompt,
		PasekaPath(root, "prompts", "hivewright.md"):                           hivewrightPrompt,
		PasekaPath(root, "prompts", "_partials", "emit-howto.md"):              emitHowtoPartial,
		PasekaPath(root, "prompts", "_partials", "emit-insight.md"):            emitInsightPartial,
		PasekaPath(root, "prompts", "_partials", "emit-signal.md"):             emitSignalPartial,
		PasekaPath(root, "prompts", "_partials", "emit-verification.md"):       emitVerificationPartial,
		PasekaPath(root, "prompts", "_partials", "builder-intent-general.md"):  builderIntentGeneralPartial,
		PasekaPath(root, "prompts", "_partials", "builder-intent-feature.md"):  builderIntentFeaturePartial,
		PasekaPath(root, "prompts", "_partials", "builder-intent-bugfix.md"):   builderIntentBugfixPartial,
		PasekaPath(root, "prompts", "_partials", "builder-intent-test-fix.md"): builderIntentTestFixPartial,
		PasekaPath(root, "prompts", "_partials", "builder-intent-refactor.md"): builderIntentRefactorPartial,
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
			Slug: slug,
			Defaults: Defaults{
				PromptTemplate: "default.md",
				EnergyBudget:   protocol.DefaultEnergyBudget,
			},
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
	if manifest.Defaults.EnergyBudget == 0 {
		manifest.Defaults.EnergyBudget = protocol.DefaultEnergyBudget
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

func (r *InitResult) scaffoldHome(slug, repoRoot, adapter string) error {
	homeDir, err := HomeDir(slug)
	if err != nil {
		return err
	}
	if err := mkdirAll(filepath.Join(homeDir, "adapters")); err != nil {
		return err
	}

	cfgPath := filepath.Join(homeDir, "config.yaml")
	cfgContent := homeConfigYAML(repoRoot, slug, adapter)
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
	if err := r.track(created, cursorPath, err); err != nil {
		return err
	}

	claudePath := filepath.Join(homeDir, "adapters", "claude.yaml")
	created, err = writeFileIfMissing(claudePath, []byte(claudeAdapterYAML), 0o644)
	if err := r.track(created, claudePath, err); err != nil {
		return err
	}

	if adapter == "pi" {
		piPath := filepath.Join(homeDir, "adapters", "pi.yaml")
		created, err = writeFileIfMissing(piPath, []byte(piAdapterYAML), 0o644)
		return r.track(created, piPath, err)
	}
	return nil
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
	hivewrightBeeYAML = `role: hivewright
adapter: cursor
prompt_template: hivewright.md
params:
  model: composer-2.5
  output_format: stream-json
  trust: true
  force: true
worktree: true
publishes:
  - type: MUTATION
    kind: code.proposal
  - type: INSIGHT
    kind: run.summary
  - type: INSIGHT
    kind: context.note
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

{{template "emit-howto" .}}
{{template "emit-insight" .}}
{{template "emit-signal" .}}
`
	builderPrompt = `You are Builder Bee. Implement the task in the workspace.

Colony: {{.ColonyRoot}}
Flight trail: {{.TraceID}}
Workspace: {{.Workspace}}
Intent: {{.Intent}}{{if and .IntentRaw (ne .IntentRaw .Intent)}}
Requested intent: {{.IntentRaw}}{{end}}

## Task
{{.Task}}

## Prior discoveries
{{range .Insights}}- {{.}}
{{end}}

## Mission guidance
{{if eq .Intent "feature"}}
{{template "builder-intent-feature" .}}
{{else if eq .Intent "bugfix"}}
{{template "builder-intent-bugfix" .}}
{{else if eq .Intent "test-fix"}}
{{template "builder-intent-test-fix" .}}
{{else if eq .Intent "refactor"}}
{{template "builder-intent-refactor" .}}
{{else}}
{{template "builder-intent-general" .}}
{{end}}

Stage the changes, DON'T commit them yet.

Success criteria (must confirm all):
- All acceptance criteria in the task are met
- Build passes (module-level build succeeds)
- No new compiler errors or warnings that are not explicitly accepted
- Related tests (if any) pass

{{template "emit-howto" .}}
{{template "emit-insight" .}}

Runtime persists a human-readable run log at {{.ResultFile}}. If you do not emit run.summary, runtime will synthesize one from the normalized run outcome when possible.
`
	hivewrightPrompt = `You are Hivewright Bee. Your craft is the hive itself — how bees are defined,
prompted, and wired into the Air — not the Colony's product code.

Colony: {{.ColonyRoot}}
Flight trail: {{.TraceID}}
Workspace: {{.Workspace}}

## Mandate
- Improve sibling bees: prompt templates, partials, bee YAML, and choreography
  contracts under .paseka/.
- Ground changes in Beekeeper intent and Flight Trail analysis when available.
- Use published Hive documentation for Paseka capabilities. Do not rely on
  the Paseka platform source tree (internal/, cmd/, Go packages).
- Read the project only enough to sharpen each bee's focus for this Colony.
- Prefer small, reviewable Comb Proposals with explicit rationale.
- Do not implement product features; leave that to Builder / Worker bees.
- Do not impersonate the Queen or invent central orchestration.

## Hive docs (capabilities)
Canonical index (fetch and follow links as needed):
https://russ-p.github.io/paseka/llms.txt

Full corpus (optional single-fetch):
https://russ-p.github.io/paseka/llms-full.txt

Start with bee YAML, prompt templates, routing, and INSIGHT kinds before
mutating colony config.

## Task
{{.Task}}

## Prior discoveries
{{range .Insights}}- {{.}}
{{end}}

## Workflow
1. Read the task and any Beekeeper guidance in prior discoveries.
2. Consult Hive docs above for current contracts and template rules.
3. Inspect relevant .paseka/bees/*.yaml and .paseka/prompts/ (and partials).
4. When Flight Trails matter, inspect .paseka/runs/ for the current or named
   trail — prompts, results, and emitted events — without rewriting platform code.
5. Propose focused edits under .paseka/ only. Stage changes; do not commit.
6. Summarize what changed and why (tie to Beekeeper intent and/or trail evidence).

Success criteria (must confirm all):
- Changes stay inside .paseka/ (prompts, bee YAML, related colony config)
- No product/application code outside .paseka/ is modified
- Edits are justified by Beekeeper intent and/or Flight Trail evidence
- Bee roles become more focused, not more generic
- Staged diff is reviewable and scoped to the task

{{template "emit-howto" .}}
{{template "emit-insight" .}}

Runtime persists a human-readable run log at {{.ResultFile}}. If you do not emit run.summary, runtime will synthesize one from the normalized run outcome when possible.
`
	builderIntentGeneralPartial = `Implement or fix the requested change with minimal scope. Follow existing code conventions, run relevant tests when practical, and prefer small focused diffs over broad rewrites.
`
	builderIntentFeaturePartial = `You are adding new capability. Implement the feature end-to-end in the workspace, match surrounding patterns, and include tests when the codebase already tests similar behavior.
`
	builderIntentBugfixPartial = `You are fixing incorrect behavior. Reproduce or reason about the failure, change only what is needed to correct it, and add or update a regression test when feasible.
`
	builderIntentTestFixPartial = `You are repairing failing or missing tests. Preserve intended product behavior unless the task explicitly asks to change it. Focus on making the test suite pass without unrelated refactors.
`
	builderIntentRefactorPartial = `You are restructuring code without changing behavior. Keep the diff focused, avoid feature creep, and run tests to confirm behavior is unchanged.
`
	emitHowtoPartial = `When you need to publish a bus event during a run:

1. Build one valid JSON object for the event.
2. Validate and publish it with Paseka CLI via stdin.
3. If validation fails, inspect the returned JSON error, fix the event, and retry once.
4. After successful publish, continue with a normal human-readable summary.

Do not print raw event JSON in the final answer.
Do not write event JSON directly to files.

Use this command form:

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"context.note","summary":"Short narrative context"}}
EOF

Each event JSON object must include:
- traceId — current flight trail id ({{.TraceID}})
- agentId — current agent run id ({{.AgentID}})
- type — the event type your bee role may publish (see role-specific emit guidance below)
- payload — event-specific object with required payload.kind

If the command returns "ok": false, treat it as a failed publish and correct the payload before continuing.`
	emitInsightPartial = `## INSIGHT events

Use type: INSIGHT for context, audit, and dashboard narrative. INSIGHT events do not drive workflow routing.

Runtime automatically projects selected narrative INSIGHT kinds into {{.Insights}} for subsequent bees on the same trace.

| payload.kind | Role | Included in prompt memory |
| -------------- | ---- | ------------------------- |
| run.summary | Short run outcome for the next bee | yes |
| review.note | Reviewer observation (non-gate) | yes |
| context.note | Trace/task context fact | yes |
| human.feedback | Beekeeper HITL feedback | yes |
| task.plan | Task ledger planning | no (operational) |

### run.summary — narrative after work (runtime may auto-synthesize)

Runtime auto-publishes INSIGHT/run.summary after a successful AFK run when the bee policy allows and no summary was emitted during the run. You may still publish one explicitly:

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"run.summary","summary":"Implemented OAuth callback and added focused tests","taskId":"{{.TaskID}}"}}
EOF

### review.note — optional reviewer context

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"review.note","summary":"Token refresh path still lacks retry handling","taskId":"{{.TaskID}}","severity":"medium"}}
EOF

### context.note — optional trace context

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"context.note","summary":"NATS KV is the source of truth for task ledger state"}}
EOF

### task.plan — task breakdown

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"task.plan","tasks":[{"taskId":"task-1","title":"Add endpoint","bee":"builder","sector":"backend-users"}]}}
EOF`
	emitSignalPartial = `## SIGNAL events

Use type: SIGNAL to mark operational signals on the bus.

### task.ready — mark a task ready to run

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"task.ready","taskId":"task-1","title":"Add endpoint","bee":"builder","sector":"backend-users"}}
EOF`
	emitVerificationPartial = `## VERIFICATION events

Use type: VERIFICATION for gate outcomes that drive workflow routing.

Publish exactly one final VERIFICATION gate decision when your bee role requires it:
- verification.success when all requirements, scope checks, and targeted checks pass.
- verification.failed when anything required is missing or failing.

Optional: publish one INSIGHT/review.note for extra reviewer context. It does not replace the required VERIFICATION.

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"VERIFICATION","payload":{"kind":"verification.success","summary":"All requirements met"}}
EOF

Change payload.kind to verification.failed when rejecting.

### task.completed — report task passed review/commit gate

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"VERIFICATION","payload":{"kind":"task.completed","taskId":"task-1","status":"completed","summary":"Endpoint implemented and committed"}}
EOF

Each event must include traceId, agentId, type, and payload.kind.`
	cursorAdapterYAML = `binary: agent
api_key_env: CURSOR_API_KEY
`
	claudeAdapterYAML = `binary: claude
# When ANTHROPIC_API_KEY is unset, Claude Code uses your subscription
# login (claude login) instead of an API key.
api_key_env: ANTHROPIC_API_KEY
`
	scoutBeePiYAML = `role: scout
adapter: pi
prompt_template: scout.md
params:
  output_format: json
  plan: true
worktree: false
publishes:
  - type: INSIGHT
    kind: task.plan
`
	builderBeePiYAML = `role: builder
adapter: pi
prompt_template: builder.md
params:
  output_format: json
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
	hivewrightBeePiYAML = `role: hivewright
adapter: pi
prompt_template: hivewright.md
params:
  output_format: json
worktree: true
publishes:
  - type: MUTATION
    kind: code.proposal
  - type: INSIGHT
    kind: run.summary
  - type: INSIGHT
    kind: context.note
`
	piAdapterYAML = `binary: pi
# api_key_env: GEMINI_API_KEY   # optional; passed as --api-key when set in env
`
)
