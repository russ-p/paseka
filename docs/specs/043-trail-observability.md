# Spec 043: Trail observability

## Status

**(Draft)**
Five related runtime changes make a trail readable by code: its recorded cost, completion-contract verdicts, cue provenance, work-completion milestone, and review severity. Work completion means the working tasks and mandatory checks have passed, before final human approval and delivery; honey exhaustion alone is not completion. Analysis stays in the same trail, is accounted separately, and can run alongside final review without blocking delivery. Retries do not depend on a literal Bee role name. V1 records initial honey allocation and every cue ingress, including signal cues. The broader delivery outcome protocol remains a separate design question in Further Notes.

Related: [040-trail-event-log](040-trail-event-log.md) is the trail record these primitives read; [041-skills-layer](041-skills-layer.md) Phase 2 adds the attribution half; [042-trail-fork](042-trail-fork.md) consumes the metrics to keep a fork comparable to its source; [003-hive-evals](003-hive-evals.md) reads them to score a case. Context: [idea: self-improving colonies](../idea/self-improving-colony.md).

## Problem Statement

A Beekeeper — or a role working for one — who wants to know whether the colony is getting better has to answer simple questions: how much did this trail cost, how many times was the same task rebuilt, did every run do what its contract said, and which trails were worth studying. Today every one of those answers needs a hand-written collector over `.paseka/runs/`. Some of the answers on disk are wrong.

**The run record contradicts the run.**

- The cursor adapter writes `result.json` and `status.json` when the CLI exits (`internal/adapters/cursor/cursor.go:202-206`; the same pattern holds in the script, claude, pi and opencode adapters).
- The completion contract is checked afterwards, in finalize (`internal/runtime/dispatch_stages.go:383-385`). It downgrades only the in-memory result (`internal/runtime/completion.go:11-23`).
- So a run that broke its contract is `completed` on disk. The CLI's exit code and the reactor's ledger say `failed`. Trail scans built on the on-disk state (`internal/runs/trace_scan.go:341-343`) miss it too.

**A trail does not know where it came from.** `paseka cue run` builds the task from the cue but drops the cue's id. `TaskSpec` carries `title/body/bee/intent/review` only (`internal/cues/run.go:149-156`, `internal/protocol/task.go:55-64`). Two trails started by `feature` and by `hotfix` with the same bee and intent cannot be told apart afterwards. So "is this trail expensive for its kind" has no kind to compare against.

**There is no trail-level metrics surface.**

- `paseka inspect usage --trace` aggregates tokens (`cmd/paseka/inspect.go:25-66`, `internal/runs/trace_scan.go:25-35`).
- `paseka export --include usage,durations` renders a human report (`cmd/paseka/export.go:55`).
- Neither gives the honey spent (Σ `energy.consume`, `internal/runtime/energy.go:40-49`), the dispatches per task, the verification failures, or a contract outcome as one machine-readable object.

**There is no trail-level work-completion moment before delivery.** Per-task completion does not identify when the whole working result is ready for final approval. Requiring every task to be terminal is too late: the final `_review` gate is still waiting before merge, and PR delivery leaves it waiting even after a PR is published. Analysis needs a milestone after work and its mandatory checks pass, independently of those later acceptance and delivery steps. See [task ledger](../reference/task-ledger.md) and [pull-request delivery](../guide/pull-request-delivery.md).

**Free strings where a reader needs a closed set.**

- `review.note` severity is any string (`internal/protocol/insight.go:36-41`) and is not validated.
- `human.feedback` carries only a message (`internal/protocol/insight.go:44-49`), although its only platform emitter is the review gate's rejection (`internal/review/gate.go:180-207`; `Approve` publishes no feedback).

## Solution

Five primitives, exposed through existing run/event records and a new trail metrics read surface:

1. **Truthful run records.** The runtime reconciles `status.json` and `result.json` with the final completion-contract verdict, using reason `completion_contract` for a contract-caused downgrade. Legacy `meta.json` has no state field and is not a third outcome record.
2. **Cue provenance.** Every cue invocation has a recorded ingress identity, including signal cues. Trail origin distinguishes the initial ingress from later additions. Task specifications/projections retain their creating cue's identity where applicable.
3. **Trail metrics.** `paseka inspect metrics --trace <id> [--json]` projects one trail into a versioned `TrailMetrics` object. The runtime records the initial honey allocation needed for offline reads, and persists metrics with each work-completion checkpoint.
4. **Closed vocabularies.** `review.note` severity becomes an enum with case-insensitive normalization; `human.feedback` gains an optional `verdict`.
5. **Work completed.** The reactor publishes `SIGNAL/trail.work.completed` once the trail's working tasks and mandatory checks have passed and their finalization has drained. It captures immutable work metrics before admitting analysis reactions. Final human approval, merge, and PR publication do not gate this milestone.

