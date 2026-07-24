### `task.plan` — task breakdown

Emit when registering builder-sized slices on the task ledger. Prefer one event with the full `tasks` array for that plan.

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"task.plan","tasks":[{"taskId":"task-1","title":"Add endpoint","bee":"builder","sector":"backend-users"}]}}
EOF
```
