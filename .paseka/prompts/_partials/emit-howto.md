When you need to publish a bus event during a run:

1. Build one valid JSON object for the event.
2. Validate and publish it with Paseka CLI via stdin (`paseka event emit --stdin`).
3. If validation fails, inspect the returned JSON error, fix the event, and retry once.
4. After successful publish (or successful defer), continue with a normal human-readable summary.

Do not print raw event JSON in the final answer.
Do not write event JSON directly to files.

### Live vs deferred emit

By default, `event emit` publishes to the bus **immediately** (live). Add **`--defer`** to queue the event in the run's pending buffer; runtime flushes the queue to the bus **only after successful run or session completion** (FIFO). Failed or cancelled runs do not auto-flush — use `paseka event pending` to inspect and `paseka event flush` to recover or `--discard` to clear.

**`--defer` requires an existing run directory** for the event's `traceId` and `agentId` (`.paseka/runs/<traceId>/<agentId>/`). Without that audit boundary, defer fails closed.

| Use `--defer` | Use live (default) |
| ------------- | ------------------ |
| Handoffs meant for after this bee finishes (`task.plan`, `task.ready` kick after plan, `context.note`, `spec.ready`, optional explicit deferred `artifact.written`) | Need another bee or human reaction **during** this run/session |
| Invite `done_when` should complete at run boundary, not mid-session | Mid-run control (`feature.classified`, `session.invite`, energy, kill) |
| Bundle ledger + narrative at successful exit (emit `task.plan` then `task.ready` in that order when starting now) | Debugging with immediate timeline feedback |

**Hard-deny for `--defer`** (live-only platform kinds): `system.kill`, `energy.add`, `energy.consume`, `session.invite`, `beekeeper.ready`, `task.status`.

**Soft guidance:** do not defer `run.summary` when runtime auto-synthesis applies — let the platform publish one after your successful exit unless you need a custom summary before that.

Trail comb artifacts ([014](../../../docs/specs/014-artifacts-protocol.md)): write handoff files under `{{.ArtifactsDir}}` during the run. Runtime scan-flushes `SIGNAL/artifact.written` on **successful** exit for files this run added or changed — do not emit `artifact.written` on every save. Optional explicit deferred `artifact.written` is available when you want intent visible in the pending queue; coexistence skips duplicate scan flush when deferred artifact lines are already pending/flushed.

Use this command form (live):

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"context.note","summary":"Short narrative context"}}
EOF
```

For end-of-run handoffs, prefer defer:

```bash
paseka event emit --defer --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"context.note","summary":"Short narrative context"}}
EOF
```

Each event JSON object must include:
- `traceId` — current flight trail id (`{{.TraceID}}`)
- `agentId` — current agent run id (`{{.AgentID}}`)
- `type` — the event type your bee role may publish (see role-specific emit guidance below)
- `payload` — event-specific object with required `payload.kind`

If the command returns `"ok": false`, treat it as a failed publish and correct the payload before continuing. Deferred success includes `"deferred": true` and does not publish to the bus until flush.
