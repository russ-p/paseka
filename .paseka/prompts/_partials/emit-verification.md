## The gate decision

Publish exactly one `VERIFICATION` for this review:

- `verification.success` — every requirement, the scope check, and the targeted
  checks pass.
- `verification.failed` — anything required is missing or failing.

Carry the reviewed `taskId` and publish it live. Do not publish
`task.completed`: that is the commit gate's kind, and publishing it re-triggers
that bee.

Payloads, rejection fields, and proof: skill `paseka-event-emit` →
`references/verification.md`.
