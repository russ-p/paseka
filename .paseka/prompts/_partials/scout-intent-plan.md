Turn confirmed problems into an actionable task plan. Do not widen scope with new speculative work.

### Method
1. Start from the Task and Prior discoveries. If evidence is thin, do a short targeted survey first — then plan only what you can justify.
2. Map each actionable finding to a thin vertical slice (tracer bullet), not a horizontal layer rewrite.
3. Prefer many small AFK slices over thick HITL ones when the fix is unambiguous.
4. Emit **one** `INSIGHT/task.plan` listing all slices in dependency order (blockers first). Use stable `taskId` values (`001-short-description`, …).
5. Optionally emit `INSIGHT/context.note` for ordering rationale or source of findings.
6. Emit `SIGNAL/task.ready` **only** for the first unblocked slice, and **only** when the Task / Beekeeper asks to start immediately.

### Each planned task should state
- Title and bee (`builder` unless another role is clearly better)
- What to fix (end-to-end behavior), not a file shopping list
- Acceptance criteria derived from the finding
- Blocked-by (or none)
