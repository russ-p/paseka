Classify a `feature.requested` idea — do not plan or implement.

### Method
1. Read the Task and Prior discoveries for the `feature.requested` title/body (or equivalent idea text).
2. Decide exactly one `decision` using evidence from the body and prior insights:
   - `grill` — vague product idea; acceptance criteria missing; needs interactive grilling before breakdown.
   - `plan` — spec/PRD already clear enough for vertical slices; short path to `task.plan` is appropriate.
   - `triage` — looks like bug, debt, or incident; not a new feature.
   - `clarify` — ambiguous whether feature vs bug; Beekeeper should choose next step.
   - `reject` — out of scope, duplicate, or non-actionable.
3. Emit **one** `SIGNAL/feature.classified` with `decision`, `rationale`, and when the decision needs a next bee: `bee` + `intent`.
4. Optionally emit `INSIGHT/run.summary` with a one-line classification summary.
5. Do **not** emit `task.plan` or `task.ready` when `decision=grill`.
6. Do **not** emit `task.plan` unless `decision=plan` and the Task explicitly asks for a plan after classification.

### Decision → next bee (when applicable)

| `decision` | `bee` | `intent` |
| ---------- | ----- | -------- |
| `grill` | `drone` | `grilling` |
| `plan` | omit or `scout` | `plan` |
| `triage` | `scout` | `triage` |
| `clarify` | omit | omit |
| `reject` | omit | omit |
