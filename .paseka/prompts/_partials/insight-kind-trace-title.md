### `trace.title` — Flight Trail name

Emit when classifying or planning work. Last event wins for Console display and `{{.TraceTitle}}`.

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"trace.title","title":"Live bees in Queen Console header"}}
EOF
```
