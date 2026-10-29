# Spec 045: Trail closing phase

## Status

**(Draft)**
Runtime-owned `closing` ledger phase, colony-declared policy, and a guaranteed AFK wrap-up task that sees the whole trail before the final review gate. Design agreed in conversation; implementation not started.

## Problem Statement

A Flight Trail today ends with the last work task and then the synthesized `_review` gate. There is no place where a Bee looks at everything the trail produced. Two concrete symptoms follow from that hole:

1. **The pull-request description is written by the last task.** The only actor told to emit `pr.body` is whichever task happens to be `IsLastWorkTask` at dispatch — the sole incomplete non-final task. It sees its own task body and a few narrative insights, not the trail. A multi-task trail therefore ships a PR description that describes only its last slice, or a run of unrelated per-task summaries that never add up to a change set.
2. **Per-task review does not accumulate.** A chain like `task-1 → review-of-task-1 → task-2 → review-of-task-2` verifies each slice against its own acceptance criteria and can still miss that the combined change is wrong, incomplete, or inconsistent. There is no point where a stronger model reviews the accumulated diff, and no supported way to turn the result into further work on the same trail.

The platform already has most of the pieces: the task ledger orders work by dependency, `task.plan` is accepted at any time, a planned task keeps the final gate closed until it completes, and Bees can publish new tasks. What is missing is a first-class notion of "the work is done, now close the trail", owned by the runtime so it cannot be forgotten, and configured by the colony so it can be switched on for large trails only.

## Solution

Introduce one new ledger phase, `closing`, assigned only by the runtime. The colony declares a `closing` policy: whether the phase is used, which predicates decide it, which Bee and model run it, how many rounds are allowed, and whether its failure blocks delivery. When all `work` tasks complete and the policy predicates match, the runtime ensures exactly one AFK `closing` task for the current round (`closing-1`, `closing-2`, …) and the existing scheduler dispatches it like any other task.

The closing task is an ordinary AFK Bee run with trail-wide context: every task, its intent, status, commit, and summary, plus the accumulated worktree. It reviews the whole change set, writes the authoritative `pr.body` and `trace.summary`, and — when it finds blockers or high-severity issues — plans further `work` tasks on the same trail with a dependency on itself. Those tasks run, and once they complete the runtime closes the loop with the next `closing` round, up to `max_rounds`. The final review gate opens only after the last closing round completes, so a human merges something that was reviewed as a whole.

Because the phase lives in the ledger and is written into the task event, replay, fork, verify, and the Console keep working without consulting colony configuration. Because the policy lives in colony configuration and is pinned into the trail's configuration snapshot, enabling closing for big trails changes nothing about trails already running.

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

## Implementation Decisions

### 1. Phase is ledger semantics, not colony configuration

- Add `phase` to the task spec, the ledger task snapshot, and the `task.md` frontmatter. Closed vocabulary in v1: `work` (default; empty means `work`) and `closing`.
- `phase` is written into the `task.plan` event and persisted, so the ledger is self-describing. The reducer never reads colony configuration; it derives all ordering from recorded phase values.
- Colony-defined phase names are rejected. A configurable phase vocabulary would make eligibility depend on `colony.yaml` at replay time, break trail forks and `verify`, and turn pure ledger predicates into functions of mutable state.
- Phases stay orthogonal to review policy. `phase` answers "when may this start"; `review` answers "does a human gate it". A closing task is normally `review: none`; the trace merge gate remains `review: final`.

### 2. Only the runtime assigns `closing`

- A `task.plan` is honoured as `closing` only when its publisher is the runtime. Any other publisher — planner, builder, reviewer, console, CLI — is coerced to `work` at reduce time.
- This is a defaulting and backward-compatibility rule, not a security boundary: event forgery is out of scope, exactly as it is for other platform kinds.
- Consequence: no existing Bee configuration, prompt, or script changes behaviour, and a Bee that writes `phase: closing` by mistake cannot create a wrap-up or bypass the round cap.
- Console and CLI task creation never offer `closing`.

### 3. Two predicates, one split

- `AllWorkCompleted` — every `work`-phase task is `completed`. This is the closing trigger.
- `AllAFKTasksCompleted` — every non-final task, `work` and `closing`, is `completed`. This stays the final-gate condition.
- Closing tasks become eligible only when `AllWorkCompleted` holds; the general "oldest eligible planned task, one slot at a time" rule is unchanged.
- `review: final` keeps its current meaning and dispatch skip; `_review` remains the synthesized gate when no explicit final task was planned.
- A closing task may carry explicit dependencies in addition to the implicit phase condition; the runtime seeds it without dependencies.

### 4. Colony policy

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
- `task` is a template, not a ledger task. `phase: closing` is added by the runtime; `review: final` is rejected at load; the Bee must exist and be a valid task target.
- `max_rounds` bounds the number of closing rounds; default 2 (one review plus one verification round). `1` means a single pass with no re-review after follow-up work.
- A closing task that ends `failed` or `blocked` keeps the final gate closed, exactly as any other non-completed task does today. There is no advisory mode in v1.
- The policy is resolved at trail start and pinned into the trail's immutable configuration snapshot; a later `colony.yaml` edit reaches running trails only through the existing safe configuration cutover, never mid-round.

