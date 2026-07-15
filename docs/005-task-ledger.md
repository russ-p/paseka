# Task Ledger Protocol

Paseka models a feature flow as a **trace** (`traceId`) containing one or more **tasks** (`taskId`). Each task may spawn one or more **agent runs** (`agentId`). The Task Ledger is the projected state of all tasks within a trace.

Implementation: [`internal/protocol/task.go`](../internal/protocol/task.go), [`internal/taskledger`](../internal/taskledger/).

---

## 1. Identifiers

| ID | Scope | Meaning |
| -- | ----- | ------- |
| `traceId` | Whole feature / PRD / bloom | One flight trail from initial signal through PR/merge |
| `taskId` | One subtask within a trace | A unit of work in the task queue (nectar) |
| `agentId` | One adapter invocation | A single bee run (Cursor CLI process) |

Auto-generated `traceId` values use a compact time-ordered format: `trace-` + 16 lowercase hex chars (48-bit UTC ms + 16-bit random). Lexicographic sort matches creation order. Manual ids via `--trace` (e.g. `trace-auth-01`) are allowed and need not follow this layout. See [`internal/colony/traceid.go`](../internal/colony/traceid.go).

Relationship:

```text
traceId
  └── task-1
  │     └── agentId-a (builder run)
  │     └── agentId-b (guard run)
  └── task-2
        └── agentId-c (builder run)
```

**Important:** `RunStatus` (`completed`, `failed`, …) on an agent run is **not** the same as task completion. A task is complete only after the review/commit gate emits `task.completed`.

---

## 2. Task lifecycle

```mermaid
flowchart LR
  prd["SIGNAL: feature.requested"] --> plan["INSIGHT: task.plan"]
  plan --> ready["SIGNAL: task.ready"]
  ready --> run["Bee run with traceId and taskId"]
  run --> completed["VERIFICATION: task.completed"]
  completed --> nextReady["Next task.ready"]
```

| Status | Meaning |
| ------ | ------- |
| `planned` | Task registered from `task.plan`; waiting on dependencies |
| `ready` | Dependencies satisfied; eligible for dispatch |
| `running` | Bee dispatched for this task |
| `waiting_review` | Code changed or trace gate open; awaiting guard/HITL |
| `completed` | Task gate passed (AFK success or human approval) |
| `failed` | Adapter run failed or dispatch error; retry with `paseka task retry` or Queen Console |
| `blocked` | Cannot proceed (manual intervention or honey reserve exhausted) |

Task lifecycle events use `payload.kind` inside existing top-level event types — no new `EventType` values.

### Honey reserve (`energyToken`)

Each trace carries a shared honey reserve on the task ledger snapshot:

| Field | Meaning |
| ----- | ------- |
| `energyBudget` | Initial reserve seeded from `colony.yaml` → `defaults.energy_budget` (default `12`) |
| `energyRemaining` | Tokens left; each adapter dispatch consumes `1` |

When `energyRemaining` reaches `0`, further dispatches set the task to `blocked` with summary `Honey reserve exhausted`. Top up with `paseka energy add --trace <id> --amount <n>` (`SIGNAL` / `energy.add`). Runtime audit events use `SIGNAL` / `energy.consume`.

`energy.add` only increments `energyRemaining`; it does not set `energyBudget`. Formal seeding (`SeedEnergy` / reactor dispatch) applies `defaults.energy_budget` from `colony.yaml`. Runtime-generated ledger events are applied locally before publish; the reactor skips its own JetStream echo so non-idempotent reducers (notably `energy.consume`) are not applied twice.

### Review policy

Each task may declare an optional `review` field in `task.plan`:

| `review` | Behavior |
| -------- | -------- |
| `none` (default) | AFK runs auto-complete on adapter success |
| `required` | After a successful bee run, task moves to `waiting_review` until human approval |
| `final` | Trace-level merge gate — activates when all other tasks complete; no AFK dispatch |

When no `review: final` task is planned, the runtime synthesizes task `_review` after the last AFK task completes.

