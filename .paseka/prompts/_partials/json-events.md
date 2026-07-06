When you need to publish a bus event during a run:

1. Build one valid JSON object for the event.
2. Validate and publish it with Paseka CLI via stdin.
3. If validation fails, inspect the returned JSON error, fix the event, and retry once.
4. After successful publish, continue with a normal human-readable summary.

Do not print raw event JSON in the final answer.
Do not write event JSON directly to files.

Use this command form:

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"VERIFICATION","payload":{"kind":"verification.success","summary":"All requirements met"}}
EOF
```

Each event JSON object must include:
- `traceId` — current flight trail id (`{{.TraceID}}`)
- `agentId` — current agent run id (`{{.AgentID}}`)
- `type` — one of `SIGNAL`, `INSIGHT`, `MUTATION`, `VERIFICATION`
- `payload` — event-specific object with required `payload.kind`

**Routing vs narrative:**
- `VERIFICATION` — gate outcomes that drive workflow routing (`verification.success`, `verification.failed`, `task.completed`)
- `INSIGHT` — narrative context for audit and prompt memory (`run.summary`, `review.note`, `context.note`, `human.feedback`)

If the command returns `"ok": false`, treat it as a failed publish and correct the payload before continuing.
