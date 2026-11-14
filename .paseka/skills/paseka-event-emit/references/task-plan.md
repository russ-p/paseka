# `task.plan` — the breakdown

One event carries the whole plan; the ledger creates the tasks. `tasks[]` must be
non-empty and every task needs `taskId` and `title`. `bee`, `intent`, `phase`,
`body`, and `dependsOn` are optional; `dependsOn` names sibling task ids and is
what keeps a final gate closed until its work lands.

`ROOT`, `TRACE`, `AGENT`, and `SKILL_DIR` are defined in `SKILL.md` § *The three
values every emit needs*.

```bash
PLAN=$(jq -cn '{kind:"task.plan",tasks:[
  {taskId:"001-mount-console-at-root",title:"Mount the console at /",bee:"builder"},
  {taskId:"002-rename-console-embed-tree",title:"Drop the next segment from the build and embed",bee:"builder",dependsOn:["001-mount-console-at-root"]}]}')

printf '%s' "$PLAN" \
| sh "$SKILL_DIR/scripts/emit.sh" --root "$ROOT" --trace "$TRACE" --agent "$AGENT" \
    --type INSIGHT --defer
```

Good result — deferred, and the queue names the kind:

```json
{"ok":true,"deferred":true,"traceId":"trace-…","type":"INSIGHT","kind":"task.plan"}
{"ok":true,"traceId":"trace-…","agentId":"…","count":1,"kinds":["task.plan"]}
```

Proof of the board, after the runtime flushes:

```bash
ls "$ROOT/.paseka/runs/$TRACE/tasks"
```

A plan is not a dispatch: it records the slices. `task.ready` is what starts one
(see `task-ready.md`).

`phase` defaults to `work` and that is the only value a bee can set directly: the
reducer owns `closing`, and a plan carrying `review: final` is recorded as
`merge` regardless of publisher. A plan that claims `phase: merge` without it is
coerced to `work`.