## User Stories

1. As a Beekeeper, I want a run that broke its completion contract to read `failed` in `status.json`, so that every surface agrees with the exit code.
2. As a Beekeeper, I want the reason a run was downgraded to be machine-readable, so that a contract miss is distinguishable from a crash or a timeout.
3. As a Beekeeper, I want a trail started by a cue to remember which cue, so that I can compare trails of the same kind.
4. As a Beekeeper, I want one command to print a trail's cost and rework counts as JSON, so that I do not maintain a jq collector.
5. As a Beekeeper, I want work metrics written when the trail's result becomes ready for final approval, so that a reader reads one snapshot instead of every run directory.
6. As a Beekeeper, I want metrics to report a missing measurement as missing, so that a trail without usage is not read as a free trail.
7. As a Beekeeper, I want review severities to be a fixed set, so that thresholds like "medium or worse" mean the same thing for every role.
8. As a Beekeeper, I want a rejection to say it is a rejection, so that feedback can be counted without reading prose.
9. As a Beekeeper, I want an observation when the trail's work is ready before delivery, so that analysis can start without waiting for a merge.
10. As a Beekeeper, I want a damaged or incomplete trail record to expose its measurement gaps, so that retained counts are not mistaken for exact totals.
11. As a Beekeeper, I want parallel runs and time spent waiting for human review to have explicit duration semantics, so that I can interpret the trail's timing.
12. As a Beekeeper, I want an interactive session's recorded usage included once, so that switching from AFK to HITL does not hide or double cost.
13. As a Beekeeper, I want a historical work-completion event to identify the result and metrics it announced, so that reopening the trail does not silently change that event's evidence.
14. As a Beekeeper, I want duplicate delivery and Reactor restarts to preserve a completion milestone's identity, so that a reader can handle the observation once.
15. As a Beekeeper, I want metrics to expose configuration changes and fork lineage, so that an aggregate from several procedures is not presented as one controlled experiment.
16. As a Beekeeper, I want analysis to stay in the original trail while its cost and failures remain separately visible, so that the work is not made to look worse or more expensive by studying it.
17. As a Beekeeper, I want analysis completion to leave the work-completion observation unchanged, so that a retrospective does not recursively trigger itself.
18. As a Beekeeper, I want execution and retry counts for custom task roles, so that renaming a Bee does not change the meaning of the metrics.
19. As a Beekeeper, I want analysis to continue from retained evidence if delivery removes the worktree, so that studying the result does not delay merge or PR publication.
20. As a Beekeeper, I want a paused or failed trail distinguished from completed work, so that exhausted honey is never presented as achieving the goal.
21. As a Beekeeper, I want a new trail's initial honey reserve recorded before work spends it, so that offline metrics preserve cue overrides and experimental budgets.
22. As a Beekeeper, I want signal cues and later cue invocations recorded separately from the original ingress, so that adding work does not rewrite where a trail came from.

## Implementation Decisions

### 1. Contract outcome is persisted

1. The protocol gains reason `completion_contract`, distinct from `timeout` and ordinary process errors.
2. Runtime finalization owns the final persisted outcome after completion checks, before a successful run outcome is exposed to its caller. When a contract changes an otherwise completed run to failed, it reconciles both outcome records. Each rewrite uses a temporary file followed by rename:
   - `status.json`: `state=failed`, `reason=completion_contract`, `error` = the contract message;
   - `result.json`: `status=failed`, `diagnostics.error`;

   Preserve all unrelated fields, including usage, provider session identity, process exit code, timestamps, artifacts, and diagnostics. A process exit code of zero can coexist with a failed runtime contract.
3. Optional `result.json` field `contract` records `status` (`passed`, `failed`, or `unavailable`), the effective declared rules, and rule observations or the evaluation error. It is present for every finalized AFK run whose Bee declares a contract. An unreadable event record is `unavailable`, never a pass; an otherwise completed adapter run then fails runtime finalization with reason `completion_contract` and the evaluation error. Bees without a contract omit it; an older record without it does not prove either the absence or success of a contract.
4. Adapter failure, timeout, and cancellation retain their original status/reason even if the contract also fails. A failed contract observation is counted separately from the primary run failure reason. Requiring `run.summary` is a separate runtime check and must not be mislabeled as a declared completion-contract failure.
5. Atomic replacement of each file is not a transaction across both files. A failed rewrite is surfaced as a finalization error; readers report conflicting or incomplete records explicitly and do not select a stale `completed` value over a recorded failure. Repeating finalization preserves the same verdict and retained fields.
6. **Backward compatibility.** These changes correct AFK run records; they do not introduce completion-contract enforcement for interactive sessions. Interactive sessions remain part of cost and activity reads, with contract evaluation unavailable unless explicitly recorded. No protocol version bump is needed for additive fields and existing status values; metrics have their own schema version.

