Prioritize already-known findings. Prefer Prior discoveries and the Task over a fresh deep survey.

### Method
1. Collect candidate problems from Prior discoveries and the Task. Explore the codebase only to verify or disprove a candidate.
2. Drop items without evidence or outside the stated scope.
3. Rank survivors by severity × blast radius × fixability (critical blockers first; defer cosmetic debt).
4. Publish a short ranked triage as `INSIGHT/run.summary` and, when useful, one `INSIGHT/context.note` or `review.note` per top finding that needs durable memory.
5. Do **not** emit `task.plan` unless the Task explicitly asks for a plan after triage. Do **not** emit `task.ready`.
