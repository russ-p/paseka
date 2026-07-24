## VERIFICATION events (review gate)

Use `type: VERIFICATION` for **review** gate outcomes that drive workflow routing.

Publish exactly one final gate decision:
- `verification.success` when all requirements, scope checks, and targeted checks pass.
- `verification.failed` when anything required is missing or failing.

Do **not** publish `task.completed` from a review bee — that is the receiver / commit-gate role.

Optional: publish one `INSIGHT/review.note` via the guard INSIGHT partial for extra reviewer context. It does not replace the required `VERIFICATION`.

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"VERIFICATION","payload":{"kind":"verification.success","taskId":"{{.TaskID}}","summary":"All requirements met"}}
EOF
```

Change `payload.kind` to `verification.failed` when rejecting.

Each event must include `traceId`, `agentId`, `type`, and `payload.kind`. Include `payload.taskId` when known.
