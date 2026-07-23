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
		PasekaPath(root, ".gitignore"):                                            gitignoreContent,
		PasekaPath(root, "bees", "scout.yaml"):                                    scoutBeeYAMLFor(adapter),
		PasekaPath(root, "bees", "builder.yaml"):                                  builderBeeYAMLFor(adapter),
		PasekaPath(root, "bees", "hivewright.yaml"):                               hivewrightBeeYAMLFor(adapter),
		PasekaPath(root, "prompts", "default.md"):                                 defaultPrompt,
		PasekaPath(root, "prompts", "scout.md"):                                   scoutPrompt,
		PasekaPath(root, "prompts", "builder.md"):                                 builderPrompt,
		PasekaPath(root, "prompts", "hivewright-system.md"):                       hivewrightSystemPrompt,
		PasekaPath(root, "prompts", "hivewright-task.md"):                         hivewrightTaskPrompt,
		PasekaPath(root, "prompts", "_partials", "emit-howto.md"):                 emitHowtoPartial,
		PasekaPath(root, "prompts", "_partials", "emit-insight.md"):               emitInsightPartial,
		PasekaPath(root, "prompts", "_partials", "emit-signal.md"):                emitSignalPartial,
		PasekaPath(root, "prompts", "_partials", "emit-verification.md"):          emitVerificationPartial,
		PasekaPath(root, "prompts", "_partials", "emit-task-completed.md"):        emitTaskCompletedPartial,
		PasekaPath(root, "prompts", "_partials", "builder-intent-general.md"):     builderIntentGeneralPartial,
		PasekaPath(root, "prompts", "_partials", "builder-intent-feature.md"):     builderIntentFeaturePartial,
		PasekaPath(root, "prompts", "_partials", "builder-intent-bugfix.md"):      builderIntentBugfixPartial,
		PasekaPath(root, "prompts", "_partials", "builder-intent-test-fix.md"):    builderIntentTestFixPartial,
		PasekaPath(root, "prompts", "_partials", "builder-intent-refactor.md"):    builderIntentRefactorPartial,
		PasekaPath(root, "prompts", "_partials", "scout-intent-survey.md"):        scoutIntentSurveyPartial,
		PasekaPath(root, "prompts", "_partials", "scout-intent-intake.md"):        scoutIntentIntakePartial,
		PasekaPath(root, "prompts", "_partials", "scout-emit-intake.md"):          scoutEmitIntakePartial,
		PasekaPath(root, "prompts", "_partials", "cursor-interactive-kickoff.md"): cursorInteractiveKickoffPartial,
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
			AutoInvites: DefaultAutoInviteRules(),
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
	if len(manifest.AutoInvites) == 0 {
		manifest.AutoInvites = DefaultAutoInviteRules()
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
default_intent: intake
params:
  output_format: stream-json
  trust: true
  force: true
  plan: false
worktree: false
subscribes:
  - type: SIGNAL
    kind: feature.requested
    dispatch: direct
publishes:
  - type: SIGNAL
    kind: feature.classified
  - type: INSIGHT
    kind: task.plan
  - type: SIGNAL
    kind: task.ready
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
    kind: code.proposal.isolated
  - type: VERIFICATION
    kind: task.completed
`
	hivewrightBeeYAML = `role: hivewright
adapter: cursor
system_template: hivewright-system.md
prompt_template: hivewright-task.md
params:
  model: composer-2.5
  output_format: stream-json
  trust: true
  force: true
worktree: false
publishes:
  - type: MUTATION
    kind: code.proposal.root
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
	scoutPrompt = `You are Scout Bee. Your job is problem discovery and intake routing, not implementation.

Colony: {{.ColonyRoot}}
Flight trail: {{.TraceID}}
Flight trail title: {{if .TraceTitle}}{{.TraceTitle}}{{else}}(unset){{end}}
Intent: {{.Intent}}{{if and .IntentRaw (ne .IntentRaw .Intent)}}
Requested intent: {{.IntentRaw}}{{end}}

## Rules
- Do not edit files unless necessary to inspect behavior.
- Do not invent work: only report problems with evidence (path, symbol, symptom).
- For intake: classify the entry, then emit one-slice task.plan only when decision=plan or decision=triage.
- Never emit a vague plan ("improve the codebase"). Emit task.ready only when the entry text explicitly asks to start now (e.g. "do now", "fix needed now", "фикс нужен сейчас").

## Task
{{.Task}}

## Prior discoveries
{{range .Insights}}- {{.}}
{{end}}

## Mission guidance
{{if eq .Intent "intake"}}
{{template "scout-intent-intake" .}}
{{template "scout-emit-intake" .}}
{{else}}
{{template "scout-intent-survey" .}}
{{end}}

## Human summary shape
For each finding: severity | location | symptom | why it matters | fix direction.
End with top-N ranked list. Optionally note what you deliberately skipped (out of scope / no evidence).

{{template "emit-howto" .}}
{{template "emit-insight" .}}
{{template "emit-signal" .}}

Runtime persists a human-readable run log at {{.ResultFile}}. If you do not emit run.summary, runtime will synthesize one from the normalized run outcome when possible.
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
	hivewrightSystemPrompt = `You are Hivewright Bee. Your craft is the hive itself — how bees are defined,
prompted, and wired into the Air — not the Colony's product code.

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

## Session context
Colony: {{.ColonyRoot}}
Flight trail: {{.TraceID}}
Workspace: {{.Workspace}}

{{if .Insights}}
## Prior discoveries
{{range .Insights}}- {{.}}
{{end}}
{{end}}

Runtime persists a human-readable run log at {{.ResultFile}}. If you do not emit run.summary, runtime will synthesize one from the normalized run outcome when possible.
`
	hivewrightTaskPrompt = `{{if .Task}}
## Task
{{.Task}}
{{end}}
{{if and .Interactive (eq .Adapter "cursor")}}
{{template "cursor-interactive-kickoff" .}}
{{end}}
`
	cursorInteractiveKickoffPartial = `Interactive Cursor session: greet the user briefly, then wait for further instructions. Do not start the mission until they reply.
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
	scoutIntentSurveyPartial = `Survey the Task scope for concrete problems. Discovery first; planning is optional and secondary.

### Method
1. Bound the search to the Task (module, symptom, path, or stated concern). If the scope is vague, prefer the highest-risk areas over a shallow full-repo skim.
2. Gather signals: failing or missing tests, TODO/FIXME on live paths, error-handling gaps, race/concurrency smells, authz/secret risks, docs↔code drift, silent failures, fragile contracts.
3. For each finding record: symptom → location (file/symbol) → why it is a problem → severity → suggested fix direction (not a full design).
4. Rank findings (critical → low). Skip pure style nits unless they hide correctness or security issues.
5. Publish notable findings as INSIGHT/context.note or INSIGHT/review.note (include severity when useful). Use run.summary for a short ranked digest.
6. Emit task.plan only if findings already form clear builder-sized slices and the Task benefits from a queue — otherwise stop at the findings report.

### Problem classes (repo/area survey, not staged-diff review)
| Class | Examples |
| ----- | -------- |
| Correctness | silent failures, wrong invariants, missing edge cases |
| Reliability | no retries, swallowed errors, weak timeouts |
| Security | secrets, authz gaps, unsafe defaults |
| Operability | missing logs/metrics, unclear failure modes |
| Maintainability | duplicated critical paths, stale contracts |
| Debt with signal | TODO/FIXME that blocks a real path (not cosmetic) |
`
	scoutIntentIntakePartial = `Intake a feature.requested entry — classify, then hand off clear work to the ledger. Do not implement.

### Method
1. Read the Task and Prior discoveries for the feature.requested title/body (or equivalent entry text).
2. Decide exactly one decision using evidence from the body and prior insights:
   - grill — vague product idea; acceptance criteria missing; needs interactive grilling before breakdown.
   - plan — clear small feature or improve; one builder-sized slice; no grilling needed.
   - triage — bug, regression, or incident; one builder-sized fix.
   - clarify — ambiguous whether feature vs bug; Beekeeper should choose next step.
   - reject — out of scope, duplicate, or non-actionable.
3. Emit one SIGNAL/feature.classified with decision and rationale only (do not set bee / intent on this event — colony rules / Beekeeper react to decision).
4. When decision=plan: emit one INSIGHT/task.plan with a single task — bee: builder, intent: feature. Include title, body with acceptance criteria, and stable taskId (001-short-slug).
5. When decision=triage: emit one INSIGHT/task.plan with a single task — bee: builder, intent: bugfix. Include reproduction/symptom in the body.
6. When decision=grill, clarify, or reject: do not emit task.plan or task.ready.
7. Start now: if title/body/task text explicitly asks to start immediately (e.g. "do now", "fix needed now", "фикс нужен сейчас", "start immediately"), emit SIGNAL/task.ready for that single planned task after task.plan. Otherwise publish task.plan only.
8. Optionally emit INSIGHT/run.summary with a one-line intake summary.
9. Emit INSIGHT/trace.title with a short Flight Trail name on every intake (from entry title/body or a refined label).
`
	scoutEmitIntakePartial = `## Publish events (intake only)

Always emit feature.classified first. Then follow the decision branch below.

| Event | payload.kind | When |
| ----- | ------------ | ---- |
| SIGNAL | feature.classified | Always (required) |
| INSIGHT | task.plan | decision=plan or decision=triage (one builder task) |
| SIGNAL | task.ready | Same slice, only when entry text asks to start now |
| INSIGHT | trace.title | Short Flight Trail name for Console (emit on every intake) |
| INSIGHT | run.summary | Optional one-line summary |

Do not emit task.plan or task.ready when decision=grill, clarify, or reject. Still emit trace.title when the entry has a usable short name.

### feature.classified — classification decision

Emit one SIGNAL/feature.classified. Set decision and rationale. Do not set bee / intent.

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"feature.classified","decision":"grill","rationale":"Product idea without acceptance criteria; needs grilling before breakdown."}}
EOF

When decision=plan (clear small feature):

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"feature.classified","decision":"plan","rationale":"Single clear improvement; one builder slice is enough."}}
EOF

When decision=triage (bug):

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"feature.classified","decision":"triage","rationale":"Regression on Windows; one focused bugfix slice."}}
EOF

When decision=reject:

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"feature.classified","decision":"reject","rationale":"Duplicate of an existing spec; no new work."}}
EOF

### task.plan — one builder slice (plan or triage)

Emit one INSIGHT/task.plan with a single task in payload.tasks.

Clear feature (decision=plan):

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"task.plan","tasks":[{"taskId":"001-add-health-badge","title":"Add health badge to header","body":"## What to build\n\nShow colony health in the console header.\n\n## Acceptance criteria\n\n- [ ] Badge visible when NATS is connected\n- [ ] Badge shows disconnected state clearly\n\n## Blocked by\n\nNone - can start immediately","bee":"builder","intent":"feature","dependsOn":[]}]}}
EOF

Bug (decision=triage):

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"task.plan","tasks":[{"taskId":"001-fix-windows-path","title":"Fix path handling on Windows","body":"## Symptom\n\nColony init fails on Windows when path contains spaces.\n\n## What to fix\n\nCorrect path normalization so init succeeds on Windows.\n\n## Acceptance criteria\n\n- [ ] Init succeeds on Windows with spaced paths\n- [ ] Regression test added when feasible\n\n## Blocked by\n\nNone - can start immediately","bee":"builder","intent":"bugfix","dependsOn":[]}]}}
EOF

### task.ready — start now only

Emit only when title/body/task text explicitly requests immediate start. Use the same taskId, title, body, bee, and intent as the single planned task.

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"task.ready","taskId":"001-fix-windows-path","title":"Fix path handling on Windows","body":"## Symptom\n\nColony init fails on Windows when path contains spaces.\n\n## What to fix\n\nCorrect path normalization so init succeeds on Windows.\n\n## Acceptance criteria\n\n- [ ] Init succeeds on Windows with spaced paths\n- [ ] Regression test added when feasible\n\n## Blocked by\n\nNone - can start immediately","bee":"builder","intent":"bugfix"}}
EOF

### run.summary — optional

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"run.summary","summary":"Intake: triage bug, one builder slice planned"}}
EOF

### trace.title — Flight Trail name (required on intake)

Emit one INSIGHT/trace.title with a short human name (from feature.requested title/body or your refined label).

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"trace.title","title":"Live bees in Queen Console header"}}
EOF
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
| trace.title | Flight Trail display name | no (operational) |
| trace.summary | Flight Trail description | no (operational) |

{{if .IsLastWorkTask}}
### trace.summary — Flight trail summary (required on last work task)

You **must** emit one INSIGHT/trace.summary with 1–3 sentences of plain prose describing what this flight trail accomplished. This is a prompt-level obligation, not a runtime completion-contract failure.

Do not use conventional commit prefixes (feat:, fix:, etc.).

Naming (do not confuse):
- paseka proposal approve --summary → VERIFICATION/task.completed completion note
- mergeMessage / --merge-message → git merge commit **subject** (and optional HITL body)
- INSIGHT/trace.summary → trail description and default merge commit **body**

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"trace.summary","summary":"Implemented OAuth callback and added focused tests for token refresh."}}
EOF
{{end}}

### trace.title — Flight Trail name (planner bees)

Emit when classifying or planning work. Last event wins for Console display and {{.TraceTitle}}.

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"trace.title","title":"Live bees in Queen Console header"}}
EOF

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
	emitVerificationPartial = `## VERIFICATION events (review gate)

Use type: VERIFICATION for review gate outcomes that drive workflow routing.

Publish exactly one final gate decision:
- verification.success when all requirements, scope checks, and targeted checks pass.
- verification.failed when anything required is missing or failing.

Do not publish task.completed from a review bee — that is the receiver / commit-gate role.

Optional: publish one INSIGHT/review.note for extra reviewer context. It does not replace the required VERIFICATION.

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"VERIFICATION","payload":{"kind":"verification.success","taskId":"{{.TaskID}}","summary":"All requirements met"}}
EOF

