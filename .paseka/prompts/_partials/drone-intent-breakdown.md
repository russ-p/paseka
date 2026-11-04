# To Tasks

Break a plan into independently-grabbable tasks using vertical slices (tracer bullets).

User input: {{.Task}}

## Process

### 1. Gather context

Work from whatever is already in the conversation context. If the user passes an issue reference (issue number, URL, or
path) as an argument, fetch it from the issue tracker and read its full body and comments. When breaking down after grilling, read the spec at the path in `{{.Task}}` or from a prior `spec.ready.ref` in Prior discoveries — breakdown requires a readable spec file.

### 2. Explore the codebase (optional)

If you have not already explored the codebase, do so to understand the current state of the code. Issue titles and
descriptions should use the project's domain glossary vocabulary, and respect ADRs in the area you're touching.

### 3. Draft vertical slices

Break the plan into **tracer bullet** issues. Each issue is a thin vertical slice that cuts through ALL integration
layers end-to-end, NOT a horizontal slice of one layer.

Slices may be 'HITL' or 'AFK'. HITL slices require human interaction, such as an architectural decision or a design
review. AFK slices can be implemented and merged without human interaction. Prefer AFK over HITL where possible.

<vertical-slice-rules>
- Each slice delivers a narrow but COMPLETE path through every layer (schema, API, UI, tests)
- A completed slice is demoable or verifiable on its own
- Prefer many thin slices over few thick ones
</vertical-slice-rules>

### 4. Present the breakdown

{{if .Interactive}}
Present the proposed breakdown as a numbered list. For each slice, show:

- **Title**: short descriptive name
- **Type**: HITL / AFK
- **Blocked by**: which other slices (if any) must complete first
- **User stories covered**: which user stories this addresses (if the source material has them)

Ask the user:

- Does the granularity feel right? (too coarse / too fine)
- Are the dependency relationships correct?
- Should any slices be merged or split further?
- Are the correct slices marked as HITL and AFK?

Iterate until the user approves the breakdown.
{{else}}
This is an **AFK planning run**: there is no Beekeeper in the session to answer a quiz. Do not present a
questionnaire and do not wait for approval. Treat the readable spec as the approved source of truth, draft
the slices, and publish them in one `task.plan`.

If a slice genuinely needs a human decision, mark it **HITL** and state the decision it needs in the task
body instead of stopping to ask. Prefer AFK slices everywhere else.
{{end}}

### 5. Publish the tasks

taskId Format: `nnn-short-description`
- Sequential numbering
- Three-digit format (001, 002, 003…)

Emit **one** `INSIGHT/task.plan` whose `tasks` array lists every slice (dependency order: blockers first).
Put the task body template below into each task's `body` field. Do not emit one `task.plan` per slice.

{{if .Interactive}}
After approval, ask whether to start the first unblocked AFK slice immediately. Emit deferred
`SIGNAL/task.ready` (`taskId` only) **immediately after** deferred `task.plan`, **only** for that first
task, and **only** when the Beekeeper confirms immediate start. Never mark later tasks ready yourself —
the ledger unlocks them after prior tasks complete.
{{else}}
Planning is the whole deliverable of this run: publish the plan and stop. Do **not** emit `task.ready` and
do not start the first slice yourself — a Beekeeper or a follow-up cue starts it later. Do **not** add a
standalone review or merge task (`review: final` / "human review and merge"): the colony closing policy and
the runtime-ensured merge gate own trail review, and a breakdown-authored review card would preempt both.
{{end}}

Optionally emit `INSIGHT/context.note` for a short trace fact (source doc, ordering rationale).

<task-template>
## Parent

A reference to the parent document (PRD,TDD,spec) (if the source was an existing issue, otherwise omit this section).

## What to build

A concise description of this vertical slice. Describe the end-to-end behavior, not layer-by-layer implementation.

Avoid specific file paths or code snippets — they go stale fast. Exception: if a prototype produced a snippet that
encodes a decision more precisely than prose can (state machine, reducer, schema, type shape), inline it here and note
briefly that it came from a prototype. Trim to the decision-rich parts — not a working demo, just the important bits.

## Acceptance criteria

- [ ] Criterion 1
- [ ] Criterion 2
- [ ] Criterion 3

## Blocked by

- A reference to the blocking task (if any)

Or "None - can start immediately" if no blockers.

</task-template>
