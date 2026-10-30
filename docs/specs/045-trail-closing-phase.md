# Spec 045: Trail closing and merge phases

## Status

**(Approved)**
Runtime-owned `closing` and `merge` ledger phases, colony-declared closing policy, configurable merge-gate copy, and a guaranteed AFK wrap-up task that sees the whole trail before the final review gate. The `dependsOn` validation and doctor warnings from §10 have already landed; the phases, the closing policy, and the gate copy are not implemented.

## Problem Statement

A Flight Trail today ends with the last work task and then the synthesized `_review` gate. There is no place where a Bee looks at everything the trail produced. Three concrete symptoms follow from that hole:

1. **The pull-request description is written by the last task.** The only actor told to emit `pr.body` is whichever task happens to be `IsLastWorkTask` at dispatch — the sole incomplete non-final task. It sees its own task body and a few narrative insights, not the trail. A multi-task trail therefore ships a PR description that describes only its last slice, or a run of unrelated per-task summaries that never add up to a change set.
2. **Per-task review does not accumulate.** A chain like `task-1 → review-of-task-1 → task-2 → review-of-task-2` verifies each slice against its own acceptance criteria and can still miss that the combined change is wrong, incomplete, or inconsistent. There is no point where a stronger model reviews the accumulated diff, and no supported way to turn the result into further work on the same trail.
3. **The gate itself says nothing and cannot be described.** `review: final` is asked to mean two unrelated things — "this task is last" and "a human must approve it" — so the ledger has no phase to reason about, and the synthesized gate ships fixed copy ("Human review and merge", one generic sentence) while the human is about to merge a whole change set.

The platform already has most of the pieces: the task ledger orders work by dependency, `task.plan` is accepted at any time, a planned task keeps the final gate closed until it completes, and Bees can publish new tasks. What is missing is a first-class notion of "the work is done, now close the trail", owned by the runtime so it cannot be forgotten, and configured by the colony so it can be switched on for large trails only — plus an explicit lifecycle for the gate that closes the trail.

## Solution

Introduce two new ledger phases, `closing` and `merge`, both assigned only by the runtime. `work` stays the default and the only phase a Bee or operator can create directly.

The colony declares a `closing` policy: whether the phase is used, which predicates decide it, which Bee and model run it, and how many rounds are allowed. When all `work` tasks complete and the policy predicates match, the runtime ensures exactly one AFK `closing` task for the current round (`closing-1`, `closing-2`, …) and the existing scheduler dispatches it like any other task.

The closing task is an ordinary AFK Bee run with trail-wide context: every task, its intent, status, commit, and summary, plus the accumulated worktree. It reviews the whole change set, writes the authoritative `pr.body` and `trace.summary`, and — when it finds blockers or high-severity issues — plans further `work` tasks on the same trail with a dependency on itself. Those tasks run, and once they complete the runtime closes the loop with the next `closing` round, up to `max_rounds`.

`merge` is the existing trace-level gate, now named as a phase. A task planned with `review: final` — by a Bee, a prompt, an operator, or the runtime's `_review` synthesis — is recorded as `phase: merge`. It is never dispatched; it is presented to a human at `waiting_review`, or auto-completed when there is nothing to merge. The gate opens only when every work and closing task has completed, and approval is refused while any of them is still incomplete, so a change set can never be merged underneath a running closing round or an unfinished rework task.

Because the phases live in the ledger and are written into the task event, replay, fork, verify, and the Console keep working without consulting colony configuration. Because the policies live in colony configuration and are pinned into the trail's configuration snapshot, enabling a closing review for big trails changes nothing about trails already running.

The only merge behaviour the colony configures is the gate's copy: a title and body template rendered with the same trail inventory the closing task receives, so the human sees what the trail did before merging it.

## User Stories

