# Spec 043a: Truthful run records and closed review vocabularies

## Status

**(Approved)**
First half of the 043 split, paired with [043b](043b-trail-metrics-and-work-completion.md). The on-disk run record tells the truth about its own outcome, and the review vocabulary a reader thresholds on is closed. It is independent of the other half: nothing here needs metrics, provenance, or a work-completion milestone, so it can land alone.

Related: [046-bee-check-gate](046-bee-check-gate.md) reuses the same two-record reconciliation for its gate verdict and adds the `checks_failed` reason this spec must preserve; [040-trail-event-log](040-trail-event-log.md) is the record the contract is evaluated against, so its identity and dedupe rules define what "unreadable" means.

## Problem Statement

Two defects make a run's own record unusable as evidence.

**The run record contradicts the run.** Every AFK adapter writes `result.json` and `status.json` as soon as the CLI exits, before finalization has judged the run. The completion contract is evaluated afterwards, in the finalizer, and downgrades only the in-memory result. So a run that exited zero but never emitted a required event is `completed` on disk while the reactor's ledger and the human-visible outcome say failed. A scan over `.paseka/runs/` therefore reports success, and a Beekeeper reading the run directory cannot tell a contract miss from a crash, a timeout, or a clean run.

[Combined with what shipped][check-gate], the same finalizer already corrects a persisted adapter outcome for a mechanical gate rejection: the run reads `failed` with reason `checks_failed` on both records, and the proposal is suppressed. That proves the mechanism is sound, but it is one purpose-built path. The completion contract still does not use it, and the two can now disagree in a new way: a gate-rejected run must not have its reason rewritten by a contract verdict that runs later.

**A failed contract evaluation leaves no trace at all.** When the run's event record cannot be read, the contract is not evaluated. Nothing records that. An absent verdict is then indistinguishable from a passing one, which is the one reading a fail-closed platform must never allow.

**Free strings where a reader needs a closed set.** `review.note` severity is any string and is not validated, so a threshold like "medium or worse" means something different depending on which Bee wrote the note and how it spelled the word. `human.feedback` carries only a prose message, although its only platform emitter is the review gate's rejection — approval publishes no feedback — so a rejection cannot be counted without reading English, and a positive comment cannot be told from a complaint.

[check-gate]: 046-bee-check-gate.md

## Solution

1. **Finalization owns the persisted outcome.** The finalizer reconciles `status.json` and `result.json` with the contract verdict before a successful outcome is exposed to the caller. A contract-caused downgrade writes reason `completion_contract` and the contract message onto both records, preserving everything unrelated.
2. **The contract verdict is recorded, not just applied.** A run whose Bee declares a contract carries the verdict — passed, failed, or unavailable — with the effective declared rules and the observations or evaluation error.
3. **A gate rejection keeps its own outcome.** The gate verdict and the contract verdict are separate facts; neither overwrites the other's reason. A failed contract observation is still counted, but it cannot relabel a mechanical rejection.
4. **Conflicting records are reported, never resolved to success.** Two files are not a transaction. If only one was rewritten, or they disagree, a reader reports the conflict instead of selecting the friendlier value.
5. **The review vocabulary is closed.** `review.note` severity becomes a fixed enum with case-insensitive normalization applied at construction, not only at validation. `human.feedback` gains an optional `verdict` so a rejection says it is a rejection.
6. **Nothing else changes.** Interactive sessions stay ungated, legacy records stay readable, and no protocol version bump is required for additive fields or existing status values.

## User Stories

