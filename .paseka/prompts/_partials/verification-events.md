## Verification events

When you need to publish a bus event during a run:

1. Build one valid JSON object.
2. Publish it with `paseka event emit --stdin`.
3. If validation fails, inspect the JSON error, fix the payload, and retry once.

Do not print raw event JSON in the final answer.
Do not write event JSON directly to files.

Publish exactly one final `VERIFICATION` gate decision:
- `verification.success` when all requirements, scope checks, and targeted checks pass.
- `verification.failed` when anything required is missing or failing.

Optional: publish one `INSIGHT/review.note` for extra reviewer context. It does not replace the required `VERIFICATION`.

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"VERIFICATION","payload":{"kind":"verification.success","summary":"All requirements met"}}
EOF
```

Change `payload.kind` to `verification.failed` when rejecting. Each event must include `traceId`, `agentId`, `type`, and `payload.kind`.