### 2. Cue ingress and trail origin

1. Every validated cue invocation records an `invocationId`, `cueId`, ingress source/channel, emission mode, invocation time, and the identities of its normal ingress events and created tasks, if any. This applies to both `emit: task` and `emit: signal`, fresh traces, explicit existing traces, and standing ticks. Source cue identity is technical provenance, independent of Bee and intent names.
2. The cue subsystem writes a `SIGNAL/cue.ingress` observation through the canonical trail publisher before publishing that invocation's normal task/signal events. A repeated publication uses the same invocation/event identities; a genuinely new cue invocation receives a new identity. The observation records ingress intent, not a guarantee that every associated event reached the broker (spec 040). It does not itself create tasks, seed honey, or dispatch a Bee; normal ingress retains those responsibilities. It is excluded from AFK direct routing as a platform observation.
3. New trail creation/admission retains its initial ingress identity exactly once in a durable trail-owned origin record. All creation paths participate: a cue origin names its invocation, while manual or other origins do not invent a cue. Concurrent creation cannot select two origins. Adding a cue to an existing trail never replaces the origin. Source timestamps alone or the earliest task are insufficient to determine this identity.
4. Top-level metrics `cueId` names the initial cue only when the retained origin proves it; `cueIngresses` lists all distinct recorded cue invocations. A later cue on a manually created trail appears in that list but leaves the original cue field empty. Missing legacy origin remains unknown even when a later cue invocation is readable; do not label it initial merely because it is the first retained cue.
5. Task specifications, ledger snapshots, and task projections gain optional `cueId` and `cueInvocationId` for the cue that directly created the task. Role/manual task creation does not invent these fields. Signal ingress keeps its cue identity in the invocation record even though no task was created directly. Associated normal ingress events retain their invocation linkage without depending on a custom signal payload having a particular business schema.
6. Older events/tasks without provenance still decode. The durable origin and invocation records are retained with the trail and survive worktree-only cleanup. Cue definitions can change later; readers use recorded identities and available configuration evidence rather than today's cue contents.

### 3. Trail metrics

1. The run-record reader exposes a trail metrics projection from colony root and trace identity. It reads only retained disk evidence, so it works without NATS:
   - `trail-events.ndjson` ([spec 040](040-trail-event-log.md));
   - run directories;
   - `tasks/*/task.md`;
   - proposal records ([spec 039](039-proposal-patch-records.md));
   - the retained trail origin, seed observations, and work-completion evidence introduced here.
