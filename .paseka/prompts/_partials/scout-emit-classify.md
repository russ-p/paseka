## Publish events (classify only)

For classify, publish **only** these kinds. Do not emit `task.plan` or `task.ready` when `route=grill`.

| Event | `payload.kind` | Role |
| ----- | -------------- | ---- |
| `SIGNAL` | `feature.classified` | Route the idea (required) |
| `INSIGHT` | `run.summary` | Short classification summary (optional) |

### `feature.classified` — one routing decision

Emit **one** `SIGNAL/feature.classified` after classification. Set `route`, `rationale`, and when the route needs a next bee: `bee` + `intent`. `confidence` is optional and advisory.

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"feature.classified","route":"grill","bee":"drone","intent":"grilling","rationale":"Product idea without acceptance criteria; needs grilling before breakdown."}}
EOF
```

When `route=plan` and the spec is already clear:

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"feature.classified","route":"plan","rationale":"PRD is clear enough for vertical-slice breakdown without grilling."}}
EOF
```

When `route=reject`:

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"feature.classified","route":"reject","rationale":"Duplicate of an existing spec; no new work."}}
EOF
```

### `run.summary` — optional

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"run.summary","summary":"Classified feature.requested as grill → drone grilling"}}
EOF
```
