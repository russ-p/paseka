## Publish events (intake only)

Always emit `feature.classified` first, then follow the decision.

| `payload.kind` | When |
| --- | --- |
| `SIGNAL/feature.classified` | Always. `decision` is `grill`, `plan`, `triage`, `clarify`, or `reject`, with a `rationale`. Do not set `bee` or `intent`. |
| `INSIGHT/task.plan` | `decision=plan` or `triage`: one builder task |
| `SIGNAL/task.ready` | That task, only when the entry explicitly asks to start now |
| `INSIGHT/trace.title` | Every intake with a usable short name |
| `INSIGHT/worktree.branch` | `plan` or `triage` |
| `INSIGHT/run.summary` | Optional one-line digest |

Never publish `task.plan` or `task.ready` for `grill`, `clarify`, or `reject`.

### `feature.classified`

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"feature.classified","decision":"grill","rationale":"Product idea without acceptance criteria; needs grilling before breakdown."}}
EOF
```

Change `decision` and `rationale`; the shape stays.

### `task.plan` — one builder slice

One task in `payload.tasks`, deferred. Shape:

```json
{"kind":"task.plan","tasks":[{"taskId":"001-add-health-badge","title":"Add health badge to header","bee":"builder","intent":"feature","dependsOn":[],"body":"## What to build\n\n…\n\n## Acceptance criteria\n\n- [ ] …\n\n## Blocked by\n\nNone - can start immediately"}]}
```

`intent` is `feature` for a plan and `bugfix` for a triage. The body is the slice
contract: what to build or fix, acceptance criteria as a checklist, and what
blocks it.

### `task.ready` — start now only

Only when the entry text explicitly asks to start immediately. Emit it directly
after the plan, also deferred, so both flush in order; `taskId` only — the ledger
already has the title and body from the plan.

### Trail identity

`trace.title` names the trail in the Console; `worktree.branch` chooses the
branch for isolated work (`feature/<slug>` for a plan, `fix/<slug>` for a
triage). Payloads: skill `paseka-event-emit` → `references/insight.md`.