Change payload.kind to verification.failed when rejecting.

Each event must include traceId, agentId, type, and payload.kind. Include payload.taskId when known.`
	emitTaskCompletedPartial = `## VERIFICATION / task.completed (commit gate)

After you commit the approved changes, publish exactly one task.completed event.
Do not publish verification.success or verification.failed — those are review-gate outcomes from Guard, and re-emitting them re-triggers this bee.

paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"VERIFICATION","payload":{"kind":"task.completed","taskId":"{{.TaskID}}","status":"completed","summary":"Endpoint implemented and committed"}}
EOF

Each event must include traceId, agentId, type, and payload.kind. Prefer the real payload.taskId from the task context when known.`
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
default_intent: intake
params:
  output_format: json
  plan: false
worktree: false
subscribes:
  - type: SIGNAL
    kind: feature.requested
    dispatch: direct
publishes:
  - type: SIGNAL
    kind: feature.classified
  - type: INSIGHT
    kind: task.plan
  - type: SIGNAL
    kind: task.ready
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
    kind: code.proposal.isolated
  - type: VERIFICATION
    kind: task.completed
`
	hivewrightBeePiYAML = `role: hivewright
adapter: pi
system_template: hivewright-system.md
prompt_template: hivewright-task.md
params:
  output_format: json
worktree: false
publishes:
  - type: MUTATION
    kind: code.proposal.root
  - type: INSIGHT
    kind: run.summary
  - type: INSIGHT
    kind: context.note
`
	piAdapterYAML = `binary: pi
# api_key_env: GEMINI_API_KEY   # optional; passed as --api-key when set in env
`
)
