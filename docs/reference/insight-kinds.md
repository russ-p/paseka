# INSIGHT kinds and prompt memory

This document defines the `INSIGHT` event taxonomy, how narrative insights differ from workflow `VERIFICATION` events, and how runtime projects prior insights into `{{.Insights}}` for subsequent bees.

---

## Role split

| Event type | Purpose | Drives routing? |
| ---------- | ------- | --------------- |
| `VERIFICATION` | Gate outcomes and verified domain facts | yes |
| `INSIGHT` | Narrative context, audit trail, dashboard timeline | no |
| `MUTATION` | Code change proposals | yes (via direct dispatch) |
| `SIGNAL` | Triggers and task-ready notifications | yes |

**Rule of thumb:** publish `VERIFICATION` when the colony must decide what happens next; publish `INSIGHT` when you want downstream bees (or humans) to understand what happened.

---

## INSIGHT kinds

### Operational (not projected into prompts)

| `payload.kind` | Type | Description |
| -------------- | ---- | ----------- |
| `task.plan` | `INSIGHT` | Scout/planner task breakdown for the task ledger |
| `trace.title` | `INSIGHT` | Human-readable Flight Trail name for Console and `{{.TraceTitle}}` |
| `trace.summary` | `INSIGHT` | Human-readable Flight Trail description for Console subtitle and merge commit body |
| `pr.body` | `INSIGHT` | Markdown pull-request description (last-write-wins; not prompt memory) |
| `worktree.branch` | `INSIGHT` | Git branch name for the trace isolated worktree (path stays `.paseka/worktrees/<traceId>/`) |

`task.plan` is consumed by the Task Ledger and reactor. It is **not** auto-included in `{{.Insights}}` because it is structured queue data, not narrative memory.

`trace.title` is trace identity metadata (last-write-wins). It is **not** projected into `{{.Insights}}`. Runtime and Console resolve the display title with fallbacks from `feature.requested` and task ledger titles. Standing cue first seed publishes one `trace.title` from the cue description (or cue id) when none exists; later ticks do not overwrite. See [specs/011-trace-title.md](../specs/011-trace-title.md) and [Forage Cues](../guide/cues.md).

`trace.summary` is operational trail metadata (last-write-wins). It is **not** projected into `{{.Insights}}` or dashboard Recent insights. Runtime resolves the latest `INSIGHT/trace.summary` by `createdAt`, then `seq`. Payload shape: `{ "kind": "trace.summary", "summary": "<prose>" }` (max 800 characters after trim). Bees on the sole incomplete non-final AFK work task receive must-emit guidance via `{{.IsLastWorkTask}}` in emit partials. See [specs/012-trace-summary.md](../specs/012-trace-summary.md).

`pr.body` is operational pull-request copy (last-write-wins). It is **not** projected into `{{.Insights}}`. Payload `{ "kind": "pr.body", "body": "<markdown>" }` (non-empty after trim, max 8000). Publish uses latest `pr.body`, else `trace.summary`. Last AFK work task gets must-emit guidance via `{{.IsLastWorkTask}}` in init emit partials (prompt-level, not a completion contract). Do not subscribe with `dispatch: direct`. See [pull-request delivery](../guide/pull-request-delivery.md).

`worktree.branch` is operational git metadata (last-write-wins). It is **not** projected into `{{.Insights}}`. Runtime resolves the branch for ensure, rename, merge, merge-diff, and PR head: latest valid insight, else default `paseka/<traceId>`. Scout/Drone emit when planning isolated work. See [specs/020-worktree-branch.md](../specs/020-worktree-branch.md).

### Narrative (projected into `{{.Insights}}`)

| `payload.kind` | Required fields | Optional fields | Typical producer |
| -------------- | --------------- | --------------- | ---------------- |
| `run.summary` | `summary` | `taskId` | builder, scout; runtime may auto-synthesize after AFK runs |
| `review.note` | `summary` | `taskId`, `severity` (`info`, `low`, `medium`, `high`, `critical`) | guard |
| `context.note` | `summary` | `taskId` | any bee |
| `human.feedback` | `taskId`, `message` | `ref` (comb path when annotated review was written), `verdict` (`positive`, `negative`, `neutral`) | beekeeper via CLI or Queen Console |

### Example payloads

**`run.summary`**

```json
{
  "traceId": "trace-auth-01",
  "agentId": "a1b2c3d4",
  "type": "INSIGHT",
  "payload": {
    "kind": "run.summary",
    "summary": "Implemented OAuth callback and added focused tests",
    "taskId": "task-1"
  }
}
```

**`review.note`**

```json
{
  "traceId": "trace-auth-01",
  "agentId": "e5f6a7b8",
  "type": "INSIGHT",
  "payload": {
    "kind": "review.note",
    "summary": "Token refresh path still lacks retry handling",
    "taskId": "task-1",
    "severity": "medium"
  }
}
```

