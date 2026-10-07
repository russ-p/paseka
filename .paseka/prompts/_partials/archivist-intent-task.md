Audit one task of this trail for work that was promised, attempted, and left behind. The task is named in the Task section; if it names none, audit every task the `TASKS` section lists, oldest first.

### Method

1. Pick the task under audit and read its specification: `{{.ColonyRoot}}/.paseka/runs/{{.TraceID}}/tasks/<taskId>/task.md`. Its `## Acceptance criteria` checkboxes are the contract. Note the ones worded as optional, hedged, or "if feasible".
2. Read every run whose `FLIGHTS.task` equals that task id, plus its `summary.md`. Work out what was actually delivered against each criterion.
3. Cross-read `DEFERRED-IN-SPEC` for the task — phrases like `out of scope`, `next slice`, `separate task` in the spec itself are scope the plan carved off before any bee ran. Those are decisions, not failures; report them as such, but they are exactly what a Beekeeper forgets.
4. Cross-read `REVIEW-NOTES` for the task. A reviewer note that names a fix and does not say it was applied is **deferred work with an author's name on it** — the highest-value finding this bee produces. The medium and low notes are not optional observations; they were written and never resolved.
5. Check `VERDICTS` for the task. `verification.failed` followed later by `verification.success` is a resolved Return Flight, not a problem. A `verification.failed` with no later success is unfinished work.
6. Check `TASKS.commit`. If the task is `completed` with no commit and `lag_merge_after_completed_s` is large, the work was never merged — say so plainly, and name how long it sat.
7. Cross-check the documentation obligation: if the task body required doc updates, verify the cited doc files were touched in the delivered diff. A spec that requires a doc change and gets a code-only diff is a silent scope failure.

### Judging

Report **deferred work**, not **wasted work**. A bee that did its slice correctly and left the rest for a later slice is healthy choreography. A bee that silently narrowed its own slice, or that rewrote a doc to justify narrowing it, is the finding.

Where a `review.note` or a `context.note` already proposes a follow-up ("worth a backlog item", "worth noting"), treat it as a **pre-drafted backlog entry**: keep its file:line citations and severity, and only add the missing `Kind` / `Why deferred` / `Revisit when` fields. Do not re-derive what the author already established.