1. As a Beekeeper, I want one Bee to review the accumulated changes of a whole trail, so that I merge a change set that was judged as a whole rather than slice by slice.
2. As a Beekeeper, I want the pull-request description to be written by the Bee that saw every task, so that the PR body explains the entire change and not only the last slice.
3. As a Beekeeper, I want the wrap-up to run automatically after all work completes, so that I do not have to remember to ask for it.
4. As a Beekeeper, I want the wrap-up to be optional and switchable per colony, so that small one-task trails do not pay for a review of nothing.
5. As a Beekeeper, I want to require a minimum number of work tasks before a wrap-up runs, so that the overhead appears only on trails big enough to need it.
6. As a Beekeeper, I want the wrap-up to be skipped when the trail produced no isolated proposal, so that a scout-only or documentation-only trail does not get a hollow closing round.
7. As a Beekeeper, I want to point the closing phase at a specific Bee and model alias, so that a stronger model reviews large change sets while builders stay cheap.
8. As a Beekeeper, I want the wrap-up to be an AFK run, so that it costs me no attention until the merge gate.
9. As a Beekeeper, I want the reviewer to be able to file follow-up work as real tasks on the same trail, so that a found blocker becomes a tracked, reviewed, retried unit instead of an untracked edit.
10. As a Beekeeper, I want the trail to review itself again after those follow-up tasks complete, so that the fixes are covered by the same whole-trail judgement.
11. As a Beekeeper, I want a bounded number of closing rounds, so that an opinionated reviewer cannot loop the trail forever and drain honey.
12. As a Beekeeper, I want the final gate to stay closed when the wrap-up fails, so that an unreviewed change set is never presented for merge.
13. As a Beekeeper, I want to see the closing phase in the Queen Console task board and on `task.md`, so that I can tell "still working" from "closing" at a glance.
14. As a Beekeeper, I want to be told when a closing round was skipped because a cap or predicate said no, so that silence is explained rather than mysterious.
15. As an operator, I want colony validation to reject a closing policy that names a missing Bee, an impossible review policy, or an unknown predicate, so that misconfiguration fails at load rather than at the merge gate.
16. As an operator, I want a closing task that cannot start to fail loudly instead of leaving the gate closed silently, so that a dependency typo does not strand a trail.
17. As an operator, I want a trail's closing policy pinned at trail start, so that editing `colony.yaml` mid-trail does not change what a running trail is doing.
18. As a Bee author, I want a `task.plan` I publish to be treated as `work` no matter what phase I write, so that existing Bee configs and prompts keep working unchanged.
19. As a Bee author, I want the closing task's prompt to carry the trail's task list and commits, so that "review everything" is a task a Bee can actually perform.
20. As a Bee author, I want the closing review to be expressible with the existing ledger primitives (`task.plan`, `task.completed`), so that I do not need a new adapter contract.
21. As a fork user, I want closing tasks never copied into a fork, so that an experiment does not inherit a wrap-up that belonged to its source trail.
22. As a maintainer, I want the phase vocabulary closed and versioned in the protocol, so that replaying a recorded trail never depends on current configuration.
23. As a Beekeeper, I want the merge gate to be a real phase rather than an overloaded review policy, so that "this is the end of the trail" and "a human must approve" stop being the same field.
24. As an operator, I want to author the gate's title and body, so that the review screen says what the trail did instead of a generic sentence.
25. As a Beekeeper, I want the gate copy rendered from the trail's task inventory, so that the review screen lists the actual work before I approve it.
26. As a Beekeeper, I want approval refused while any other work or closing task is still incomplete, so that I cannot merge while a rework or a closing round is in flight.
27. As a Beekeeper, I want closing rounds to stop once the trail has been delivered, so that a merged trail does not sprout new review rounds.
28. As an operator, I want an explicit final gate authored by a Bee or by me to keep working exactly as before, so that existing colonies need no migration.
29. As an operator, I want duplicate merge gates to be impossible, so that the Reviews tab never shows two merge targets.
30. As a maintainer, I want the merge phase to change no delivery behaviour, so that this spec cannot silently alter how pull requests are published.

## Implementation Decisions

### 1. Phase is ledger semantics, not colony configuration

- Add `phase` to the task spec, the ledger task snapshot, and the `task.md` frontmatter. Closed vocabulary: `work` (default; empty means `work`), `closing`, and `merge`.
- `phase` is written into the `task.plan` event and persisted, so the ledger is self-describing. The reducer never reads colony configuration; it derives all ordering from recorded phase values.
- Colony-defined phase names are rejected. A configurable phase vocabulary would make eligibility depend on `colony.yaml` at replay time, break trail forks and `verify`, and turn pure ledger predicates into functions of mutable state.
- Phases stay orthogonal to review policy. `phase` answers "when may this start and where does it sit"; `review` answers "does a human gate it". A closing task is normally `review: none`; the merge task is `phase: merge` with `review: final`.

