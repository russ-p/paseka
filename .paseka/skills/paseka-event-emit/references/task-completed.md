# `task.completed` — committed work

The kind records that the task's change is committed, and it carries the commit
SHA so the ledger can show it. `payload.status` is required even though the
platform catalog lists only `taskId`: validation refuses the payload without it
(`payload.status: required`).

Publishing `verification.success` or `verification.failed` here is wrong: those
are review decisions and re-trigger the review. One gate owns each kind.

`ROOT`, `TRACE`, `AGENT`, and `SKILL_DIR` are defined in `SKILL.md` § *The three
values every emit needs*. Take the SHA from the worktree that holds the commit;
publish from the colony root.

```bash
TASK=007-build-output-root-guard
SHA=$(git -C "${PASEKA_WORKSPACE:-$ROOT/.paseka/worktrees/$TRACE}" rev-parse HEAD)

printf '%s' "$(jq -cn --arg t "$TASK" --arg c "$SHA" \
  '{kind:"task.completed",taskId:$t,status:"completed",commit:$c,
    summary:"Guarded the console bundle against a /next base path"}')" \
| sh "$SKILL_DIR/scripts/emit.sh" --root "$ROOT" --trace "$TRACE" --agent "$AGENT" \
    --type VERIFICATION
```

Good result — the emit names the kind and the ledger projects the SHA:

```json
{"ok":true,"traceId":"trace-…","type":"VERIFICATION","kind":"task.completed","subject":"paseka.events.VERIFICATION.task.completed","eventLogPath":"…/events.ndjson"}
```

Proof:

```bash
jq -r 'select(.payload.kind=="task.completed")|.payload.commit' \
  "$ROOT/.paseka/runs/$TRACE/$AGENT/events.ndjson" | tail -1
grep -m1 '^commit:' "$ROOT/.paseka/runs/$TRACE/tasks/$TASK/task.md"
```

Both print the same SHA. An empty `commit` in `task.md` while the run published
one means the reactor is down — the event is recorded, not lost.