Human actions (CLI and Queen Console Reviews use the same domain flows):

- `paseka proposal approve --trace <id> --task <id>` — merge trace worktree (when present) and emit `task.completed`
- `paseka proposal reject --trace <id> --task <id>` — publish `human.feedback`; `required` tasks return to `ready` for rework
- `paseka task retry --trace <id> --task <id>` — re-publish `task.ready` for a `failed` or stuck `running` task (same bee, intent, body)

For `review: final` / `_review`, Queen Console Reviews shows an accumulated worktree merge preview (`GET /api/traces/:traceId/merge-diff`) before approve. See [specs/002-queen-console-mvp.md](specs/002-queen-console-mvp.md).

### Adapter failure

When a `task.ready` dispatch returns a non-completed adapter result or a dispatch error, the runtime emits `SIGNAL` / `task.status` with `failed` and a summary (adapter error text or run status). Run-level `failed` in `.paseka/runs/.../status.json` and task-level `failed` are now aligned.

Retry transitions `failed` or `running` → `ready` via `task.ready`, then the reactor dispatches again when `paseka run` is active. Queen Console exposes the same flow as `POST /api/traces/:traceId/tasks/:taskId/retry`.

### `task.status` — SIGNAL

Runtime publishes intermediate task transitions (`running`, `waiting_review`, `ready`, `failed`). The `summary` field on `task.status` replaces the previous task summary (omit or empty clears it — used when unblocking honey-exhausted tasks or clearing a failure reason on retry).

```json
{
  "traceId": "trace-auth-01",
  "type": "SIGNAL",
  "payload": {
    "kind": "task.status",
    "taskId": "task-1",
    "status": "waiting_review",
    "summary": "Awaiting human review"
  }
}
```

---

## 3. Event contract

### `task.plan` — INSIGHT

Scout (or planner bee) publishes a breakdown after analyzing the initial signal.

```json
{
  "traceId": "trace-auth-01",
  "type": "INSIGHT",
  "payload": {
    "kind": "task.plan",
    "tasks": [
      {
        "taskId": "task-1",
        "title": "Add backend endpoint",
        "body": "POST /api/auth/login with JWT",
        "bee": "builder",
        "sector": "backend-users",
        "intent": "feature",
        "dependsOn": []
      },
      {
        "taskId": "task-2",
        "title": "Add login UI",
        "body": "Login form component",
        "bee": "builder",
        "dependsOn": ["task-1"]
      },
      {
        "taskId": "review",
        "title": "Human review and merge",
        "review": "final",
        "dependsOn": ["task-2"]
      }
    ]
  }
}
```

### `task.ready` — SIGNAL

Runtime or Task Reactor marks a task as ready for dispatch. Emitted when:

- A task has no dependencies and is first in the queue, or
- All `dependsOn` tasks have `status: completed`.

```json
{
  "traceId": "trace-auth-01",
  "type": "SIGNAL",
  "payload": {
    "kind": "task.ready",
    "taskId": "task-1",
    "title": "Add backend endpoint",
    "body": "POST /api/auth/login with JWT",
    "bee": "builder",
    "sector": "backend-users",
    "intent": "feature"
  }
}
```

`sector` is optional. When set, it must match a name from `.paseka/colony.yaml` `sectors`. Runtime uses it to choose the adapter workspace (module/subfolder cwd). When omitted, the bee's default `sector` applies; otherwise the colony root (or trace worktree root) is used.

`intent` is optional. When omitted, Builder Bee renders with the `general` mission partial.

### `task.completed` — VERIFICATION

Task passed the AFK gate: implement → review → commit. Use `VERIFICATION` (not agent `RunStatus`) because completion is a verified domain fact.

```json
{
  "traceId": "trace-auth-01",
  "type": "VERIFICATION",
  "payload": {
    "kind": "task.completed",
    "taskId": "task-1",
    "status": "completed",
    "summary": "Endpoint implemented, reviewed, committed",
    "commit": "abc123def",
    "completedAt": "2026-07-05T08:30:00Z"
  }
}
```

