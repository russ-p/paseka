## SIGNAL events

Use `type: SIGNAL` to mark operational signals on the bus.

### `task.ready` — mark a task ready to run

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"task.ready","taskId":"task-1","title":"Add endpoint","bee":"builder","sector":"backend-users"}}
EOF
```
