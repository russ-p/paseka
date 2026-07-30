### `context.note` — optional trace context

Prefer `--defer` for end-of-run handoff notes; use live only when another bee must see the fact during this run.

```bash
paseka event emit --defer --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"context.note","summary":"NATS KV is the source of truth for task ledger state"}}
EOF
```