### 5. Runtime ensures the seed

- On the same event boundaries where the final gate is evaluated, and before it, the runtime ensures the current closing round exists.
- Idempotency is by stable task id: `closing-1`, `closing-2`, … derived from the highest existing closing round, not from a random id and not from a count. Re-application, restart, and event echo are no-ops because the reducer ignores a `task.plan` for an id that already has a status.
- A round is seeded only when: the policy is enabled, `when` predicates hold, `AllWorkCompleted` holds, no closing task is in flight, and either no closing round exists or new work completed after the last closing round completed. When `trail.work.completed` from spec 043 lands, its revision/`completionId` replaces the "new work completed after" comparison.
- When the cap is reached, the runtime does not seed another round and records an operational note explaining the skip. No task is created and cancelled — a cancelled task would block the gate just like a planned one, because the predicate requires `completed`.

### 6. The closing round is a normal AFK dispatch

- The closing task is dispatched by the existing scheduler with a normal honey charge. It is not a direct dispatch and not a special agent class.
- The closing Bee may publish `task.plan` for `work` tasks with a dependency on the closing task's id. Those tasks are ordinary work: they commit, they can open isolated proposals, and they can be reviewed.
- The closing Bee must not edit the trail's files itself. If it does, it becomes the newest completed isolated-proposal task and hijacks review rework routing, which attributes follow-up work to the wrong Bee with the wrong task body.
- v1 relies on the prompt and convention for read-only behaviour. As a safety net, closing-phase tasks are excluded from the rework-source predicate, so a closing task that opens a proposal is recorded but never becomes the source of "apply review comments" work.

### 7. Trail-wide prompt context

- The closing task receives the existing template context plus a task inventory: id, title, phase, intent, status, commit, summary, and dependencies for every task in the trail.
- The existing narrative insight projection is unchanged; `task.plan` stays operational and is not injected as prose. The inventory is structured, not a transcript.
- `pr.body` and `trace.summary` keep last-write-wins. The closing task always emits both and therefore supersedes per-task text.
- When the pinned policy guarantees a closing round for the current trail, the last work task stops receiving the `pr.body` must-emit guidance; otherwise the current fallback remains, so a small trail still produces a description.
- No new predicate for this duty: at dispatch the closing task is the sole incomplete non-final task and already receives the existing guidance. The change is the added context and the suppression rule, not a second "last task" notion.

### 8. Validation and failure visibility

- `dependsOn` is validated wherever a snapshot is available — CLI/console task creation, plan builders, and the doctor — for unknown ids, self-dependency, and cycles. The reducer stays tolerant, as today.
- Colony loading validates the closing policy: known predicates, existing Bee, `review` not `final`, `max_rounds >= 1`.
- The doctor reports: a trail whose work is complete but whose closing seed cannot be built, a closing task stuck in `planned` with unsatisfiable dependencies, and a trail that reached the cap with follow-up work still unseeded.
- Closing tasks are never copied into a fork. A fork inherits work tasks only; the source trail's closing round stays with the source.

### 9. Surfaces

- Queen Console task views expose `phase` as a badge and distinguish closing from work in counts.
- `task.md` frontmatter gains `phase`; older files without it read as `work`.
- The CLI task list and `paseka status` show the phase; `paseka task create` cannot set it.

## Testing Decisions

- Test external behaviour: given a recorded event sequence and a policy, which tasks exist, what phase they carry, what is dispatched, and when the gate opens. Avoid asserting internal call order.
- Ledger tests: phase defaulting for non-runtime planners, `AllWorkCompleted` versus `AllAFKTasksCompleted`, closing eligibility only after work completes, idempotent re-plan, round numbering from the highest existing round. Table-driven in the style of the existing eligibility and apply tests.
- Reactor tests: seed on the completion boundary, no seed while a closing task is in flight, no seed when a predicate fails, re-seed after follow-up work completes, no seed past `max_rounds`, gate stays closed on a failed closing task. Use the existing test reactor with an injected publisher and a fake ledger.
- Colony tests: policy load and validation failures, default-disabled behaviour, pinned policy snapshot.
- Prompt tests: the closing context renders the task inventory; the work task's `pr.body` guidance is suppressed only when the policy guarantees a closing round.
- Fork tests: a fork of a trail with a completed closing round contains no closing task.
- End-to-end trail test: two work tasks, one closing round that plans one fix task, one second closing round, then the gate — asserting the gate never opened before the last closing task completed.

## Out of Scope

- Publishing the pull request itself (branch push, forge PR creation or update). Closing prepares the change set and the `pr.body`; delivery stays with the pull-request delivery design.
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
- Prior art for the colony block: `auto_invites` (conditions plus choreography), `defaults.delivery` (a closed value set chosen by the colony), and `defaults.energy_budget` (a numeric policy pinned per trail).