2. `TrailMetrics` uses `schemaVersion: 1`. The following is a field contract, not an assertion that all historical trails contain complete evidence:

   | Field | Meaning |
   | --- | --- |
   | `traceId`, `observedAt` | Trail identity and the projection's explicit observation time. |
   | `cueId`, `cueIngresses` | Optional initial cue identity, plus all distinct recorded cue invocations with origin/addition attribution. Coverage distinguishes known non-cue origin from unknown historical origin; neither invents a `manual` cue name. |
   | `class` | Entry-task descriptor (`taskId`, recorded `bee`, `intent`) plus total task count. It is `null` when no entry task can be identified. |
   | `work` | Recorded work state (`open`, `blocked`, `completed`, `failed`, `cancelled`, or `null` when unknown), work revision, and the current completion's identity/time/result reference when present. Final acceptance and delivery are separate facts. An old completion may remain in history without describing the current work revision. |
   | `forkOf` | When spec 042 evidence exists, an object naming source trace, task, selected attempt/run, source configuration reference and revision. Values come from the retained fork record, not the source's current state. Otherwise `null`, with lineage coverage distinguishing known ordinary trails from unavailable provenance. |
   | `configuration` | Optional reference to retained configuration history, initial/active references, the distinct references actually used by recorded executions, and an explicit `multipleVersions` flag. Legacy unknown execution versions remain unknown. Spec 042 supplies this evidence; it is not a prerequisite for basic metrics. |
   | `honey` | Initial recorded `budget`, `spent`, and `byReason`. `spent` sums distinct recorded `energy.consume` amounts, not run directories. Newly seeded trails retain the effective budget; missing historical seed evidence yields `null`. |
   | `tokens` | Recorded input/output/cache-read/cache-write sums, `runsWithUsage`, and total `runs`; `null` when no run reports usage. Partial reporting is explicit. |
   | `runs` | Retained execution count, counts by Bee, `failed`, `cancelled`, `contractFailed`, `contractUnavailable`, and `timeout`. |
   | `tasks` | Stable task-id ordered task observations, with recorded purpose/source completion, status, `executions`, `retries`, all joined run identities, and verification failures. Executions count the task's assigned work role, independent of its name. |
   | `byPurpose` | Separate work and analysis measurements for honey, tokens, runs, failures, retries, verification/review observations, and execution time. Root aggregates remain total observed trail activity. |
   | `verificationFailed` | Count of distinct recorded `VERIFICATION/verification.failed` events, including events not joined to a task. |
   | `reviewNotes` | Counts by `info`, `low`, `medium`, `high`, `critical`, `unspecified`, and historical `unknown`. |
   | `humanFeedback` | Counts by `negative`, `positive`, `neutral`, `unspecified`, and historical `unknown`; these are feedback events, not all human review decisions. |
   | `activeTimeSec` | Sum of recorded execution intervals; overlapping runs add, so this may exceed wall time. |
   | `wallTimeSec` | For a live total projection, elapsed time from the earliest retained ingress/task/execution evidence to `observedAt`. For a work-completion snapshot, elapsed time to `completedAt`; includes working review waits and honey pauses, but excludes later final approval, delivery, and analysis. |
   | `coverage`, `warnings` | Completeness (`complete`, `partial`, `unavailable`) by measurement group and concrete reasons for missing, unreadable, conflicting, or legacy evidence. A non-null partial total describes retained observations only. |

   **Projection rules:**

   - Use the common trail-scoped merged event reader and deduplicate by event identity before counting. The log records publication intent before the broker attempt (spec 040); an event count is not proof of broker delivery or successful ledger application. Accepted task state comes from the runtime's task projection/lifecycle evidence, not blindly replaying every recorded publication.
   - The entry-task descriptor is selected deterministically from known tasks by earliest `createdAt`, then `taskId`. This is a descriptor, not proof of a unique causal root; multi-task intake can have several roots. Unknown creation time makes classification partial. Do not consult current colony defaults to fill historical Bee or intent values.
   - Count each retained execution once. AFK usage comes from the run result; interactive usage may come from the session record. If both represent the same execution, choose one source rather than summing both. Missing usage differs from reported zeros, and an explicitly unavailable usage source is not a measurement.
   - `request.json` supplies task-to-run joins for both task and direct dispatch; task `runs.ndjson` alone misses direct-dispatch runs. An unjoinable execution remains in trail totals with a warning. Reuse of an execution directory may have overwritten earlier results: retained execution counts cannot claim to count every historical dispatch. Honey charges and execution counts need not match.
   - `runs.failed` counts primary failed status; timeout is normally a cancelled run with reason `timeout`. `contractFailed` and `contractUnavailable` count explicit verdicts, including for an already-failed adapter run. These dimensions may overlap and must not be added together as a total failure count. Missing legacy contract evidence affects coverage, not the explicit-verdict counters; it proves neither a successful contract nor an evaluation failure.
   - Per-task `executions` counts invocations performing the task's assigned work role, using execution-time ownership rather than a literal `builder` name or today's registry. Guard and other associated roles remain in total joined runs but do not become task retries merely because they share a `taskId`. Direct-dispatch rework and HITL execution count when they perform the assigned work role. `retries = max(0, executions - 1)` includes repeats after crashes, operator retries, and returns after verification; it does not claim to count code rework alone. Missing historical ownership makes this measurement partial/unavailable rather than authorizing a guess. Spec 042's task-attempt identity is distinct from an invocation count.
   - Per-task verification failures join on recorded `taskId`. Their sum may be smaller than the trail total because taskless verifications remain visible. Verification failures are independent observations; they are not converted into retries. There is no `builderRuns` or `returnFlights` field in v1.
   - Work-completion comparisons use `byPurpose.work` from the frozen snapshot, not its root total aggregates. This baseline includes cumulative work on the goal through that revision, including earlier failed work executions, but excludes analysis of every revision. Live total aggregates may continue growing as analysis runs in the same trail. Analysis stays in the total Honey Reserve and all trail records; it is not a free dispatch and cannot bypass honey exhaustion or kill. Work revision identifies accepted goal work and is distinct from spec 042's configuration revision.
   - Initial honey reserve is currently seeded directly in the ledger. V1 adds a durable `SIGNAL/energy.seeded` observation with the winning seed identity, effective budget, and actual seed time. The authoritative seed transition retains this observation's identity/data so a crash between ledger commit and local publication can be repaired without reseeding or choosing a new event identity. It is captured before the first charged dispatch, through every seed path: colony defaults, cue override, standing first seed, session entry, and experimental budget.
   - `energy.seeded` observes an already accepted allocation; receiving/replaying it must not seed/add/consume again or dispatch a Bee. A competing seed request observes the existing winner rather than logging its own proposed budget. Pending observation publication retries use that winner. Legacy ledger entries lacking seed provenance are not assigned an invented historical timestamp or configuration. Offline readers never infer a missing budget from current colony/cue defaults.
   - Top-ups and standing stipends retain their existing semantics and do not replace the original seed observation or its `budget`. A budget is not total lifetime expenditure. Per-purpose spending divides charges, not the shared budget allocation; analysis and work use one recorded reserve.
   - Durations use recorded timestamps with non-negative intervals and a single explicit `observedAt`. Running executions use `observedAt` as their temporary end. Missing or invalid timestamps make the corresponding duration partial or unavailable; no usable intervals yields `null`, not zero. Observation/completion events and metrics-file modification times are excluded from the start/end calculation.
   - Empty, missing, or malformed sources do not silently become exact zero counts. A missing trail is a CLI error. A readable partial trail returns observed values with coverage/warnings. Live reads are observations that may race updates; only the runtime's work-completion barrier can certify a completion snapshot.
