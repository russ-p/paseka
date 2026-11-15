# Run memory — `context.note`, `run.summary`

These two record what this run and its task did; later bees on the same trail read
them as prompt memory. The trail as a whole is elsewhere: `trace.title`,
`trace.summary`, `worktree.branch`, and `pr.body` are in `insight-trail.md`, and a
`review.note` sits beside the gate decision in `verification.md`.

`ROOT`, `TRACE`, `AGENT`, and `SKILL_DIR` are defined in `SKILL.md` § *The three
values every emit needs*.

| Kind | Records | Later prompts |
| --- | --- | --- |
| `context.note` | A fact the next bee must know | yes |
| `run.summary` | What this run did | yes |

Both are records, so both defer (`SKILL.md` § *Live or deferred*). Prompt memory
is task-scoped: a note carrying `taskId` reaches that task only, so leave it out
when a later round on the trail must read it.

`human.feedback` is the Beekeeper's, published from the CLI or the Console. A bee
never publishes it.

## `context.note` — one fact the next bee needs

A decision, a gotcha, a path. Defer it: it is a record, and nothing reacts to it
mid-run. Publish live only when another bee must read the fact before this run
ends.

```bash
printf '%s' '{"kind":"context.note","summary":"The AFK gate runs the trusted colony copy of checks.sh, so a worktree-local guard line is absent from its log"}' \
| sh "$SKILL_DIR/scripts/emit.sh" --root "$ROOT" --trace "$TRACE" --agent "$AGENT" \
    --type INSIGHT --defer
```

## `run.summary` — what this run did

The runtime synthesizes one for a successful run when the bee policy allows and
the run published none. Publish explicitly, deferred, only when the automatic
text would lose the thing that must be remembered; do not publish a duplicate.

```bash
printf '%s' '{"kind":"run.summary","summary":"Deleted the legacy console and its contract tests","taskId":"004-delete-legacy-console"}' \
| sh "$SKILL_DIR/scripts/emit.sh" --root "$ROOT" --trace "$TRACE" --agent "$AGENT" \
    --type INSIGHT --defer
```

## Proof

The deferred emit is queued, and the flush is visible in the trail record after
the run exits:

```bash
paseka -C "$ROOT" event pending --trace "$TRACE" --agent "$AGENT"
# {"ok":true,"traceId":"trace-…","agentId":"…","count":1,"kinds":["context.note"]}

jq -r 'select(.payload.kind=="run.summary")|.createdAt' \
  "$ROOT/.paseka/runs/$TRACE/trail-events.ndjson" | tail -1
```

Say "queued for flush" — never "broadcast" — until the trail record shows it.
