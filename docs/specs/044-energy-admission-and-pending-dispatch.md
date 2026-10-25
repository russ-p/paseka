# Spec 044: Energy admission and pending dispatch

## Status

**(Approved)**
The direct-dispatch half is implemented. The task-dispatch split, standing `skipped` outcome, and `energy.paused` / `energy.resumed` observations remain to be built.

## Problem Statement

A Flight Trail has one shared honey reserve, but the runtime also writes that reserve's exhaustion into `task.status=blocked`. The two concepts are not the same:

- A task status describes one unit of work, while honey belongs to the whole trail.
- A direct dispatch is a reaction to an event. It may run while its task is `waiting_review` or `running`, so it is not a task lifecycle transition at all.
- When a direct dispatch is denied for lack of honey, overwriting the task status destroys the real review or execution state. A later `energy.add` then resumes the task's assigned Bee instead of the blocked guard, receiver, or fix-up Bee, and the source event is lost.
- Nothing durable records which work was postponed, so a restart also loses the fact that anything is waiting.

Operators cannot tell the difference between "this task cannot proceed" and "the trail has no honey for any next dispatch". Standing Trails depend on the confusion as well: a previous tick's `blocked` task is what lets the next tick start, even though the tick boundary is a new allocation rather than a task failure.

## Solution

Separate the concepts into two durable layers:

1. **Task lifecycle stays authoritative.** Energy exhaustion never invents a task status. A task keeps the status its own work earned, including `waiting_review`.
2. **Trail admission owns deferred work.** When the honey reserve cannot pay for a dispatch, the runtime records a durable `PendingDispatch` instead of changing the task. `energy.add` drains pending work in creation order. `energy.stipend` starts a new allocation and does not resurrect the previous tick's work.
3. **The trail exposes a pause, not a fake task block.** A platform observation pair, `SIGNAL/energy.paused` and `SIGNAL/energy.resumed`, tells operators and Human Gateway surfaces when the trail stopped admitting dispatches and when it can admit them again.
4. **Standing ticks get an honest terminal outcome.** A task that was deferred by a previous tick and is superseded by stipend becomes `skipped`: it was eligible but deliberately not delivered, and it is neither failed nor completed.
5. **Legacy stays readable.** Existing honey-blocked tasks keep their old meaning until purge. No historical ledger is rewritten.

## User Stories

