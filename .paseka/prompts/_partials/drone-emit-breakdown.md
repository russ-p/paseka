## Publish events (breakdown only)

| `payload.kind` | When |
| --- | --- |
| `INSIGHT/task.plan` | The full task ledger — one event, dependency order, blockers first |
| `SIGNAL/task.ready` | Interactive only: the first runnable task, after explicit confirmation |
| `INSIGHT/trace.title` | The Flight Trail name |
| `INSIGHT/worktree.branch` | When the breakdown includes isolated builder slices |
| `INSIGHT/context.note` | Optional short trace fact for later bees |

Nothing else: no `run.summary`, `review.note`, or `human.feedback`.

### Planning is the deliverable

Breakdown registers work; it does not review it and does not start it.

- **Never author a standalone review or merge task.** The colony `closing` policy
  seeds trail review and the runtime ensures the merge gate; a breakdown-authored
  review card preempts both. A HITL work slice may still carry `review: required`
  when the spec calls for human approval of that slice.
- **Never start implementation on an AFK run.** An AFK breakdown ends with
  `task.plan`: no `task.ready`, no builder woken by you, no question about
  starting — there is no Beekeeper in the session. A human or a cue kicks the
  first unblocked slice later.

### `task.plan` — full breakdown

One event after approval in an interactive session, or after drafting from the
spec in an AFK run. Ids are `nnn-short-description`; each task lists its
dependencies in `dependsOn`; `--defer` because the ledger consumes the plan after
this run ends.

```json
{"kind":"task.plan","tasks":[
 {"taskId":"001-live-status-api","title":"Live bee status API slice","bee":"builder","intent":"feature","dependsOn":[],
  "body":"## Parent\n\ndocs/specs/004-live-bees-indicator.md\n\n## What to build\n\n…\n\n## Acceptance criteria\n\n- [ ] …\n\n## Blocked by\n\nNone - can start immediately"},
 {"taskId":"002-live-status-ui","title":"Live bee status UI slice","bee":"builder","intent":"feature","dependsOn":["001-live-status-api"],
  "body":"## Parent\n\n…\n\n## What to build\n\n…\n\n## Acceptance criteria\n\n- [ ] …\n\n## Blocked by\n\n- 001-live-status-api"}]}
```

The body is the slice contract: parent spec, what to build, acceptance criteria as
a checklist, and what blocks it.

### `task.ready` — interactive only

Emit it **only** when both hold: the Beekeeper explicitly confirmed starting the
first task now, and you name exactly one task — the first approved slice with no
blockers. Directly after the plan, also deferred, so both flush in order;
`taskId` only, no title or body copy. Later tasks are unlocked by the ledger as
their dependencies complete.

If the plan was approved without "start now", publish the plan and stop.

### Trail identity

`trace.title` names the trail; `worktree.branch` chooses the branch for isolated
builder work. Payloads: skill `paseka-event-emit` → `references/insight.md`.