### 2. Only the runtime assigns `closing`, and `merge` is derived from `review: final`

- A `task.plan` is honoured as `closing` only when its publisher is the runtime (the reserved runtime agent id). Any other publisher — planner, builder, reviewer, console, CLI — is coerced to `work` at reduce time.
- `merge` is derived, not authored: a task planned with `review: final` is recorded as `phase: merge`, by anyone, exactly as an explicit final gate works today. A plan that writes `phase: merge` with any other review policy is coerced to `work`.
- The rule stays consistent: `review: final` remains the only supported way for a Bee or operator to place a gate, and the runtime is the only actor that can introduce a phase a task did not earn through its review policy.
- This keeps user-authored and prompt-authored final gates working unchanged, and keeps `closing` a runtime-only capability.
- This is a defaulting and backward-compatibility rule, not a security boundary: event forgery is out of scope, exactly as it is for other platform kinds.
- Console and CLI task creation never offer `closing` or `merge` directly; `review: final` remains the supported way for an operator to place a gate.

### 3. Phase model and transitions

The three phases form a small state machine, not a straight line:

```text
work* → closing* → merge → (request changes) → work* → closing* → merge
```

- `work` is the default and the only phase a Bee or operator can create.
- `closing` is seeded by the runtime after all work completes, and only while the trail is not closed.
- `merge` is the trace-level gate. It is never dispatched: it is presented at `waiting_review`, or auto-completed when there is nothing to merge.
- Re-entry is the existing rework path: a rejected gate plans work and keeps the gate at `waiting_review`. With closing enabled, that work is followed by another closing round before the gate is actionable again.
- Closing is not seeded once a merge task is `completed`: the trail has been delivered and is closed to new rounds.
- A `planned` merge task (an explicit final gate authored at planning time) coexists with closing rounds and does not open the gate early. Only `AllPreMergeCompleted` does.
- Exactly one merge task exists per trail. The runtime ensures it with a stable id; the reducer tolerates duplicates in recorded history but never creates them.
- Approve is refused while any other non-merge task is incomplete. This makes the invariant enforceable and also closes an existing hole where rework planned from a rejection could be merged before it ran.

### 4. Predicates and gate activation

- `AllWorkCompleted` — every `work`-phase task is `completed`. This is the closing trigger.
- `AllPreMergeCompleted` — every non-merge task, `work` and `closing`, is `completed`. This is the final-gate condition; it replaces `AllAFKTasksCompleted`, whose name no longer describes what it tests.
- `IsMergeTask` — `phase == merge` (which implies `review: final`). `IsReviewGate` stays "merge task or `review: required`" for the approve/reject/discard guards.
- Closing tasks become eligible only when `AllWorkCompleted` holds; the general "oldest eligible planned task, one slot at a time" rule is unchanged.
- Merge tasks are never dispatched. `ShouldSkipDispatch` keys on `IsMergeTask`.
- Whether the trail has anything to merge is a git question (`MergeDiff`), not a ledger question. The phase does not claim to know it; the runtime decides activation, as today. Keeping the impure check outside the ledger is deliberate.
- Nothing to merge still auto-completes the gate instead of presenting a hollow human review. That behaviour is not configurable in v1.

### 5. Colony policy: closing

A `closing` block in the colony manifest, all fields optional and disabled by default:

```yaml
closing:
  enabled: true
  when:
    min_work_tasks: 2
    has_isolated_proposal: true
  task:
    bee: reviewer
    intent: closing
    title: Review the trail and prepare the pull request
    review: none
    params:
      model: strong
  max_rounds: 2
```

- `when` uses named predicates evaluated by the runtime over the ledger snapshot, in the style of `auto_invites.match`. v1 predicates: `min_work_tasks` (counts `work`-phase tasks in the trail regardless of status) and `has_isolated_proposal`. Expressions are out of scope.
- `task` is a template, not a ledger task. `review: final` and `phase` are rejected in the template; the Bee must exist and be a valid task target.
- `max_rounds` bounds the number of closing rounds; default 2 (one review plus one verification round). `1` means a single pass with no re-review after follow-up work.
- A closing task that ends `failed` or `blocked` keeps the final gate closed, exactly as any other non-completed task does today. There is no advisory mode in v1.
- The policy is resolved at trail start and pinned into the trail's immutable configuration snapshot; a later `colony.yaml` edit reaches running trails only through the existing safe configuration cutover, never mid-round.

