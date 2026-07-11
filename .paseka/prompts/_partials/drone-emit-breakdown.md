## Publish events (breakdown only)

For breakdown, publish **only** these kinds. Do not emit `run.summary`, `review.note`, or `human.feedback`.

| Event | `payload.kind` | Role |
| ----- | -------------- | ---- |
| `INSIGHT` | `task.plan` | Register the full approved task ledger |
| `SIGNAL` | `task.ready` | Kick the first runnable task (optional; see rule below) |
| `INSIGHT` | `context.note` | Short trace fact for later bees (optional) |

### `task.plan` — full breakdown (one event, many tasks)

Emit **one** `INSIGHT/task.plan` after the Beekeeper approves the slices. Put every approved task in `payload.tasks` (dependency order: blockers first). Use `taskId` format `nnn-short-description`.

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"task.plan","tasks":[{"taskId":"001-live-status-api","title":"Live bee status API slice","body":"## Parent\n\ndocs/specs/004-live-bees-indicator.md\n\n## What to build\n\nEnd-to-end path that exposes current bee run status over the API and is verifiable with a focused test.\n\n## Acceptance criteria\n\n- [ ] Status endpoint returns active bee runs for a trace\n- [ ] Focused test covers the happy path\n\n## Blocked by\n\nNone - can start immediately","bee":"builder","intent":"feature","dependsOn":[]},{"taskId":"002-live-status-ui","title":"Live bee status UI slice","body":"## Parent\n\ndocs/specs/004-live-bees-indicator.md\n\n## What to build\n\nUI indicator that consumes the status API and shows live bee activity.\n\n## Acceptance criteria\n\n- [ ] Indicator reflects running vs idle bees\n- [ ] Works against the API from 001-live-status-api\n\n## Blocked by\n\n- 001-live-status-api","bee":"builder","intent":"feature","dependsOn":["001-live-status-api"]},{"taskId":"003-merge-gate","title":"Human review and merge","bee":"receiver","review":"final","dependsOn":["002-live-status-ui"]}]}}
EOF
```

### `task.ready` — first task only, only when Beekeeper confirms immediate start

Emit `SIGNAL/task.ready` **only if** both are true:

1. The Beekeeper explicitly confirmed starting the first task now (not merely approved the plan).
2. You emit it for **exactly one** task: the first approved slice with no blockers (`dependsOn` empty / "can start immediately").

Do **not** emit `task.ready` for later tasks — the ledger/reactor unlocks them after prior tasks complete.

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"task.ready","taskId":"001-live-status-api","title":"Live bee status API slice","body":"## Parent\n\ndocs/specs/004-live-bees-indicator.md\n\n## What to build\n\nEnd-to-end path that exposes current bee run status over the API and is verifiable with a focused test.\n\n## Acceptance criteria\n\n- [ ] Status endpoint returns active bee runs for a trace\n- [ ] Focused test covers the happy path\n\n## Blocked by\n\nNone - can start immediately","bee":"builder","intent":"feature"}}
EOF
```

If the Beekeeper approved the plan but did **not** ask to start immediately, publish `task.plan` only and stop.

### `context.note` — optional trace context

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"context.note","summary":"Breakdown sourced from docs/specs/004-live-bees-indicator.md; first AFK slice is 001-live-status-api"}}
EOF
```