`review.note.severity` is a closed vocabulary: `info`, `low`, `medium`, `high`, `critical`. The value is trimmed and lowercased when the event is constructed, so `" Medium "` is stored as `medium`; a non-empty value outside the set fails emit with `schema_validation_failed` and a `payload.severity` detail. An absent or whitespace-only severity is valid and reads as `unspecified`; a historical non-empty value outside the set reads as `unknown` and is never rewritten. No other narrative kind carries a severity requirement.

**`human.feedback`**

```json
{
  "traceId": "trace-auth-01",
  "agentId": "console",
  "type": "INSIGHT",
  "payload": {
    "kind": "human.feedback",
    "taskId": "task-1",
    "message": "Retry handling still drops the timeout path.",
    "verdict": "negative"
  }
}
```

`human.feedback.verdict` is optional and closed: `positive`, `negative`, `neutral`. The value is trimmed and lowercased when the event is constructed, so `" Negative "` is stored as `negative`; a non-empty value outside the set fails emit with `schema_validation_failed` and a `payload.verdict` detail. An absent or whitespace-only verdict is valid and reads as `unspecified`; a historical non-empty value outside the set reads as `unknown` and is never rewritten. Every review rejection publishes `negative`. On an eligible `waiting_review` task, `negative` keeps the rework behaviour (the task returns to `ready` and is dispatched); `positive` and `neutral` are observational and change no task status. Verdict-less legacy feedback keeps the rework behaviour and reads as `unspecified`. Approval does not publish feedback and is not inferred from a verdict or from prose.

---

## Prompt memory projection

Before rendering a bee prompt, runtime:

1. Reads the trail's history: `.paseka/runs/<traceId>/trail-events.ndjson` merged with every run's own `events.ndjson`, one reader for both stores.
2. Selects narrative `INSIGHT` kinds (`run.summary`, `review.note`, `context.note`, `human.feedback`).
3. Prefers insights scoped to the current `taskId`, then adds trace-scoped insights (no `taskId`).
4. Deduplicates, truncates long lines, and caps the list (default: 5 entries).
5. Merges with any manually supplied `Insights` from dispatch input.

Projected strings appear in templates via:

```markdown
## Prior discoveries
{{range .Insights}}- {{.}}
{{end}}
```

---

## MUTATION kinds (workflow)

Code proposals use `MUTATION` with explicit workspace provenance. They drive direct dispatch to reviewers and set `proposalWorkspace` on the task ledger.

| `payload.kind` | Workspace | Typical producer | Typical subscriber |
| -------------- | --------- | ---------------- | ---------------- |
| `code.proposal.isolated` | `.paseka/worktrees/<traceId>/` (+ sector) | `builder` (`worktree: true`) | `guard` (`worktree: true`) |
| `code.proposal.root` | Colony root (+ sector) | `hivewright` (`worktree: false`) | `main-guard` (`worktree: false`) |
| `code.proposal` (alias) | Same as isolated | Legacy YAML | Matches isolated subscribers |

Bare `code.proposal` is normalized to `code.proposal.isolated` on auto-publish write. Payload may include `workspace`, `baseSha`, `worktreePath` (isolated), `sector`, `diff` / `ref`, `summary`, `taskId`. See [specs/008-code-proposal-workspaces.md](../specs/008-code-proposal-workspaces.md).

---

## VERIFICATION routing

Workflow handoff remains `VERIFICATION`-driven:

| Event | Typical subscriber | Handoff |
| ----- | ------------------ | ------- |
| `MUTATION/code.proposal.isolated` (+ alias) | `guard` | diff + summary; reviewer cwd = trace worktree |
| `MUTATION/code.proposal.root` | `main-guard` | diff + summary; reviewer cwd = colony root |
| `VERIFICATION/verification.failed` | `builder` | failure summary for fix-up |
| `VERIFICATION/verification.success` | `receiver` (isolated defer) or ledger (root `review: required`) | approval / soft-gate advance |

See [bee routing](bee-routing.md).

---

## Bee completion contracts

Bees may declare required post-run events in `bees/<role>.yaml`:

```yaml
completion_contract:
  required:
    - type: VERIFICATION
      kind_one_of:
        - verification.success
        - verification.failed
      count: 1
```

Runtime validates emitted domain events in `events.ndjson` after the adapter exits. A process-level success is downgraded to **failed** when the completion contract is violated or when `run_summary: required` is set and no `INSIGHT/run.summary` is present.

The `guard` bee requires exactly one `VERIFICATION` gate decision per run.

---

## Related docs

- [architecture overview](../architecture/overview.md) — adapter contract and runs layout
- [prompt templates](../guide/prompt-templates.md) — template fields and partials
- [task ledger](task-ledger.md) — `task.plan` lifecycle
- [bee routing](bee-routing.md) — direct dispatch and advisory publishes
- [pull-request delivery](../guide/pull-request-delivery.md) — `pr.body` and forge publish