1. As a Beekeeper, I want a run that broke its completion contract to read `failed` in `status.json`, so that every surface agrees with the exit code.
2. As a Beekeeper, I want the reason a run was downgraded to be machine-readable, so that a contract miss is distinguishable from a crash or a timeout.
3. As a Beekeeper, I want a contract downgrade to preserve usage, process exit code, provider session identity, timestamps, artifacts, and diagnostics, so that the corrected record keeps everything the original recorded.
4. As a Beekeeper, I want a run that passed its contract to say so explicitly, so that absence is not my only evidence of success.
5. As a Beekeeper, I want a contract that could not be evaluated to read `unavailable`, so that an unreadable event record is never presented as a pass.
6. As a Beekeeper, I want a mechanical gate rejection to keep reason `checks_failed` even when the contract also fails, so that I know which layer rejected the run.
7. As a Beekeeper, I want a Bee without a contract to carry no contract field at all, so that "no declared contract" stays distinguishable from "contract passed".
8. As a Beekeeper, I want a legacy run record without a contract field to be read as unknown rather than successful, so that old trails keep their real uncertainty.
9. As a Beekeeper, I want an interrupted or conflicting pair of outcome records to be reported as such, so that a half-written finalization is visible instead of silently favouring the `completed` value.
10. As a Beekeeper, I want repeated finalization of the same run to produce the same verdict, so that a retried reconcile cannot flip a record.
11. As a Beekeeper, I want an interactive session to keep its current record, so that I do not lose cost and activity evidence to a rule written for AFK runs.
12. As a Beekeeper, I want review severities to be a fixed set, so that thresholds like "medium or worse" mean the same thing for every role.
13. As a Beekeeper, I want a severity written as `" Medium "` to be stored as `medium`, so that case and padding in a prompt do not fragment my counts.
14. As a Beekeeper, I want a new invalid severity to fail with a field-level error, so that a Bee fixes its own note instead of silently producing a value nobody can threshold.
15. As a Beekeeper, I want an absent severity to stay `unspecified` and a historical unknown value to stay `unknown`, so that I can tell "did not say" from "said something unrecognized".
16. As a Beekeeper, I want a rejection to say it is a rejection, so that feedback can be counted without reading prose.
17. As a Beekeeper, I want positive or neutral feedback not to redispatch work, so that an encouraging note does not trigger a rework round.
18. As a Beekeeper, I want verdict-less legacy feedback to keep its existing reactor behaviour, so that upgrading does not change how old notes are handled.
19. As a Beekeeper, I want a Bee to read the allowed values in its prompt, so that it emits a valid severity the first time.
20. As a maintainer, I want one finalization path to own both outcomes, so that the next runtime verdict does not need a second reconciliation mechanism.

## Implementation Decisions

### 1. Contract outcome is persisted

1. The protocol gains reason `completion_contract`, distinct from `timeout` and ordinary process errors.
2. Runtime finalization owns the final persisted outcome after completion checks, and before a successful run outcome is exposed to its caller. When a contract changes an otherwise completed run to failed, it reconciles both outcome records. Each rewrite uses a temporary file followed by a rename:
   - `status.json`: `state=failed`, `reason=completion_contract`, `error` = the contract message;
   - `result.json`: `status=failed`, `diagnostics.error`.

   Preserve all unrelated fields, including usage, provider session identity, process exit code, timestamps, artifacts, and diagnostics. A process exit code of zero can coexist with a failed runtime contract.
3. Optional `result.json` field `contract` records `status` (`passed`, `failed`, or `unavailable`), the effective declared rules, and rule observations or the evaluation error. It is present for every finalized AFK run whose Bee declares a contract. An unreadable event record is `unavailable`, never a pass; an otherwise completed adapter run with an unavailable verdict fails finalization with reason `completion_contract` and the evaluation error. Bees without a contract omit it; an older record without it proves neither the absence nor the success of a contract.
4. **The gate verdict outranks the contract verdict.** The finalizer already runs the mechanical check gate before the completion contract, and a gate rejection writes `failed` with reason `checks_failed` onto both records. The contract is still evaluated and its verdict is still recorded, including `failed`, but it must not rewrite the primary outcome or its reason after a gate rejection. The same rule holds for adapter failure, timeout, and cancellation: they retain their original status and reason even when the contract also fails. A failed contract observation is counted separately from the primary run failure reason.
5. Requiring `run.summary` is a separate runtime check and must not be mislabeled as a declared completion-contract failure.
6. Atomic replacement of each file is not a transaction across both files. A failed rewrite is surfaced as a finalization error. Readers report conflicting or incomplete records explicitly and do not select a stale `completed` value over a recorded failure. Repeating finalization preserves the same verdict and retained fields.
7. The reconciliation shape is one shared operation, not a second mechanism beside the gate record. The next runtime-owned run verdict extends it instead of writing the two files again.
8. **Backward compatibility.** These changes correct AFK run records; they do not introduce completion-contract enforcement for interactive sessions. Interactive sessions remain part of cost and activity reads, with contract evaluation unavailable unless explicitly recorded. No protocol version bump is needed for additive fields and existing status values.

### 2. Closed vocabularies

1. `review.note` `severity` ∈ `{info, low, medium, high, critical}`.
   - Canonical event construction trims and lowercases the value before persistence/publication; merely normalizing a temporary value inside validation would leave the stored payload unchanged.
   - A non-empty unknown severity is rejected through the existing `schema_validation_failed` error envelope, with a detail naming `payload.severity` and the allowed values. This spec does not introduce a second top-level validation error taxonomy.
   - An absent or whitespace-only severity stays valid and is counted as `unspecified`. Historical non-empty unknown values are counted as `unknown`; readers do not rewrite history.
   - The rejection reaches the role through `event emit`'s JSON error, and the emit how-to already tells the role to fix and retry once.
