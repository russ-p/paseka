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

`task.plan` is consumed by the Task Ledger and reactor. It is **not** auto-included in `{{.Insights}}` because it is structured queue data, not narrative memory.

### Narrative (projected into `{{.Insights}}`)

| `payload.kind` | Required fields | Optional fields | Typical producer |
| -------------- | --------------- | --------------- | ---------------- |
| `run.summary` | `summary` | `taskId` | builder, scout; runtime may auto-synthesize after AFK runs |
| `review.note` | `summary` | `taskId`, `severity` | guard |
| `context.note` | `summary` | `taskId` | any bee |
| `human.feedback` | `taskId`, `message` | — | beekeeper via CLI |

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

---

## Prompt memory projection

Before rendering a bee prompt, runtime:

1. Reads all `events.ndjson` files under `.paseka/runs/<traceId>/`.
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

## VERIFICATION routing (unchanged)

Workflow handoff remains `VERIFICATION`-driven:

| Event | Typical subscriber | Handoff |
| ----- | ------------------ | ------- |
| `MUTATION/code.proposal` | `guard` | diff + summary for review |
| `VERIFICATION/verification.failed` | `builder` | failure summary for fix-up |
| `VERIFICATION/verification.success` | `receiver` | approval summary for commit gate |

See [008-bee-routing.md](008-bee-routing.md).

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

- [003-architecture.md](003-architecture.md) — adapter contract and runs layout
- [004-prompt-templates.md](004-prompt-templates.md) — template fields and partials
- [005-task-ledger.md](005-task-ledger.md) — `task.plan` lifecycle
- [008-bee-routing.md](008-bee-routing.md) — direct dispatch and advisory publishes
