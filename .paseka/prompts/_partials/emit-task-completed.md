## VERIFICATION / task.completed (commit gate)

After you commit the approved changes, publish exactly one `task.completed` event.
Do **not** publish `verification.success` or `verification.failed` — those are review-gate outcomes from Guard, and re-emitting them re-triggers this bee.

Before publishing, read the commit you just made and carry its SHA in the payload:

```bash
git rev-parse HEAD
```

Set `payload.commit` to that SHA. The ledger projects it into the `commit` field of the task's `task.md` frontmatter, so an empty value leaves the trail without a commit record.

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"VERIFICATION","payload":{"kind":"task.completed","taskId":"{{.TaskID}}","status":"completed","summary":"Endpoint implemented and committed","commit":"HEAD-SHA-FROM-rev-parse"}}
EOF
```

Each event must include `traceId`, `agentId`, `type`, and `payload.kind`. Prefer the real `payload.taskId` from the task context when known, and always replace `HEAD-SHA-FROM-rev-parse` with the actual `git rev-parse HEAD` output from the commit you just created.