3. CLI: `paseka inspect metrics --trace <id> [--json] [--write] [-C dir]`. This follows `inspect usage`'s flag style. Without `--json` it prints a short table and measurement warnings. The default read does not write files or publish events; `--write` persists the same projection that the invocation prints and does not itself declare work completed.
4. **Persistence.** `.paseka/runs/<traceId>/metrics.json`, written atomically:
   - by the reactor before announcing `trail.work.completed` (primitive 5), alongside an immutable snapshot for that completion;
   - by `inspect metrics --write`.

   A completion's `metricsRef` is a colony-root-relative reference to its immutable trail-owned snapshot; `metrics.json` is a replaceable convenience projection and is not the target of historical completion references. Readers exclude these record directories from execution counts. Retention targeting run data removes the projections and snapshots with the trail; worktree-only pruning leaves them readable. A long-term history beyond retained trail data belongs to the reader (e.g. a methodology's own store).

### 4. Closed vocabularies

1. `review.note` `severity` ∈ `{info, low, medium, high, critical}`.
   - Canonical event construction trims and lowercases the value before persistence/publication; merely normalizing a temporary value inside validation would leave the stored payload unchanged.
   - A non-empty unknown severity is rejected through the existing `schema_validation_failed` error envelope, with a detail naming `payload.severity` and the allowed values. This spec does not introduce a second top-level validation error taxonomy.
   - An absent or whitespace-only severity stays valid and is counted as `unspecified`. Historical non-empty unknown values are counted as `unknown`; readers do not rewrite history.
   - The rejection reaches the role through `event emit`'s JSON error, and the emit how-to already tells the role to fix and retry once.
2. `human.feedback` gains optional `verdict` ∈ `{positive, negative, neutral}`, with the same trim/lowercase normalization and field-level validation. Review rejection sets `negative`. Readers count a missing/empty verdict as `unspecified` and a historical unrecognized value as `unknown`. Approval continues to use the existing approval/completion path; it is not inferred from prose or from a positive feedback count.
3. The reactor currently treats feedback on an eligible waiting-review task as a request to rework. Explicit `negative` retains that behaviour. Positive and neutral feedback are observational and do not redispatch or approve a task. Missing/empty verdict retains the existing behaviour for compatibility, but remains `unspecified` in metrics rather than being backfilled as negative.
4. Prompt examples list the allowed values. Only `review.note` severity is constrained; unrelated narrative insight kinds do not acquire a severity requirement.

### 5. `trail.work.completed`: before final approval and delivery

