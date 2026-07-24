{{if .IsLastWorkTask}}
### `trace.summary` — Flight trail summary (**required** on last work task)

You **must** emit one `INSIGHT/trace.summary` with 1–3 sentences of plain prose describing what this flight trail accomplished. This is a prompt-level obligation, not a runtime completion-contract failure.

Do not use conventional commit prefixes (`feat:`, `fix:`, etc.).

**Naming (do not confuse):**
- `paseka proposal approve --summary` → `VERIFICATION/task.completed` completion note
- `mergeMessage` / `--merge-message` → git merge commit **subject** (and optional HITL body)
- `INSIGHT/trace.summary` → trail description and default merge commit **body**

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"trace.summary","summary":"Implemented OAuth callback and added focused tests for token refresh."}}
EOF
```
{{end}}
