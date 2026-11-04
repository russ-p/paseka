## Publish events (breakdown only)

For breakdown, publish **only** these kinds. Do not emit `run.summary`, `review.note`, or `human.feedback`.

| Event | `payload.kind` | Role |
| ----- | -------------- | ---- |
| `INSIGHT` | `task.plan` | Register the full task ledger |
| `SIGNAL` | `task.ready` | Interactive only: kick the first runnable task after explicit confirmation |
| `INSIGHT` | `trace.title` | Flight Trail name (emit with breakdown) |
| `INSIGHT` | `worktree.branch` | Git branch for isolated builder work (when breakdown includes builder slices) |
| `INSIGHT` | `context.note` | Short trace fact for later bees (optional) |

### Planning is the deliverable

Breakdown registers work; it does not review it and does not start it.

- **Never author a standalone review or merge task.** Do not add a `review: final` / `phase: merge` card or a
  "human review and merge" task. The colony `closing` policy seeds trail review (trail-guard) and the runtime
  ensures the `_review` merge gate; a breakdown-authored review card preempts both. A HITL work slice may
  still carry `review: required` when the spec calls for human approval of that slice.
- **Never start implementation on an AFK run.** An AFK breakdown ends with `task.plan`. Do not emit
  `task.ready`, do not wake a builder yourself, and do not ask whether to start — there is no Beekeeper in
  the session. A human or a follow-up cue kicks the first unblocked slice later.

### `task.plan` — full breakdown (one event, many tasks)

Emit **one** `INSIGHT/task.plan` after approval in an interactive session, or after drafting from the spec in
an AFK run. Put every task in `payload.tasks` (dependency order: blockers first). Use `taskId` format
`nnn-short-description`.

```bash
paseka event emit --defer --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"task.plan","tasks":[{"taskId":"001-live-status-api","title":"Live bee status API slice","body":"## Parent\n\ndocs/specs/004-live-bees-indicator.md\n\n## What to build\n\nEnd-to-end path that exposes current bee run status over the API and is verifiable with a focused test.\n\n## Acceptance criteria\n\n- [ ] Status endpoint returns active bee runs for a trace\n- [ ] Focused test covers the happy path\n\n## Blocked by\n\nNone - can start immediately","bee":"builder","intent":"feature","dependsOn":[]},{"taskId":"002-live-status-ui","title":"Live bee status UI slice","body":"## Parent\n\ndocs/specs/004-live-bees-indicator.md\n\n## What to build\n\nUI indicator that consumes the status API and shows live bee activity.\n\n## Acceptance criteria\n\n- [ ] Indicator reflects running vs idle bees\n- [ ] Works against the API from 001-live-status-api\n\n## Blocked by\n\n- 001-live-status-api","bee":"builder","intent":"feature","dependsOn":["001-live-status-api"]}]}}
EOF
```

### `task.ready` — interactive only, first task only, only when the Beekeeper confirms

Skip this event entirely on an AFK run. In an interactive session, emit `SIGNAL/task.ready` **only if**
both are true:

1. The Beekeeper explicitly confirmed starting the first task now (not merely approved the plan).
2. You emit it for **exactly one** task: the first approved slice with no blockers (`dependsOn` empty / "can start immediately").

Do **not** emit `task.ready` for later tasks — the ledger/reactor unlocks them after prior tasks complete.

Use **`--defer` immediately after** the `task.plan` above (same flush, FIFO). Payload: `kind` + `taskId` only — do not copy title/body from plan.

```bash
paseka event emit --defer --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"task.ready","taskId":"001-live-status-api"}}
EOF
```

If the Beekeeper approved the plan but did **not** ask to start immediately, publish `task.plan` only and stop.
On an AFK run this is always the case: `task.plan` only, then stop.

Emit **one** `INSIGHT/trace.title` when publishing the breakdown (use spec title or a clearer short name for the whole trail).

When the breakdown includes isolated builder slices, emit **one** `INSIGHT/worktree.branch` with a conventional git branch name.

{{template "insight-kind-trace-title" .}}
{{template "insight-kind-worktree-branch" .}}
{{template "insight-kind-context-note" .}}