1. A new `SIGNAL` kind `trail.work.completed` carries `completionId`, `workRevision`, `completedAt`, `resultRef`, and `metricsRef`; the envelope identifies the trail. It announces that the accepted working task set and its mandatory checks have passed. It is not a general idle notification or proof of human acceptance. This replaces the draft-only `trail.settled` design; no compatibility alias is required for an unshipped event.
2. A non-empty working task set must have completed its work and mandatory checks. Review records explicitly distinguish work-enabling review from final goal acceptance; `waiting_review` alone is insufficient to make that distinction. The isolated final gate (`review: final`, commonly `_review`) is final acceptance and may still be waiting. Ordinary task-level `review: required` remains a work gate when further substantive work depends on it. If a root proposal's R1 soft-ack is the final goal acceptance, the review subsystem must record the completed work/check evidence separately from that pending ack; root proposals cannot be given the isolated-only `review: final` policy merely to make this predicate work. Unknown gate purpose is not silently excluded.
3. The predicate also covers a result with no isolated code changes, for which the final delivery gate is absent or skipped. A taskless/event-only trail is not complete by vacuous truth; automatic completion for such a goal needs a separate contract. Standing-trail ticks do not automatically complete the standing identity in v1.
4. The runtime evaluates after working task transitions, mandatory verification, work direct-dispatch completion, registered work-session completion, and run finalization. A hook in `applyAndSync` alone misses paths and runs before the last dispatcher leaves its in-flight set. The barrier serializes with new work admission and requires working invocations, pending work dispatches, and required work finalization effects to have drained. Persisted results, task projections, usage, and result evidence must be ready. Final approval, PR review, analysis runs, and delivery reconciliation are not outstanding work for this predicate.
5. Snapshot and result references resolve to immutable trail-owned evidence for that completion. For a code result, retain the relevant base/revision and final patch or equivalent retained tree evidence before delivery can remove the execution worktree; retain the non-code result artifacts needed to inspect other outcomes. Analysis must not depend on a mutable live worktree or mutable handoff file as its only input. Required evidence failing to persist is a visible finalization error, not a successfully announced completion.
6. The checkpoint, completion identity, and snapshot are durable before event publication and before delivery is allowed to remove their source evidence. Completion publication failure is retryable with the same identity; it does not require analysis to run before delivery. A fast final-approval action must ensure that the corresponding checkpoint is captured before its delivery side effects. Delivery and analysis then proceed independently under existing permissions.
7. Work blocked by honey, failed work, cancellation, or a failed dependency's still-planned descendants do not emit this successful-completion signal. Metrics remain inspectable for those states. `energy add` only makes blocked work runnable; it does not assert goal completion. No general paused/failed/cancelled notification protocol is introduced here.
8. Durable `completionId` survives Reactor restart, duplicate events, and publish retry. Bus delivery is at least once; consumers deduplicate by completion identity. New accepted substantive work, including requested changes at final review, reopens work and advances its revision. A later completion receives a new identity. Final approval, PR publication/merge, repeated reads, analysis activity, and late narrative notes do not themselves create a new work revision. An older completion remains historical when new work is accepted.
9. Read-only inspection never emits the milestone; `--write` does not certify completion. The runtime records completion for accepted work at that boundary and cannot promise that a delayed external request will never arrive. This event alone does not establish semantic success beyond the task/check contracts recorded for that goal.
10. Work completion, final acceptance, analysis, and delivery are separate observations. Existing `local_merge`, PR publication/reconciliation, root soft-ack, and experimental no-delivery behaviour remain the delivery mechanisms. In particular, publishing a PR is observable before its merge, and spec 042's experiment can finish its work without either. The full delivery outcome vocabulary and notification contract remain D6; they are not overloaded onto this work milestone.

### 6. Analysis in the original trail

1. Execution admission records purpose `work` or `analysis`. An explicitly configured analysis reaction or operator-started analysis action records the work milestone it studies (`analysisOf`); the source trace remains unchanged. Purpose is runtime-owned provenance, not inferred from the Bee's name or free-form output. Ordinary work defaults to `work`; unsupported historical attribution stays unknown and cannot be silently excluded as analysis.
2. Analysis-owned tasks, direct dispatches, sessions, and emitted events retain their purpose and source milestone through their causal chain. Metrics expose work, analysis, and total activity separately. A failing analysis run is visible as an analysis failure; it does not retrospectively turn successful work into failed work.
3. Analysis activity and its narrative events do not reopen the work episode, satisfy or block work-task completion, or generate another work-completion milestone. Final-gate activation and change-request eligibility must evaluate the working task set rather than treating analysis tasks as unfinished product work. Reader dispatch is deduplicated by source milestone and analysis subscription/action identity, including after restart. Explicitly retrying failed analysis remains possible and is counted within analysis.
4. Findings can recommend additional work, but accepting substantive rework is an explicit normal-work admission. That reopens the work episode, creates new work evidence, and can later produce a new completion milestone. Analysis attribution must not silently exempt implementation changes from work accounting. Analysis inspects the captured result and does not mutate the candidate being reviewed; changing that result is ordinary work subject to its checks.
5. Analysis can run alongside final review and does not block merge or PR publication. Its launch workspace must be independent of the removable delivery worktree, while its run/audit records remain in the same trace. It reads the captured evidence even if delivery already removed the original worktree. Existing configuration provenance, Honey Reserve, and kill controls still apply. Analysis has no implicit merge, push, or publication permission; adding a separate mandatory-analysis policy is out of scope for v1.

## Testing Decisions

