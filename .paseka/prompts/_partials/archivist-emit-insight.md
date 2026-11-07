{{if eq .Intent "skill"}}Publish **exactly one** `INSIGHT/skill.proposed` at the end of the run. Nothing else — no `MUTATION`, no `SIGNAL`, no `VERIFICATION`, and no `context.note`.

`name`, `description`, `bundle`, and `procedureHash` must describe the bundle you just wrote. `bundle` is relative to the comb (`skill-proposals/<name>`); an absolute path or a store path is refused at emit. `reason` cites the runs, files, and events that motivated the proposal inline. Leave `taskId` out: a proposal belongs to the trail, not to one task.

Use `--defer`: the proposal is a record for the Beekeeper and the decisions on it, and nothing needs to react to it mid-run.

```bash
paseka event emit --defer --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"skill.proposed","name":"paseka-release","description":"Run the colony release procedure. Use when shipping a version.","reason":"Two runs re-ran the release checklist by hand after task 002 failed verification (verification.failed at 01a1b4..., rework run 01a1b7...); docs/guide/colony-layout.md has no release procedure.","bundle":"skill-proposals/paseka-release","procedureHash":"<sha256 printed by the bundle hash command>"}}
EOF
```

The bundle itself belongs in `{{.ArtifactsDir}}/skill-proposals/<name>/`; runtime scan-flushes it as a trail comb artifact on successful exit.

{{else}}Publish **exactly one** `INSIGHT/context.note` at the end of the run. Nothing else — no `MUTATION`, no `SIGNAL`, no `VERIFICATION`, and no `skill.proposed`.

Set `taskId` to the audited task when the intent is `task` and one task was audited. Leave it out when the intent is `chronicle` or when several tasks were audited.

Use `--defer`: the note is a record for the next bee and for the Beekeeper, and nothing needs to react to it mid-run.

```bash
paseka event emit --defer --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"context.note","taskId":"001-adapter-probe-system-tab","summary":"Trail audit: 1 review note left unresolved (low, roster duplicated across Go/TS). Draft at artifacts/audit.md. Offline blind spot: synthesized MUTATION absent from events.ndjson."}}
EOF
```

The `summary` is one paragraph, not the report: what the trail cost, what remains owed, and where the draft is. The report itself belongs in `{{.ArtifactsDir}}/audit.md`, which runtime scan-flushes as a trail comb artifact on successful exit.{{end}}