After `task.completed`, the ledger unlocks dependent tasks and may emit `task.ready` for the next item.

---

## 4. End-to-end feature flow

**Short path** (clear PRD already exists):

```text
PRD (SIGNAL: feature.requested)
  → Scout INSIGHT task.plan
  → Task Reactor: task-1 → ready
  → Builder run (traceId + taskId)
  → AFK tasks auto-complete; review-marked tasks enter waiting_review
  → All AFK tasks done → final review gate (planned or synthesized)
  → Human approve → merge worktree → VERIFICATION task.completed
  → Task Reactor: next task.ready (if any)
  → … repeat …
  → Trace merge gate completed
```

**Ideation path** (raw idea → grilling → spec → breakdown → same ledger): see [specs/005-feature-ideation-flow.md](specs/005-feature-ideation-flow.md). Scout classifies and must not emit `task.plan` for vague ideas; Drone interactive grilling produces `docs/specs/…` + `SIGNAL/spec.ready` before breakdown publishes `task.plan`.

---

## 5. Ledger interface

The `taskledger.Ledger` interface defines how trace state is stored and updated:

```go
type Ledger interface {
    Snapshot(traceID string) (TraceSnapshot, error)
    Apply(event protocol.Event) (ApplyResult, error)
}
```

**Current scope:** protocol types, pure reducer (`taskledger.ApplyEvent`), in-memory ledger for tests, and JetStream KV ledger (`taskledger.KVLedger`) used by `paseka run`. See [008-bee-routing.md](008-bee-routing.md) for declarative bee subscriptions.

`ApplyResult.Ready` lists tasks that newly transitioned to `ready` after applying an event — the hook for a future scheduler to dispatch the next bee.

---

## 6. Runtime integration (MVP)

| Field | Where | Notes |
| ----- | ----- | ----- |
| `taskId` | `protocol.Request`, `adapters.RunRequest`, `prompts.Context` | Optional; empty for one-shot CLI runs |
| `{{.TaskID}}` | Prompt templates | Available when dispatch includes a task id |
| `intent` | `task.plan`, `task.ready`, CLI `--intent` | Optional builder mission hint; normalized at prompt render time |
| Task events | `paseka event emit --stdin` | Validated CLI publish with machine-readable feedback |

CLI behavior is unchanged when `taskId` is omitted.

---

## 6.1 Filesystem task projection

The runtime mirrors each trace task into `.paseka/runs/<traceId>/tasks/<taskId>/` as a **projection** of JetStream KV state (not a second source of truth).

```text
.paseka/runs/<traceId>/
  tasks/
    <taskId>/
      task.md        # markdown + YAML frontmatter snapshot
      runs.ndjson    # agent run history for this task
```

`task.md` frontmatter stores machine-readable fields (`traceId`, `taskId`, `title`, `bee`, `status`, `dependsOn`, `summary`, `commit`, `updatedAt`). The markdown body stores the human-readable task description (`body`).

`runs.ndjson` links task executions to existing agent run directories (`agentId`, `bee`, `runDir`, `startedAt`, `finishedAt`, `runStatus`).

The hive runtime updates this projection after `ledger.Apply(...)` and when task-queue dispatches start or finish.

For human-friendly task injection from the CLI, use `paseka task create` to publish `task.plan` (and optionally `task.ready` with `--autorun`). See [007-cli.md](007-cli.md).

---

## 7. Related docs

- [003-architecture.md](003-architecture.md) — colony layout, adapter contract, worktrees
- [004-prompt-templates.md](004-prompt-templates.md) — template variables including `TaskID`
- [002-paseka-glossary.md](002-paseka-glossary.md) — Task/Nectar, TraceID/Flight Trail
- [specs/002-queen-console-mvp.md](specs/002-queen-console-mvp.md) — Reviews UI and merge-diff preview
- [specs/005-feature-ideation-flow.md](specs/005-feature-ideation-flow.md) — classify → invite → grilling → `spec.ready` → breakdown before ledger work