### 6. Colony policy: merge gate copy

The gate's wording is the only merge behaviour that is configurable in v1:

```yaml
merge:
  task:
    title: "Review and merge: {{.TraceTitle}}"
    body: |
      ## What this trail did
      {{range .Tasks}}- {{.TaskID}} ({{.Intent}}) — {{.Summary}}
      {{end}}
```

- `title` and `body` are templates rendered with the same trail inventory as the closing task; unset fields fall back to today's literal copy, so existing colonies see no change.
- Only copy is configurable. Activation, the empty-gate skip, approval (human-only), and delivery (`defaults.delivery`) are unchanged and stay out of this block, because they are behaviour rather than presentation and delivery belongs to the separate delivery design.
- Copy is also not a completion contract: a malformed template fails colony validation, and a rendered body is informational only.

### 7. Runtime ensures the merge task and the closing seed

- On the same event boundaries where the final gate is evaluated, and before it, the runtime ensures the current closing round exists.
- Idempotency is by stable task id: `closing-1`, `closing-2`, … derived from the highest existing closing round, not from a random id and not from a count. Re-application, restart, and event echo are no-ops because the reducer ignores a `task.plan` for an id that already has a status.
- A closing round is seeded only when: the policy is enabled, `when` predicates hold, `AllWorkCompleted` holds, no closing task is in flight, no merge task is `completed`, and either no closing round exists or new work completed after the last closing round completed. When `trail.work.completed` from spec 043 lands, its revision/`completionId` replaces the "new work completed after" comparison.
- When the cap is reached, the runtime does not seed another round and records an operational note explaining the skip. No task is created and cancelled — a cancelled task would block the gate just like a planned one, because the predicate requires `completed`.
- The existing `_review` synthesis becomes the ensure path for the merge task: a stable id (`_review` is kept for continuity with recorded trails, console routes, and docs), `phase: merge`, `review: final`, and the configured copy. An explicit final gate planned by a Bee or operator still wins; the runtime does not add a second one.

### 8. The closing round is a normal AFK dispatch

- The closing task is dispatched by the existing scheduler with a normal honey charge. It is not a direct dispatch and not a special agent class.
- The closing Bee may publish `task.plan` for `work` tasks with a dependency on the closing task's id. Those tasks are ordinary work: they commit, they can open isolated proposals, and they can be reviewed.
- The closing Bee must not edit the trail's files itself. If it does, it becomes the newest completed isolated-proposal task and hijacks review rework routing, which attributes follow-up work to the wrong Bee with the wrong task body.
- v1 relies on the prompt and convention for read-only behaviour. As a safety net, closing-phase tasks are excluded from the rework-source predicate, so a closing task that opens a proposal is recorded but never becomes the source of "apply review comments" work.

### 9. Trail-wide prompt context and gate copy context

- The closing task receives the existing template context plus a task inventory: id, title, phase, intent, status, commit, summary, and dependencies for every task in the trail.
- The same inventory is available to the merge gate copy template, without a running Bee.
- The existing narrative insight projection is unchanged; `task.plan` stays operational and is not injected as prose. The inventory is structured, not a transcript.
- `pr.body` and `trace.summary` keep last-write-wins. The closing task always emits both and therefore supersedes per-task text.
- When the pinned policy guarantees a closing round for the current trail, the last work task stops receiving the `pr.body` must-emit guidance; otherwise the current fallback remains, so a small trail still produces a description.
- No new predicate for this duty: at dispatch the closing task is the sole incomplete non-final task and already receives the existing guidance. The change is the added context and the suppression rule, not a second "last task" notion.

### 10. Validation and failure visibility

