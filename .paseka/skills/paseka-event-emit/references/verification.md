# `verification.success` / `verification.failed` — the gate decision

One decision per reviewed task, carrying `taskId`. On success the task leaves
`waiting_review`; on failure it returns to `ready` for rework. This is what
routes work, so the kind is the decision, not the prose around it.

`ROOT`, `TRACE`, `AGENT`, and `SKILL_DIR` are defined in `SKILL.md` § *The three
values every emit needs*.

```bash
printf '%s' '{"kind":"verification.success","taskId":"007-build-output-root-guard","summary":"Requirement match 100%, scope clean, gate passed in 146s"}' \
| sh "$SKILL_DIR/scripts/emit.sh" --root "$ROOT" --trace "$TRACE" --agent "$AGENT" \
    --type VERIFICATION
```

On rejection, name the reason and — when a mechanical gate failed — the
colony-root-relative log path:

```bash
printf '%s' '{"kind":"verification.failed","taskId":"007-build-output-root-guard","summary":"the guard rejects the real bundle","reason":"checks_failed","details":".paseka/runs/trace-…/01a1d6762a9b5f46/checks.log"}' \
| sh "$SKILL_DIR/scripts/emit.sh" --root "$ROOT" --trace "$TRACE" --agent "$AGENT" \
    --type VERIFICATION
```

Publish live: the rework or approval must be dispatched as part of this run's
outcome, and a deferred decision that never flushes looks like an unanswered
review.

Good result — exactly one decision row per task, never a success and a failure
for the same review:

```bash
jq -r 'select(.payload.kind|test("^verification\\."))|[.payload.kind,(.payload.taskId//"-")]|@tsv' \
  "$ROOT/.paseka/runs/$TRACE"/*/events.ndjson | sort | uniq -c
```

```text
      1	verification.success	007-build-output-root-guard
```

A review note is not a decision: publish it in addition, never instead (see
`insight.md`). `task.completed` is the other gate's kind — see
`task-completed.md`.
