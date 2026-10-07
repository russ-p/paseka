Audit how this trail *ran* — not what it produced. Find Return Flights, Bees Flying in Circles, and honey spent without progress. Then say which of your findings should become a **skill**: a repeatable procedure this colony now knows it needs, written down where it will be reused.

### Method

1. From `FLIGHTS`, count runs per bee and per task. Two or more runs of the same bee on the same task is a **Return Flight**. Report each one with both durations.
2. Separate **productive** from **circling** Return Flights. A builder→guard→builder→guard alternation on one task that ends in `verification.success` is the guard loop working. The same shape that never reaches success, or that reaches success with the guard raising the same finding twice, is Bees Flying in Circles — and that is a prompt or contract defect, not a bee defect.
3. Compare the first run of a bee against its Return Flights. If the first attempt took several times the correction (the usual pattern is a large first run followed by a short rework), the cost is in the first pass, not the rework. Say that, because it changes what to fix.
4. From `TASKS`, check whether every `completed` task actually reached `main`. A `completed` task with an empty `commit` and a non-trivial `lag_merge_after_completed_s` is a finding on its own: the board read clean while the work was not merged.
5. From `FLIGHTS`, compare `adapter` and `profile` against what the colony declares for that bee. A run executed under a different profile or adapter than the bee YAML names is a reproducibility problem worth naming.
6. Compute `lag_merge_after_completed_s` in minutes and state whether the merge was human-paced or instant. A large value with zero intermediate events means **nothing in the system noticed the trail was sitting finished** — that is the finding, not the delay itself.

### Judging

Report ratios and counts, not adjectives: `2 of 6 runs were Return Flights, 9m44s of 63m42s wall clock` beats `the guard was thorough`.

### Proposing skills

A finding becomes a skill candidate only when it satisfies **all three**:

1. It is **recurring** — the same shape could happen on the next trail, not just this one.
2. It is **procedural** — the answer is a sequence of steps or a rule to follow, not a fact to record.
3. It is **not already written down** — check `docs/` and `.agents/skills/` before proposing. A finding that duplicates an existing spec, guide, or skill is a *pointer*, not a skill.

When all three hold, draft the skill as `name`, one-line `description` with trigger conditions, and an ordered procedure with the evidence from this trail cited inline. Where a finding fails test 1 or 2, report it as a one-off observation in the audit draft and do not propose a skill.