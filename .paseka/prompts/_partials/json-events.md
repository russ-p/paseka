When emitting bus events during a run, append one JSON object per line (NDJSON) to the event log or stdout.

Each line must be valid JSON with fields:
- `traceId` — current flight trail id
- `type` — one of SIGNAL, INSIGHT, MUTATION, VERIFICATION
- `payload` — event-specific object

Example:
{"traceId":"{{.TraceID}}","type":"INSIGHT","payload":{"summary":"found auth gap"}}
