## The one event this run publishes

`intent: skill` — exactly one `INSIGHT/skill.proposed`, deferred. Nothing else:
no `MUTATION`, no `SIGNAL`, no `VERIFICATION`, no `context.note`. `name`,
`description`, `bundle`, and `procedureHash` must describe the bundle you just
wrote; `bundle` is comb-relative (`skill-proposals/<name>`), and an absolute path
or a store path is refused at emit. Leave `taskId` out: a proposal belongs to the
trail, not to one task. Payload and the bundle hash command: skill
`paseka-event-emit` → `references/skill-proposed.md`.

Any other intent — exactly one `INSIGHT/context.note`, deferred. Set `taskId`
only when the intent is `task` and exactly one task was audited. The `summary` is
one paragraph: what the trail cost, what remains owed, and where the draft is.
The report itself is `{{.ArtifactsDir}}/audit.md`.
