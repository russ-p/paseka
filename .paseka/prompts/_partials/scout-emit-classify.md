## Publish events (classify only)

For classify, publish **only** these kinds. Do not emit `task.plan` or `task.ready` when `decision=grill`.

| Event | `payload.kind` | Role |
| ----- | -------------- | ---- |
| `SIGNAL` | `feature.classified` | Classification decision (required) |
| `INSIGHT` | `run.summary` | Short classification summary (optional) |

### `feature.classified` — one classification decision

Emit **one** `SIGNAL/feature.classified` after classification. Set `decision`, `rationale`, and when the decision needs a next bee: `bee` + `intent`. `confidence` is optional and advisory.

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"feature.classified","decision":"grill","bee":"drone","intent":"grilling","rationale":"Product idea without acceptance criteria; needs grilling before breakdown."}}
EOF
```

When `decision=plan` and the spec is already clear:

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"feature.classified","decision":"plan","rationale":"PRD is clear enough for vertical-slice breakdown without grilling."}}
EOF
```

When `decision=reject`:

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"feature.classified","decision":"reject","rationale":"Duplicate of an existing spec; no new work."}}
EOF
```

### `run.summary` — optional

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"run.summary","summary":"Classified feature.requested as grill → drone grilling"}}
EOF
```
