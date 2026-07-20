## INSIGHT events

Use `type: INSIGHT` for context, audit, and dashboard narrative. INSIGHT events do **not** drive workflow routing.

Runtime automatically projects selected narrative INSIGHT kinds into `{{.Insights}}` for subsequent bees on the same trace.

| `payload.kind` | Role | Included in prompt memory |
| -------------- | ---- | ------------------------- |
| `run.summary` | Short run outcome for the next bee | yes |
| `review.note` | Reviewer observation (non-gate) | yes |
| `context.note` | Trace/task context fact | yes |
| `human.feedback` | Beekeeper HITL feedback | yes |
| `task.plan` | Task ledger planning | no (operational) |
| `trace.title` | Flight Trail display name | no (operational) |

### `trace.title` — Flight Trail name (planner bees)

Emit when classifying or planning work. Last event wins for Console display and `{{.TraceTitle}}`.

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"trace.title","title":"Live bees in Queen Console header"}}
EOF
```

### `run.summary` — narrative after work (runtime may auto-synthesize)

Runtime auto-publishes `INSIGHT/run.summary` after a successful AFK run when the bee policy allows and no summary was emitted during the run. You may still publish one explicitly:

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"run.summary","summary":"Implemented OAuth callback and added focused tests","taskId":"{{.TaskID}}"}}
EOF
```

### `review.note` — optional reviewer context

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"review.note","summary":"Token refresh path still lacks retry handling","taskId":"{{.TaskID}}","severity":"medium"}}
EOF
```

### `context.note` — optional trace context

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"context.note","summary":"NATS KV is the source of truth for task ledger state"}}
EOF
```

### `task.plan` — task breakdown

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"task.plan","tasks":[{"taskId":"task-1","title":"Add endpoint","bee":"builder","sector":"backend-users"}]}}
EOF
```