1. As a Beekeeper, I want a task in `waiting_review` to stay in `waiting_review` when its direct reviewer cannot run because honey is exhausted, so that the ledger still tells me what stage the work reached.
2. As a Beekeeper, I want `energy.add` to resume the guard, receiver, or fix-up Bee that was actually blocked, so that top-up continues the choreography instead of restarting the builder.
3. As a Beekeeper, I want the blocked source event to be recoverable from the trail record, so that a resumed direct dispatch is the same reaction, not a fabricated one.
4. As a Beekeeper, I want deferred work to survive a reactor restart, so that restarting the hive does not silently forget blocked work.
5. As a Beekeeper, I want pending work drained oldest-first, so that a top-up continues the queue in the order the trail attempted it.
6. As a Beekeeper, I want a task dispatch denied by honey to remain in its task lifecycle state rather than becoming `blocked`, so that honey is not presented as a task defect.
7. As a Beekeeper, I want a task dispatch that was never admitted to remain visibly pending, so that `ready` still means "eligible" and the trail shows why nothing started.
8. As a Beekeeper, I want repeated deferrals of the same work to produce one pending entry, so that duplicate bus delivery cannot multiply dispatches.
9. As a Beekeeper, I want a repeated `energy.add` not to dispatch the same pending work twice, so that a retried top-up is safe.
10. As a Beekeeper, I want an `energy.add` that is still too small to leave the remaining work pending, so that the trail does not claim to have resumed everything.
11. As a Beekeeper, I want a direct dispatch whose source event can no longer be recovered to become visibly unresolved, so that missing evidence is not mistaken for successful work.
12. As a Beekeeper, I want a paused trail to publish one `energy.paused` observation per pause episode, so that notifications do not repeat for every deferred item.
13. As a Beekeeper, I want a resumed trail to publish one `energy.resumed` observation when the reserve can admit work again, so that the Human Gateway can close the alert without polling.
14. As a Beekeeper, I want pause/resume observations to identify whether an `energy.add` or a standing `energy.stipend` changed admission, so that I can tell a live top-up from a new tick.
15. As a Beekeeper, I want pause/resume observations to survive restart without duplicate cards, so that a Reactor bounce does not spam me.
16. As a Beekeeper, I want a standing `energy.stipend` not to resume the previous tick's pending work, so that a new tick is new work rather than a resurrection.
17. As a Beekeeper, I want a previous standing tick's deferred task to become `skipped`, so that the next tick is not refused as busy and I can still see that the old tick did not run.
18. As a Beekeeper, I want `skipped` to be distinct from `failed`, `cancelled`, and `completed`, so that a superseded allocation is not counted as a defect, a kill, or a success.
19. As a Beekeeper, I want a standing stipend to abandon the previous tick's pending direct reactions, so that a new tick cannot accidentally run an old guard or receiver.
20. As a Beekeeper, I want `paseka energy add` to work the same for task and direct pending work, so that the recovery rule does not depend on the dispatch path.
21. As a Beekeeper, I want an existing honey-blocked task from an older ledger to keep working until purge, so that upgrading the runtime does not strand old trails.
22. As a Beekeeper, I want `blocked` to remain available for genuine task-level blockers, so that a future manual or dependency block does not need another status name.
23. As a Beekeeper, I want Queen Console to show the trail's paused state and pending count beside the true task statuses, so that the board no longer lies about review work.
24. As a Beekeeper, I want `paseka energy show` to print pause and pending information, so that a shell operator sees the same state as the Console.
25. As a Beekeeper, I want Telegram to send energy pause and resume cards instead of fake task-blocked cards, so that the alert's action matches its meaning.
26. As a Beekeeper, I want a killed trail to abandon pending work and never resume it after a top-up, so that kill remains the hard stop.
27. As a Beekeeper, I want the final review gate not to open for pending, `skipped`, or still-incomplete tasks, so that honey exhaustion and supersession are never presented as completed work.
28. As a Beekeeper, I want request-changes eligibility to treat `skipped` work as not in flight, so that a stale tick cannot block a new review action.
29. As a Beekeeper, I want task and trail metrics to distinguish pending, `skipped`, and legacy `blocked`, so that trail cost and rework counts remain interpretable.
30. As a Beekeeper, I want the reactor to resume add-approved pending work after a crash, so that a top-up committed to the ledger is not lost between apply and dispatch.
31. As a Beekeeper, I want the reactor never to resume stipend-superseded work after a crash, so that recovery preserves the tick boundary.
32. As a Bee, I want `energy.paused` and `energy.resumed` excluded from direct routing, so that an observation cannot wake me as work.
33. As a colony author, I want this to remain ledger and reactor behavior, without a new orchestrator or queue service, so that choreography stays event-driven.
34. As an implementer, I want older snapshots without pending fields to decode unchanged, so that the protocol change is additive.
35. As an implementer, I want pending transitions to use the ledger's compare-and-set path, so that concurrent deferral, top-up, and kill cannot overwrite each other.

## Implementation Decisions

### 1. Authority and vocabulary

1. `task.status` describes task lifecycle only. Energy exhaustion must not write `ready`, `failed`, `running`, `waiting_review`, or `blocked`.
2. `blocked` remains a protocol status. New runtime code does not use it for energy. It stays readable for legacy honey-blocked tasks and remains available for a future task-level block reason.
3. `skipped` is added as a terminal task status. It means eligible work was deliberately not delivered because a new allocation superseded it. It is not a failure, cancellation, or completion.
4. Trail energy remains the existing shared reserve fields. Admission is derived from `energyRemaining`; a durable pause episode exists only to identify and deduplicate observations.
5. Pending work and pause observations are runtime state, not a task state machine and not a second scheduler.

### 2. Pending dispatch record

1. A pending dispatch is stored in the trail ledger and survives restart. Its durable shape carries:
   - stable id and a dedupe key;
   - kind: task or direct;
   - task id and Bee role when known;
   - source event identity for direct work;
   - reason, status, creation time, and update time;
   - the allocation identity that makes it resumable, so restart can distinguish an `energy.add` from a stipend.
2. Pending statuses are `pending`, `consumed`, `abandoned`, and `unresolved`. `unresolved` means the source event could not be recovered; it is visible and is not retried as if it were runnable.
3. A direct pending entry stores only the source event identity, not a copy of the payload. Resume recovers the event from the trail record / bus replayer and calls the normal direct dispatch path without re-applying the event to the ledger.
4. A task pending entry stores the task id and Bee. Resume reads the current task snapshot rather than replaying a task.ready event.
5. Pending entries are deduplicated while active. Re-deferring the same task reaction or the same direct dispatch identity adds no second entry.
6. Entries drain in creation order, oldest first, with the stable id breaking ties.
7. Pending transitions are written through the ledger's compare-and-set mutation path, not by rewriting the whole snapshot from the reactor.

### 3. Deferral

