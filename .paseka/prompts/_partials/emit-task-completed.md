## VERIFICATION / task.completed (commit gate)

After you commit the approved changes, publish exactly one `task.completed` event.
Do **not** publish `verification.success` or `verification.failed` — those are review-gate outcomes from Guard, and re-emitting them re-triggers this bee.

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"VERIFICATION","payload":{"kind":"task.completed","taskId":"{{.TaskID}}","status":"completed","summary":"Endpoint implemented and committed"}}
EOF
```

Each event must include `traceId`, `agentId`, `type`, and `payload.kind`. Prefer the real `payload.taskId` from the task context when known.
