## Task lifecycle events

Use these `payload.kind` values when publishing task queue events:

- `task.plan` — INSIGHT: publish a breakdown of tasks
- `task.ready` — SIGNAL: mark a task ready to run
- `task.completed` — VERIFICATION: report that a task passed review/commit gate

Examples:

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"task.plan","tasks":[{"taskId":"task-1","title":"Add endpoint","bee":"builder","sector":"backend-users"}]}}
EOF
```

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"task.ready","taskId":"task-1","title":"Add endpoint","bee":"builder","sector":"backend-users"}}
EOF
```

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"VERIFICATION","payload":{"kind":"task.completed","taskId":"task-1","status":"completed","summary":"Endpoint implemented and committed"}}
EOF
```
