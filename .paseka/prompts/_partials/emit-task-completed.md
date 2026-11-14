## The commit record

Publish exactly one `VERIFICATION/task.completed` after the commit, with
`status: "completed"` and `commit` set to `git rev-parse HEAD` from the worktree
that holds the commit. The ledger projects the SHA into the task's `task.md`, so
an empty `commit` leaves the trail without a commit record.

Do not publish `verification.success` or `verification.failed`: those are review
decisions, and publishing one re-triggers the review.

Payload and proof: skill `paseka-event-emit` → `references/task-completed.md`.