1. The task and direct admission gates are separate. Both consume one honey per admitted dispatch; neither lets energy write a task status.
2. When admission fails for energy, the runtime records a pending entry and returns without starting the adapter.
3. When admission fails for a different reason, the runtime does not record pending energy work. Configuration admission, attempt closure, kill, duplicate dispatch, and publisher-is-same-Bee checks retain their existing outcomes.
4. A direct deferral records the event identity returned by the existing event identity helper, so restart can find the exact source event.
5. A task deferral records the task id and effective Bee. It does not change the task's `ready` or other lifecycle status.
6. A killed trail records no new pending work.

### 4. Resume

1. Only `energy.add` makes pending work runnable. `energy.stipend` follows the standing rules below.
2. The runtime drains pending work oldest-first until the reserve is exhausted or the queue is empty. Each admitted dispatch consumes one honey through the existing `energy.consume` path.
3. Task pending work resumes through the normal task dispatch path with its in-flight, attempt, configuration, and kill guards intact.
4. Direct pending work recovers the source event by identity and runs the normal direct dispatch path. The event must not be replayed through the general event processor, because it was already applied and some reducers are not idempotent.
5. If the source event is missing, the entry becomes `unresolved` with a diagnostic; it is not silently deleted and not retried forever.
6. If a task has closed or changed to a terminal state while pending, the entry becomes `abandoned` rather than dispatching stale work.
7. A dispatch that re-defers because the reserve ran out during the drain leaves the entry pending. The drain does not report a false resume.
8. A successful resume marks the entry `consumed`. At-least-once recovery is acceptable only when the normal direct/task dedupe cannot prove completion; the runtime must not create duplicate live dispatches in the same process.
9. The ledger records that an entry was made resumable by a specific energy increase. On reactor startup, add-approved pending work may resume; stipend-superseded work must already be abandoned and must not resume.
10. `system.kill` abandons pending work for the trail. A later top-up cannot revive it.

### 5. Standing trails and `skipped`

1. `energy.stipend` never drains pending work. It starts a new allocation for a new tick.
2. Before or as the stipend is applied, active pending work from the previous tick is closed:
   - task pending work becomes an `abandoned` pending entry and its task becomes `skipped` with a summary naming the superseded allocation;
   - direct pending work becomes `abandoned`.
3. `skipped` is terminal for standing overlap: it does not occupy a tick and does not prevent the next tick.
4. `skipped` is not completed work: it does not satisfy `AllAFKTasksCompleted`, does not open a final gate, and does not count as a task execution or retry.
5. `skipped` is not in flight for rework: it cannot block request-changes eligibility.
6. A first standing seed behaves like a normal initial allocation; there is no previous tick to abandon.
7. A killed standing trail still refuses stipend and ingress.

### 6. Pause and resume observations

1. `SIGNAL/energy.paused` is a platform observation. It is emitted once when the first dispatch of an episode is deferred, and carries a stable episode identity, reason, remaining/budget, and current pending count.
2. `SIGNAL/energy.resumed` is emitted when admission opens again after an energy increase and at least one pending item becomes runnable or the pause episode closes. It carries the episode identity, source (`energy.add` or `energy.stipend`), remaining/budget, pending counts before and after, and abandoned count when a stipend superseded work.
3. A stipend closes the previous pause episode by abandoning pending work, even though it does not resume that work. The resumed observation must say that explicitly through its source and abandoned counts.
4. These observations never seed, add, consume, defer, or resume anything themselves. Replaying or receiving one is observational only.
5. They are live-only platform signals: excluded from direct dispatch, from deferred-event buffers, and from auto-invite routing.
6. The ledger or an equivalent durable episode record carries the active pause identity so a Reactor restart does not publish the same observation twice.
7. If the reserve remains exhausted after a partial drain, the pause episode stays active and no false `energy.resumed` is published.

### 7. Surfaces

1. Queen Console task rows show the task's real status. A trail-level paused badge and pending count appear beside the trail, not as a fake task status.
2. `paseka energy show` prints whether the trail is paused, the pending count, and the reason; `paseka status` attention uses the same projection.
3. Telegram subscribes to `energy.paused` / `energy.resumed` for its energy category and stops synthesizing an energy alert from a task status.
4. Metrics and exports distinguish pending, `skipped`, and legacy `blocked` work. A skipped task contributes no execution or retry.
5. Existing legacy honey-blocked cards remain readable until purge, but new energy deferrals do not create them.

### 8. Compatibility and rollout

