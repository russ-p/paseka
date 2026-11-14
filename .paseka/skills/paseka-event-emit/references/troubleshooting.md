# Emit failures

| `error` | Cause | Fix |
| --- | --- | --- |
| `schema_validation_failed` + `details[].path` | A field is missing or outside its vocabulary | Fix that exact path: `task.completed` needs `status`; `review.note.severity` is a closed set; `skill.proposed` needs all five fields |
| `invalid_json` | stdin was not exactly one JSON object | Pipe one object, no trailing prose, no comments |
| `defer_denied` | The kind is live-only | Drop `--defer` |
| `run_required` | Deferred emit with no resolvable colony root | Pass `-C "$PASEKA_COLONY_ROOT"` |
| `defer_failed` | No run directory for that trace and agent | Use the current run's ids; a trail-level run publishes live |
| `audit_log_failed` | Emit from outside the run that owns the event | Emit from that run's directory |
| `colony: home config points to "…", but repo is "…"` | cwd is a worktree | Pass `-C "$PASEKA_COLONY_ROOT"` |
| `ok:true` with no `subject` | Bus is down; the event was recorded locally | Say "recorded, not broadcast"; `paseka verify <traceId>` once the bus is back |
| nothing printed, exit 2 | `emit.sh` was given no trace, agent, or payload kind | Run it with `--trace`, `--agent`, and a payload carrying `kind` |

Rules that prevent most of these:

- One object per invocation, envelope ids from this run's environment.
- `validate` first — it is the only side-effect-free check.
- Retry once after a fix. A second failure goes into the run summary as a stated
  gap; it is not a loop, and a hand-written event file is never the workaround.
