# Trail metadata — `trace.title`, `trace.summary`, `worktree.branch`, `pr.body`

These describe the whole flight trail, not one run: the Console reads the title
and the summary, the merge reads the summary, and the pull request reads
`pr.body`. Last write wins, none of them enters prompt memory, and a later round
replaces what an earlier one wrote.

Run memory is in `insight-run.md`; the gate's output is in `verification.md`.

`ROOT`, `TRACE`, `AGENT`, and `SKILL_DIR` are defined in `SKILL.md` § *The three
values every emit needs*.

| Kind | Read by | Bound |
| --- | --- | --- |
| `trace.title` | Console trail name, `{{.TraceTitle}}` | last write wins |
| `trace.summary` | Console subtitle, default merge commit body | 800 characters |
| `worktree.branch` | worktree ensure, rename, merge, pull-request head | last write wins |
| `pr.body` | the pull-request description | 8000 characters |

Defer all four. Each claims something about a trail that has not shipped yet, and
a run that fails after publishing one must not leave the claim behind.

## `trace.title` and `trace.summary` — trail identity and prose

`trace.title` names the trail. `trace.summary` is one to three sentences of plain
prose: the Console shows it as the trail subtitle, and the merge uses it as the
commit body when no other text is given. No conventional commit prefixes in
either.

```bash
printf '%s' '{"kind":"trace.title","title":"Live bees in the console header"}' \
| sh "$SKILL_DIR/scripts/emit.sh" --root "$ROOT" --trace "$TRACE" --agent "$AGENT" \
    --type INSIGHT --defer

printf '%s' '{"kind":"trace.summary","summary":"Added live bee activity to the console header and covered the empty state."}' \
| sh "$SKILL_DIR/scripts/emit.sh" --root "$ROOT" --trace "$TRACE" --agent "$AGENT" \
    --type INSIGHT --defer
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
    --type INSIGHT --defer
```

## Proof

The trail record carries the newest value of each kind after the flush:

```bash
jq -r 'select(.payload.kind=="trace.summary")|.payload.summary' \
  "$ROOT/.paseka/runs/$TRACE/trail-events.ndjson" | tail -1
```
