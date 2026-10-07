Publish **exactly one** `INSIGHT/context.note` at the end of the run. Nothing else — no `MUTATION`, no `SIGNAL`, no `VERIFICATION`.

Set `taskId` to the audited task when the intent is `task` and one task was audited. Leave it out when the intent is `chronicle` or when several tasks were audited.

Use `--defer`: the note is a record for the next bee and for the Beekeeper, and nothing needs to react to it mid-run.

```bash
paseka event emit --defer --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"context.note","taskId":"001-adapter-probe-system-tab","summary":"Trail audit: 1 review note left unresolved (low, roster duplicated across Go/TS). Draft at artifacts/audit.md. Offline blind spot: synthesized MUTATION absent from events.ndjson."}}
EOF
```

The `summary` is one paragraph, not the report: what the trail cost, what remains owed, and where the draft is. The report itself belongs in `{{.ArtifactsDir}}/audit.md`, which runtime scan-flushes as a trail comb artifact on successful exit.