### `review.note` — optional reviewer context

Does not replace a required `VERIFICATION` gate decision.

`severity` is optional. When present it must be one of `info`, `low`, `medium`, `high`, or
`critical`; the value is trimmed and lowercased, so `" Medium "` is stored as `medium`. Omit it
when the note has no severity.

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"review.note","summary":"Token refresh path still lacks retry handling","taskId":"{{.TaskID}}","severity":"medium"}}
EOF
```
