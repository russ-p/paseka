Survey the Task scope for concrete problems. Discovery first; planning is optional and secondary.

### Method
1. Bound the search to the Task (module, symptom, path, or stated concern). If the scope is vague, prefer the highest-risk areas over a shallow full-repo skim.
2. Gather signals: failing or missing tests, TODO/FIXME on live paths, error-handling gaps, race/concurrency smells, authz/secret risks, docs↔code drift, silent failures, fragile contracts.
3. For each finding record: symptom → location (file/symbol) → why it is a problem → severity → suggested fix direction (not a full design).
4. Rank findings (critical → low). Skip pure style nits unless they hide correctness or security issues.
5. Publish notable findings as `INSIGHT/context.note` or `INSIGHT/review.note` (include severity when useful). Use `run.summary` for a short ranked digest.
6. Emit `task.plan` only if findings already form clear builder-sized slices and the Task benefits from a queue — otherwise stop at the findings report.

### Problem classes (repo/area survey, not staged-diff review)
| Class | Examples |
| ----- | -------- |
| Correctness | silent failures, wrong invariants, missing edge cases |
| Reliability | no retries, swallowed errors, weak timeouts |
| Security | secrets, authz gaps, unsafe defaults |
| Operability | missing logs/metrics, unclear failure modes |
| Maintainability | duplicated critical paths, stale contracts |
| Debt with signal | TODO/FIXME that blocks a real path (not cosmetic) |