1. Snapshot and event changes are additive. Older snapshots without pending or pause fields read as empty, unpaused state.
2. No ledger migration rewrites `blocked` tasks. Existing honey-blocked tasks keep the current unblock-on-`energy.add` behavior until they are purged or naturally resolved.
3. The direct-dispatch fix may land first. The remaining behavior is staged behind the same concepts and does not require a protocol version bump.
4. Eval expectations that currently assert "energy exhaustion makes a task blocked" move to pending/`skipped` expectations for new trails; legacy fixtures may keep `blocked`.
5. Documentation updates replace the task-status story with the admission/pending story and cross-link the legacy note.

## Testing Decisions

Tests assert external behavior, persisted ledger state, published observations, and adapter calls. They do not assert private helper structure.

- **Task deferral:** a ready task with no honey stays `ready`, has one pending entry, starts no adapter, and becomes `running` only after `energy.add`. Repeated deferral creates no duplicate.
- **Direct deferral:** a `waiting_review` task whose guard, receiver, or fix-up reaction is denied stays `waiting_review`, keeps its proposal/review evidence, records one direct pending entry, and starts no adapter.
- **Resume routing:** after `energy.add`, the same guard/receiver/fix-up Bee runs, not the task's assigned builder. The resumed direct event is not applied twice to the ledger.
- **Ordering:** three pending entries added out of chronological order drain oldest-first with a stable id tie-break.
- **Durability and restart:** pending work survives a new reactor over the same ledger; add-approved work resumes; stipend-superseded work does not.
- **Partial top-up:** enough honey for one entry resumes one and leaves the rest pending without a false resume observation.
- **Missing source event:** a direct pending entry whose event is absent becomes `unresolved` and remains visible; it is neither dispatched nor deleted.
- **Kill:** `system.kill` abandons pending work; later `energy.add` does not dispatch it.
- **Standing:** a previous tick's deferred task becomes `skipped` after stipend; the next tick starts; direct pending from the old tick is abandoned; a task `skipped` by one tick is not counted as completed, failed, cancelled, or in-flight.
- **Legacy:** an old `blocked` + `Honey reserve exhausted` task still unblocks on `energy.add`; no new energy path writes `blocked`.
- **Observations:** exactly one paused observation per episode and one resumed observation per closing increase, including after restart; the observations do not wake Bees and do not change reserve.
- **Surfaces:** Console, CLI, Telegram, and metrics show paused/pending/`skipped` without presenting a false task `blocked`; legacy cards still render.

Prior art: task-ledger reducer and energy tests, runtime energy block/unblock tests, direct-dispatch routing and dedupe tests, pending-dispatch persistence tests, standing-tick overlap tests, review final-gate and rework tests, Telegram task-status tests, and Console/hiveview task projection tests.

## Out of Scope

- Per-task honey budgets, changing the one-token dispatch cost, or charging for work that was never admitted.
- Automatic top-ups, scheduled stipend changes, or a new queue service outside the ledger and reactor.
- A manual task-block UI or a taxonomy of task-level block reasons. `blocked` stays available but is not redesigned here.
- Migrating historical ledger snapshots or rewriting old `blocked` tasks.
- Replacing the task scheduler or changing which task is eligible next.
- New delivery, merge, or pull-request semantics.
- A full pending-work browser beyond counts and diagnostics; the Console may show the queue minimally.

## Further Notes

- **Relationship to existing specs.** [013-system-kill](013-system-kill.md) owns the hard stop; this spec only abandons pending work on kill. [028-standing-trails](028-standing-trails.md) owns stipend and tick overlap; this spec replaces its reliance on `blocked` with `skipped` and pending abandonment. [031-task-scheduler](031-task-scheduler.md) owns task eligibility order; pending drain uses creation order. [043-trail-observability](043-trail-observability.md) owns honey metrics and completion truth; this spec supplies the pending/skipped vocabulary it needs. [003-hive-evals](003-hive-evals.md) owns eval cases and must update energy expectations. [002-queen-console-mvp](002-queen-console-mvp.md), [010-telegram-human-gateway](010-telegram-human-gateway.md), and [018-cli-colony-status](018-cli-colony-status.md) own the operator surfaces.
- **Staging.** The direct-dispatch half is implemented first: direct reactions no longer write `blocked`, pending direct work is durable, and `energy.add` recovers the source event. This spec records the remaining task-dispatch, standing, and observation work as one coherent model rather than another local patch.
- **Resolved decisions.** Standing supersession uses a new `skipped` status. Direct pending work stores only the source event identity. Pending drain uses creation order. Pause/resume use separate `SIGNAL/energy.paused` and `SIGNAL/energy.resumed` observations. Legacy honey-blocked tasks remain readable until purge.
- **Why not keep `blocked`.** A task-level status cannot represent a shared-reserve condition, cannot carry a direct reaction, and cannot preserve `waiting_review`. Keeping it would require storing prior state and source context anyway; at that point `blocked` is a deferred-work record wearing a task-status name.