- `dependsOn` is validated wherever a snapshot is available — CLI/console task creation, plan builders, and the doctor — for unknown ids, self-dependency, and cycles. The reducer stays tolerant, as today.
- Colony loading validates both policies: known predicates, existing Bee, `review` not `final` in the closing template, `max_rounds >= 1`, and a parseable gate copy template rendered against an empty inventory.
- The doctor reports: a trail whose work is complete but whose closing seed cannot be built, a closing task stuck in `planned` with unsatisfiable dependencies, a trail that reached the cap with follow-up work still unseeded, and a trail with more than one merge-phase task.
- Closing and merge tasks are never copied into a fork. A fork inherits work tasks only; the source trail's closing round and gate stay with the source.

### 11. Surfaces

- `phase` is a separate axis from status and is never folded into a status label. Status stays the seven-value lifecycle that drives board columns, tones, and counts; a closing card is still `planned`, `running`, `blocked`, or `failed`.
- The task card and the task detail header carry a phase tag only when the phase is not `work`: `closing` or `merge`. It follows the review-policy badge precedent — a second badge on the card, not part of the status word — and uses a quiet tone so it cannot compete with the lifecycle colour that says where the task sits. Cards, detail rows, and filters read it from the polled board row, like every other badge.
- Status labels are unchanged, including `waiting review`. No composite `running · closing` label is introduced: columns, counts, tones, filters, and accessible names are all keyed by status, and the board's whole reading order is the lifecycle.
- `taskCounts` stays status-based. A second dimension in the Dashboard tile would break the quiet-text-plus-one-badge pattern; if phase counts are ever needed, they belong on the Tasks page.
- The Reviews queue derives `final gate` from `phase == merge` instead of `review == final` (equivalent by derivation, but the phase is the semantic), and renders the configured gate copy in place of the literal sentence. This is also where a merge gate is distinguishable from a mid-trail `review: required` stop, which delivers nothing.
- Approve is disabled with a reason while any other non-merge task is incomplete.
- `task.md` frontmatter gains `phase`; older files without it read as `work` when `review` is not `final`, and as `merge` when it is.
- The CLI task list and `paseka status` print the phase beside the status; `paseka task create` cannot set it.
- The Queen Console design-system document gains the phase-badge rule when this ships; until then the spec is the contract and the badge follows the existing card-badge pattern.

**Trail detail.** A trail has its own status logic and it is not the task lifecycle: `traceState` is `active | failed | idle`, derived from runs (`hasActive`, `hasFailures`), with `standing`, `fork`, and `pinned` as independent flags. Nothing in that projection reads the ledger, and there is no "current task" notion on a trail at all — only "some run is active". That axis is left alone; it answers "does this trail need attention", and redefining it would make one badge mean two different things on the Dashboard and on the trail page.

- Consequently, a trail sitting at the merge gate reads `idle` today, indistinguishable from a trail that simply finished. That is already misleading, and the closing phase makes it worse, because the moment an operator most needs to recognise — a closing round is running, or the trail is waiting for a merge decision — has no representation.
- A derived `stage` is added to the trail **detail** payload, computed server-side from the ledger with the same predicates, never in the client: `work` (default), `closing` (a closing task exists and is not completed), `gate` (the merge task is `waiting_review` and every non-merge task is completed), `delivered` (the merge task is `completed`). It is not a task phase and not a status; it is the phase machine projected onto the trail, where the client cannot see the ledger.
- The trail header renders it as one quiet tag beside the existing attention flags, so a closing round shows `active` and `closing` together — something is running, and it is the wrap-up. The flag and the stage are separate because the stage stays true when the trail is quiet: `gate` has no active run by definition.
- The trail's task list renders the same per-task phase tag as the board (above), and `TaskSummaryView` gains `phase` and `review` — today it carries only id, title, status, and bee, which is not enough to draw the tag. Task rows there also switch from the raw `status` string to `taskStatusLabel`, matching the board; `waiting_review` is currently the one status that prints with an underscore on this page.
- The trails list, Dashboard rows, and filters keep `active | failed | idle` and do not gain the stage in v1: a list row is a pointer, and the stage is a detail answer. If phase-level trail counts are ever wanted, they belong on the Traces page next to the existing counts, not in the Dashboard tile.

## Testing Decisions

