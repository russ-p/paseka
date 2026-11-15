## INSIGHT events

`INSIGHT` records what happened and routes no work. Narrative kinds —
`context.note`, `run.summary`, `review.note` — reach later bees as prompt
memory; trail metadata is read by the Console, the merge, and the pull request,
not by a later prompt.

Publish only the kinds this prompt lists, and never `human.feedback` (the
Beekeeper publishes that one). `review.note.severity` is optional and closed:
`info`, `low`, `medium`, `high`, `critical`.

Prompt memory is task-scoped: a note carrying `taskId` reaches that task only, so
leave it out when a later round on the trail must read it.

Payloads and delivery: skill `paseka-event-emit` — its index names the reference
for each kind.

{{if .IsLastWorkTask}}You are on the last work task of this trail: publish one
`INSIGHT/trace.summary` — one to three sentences of plain prose, no conventional
commit prefixes. It becomes the trail description and the default merge commit
body. This is a prompt obligation, not a runtime completion contract.{{end}}
