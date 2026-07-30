### `run.summary` — narrative after work (runtime may auto-synthesize)

Runtime auto-publishes `INSIGHT/run.summary` after a successful AFK run when the bee policy allows and no summary was emitted during the run. Do not defer a duplicate when relying on auto-synthesis. You may still publish one explicitly (live):

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"run.summary","summary":"Implemented OAuth callback and added focused tests","taskId":"{{.TaskID}}"}}
EOF
```
