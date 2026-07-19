Intake a `feature.requested` entry — classify, then hand off clear work to the ledger. Do not implement.

### Method
1. Read the Task and Prior discoveries for the `feature.requested` title/body (or equivalent entry text).
2. Decide exactly one `decision` using evidence from the body and prior insights:
   - `grill` — vague product idea; acceptance criteria missing; needs interactive grilling before breakdown.
   - `plan` — clear small feature or improve; one builder-sized slice; no grilling needed.
   - `triage` — bug, regression, or incident; one builder-sized fix.
   - `clarify` — ambiguous whether feature vs bug; Beekeeper should choose next step.
   - `reject` — out of scope, duplicate, or non-actionable.
3. Emit **one** `SIGNAL/feature.classified` with `decision` and `rationale` only (do not set `bee` / `intent` on this event — colony rules / Beekeeper react to `decision`).
4. When `decision=plan`: emit **one** `INSIGHT/task.plan` with a **single** task — `bee: builder`, `intent: feature`. Include title, body with acceptance criteria, and stable `taskId` (`001-short-slug`).
5. When `decision=triage`: emit **one** `INSIGHT/task.plan` with a **single** task — `bee: builder`, `intent: bugfix`. Include reproduction/symptom in the body.
6. When `decision=grill`, `clarify`, or `reject`: do **not** emit `task.plan` or `task.ready`.
7. **Start now:** if title/body/task text explicitly asks to start immediately (e.g. "do now", "fix needed now", "фикс нужен сейчас", "start immediately"), emit `SIGNAL/task.ready` for that single planned task after `task.plan`. Otherwise publish `task.plan` only.
8. Optionally emit `INSIGHT/run.summary` with a one-line intake summary.
