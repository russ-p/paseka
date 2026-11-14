# `task.ready` — start one task now

`SIGNAL/task.ready` is the kick, not the plan. The payload needs `kind` and
`taskId` only: the ledger already has the title and body from `task.plan`, so a
copy of them is noise.

`ROOT`, `TRACE`, `AGENT`, and `SKILL_DIR` are defined in `SKILL.md` § *The three
values every emit needs*.

```bash
printf '%s' '{"kind":"task.ready","taskId":"001-mount-console-at-root"}' \
| sh "$SKILL_DIR/scripts/emit.sh" --root "$ROOT" --trace "$TRACE" --agent "$AGENT" \
    --type SIGNAL --defer
```

Defer only when the kick belongs after this run exits — the usual case, published
immediately after the `task.plan` that created the task, so both flush FIFO.
Publish live when work must start while this run is still going.

Emit one per task and only for a slice that can start now (`dependsOn` empty).
The ledger promotes the next eligible task on every completion, so kicking later
slices by hand starts work before its dependencies land.

Proof after the flush:

```bash
jq -r 'select(.payload.kind=="task.ready")|.payload.taskId' \
  "$ROOT/.paseka/runs/$TRACE/trail-events.ndjson"
```