Tests assert observable persisted outcomes, published events, and CLI output, using fake adapters and local fixtures rather than model calls. Work completion before final approval, non-blocking same-trail analysis, role-independent retries, durable initial allocation, and complete cue-ingress provenance are selected. The broader delivery protocol D6 is not an implementation prerequisite for these behaviours.

- **Run finalization:** an adapter exits zero without a required event → both outcome records read failed, reason is `completion_contract`, contract status is `failed`, and the trail scan reports failure. Passing, already-failed, cancelled, timeout, unreadable-event, and failed-write cases preserve their distinct semantics. Usage, process exit code, provider session identity, and artifacts survive reconciliation. Readers expose interrupted/conflicting records instead of reporting a stale success.
- **Provenance:** task cues retain cue/invocation identity in the ledger and task projection; signal cues retain it without needing a task. Repeated normal events for one invocation do not duplicate ingress counts. Later cues on an existing manual trail leave origin unchanged; concurrent creation converges on one origin. Multi-task plans with equal timestamps do not choose origin by task ordering. Legacy unknown origins and old events remain readable without invented provenance.
- **Metrics:** fixtures cover rework, adapter retry, a custom task role, honey block and top-up, parallel runs, human review waits, interactive usage, absent usage, reported zero usage, missing seed evidence, taskless verification, and overwritten run evidence. Counts deduplicate an event recorded twice or present in both event stores. Timeout and contract dimensions do not incorrectly inflate primary failed totals.
- **Read/write contract:** for one invocation and observation time, printed JSON and `--write` output agree. Default reads change no files or bus state. Missing trail is an error; partial/corrupt records expose coverage and warnings. Worktree-only cleanup leaves metrics and retained provenance readable.
- **Vocabularies and behaviour:** `" Medium "` is persisted as `medium`; an invalid new value fails with `schema_validation_failed` and a severity field detail; historical unknown and absent values remain distinguishable. Rejection emits `negative`; positive/neutral feedback does not redispatch; verdict-less legacy feedback retains its existing behaviour.
- **Work completion:** the last working task completes while its dispatcher is still in flight → no premature event; leaving the in-flight set triggers the check. A waiting final `_review` permits the event, while unfinished required work review does not. A root R1 final acceptance uses explicit work/check evidence without an invalid isolated final-review policy. Unknown gate purpose is not guessed. No-code/no-delivery work also completes. Honey exhaustion, task failure, cancellation, standing ticks, and empty ledgers do not emit a success milestone. New accepted work reopens the revision; publication retry/restart retains its completion identity. Old events still resolve to their original result and metrics. Final approval, PR publication, PR merge, and narrative events do not produce another work completion.
- **Approval race and retained evidence:** immediate approval/delivery after work completion cannot remove evidence before checkpoint capture. Failed capture is visible. After a successful checkpoint, merge or PR publication can proceed without waiting for analysis, even if milestone publication needs retry. Analysis can start or finish from retained evidence after the source worktree is removed.
- **Configuration and lineage:** with spec 042 records, metrics retain selected source attempt and configuration history; multiple versions are visible. Without those records, they do not substitute today's configuration or prevent basic metrics reads.
- **Analysis scope:** analysis in the source trace increases total/analysis usage but leaves the frozen work snapshot unchanged. Analysis failure, completion, retries, and emitted notes do not reopen work, recursively dispatch the same analysis, or block final approval/delivery. Analysis-owned tasks do not suppress final-gate activation or legitimate change requests. Restart preserves attribution and duplicate suppression. Explicitly admitted substantive rework is counted as work and can produce a new work milestone.
- **Role-independent retries:** a custom task role behaves like any other assigned work role; one task execution plus one Guard execution yields zero task retries, while a second execution of the assigned work role yields one. Verification failure without another execution does not invent a retry. Historical unknown ownership is reported as a coverage gap.
- **Seed evidence:** default, cue, standing, session, and fork allocations record the actual winning initial budget. Concurrent seed requests, interrupted publication, and restart neither duplicate allocation nor choose a second seed identity. Observation replay does not alter honey. Top-up and stipend leave the initial budget unchanged; old trails without evidence return an unknown budget rather than current defaults.

Prior art: runtime completion and dispatch tests, run/task projection and usage tests, trail-event identity/merge tests, task-ledger energy tests, review-gate tests, session accounting tests, and prune retention tests.

## Out of Scope

This spec is the **recorder**. Everything that turns records into decisions belongs to a reader, and a reader is either a person or content shipped as a methodology.

