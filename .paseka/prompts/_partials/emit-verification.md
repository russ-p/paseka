## VERIFICATION events

Use `type: VERIFICATION` for gate outcomes that drive workflow routing.

Publish exactly one final `VERIFICATION` gate decision when your bee role requires it:
- `verification.success` when all requirements, scope checks, and targeted checks pass.
- `verification.failed` when anything required is missing or failing.

Optional: publish one `INSIGHT/review.note` for extra reviewer context. It does not replace the required `VERIFICATION`.

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"VERIFICATION","payload":{"kind":"verification.success","summary":"All requirements met"}}
EOF
```

Change `payload.kind` to `verification.failed` when rejecting.

### `task.completed` — report task passed review/commit gate

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"VERIFICATION","payload":{"kind":"task.completed","taskId":"task-1","status":"completed","summary":"Endpoint implemented and committed"}}
EOF
```

Each event must include `traceId`, `agentId`, `type`, and `payload.kind`.
