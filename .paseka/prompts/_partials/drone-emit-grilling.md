## Publish events (grilling completion only)

After shared understanding is reached and the spec file is written, publish only
the kinds below. No `task.plan`, no `task.ready`, no breakdown during grilling.

### `spec.ready` — required after writing the spec

1. Write or update `docs/specs/<NNN>-<slug>.md` at the colony root; prefer
   committed specs over a worktree-only path.
2. Emit one `SIGNAL/spec.ready`, deferred, with `ref` set to the repo-relative
   path of that file and a short `title`.

```bash
paseka event emit --defer --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"SIGNAL","payload":{"kind":"spec.ready","ref":"docs/specs/006-live-bees-header.md","title":"Live bees header"}}
EOF
```

Without the spec file on disk **and** this event, breakdown must not start.

Optional: one `INSIGHT/context.note` for a fact the breakdown needs — payload and
delivery in the skill `paseka-event-emit` → `references/insight.md`.
