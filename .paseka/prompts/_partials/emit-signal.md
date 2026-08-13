## SIGNAL events

Use `type: SIGNAL` to mark operational signals on the bus.

### `task.ready` — mark a task ready to run

Emit **after** `task.plan` in the same run, using `--defer` so both flush FIFO on success. Payload needs only `kind` and `taskId` (optional `bee` if not set in plan). Do not copy title/body — the ledger already has them from `task.plan`.

```bash
paseka event emit --defer --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"task.ready","taskId":"task-1"}}
EOF
```
