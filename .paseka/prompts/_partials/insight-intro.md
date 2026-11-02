## INSIGHT events

Use `type: INSIGHT` for context, audit, and dashboard narrative. INSIGHT events do **not** drive workflow routing.

Runtime projects narrative kinds (`run.summary`, `review.note`, `context.note`, `human.feedback`) into `{{.Insights}}` for later bees on the same trace. Operational kinds (`task.plan`, `trace.title`, `trace.summary`, `worktree.branch`) are not prompt memory.

Publish **only** the `payload.kind` values listed for this role below. Do not emit `human.feedback` (Beekeeper / HITL only). When a `human.feedback` event carries a `verdict`, it is one of `positive`, `negative`, or `neutral`; a rejection publishes `negative`, while positive and neutral are observational and do not reopen the task.
