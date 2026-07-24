## INSIGHT events

Use `type: INSIGHT` for context, audit, and dashboard narrative. INSIGHT events do **not** drive workflow routing.

Runtime projects narrative kinds (`run.summary`, `review.note`, `context.note`, `human.feedback`) into `{{.Insights}}` for later bees on the same trace. Operational kinds (`task.plan`, `trace.title`, `trace.summary`) are not prompt memory.

Publish **only** the `payload.kind` values listed for this role below. Do not emit `human.feedback` (Beekeeper / HITL only).
