## Publish events (grilling completion only)

After shared understanding and writing the spec file, publish **only** these kinds. Do not emit `task.plan`, `task.ready`, or start breakdown during grilling.

| Event | `payload.kind` | Role |
| ----- | -------------- | ---- |
| `SIGNAL` | `spec.ready` | Hand off the written spec (required) |
| `INSIGHT` | `context.note` | Short trace fact for later bees (optional) |

### `spec.ready` — required after writing the spec

1. Write or update `docs/specs/<NNN>-<slug>.md` at the colony root (prefer committed specs over worktree-only paths).
2. Emit **one** `SIGNAL/spec.ready` with `ref` = repo-relative path to that file.

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"spec.ready","ref":"docs/specs/006-live-bees-header.md","title":"Live bees header"}}
EOF
```

Without the spec file on disk **and** `spec.ready`, breakdown must not start.

{{template "insight-kind-context-note" .}}
