## Publish events (intake only)

Always emit `feature.classified` first. Then follow the decision branch below.

| Event | `payload.kind` | When |
| ----- | -------------- | ---- |
| `SIGNAL` | `feature.classified` | Always (required) |
| `INSIGHT` | `task.plan` | `decision=plan` or `decision=triage` (one builder task) |
| `SIGNAL` | `task.ready` | Same slice, only when entry text asks to start now |
| `INSIGHT` | `run.summary` | Optional one-line summary |

Do **not** emit `task.plan` or `task.ready` when `decision=grill`, `clarify`, or `reject`.

### `feature.classified` — classification decision

Emit **one** `SIGNAL/feature.classified`. Set `decision` and `rationale`. Do **not** set `bee` / `intent`.

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"feature.classified","decision":"grill","rationale":"Product idea without acceptance criteria; needs grilling before breakdown."}}
EOF
```

When `decision=plan` (clear small feature):

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"feature.classified","decision":"plan","rationale":"Single clear improvement; one builder slice is enough."}}
EOF
```

When `decision=triage` (bug):

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"feature.classified","decision":"triage","rationale":"Regression on Windows; one focused bugfix slice."}}
EOF
```

When `decision=reject`:

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"feature.classified","decision":"reject","rationale":"Duplicate of an existing spec; no new work."}}
EOF
```

### `task.plan` — one builder slice (`plan` or `triage`)

Emit **one** `INSIGHT/task.plan` with a single task in `payload.tasks`.

Clear feature (`decision=plan`):

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"task.plan","tasks":[{"taskId":"001-add-health-badge","title":"Add health badge to header","body":"## What to build\n\nShow colony health in the console header.\n\n## Acceptance criteria\n\n- [ ] Badge visible when NATS is connected\n- [ ] Badge shows disconnected state clearly\n\n## Blocked by\n\nNone - can start immediately","bee":"builder","intent":"feature","dependsOn":[]}]}}
EOF
```

Bug (`decision=triage`):

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"task.plan","tasks":[{"taskId":"001-fix-windows-path","title":"Fix path handling on Windows","body":"## Symptom\n\nColony init fails on Windows when path contains spaces.\n\n## What to fix\n\nCorrect path normalization so init succeeds on Windows.\n\n## Acceptance criteria\n\n- [ ] Init succeeds on Windows with spaced paths\n- [ ] Regression test added when feasible\n\n## Blocked by\n\nNone - can start immediately","bee":"builder","intent":"bugfix","dependsOn":[]}]}}
EOF
```

### `task.ready` — start now only

Emit **only** when title/body/task text explicitly requests immediate start. Use the same `taskId`, title, body, `bee`, and `intent` as the single planned task.

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"task.ready","taskId":"001-fix-windows-path","title":"Fix path handling on Windows","body":"## Symptom\n\nColony init fails on Windows when path contains spaces.\n\n## What to fix\n\nCorrect path normalization so init succeeds on Windows.\n\n## Acceptance criteria\n\n- [ ] Init succeeds on Windows with spaced paths\n- [ ] Regression test added when feasible\n\n## Blocked by\n\nNone - can start immediately","bee":"builder","intent":"bugfix"}}
EOF
```

### `run.summary` — optional

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"run.summary","summary":"Intake: triage bug, one builder slice planned"}}
EOF
```
