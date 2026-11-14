# Narrative INSIGHT kinds

`INSIGHT` records what happened; it routes no work. Three of these kinds are
projected into later prompts on the same trail, the rest are trail metadata that
the Console, the merge, and the pull request read.

`ROOT`, `TRACE`, `AGENT`, and `SKILL_DIR` are defined in `SKILL.md` § *The three
values every emit needs*.

| Kind | Records | Later prompts |
| --- | --- | --- |
| `context.note` | A fact the next bee on the trail must know | yes |
| `run.summary` | What this run did | yes |
| `review.note` | A non-blocking reviewer observation | yes |
| `trace.title` | The flight trail's display name | no, last write wins |
| `trace.summary` | The trail description and default merge body (800 chars) | no, last write wins |
| `worktree.branch` | The git branch of the trail's isolated worktree | no, last write wins |
| `pr.body` | The pull-request description (8000 chars) | no, last write wins |

`human.feedback` is the Beekeeper's, published from the CLI or the Console. A
bee never publishes it.

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
the run published none. Publish explicitly, live, only when the automatic text
would lose the thing that must be remembered; do not defer a duplicate.

```bash
printf '%s' '{"kind":"run.summary","summary":"Deleted the legacy console and its contract tests","taskId":"004-delete-legacy-console"}' \
| sh "$SKILL_DIR/scripts/emit.sh" --root "$ROOT" --trace "$TRACE" --agent "$AGENT" \
    --type INSIGHT
```

## `review.note` — non-blocking observation

It never replaces a gate decision: publish it in addition to the `VERIFICATION`
the prompt requires, never instead of one.

`severity` is optional and closed: `info`, `low`, `medium`, `high`, `critical`.
It is trimmed and lowercased, so `" Medium "` is stored as `medium`. A value
outside the set fails emit with `payload.severity: must be one of …`.

```bash
printf '%s' '{"kind":"review.note","taskId":"001-mount-console-at-root","severity":"info","summary":"fallback.html still says Queen Console Next; a later slice owns that copy change"}' \
| sh "$SKILL_DIR/scripts/emit.sh" --root "$ROOT" --trace "$TRACE" --agent "$AGENT" \
    --type INSIGHT
```

## `trace.title` and `trace.summary` — trail identity and prose

`trace.title` names the trail in the Console and in `{{.TraceTitle}}`.
`trace.summary` is one to three sentences of plain prose; the Console shows it as
the trail subtitle and the merge uses it as the commit body when no other text
is given. No conventional commit prefixes in either.

```bash
printf '%s' '{"kind":"trace.title","title":"Live bees in the Console header"}' \
| sh "$SKILL_DIR/scripts/emit.sh" --root "$ROOT" --trace "$TRACE" --agent "$AGENT" \
    --type INSIGHT --defer

printf '%s' '{"kind":"trace.summary","summary":"Added live bee activity to the console header and covered the empty state."}' \
| sh "$SKILL_DIR/scripts/emit.sh" --root "$ROOT" --trace "$TRACE" --agent "$AGENT" \
    --type INSIGHT
```

Do not confuse the three texts:

- `task.completed.summary` — the completion note on the ledger
- the merge commit *subject* — a separate argument of the merge command
- `trace.summary` — the trail description and default merge *body*

## `worktree.branch` — the branch isolated work is done on

Last write wins. The worktree path stays `.paseka/worktrees/<traceId>/`; only the
branch is chosen. A conventional kebab slug from the trail title is the usual
choice (`feature/<slug>`, `fix/<slug>`).

```bash
printf '%s' '{"kind":"worktree.branch","branch":"feature/live-bees-header"}' \
| sh "$SKILL_DIR/scripts/emit.sh" --root "$ROOT" --trace "$TRACE" --agent "$AGENT" \
    --type INSIGHT --defer
```

## `pr.body` — pull-request description

Markdown, at most 8000 characters after trim, last write wins. When none is
published the pull request falls back to `trace.summary`.

```bash
body=$(cat <<'MD'
## Why

Ships the isolated trail as a reviewable pull request.

## What changed

- Mounted the console at the repository root.
MD
)
printf '%s' "$(jq -cn --arg b "$body" '{kind:"pr.body",body:$b}')" \
| sh "$SKILL_DIR/scripts/emit.sh" --root "$ROOT" --trace "$TRACE" --agent "$AGENT" \
    --type INSIGHT
```

## Proof

The deferred note is queued, the live ones are in this run's record:

```bash
paseka -C "$ROOT" event pending --trace "$TRACE" --agent "$AGENT"
# {"ok":true,"traceId":"trace-…","agentId":"…","count":1,"kinds":["context.note"]}

jq -r 'select(.payload.kind=="review.note")|[.payload.severity,(.payload.taskId//"-")]|@tsv' \
  "$ROOT/.paseka/runs/$TRACE/$AGENT/events.ndjson"
# info	001-mount-console-at-root
```