- **Statistics over many trails** — norms, percentiles, class medians, "this trail is expensive for its kind". A reader computes those from the metrics; the runtime does not maintain a baseline.
- **Deciding that a trail is worth studying.** Which trails are interesting, and what counts as a defect, is a judgement. The runtime publishes `trail.work.completed` and the metrics; a reader decides. Analysis of paused or failed work can be requested explicitly; this success milestone is not their trigger.
- **Noise discipline.** How many runs count as evidence for a claim is a methodological choice, not a platform guarantee.
- **Promotion, quarantine, rollback.** Deciding that a procedure helped and disabling one that hurt is content, not a runtime primitive.
- **Benchmark suites, `pass@k`, scoring.** That is the eval colony's business ([spec 003](003-hive-evals.md)).
- **Cost in money.** Token counts are recorded; a price table is the reader's.
- **A colony-wide metrics database.** Metrics and immutable work-completion snapshots are trail-local projections of retained evidence. Missing historical measurements are reported explicitly; specs 040 and 039 alone do not supply every proposed field (notably the initial honey reserve).
- **Changing delivery semantics.** This spec's milestone precedes final approval and does not close a PR-backed final gate, merge/push automatically, or grant delivery to an experiment. The complete delivery outcome and notification design remains D6.
- **Re-executing history from JetStream.** `paseka replay` stays inspect-only.
- **Experimental execution isolation.** [Spec 042](042-trail-fork.md) owns trail configuration bindings, pinned fork policy, workspaces, and consumer scope on the colony's shared Reactor and bus. This spec only exposes their recorded provenance.

## Further Notes

### Open decisions

**D6 — What counts as delivery, and how is it recorded?** Work completion is now deliberately earlier than final approval. Existing delivery has several distinct observations: a local merge, successful PR publication, a later forge merge, root soft acceptance with no platform delivery, and an experiment whose delivery is prohibited. PR publication currently leaves the final gate waiting until forge merge, so "result handed over" and "merged" cannot share one implicit boolean. Design the delivery record/event vocabulary separately: mode, observed state, result identity, action outcome, references, timestamps, retry identity, and whether publication or merge fulfils the selected handover contract. Closed-without-merge and delivery failure must remain distinct from failed work. Portable delivery evidence also needs an owning trail record, since current PR identity is machine-local. None of these later decisions move the selected work-completion milestone.

### Resolved decisions

- **D1 — Work completion precedes final human approval.** Working tasks and mandatory checks have passed; final approval and delivery are still ahead. Honey-blocked, failed, and cancelled work are not this success milestone. Use `trail.work.completed` rather than a generic idle/settled notification; final delivery outcomes remain a separate design under D6.
- **D2 — Analysis stays in the source trail and does not gate delivery.** Work-completion evidence is frozen before analysis; execution-purpose provenance keeps analysis visible in total cost while excluding it from the work baseline and completion trigger. It may run alongside final review and continue from retained evidence after delivery removes the worktree.
- **D3 — Execution and retry counts are independent of role names.** Count the task's recorded assigned work executions and `max(0, executions - 1)` retries, with verification failures separate. Do not publish a return-for-rework metric without causal evidence.
- **D4 — Persist initial honey allocation in v1.** Record the authoritative seed winner and recover interrupted observation publication without allocating twice. Historical absence remains unknown; new defaults never fill old metrics.
- **D5 — Cover every cue ingress.** Record task and signal invocations, retain the trail's original ingress separately, and distinguish later additions without rewriting origin.

### Delivery and interpretation

- **Order of landing.**
  1. Land the run-record correction and vocabulary changes first, including the defined feedback compatibility behaviour.
  2. Land origin/ingress records, seed evidence, purpose attribution, retained result evidence, and work-completion checkpoints before the corresponding read surfaces and milestone publication. D6 owns a later delivery protocol; this spec does not wait for that design to redefine work completion.
  3. [Spec 042](042-trail-fork.md) builds on the metrics it writes; the fork's `forkOf` field is why `TrailMetrics` has that slot.
- **Why not a metrics daemon.** Metrics are derived from retained records. Additional evidence needed for a promised field must be recorded by its existing owning subsystem or reported as unavailable; a background service cannot reconstruct missing history.
- **Measurement, not proof.** `TrailMetrics` says what a trail cost, not whether the cost was avoidable. The same rule as [spec 041](../specs/041-skills-layer.md)'s attribution note: a recorded use means a file was read, not that its instructions were followed.
- **The boundary this spec protects.** A colony that can read its own records can have a reader for them — a person, a script, or a methodology pack. A colony that cannot read its records can only have a reader that parses logs, which is how a runtime ends up carrying an LLM's bookkeeping in its prompts. This spec keeps the bookkeeping in Go, where it is deterministic, and leaves the judgement to the reader.
