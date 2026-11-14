## Publishing bus events

Publish one event per decision, then prove it landed. The procedure — envelope,
live versus deferred, proof commands, failure handling — is the skill
`paseka-event-emit`; read it when this run must publish. If this run was not
offered the skill, publish from the kind list below and let `paseka event
validate` name any missing field.

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"…"}}
EOF
```

- `traceId` and `agentId` are this run's ids — copy them from the header above.
- Add `--defer` for a handoff consumed after this run succeeds; omit it when
  something must react during the run. Live-only kinds refuse `--defer`.
- `paseka event validate --stdin` writes nothing: run it first when the payload
  shape is not one you have published before.
- `ok:false` is a fix, not a retry loop: correct the field it names and publish
  once more.
- Never write event JSON to a file, and never print it in the final answer.

{{template "standing-checkpoint" .}}