- Test external behaviour: given a recorded event sequence and a policy, which tasks exist, what phase they carry, what is dispatched, and when the gate opens. Avoid asserting internal call order.
- Ledger tests: phase defaulting for non-runtime planners, `review: final` deriving `merge`, `phase: merge` with another review policy coerced, `AllWorkCompleted` versus `AllPreMergeCompleted`, closing eligibility only after work completes, idempotent re-plan, round numbering from the highest existing round. Table-driven in the style of the existing eligibility and apply tests.
- Gate tests: activation only when work and closing are complete, a planned merge gate does not open early, approve refused while any other non-merge task is incomplete, no closing round after a completed merge, exactly one merge task even under repeated ensure, and no behavioural change in the merge/local-merge and PR-publish paths.
- Reactor tests: seed on the completion boundary, no seed while a closing task is in flight, no seed when a predicate fails, re-seed after follow-up work completes, no seed past `max_rounds`, gate stays closed on a failed closing task. Use the existing test reactor with an injected publisher and a fake ledger.
- Colony tests: policy load and validation failures, default-disabled behaviour, pinned policy snapshot, and a gate copy template rendered with an empty inventory.
- Prompt and copy tests: the closing context renders the task inventory; the gate copy falls back to today's literal text when unset; the work task's `pr.body` guidance is suppressed only when the policy guarantees a closing round.
- Compatibility tests: a task record with `review: final` and no `phase` reads as `merge`; a record with neither reads as `work`.
- Trail projection tests: `stage` is `work` for a trail with no closing task, `closing` while one is incomplete, `gate` when the merge task waits and nothing else is open, `delivered` once it completes; a trail at the gate reports `hasActive == false` and still shows `gate`, which is the case a run-derived status cannot express.
- Fork tests: a fork of a trail with a completed closing round and a merge gate contains neither.
- End-to-end trail test: two work tasks, one closing round that plans one fix task, one second closing round, then the gate — asserting the gate never opened before the last closing task completed.

## Out of Scope

- Publishing the pull request itself (branch push, forge PR creation or update). Closing prepares the change set and the `pr.body`; delivery stays with the pull-request delivery design.
- Any merge behaviour beyond copy: activation predicates, the empty-gate skip, agent or automatic approval, and delivery mode. `defaults.delivery` keeps deciding local merge versus PR publish, and the "delivering → delivered" state that a PR-backed gate needs is deferred.
- Colony-defined or arbitrary phase names.
- Merging per-task review into the closing phase. `review: required` on a task keeps its current meaning.
- Standing trails: a recurring tick has no single "all work done" moment, so closing does not apply in v1.
- Model routing by issue severity, and multiple closing Bees per trail.
- The `trail.work.completed` signal, metrics, and accounting from spec 043. This spec only consumes its trigger once it exists.

## Further Notes

- Boundary with spec 043: 043 is an observer. It records that verified work completed and gives that milestone an identity. It does not synthesize tasks, has no mandatory-analysis policy, no delivery, and no configuration schema for reactions. This spec owns the phase, the policy, the synthesis of the closing round, and the obligations of the finalizing task; when 043 lands it replaces the v1 trigger comparison with `workRevision`/`completionId`.
- Open: whether an advisory failure mode should exist — a closing round that failed but still lets the gate open with a warning. v1 holds the gate closed, matching every other non-completed task; no policy field is introduced until someone asks for the opposite.
- Open: whether the closing Bee should be allowed to edit trivial fixes directly when it also files tasks. Current answer is no — one actor, one mode — because mixed behaviour breaks rework attribution.
- Open: for very large trails, a closing task may want its own sub-plan (a review of each sector). That is a prompt-level decomposition inside the closing round, not a new phase.
- The merge phase is an encoding change, not a delivery change. A PR-backed gate already ends in `waiting_review` until the forge reports merged, and task status has no "delivering" state. Making that explicit — delivering, delivered, delivery failed, agent approval — is the natural next spec (the delivery outcome work referenced by earlier delivery design) and should not be smuggled into this one.
- The file slug is `045-trail-closing-phase` because spec slugs are stable; the merge phase was folded into the same design record during review, so the H1 is broader than the filename.
- Prior art for the colony block: `auto_invites` (conditions plus choreography), `defaults.delivery` (a closed value set chosen by the colony), and `defaults.energy_budget` (a numeric policy pinned per trail).