2. `human.feedback` gains optional `verdict` ∈ `{positive, negative, neutral}`, with the same trim/lowercase normalization and field-level validation. Review rejection sets `negative`. Readers count a missing/empty verdict as `unspecified` and a historical unrecognized value as `unknown`. Approval continues to use the existing approval/completion path; it is not inferred from prose or from a positive feedback count.
3. The reactor currently treats feedback on an eligible waiting-review task as a request to rework. Explicit `negative` retains that behaviour. Positive and neutral feedback are observational and do not redispatch or approve a task. Missing/empty verdict retains the existing behaviour for compatibility, but remains `unspecified` in metrics rather than being backfilled as negative.
4. Prompt examples list the allowed values. Only `review.note` severity is constrained; unrelated narrative insight kinds do not acquire a severity requirement.
5. `verification.failed.reason` — the open classifier [046](046-bee-check-gate.md) introduced with `checks_failed` as its first value — deliberately stays open. The next failing layer is not predictable, and a closed set here would force a protocol change per new gate. The closed-set rule applies to the two vocabularies a reader thresholds on, not to every classifier in the protocol.

### 3. Compatibility and rollout

1. Both primitives are additive and independent of the metrics, provenance, and completion work in [043b](043b-trail-metrics-and-work-completion.md).
2. Existing readers of `status.json` see a corrected value for new runs only; no historical record is rewritten.
3. A reader that meets a record with `contract` absent must keep reporting the outcome it already reports, with the contract evidence marked unknown rather than assumed.

## Testing Decisions

Tests assert observable persisted records, published events, and CLI output, using fake adapters and local fixtures rather than model calls.

- **Run finalization:** an adapter exits zero without a required event → both outcome records read failed, reason is `completion_contract`, contract status is `failed`, and a trail scan over the run directory reports failure. Usage, process exit code, provider session identity, and artifacts survive reconciliation.
- **Verdict dimensions:** passing, already-failed, cancelled, timeout, unreadable-event, and failed-write cases preserve their distinct semantics. An unreadable event record yields `unavailable` and fails finalization; a Bee without a contract records no contract field.
- **Gate precedence:** a gate-rejected run with a failing contract keeps reason `checks_failed`, still records `contract.status=failed`, and is counted once as a gate rejection.
- **Interrupted records:** a status/result pair that disagrees is reported as conflicting; repeating finalization produces the same verdict and fields.
- **Vocabularies and behaviour:** `" Medium "` is persisted as `medium`; an invalid new value fails with `schema_validation_failed` and a severity field detail; historical unknown and absent values remain distinguishable. Rejection emits `negative`; positive/neutral feedback does not redispatch; verdict-less legacy feedback retains its existing behaviour.

Prior art: runtime completion and dispatch tests, run record read/write and status-note tests, check-gate finalization tests, protocol validation and insight payload tests, review-gate feedback tests.

## Out of Scope

- **Trail metrics, cue provenance, and work completion** — [043b](043b-trail-metrics-and-work-completion.md) owns them.
- **Enforcing completion contracts on interactive sessions.** A human is the judge of a chat.
- **Closing `verification.failed.reason`.** It stays an open classifier.
- **A severity requirement on unrelated narrative insight kinds.**
- **Rewriting historical run records.** Corrections apply to runs finalized after this ships.
- **A second validation error taxonomy** beyond the existing `schema_validation_failed` envelope.

## Further Notes

### Relationship to 043b

The split separates what can land alone from what needs a new read surface. This half repairs records that already exist and closes two vocabularies; it needs no new event kind, no new CLI command, and no new persistence. [043b](043b-trail-metrics-and-work-completion.md) builds the trail metrics read surface and the work-completion milestone, and it consumes the truthful records this half produces — a metrics reader that counted the old `completed` value would overstate success.

### Why the gate verdict wins

A run can fail mechanically and also miss its contract. Both are true, but only one can be the primary reason in a two-field record. The gate is the first decisive verdict the runtime reaches, it already writes both records, and the rework routing keys on `checks_failed`; letting a later contract evaluation replace that reason would silently change which repair loop runs. Recording the contract verdict separately keeps both facts without making the primary outcome ambiguous.

### Why reason strings are not all closed

The closed-set rule exists where a human threshold must mean the same thing across roles: review severity and feedback verdict. A failure classifier answers "which layer refused", and that set grows with each new layer — closing it early would mean a protocol revision for every future gate, and would tempt implementations to map a new cause onto an old name. Open classifier, closed vocabulary where thresholds live.
