# Spec 044b: Standing supersession and energy observations

## Status

**(Implemented)**
Task dispatch now leaves `blocked`: a honey-denied task dispatch records a durable pending entry and keeps its lifecycle status, `energy.add` drains task and direct work oldest-first, an `energy.stipend` atomically supersedes the previous tick's energy-deferred work as `skipped`/`abandoned`, the standing overlap check ignores a task with an active energy-deferred entry, and one `SIGNAL` / `energy.paused` / `energy.resumed` pair per episode replaces the fake task-blocked alert. Legacy honey-blocked tasks stay readable until purged. Builds on [044a](044a-direct-pending-dispatch.md).

## Problem Statement

A Flight Trail has one shared honey reserve, but task dispatch still writes the reserve's exhaustion into `task.status=blocked`. The two concepts are not the same: a task status describes one unit of work, while honey belongs to the whole trail. Energy exhaustion therefore invents a task defect and makes a `waiting_review` or `running` task lie about its stage.

Standing trails depend on the confusion as well: a previous tick's `blocked` task is what lets the next tick start, even though the tick boundary is a new allocation rather than a task failure. If the runtime simply stops writing `blocked`, a deferred task stays `ready` and the overlap check refuses the next tick.

[044a](044a-direct-pending-dispatch.md) fixed direct reactions: they no longer touch task status, pending direct work is durable, a top-up resumes the same reaction, missing evidence becomes unresolved, and stipend/kill abandon the queue. This spec completes the model for task dispatches and standing ticks, and gives operators a truthful pause signal instead of a fake task block.

## Solution

1. **Task lifecycle stays authoritative.** A task dispatch denied by honey records a pending entry and keeps the task's lifecycle status; `ready` still means eligible.
2. **The trail owns deferred task work.** `energy.add` drains task and direct pending work in creation order.
3. **Standing ticks get an honest terminal outcome.** `energy.stipend` starts a new allocation and never drains the previous tick's queue. In the same ledger mutation it abandons active task pending work, marks those tasks `skipped`, and abandons active direct pending work.
4. **The overlap check recognizes deferred work.** A task with an active energy-deferred entry no longer occupies a standing tick; a configuration-barrier hold still does.
5. **The trail exposes a pause, not a fake task block.** `SIGNAL/energy.paused` and `SIGNAL/energy.resumed` tell operators and Human Gateway surfaces when the trail stopped admitting dispatches and when it can admit them again.
6. **Legacy stays readable.** Existing honey-blocked tasks keep their old meaning until purge. No historical ledger is rewritten.

## User Stories

1. As a Beekeeper, I want a task dispatch denied by honey to remain in its task lifecycle state rather than becoming `blocked`, so that honey is not presented as a task defect.
2. As a Beekeeper, I want a task dispatch that was never admitted to remain visibly pending, so that `ready` still means "eligible" and the trail shows why nothing started.
3. As a Beekeeper, I want `energy.add` to start the deferred task dispatch, so that a top-up continues the trail without inventing a task failure.
4. As a Beekeeper, I want repeated deferrals of the same task reaction to produce one pending entry, so that duplicate bus delivery cannot multiply dispatches.
5. As a Beekeeper, I want an `energy.add` that is still too small to leave the remaining task work pending, so that the trail does not claim to have resumed everything.
6. As a Beekeeper, I want a paused trail to publish one `energy.paused` observation per pause episode, so that notifications do not repeat for every deferred item.
7. As a Beekeeper, I want a resumed trail to publish one `energy.resumed` observation when the reserve can admit work again, so that the Human Gateway can close the alert without polling.
8. As a Beekeeper, I want pause/resume observations to identify whether an `energy.add` or a standing `energy.stipend` changed admission, so that I can tell a live top-up from a new tick.
9. As a Beekeeper, I want pause/resume observations to survive restart without duplicate cards, so that a Reactor bounce does not spam me.
10. As a Beekeeper, I want a standing `energy.stipend` not to resume the previous tick's pending work, so that a new tick is new work rather than a resurrection.
11. As a Beekeeper, I want a previous standing tick's deferred task to become `skipped`, so that the next tick is not refused as busy and I can still see that the old tick did not run.
12. As a Beekeeper, I want `skipped` to be distinct from `failed`, `cancelled`, and `completed`, so that a superseded allocation is not counted as a defect, a kill, or a success.
13. As a Beekeeper, I want a standing stipend to abandon the previous tick's pending direct reactions, so that a new tick cannot accidentally run an old guard or receiver.
14. As a Beekeeper, I want a killed trail to abandon pending task work and never resume it after a top-up, so that kill remains the hard stop.
15. As a Beekeeper, I want `paseka energy add` to work the same for task and direct pending work, so that the recovery rule does not depend on the dispatch path.
16. As a Beekeeper, I want Queen Console to show the trail's paused state and pending count beside the true task statuses, so that the board no longer lies about review work.
17. As a Beekeeper, I want `paseka energy show` to print pause and pending information, so that a shell operator sees the same state as the Console.
18. As a Beekeeper, I want Telegram to send energy pause and resume cards instead of fake task-blocked cards, so that the alert's action matches its meaning.
19. As a Beekeeper, I want the final review gate not to open for pending, `skipped`, or still-incomplete tasks, so that honey exhaustion and supersession are never presented as completed work.
20. As a Beekeeper, I want request-changes eligibility to treat `skipped` work as not in flight, so that a stale tick cannot block a new review action.
21. As a Beekeeper, I want task and trail metrics to distinguish pending, `skipped`, and legacy `blocked`, so that trail cost and rework counts remain interpretable.
22. As a Beekeeper, I want the reactor never to resume stipend-superseded task work after a crash, so that recovery preserves the tick boundary.
23. As a Bee, I want `energy.paused` and `energy.resumed` excluded from direct routing, so that an observation cannot wake me as work.
24. As a colony author, I want this to remain ledger and reactor behavior, without a new orchestrator or queue service, so that choreography stays event-driven.
25. As an implementer, I want the task pending record to keep its source event identity only for attempt dedupe, so that resume reads the current task instead of replaying an old event.

## Implementation Decisions

This spec builds on the `PendingDispatch` record, statuses, compare-and-set writes, drain order, startup-resume condition, kill abandonment, and direct deferral defined in [044a](044a-direct-pending-dispatch.md). Only the task-specific and standing additions are repeated here.

### 1. Authority and vocabulary

1. `task.status` describes task lifecycle only. Energy exhaustion must not write `ready`, `failed`, `running`, `waiting_review`, or `blocked`.
2. `blocked` remains a protocol status. New runtime code does not use it for energy. It stays readable for legacy honey-blocked tasks and remains available for a future task-level block reason.
3. `skipped` is added as a terminal task status. It means eligible work was deliberately not delivered because a new allocation superseded it. It is not a failure, cancellation, or completion.
4. Trail energy remains the existing shared reserve fields. Admission is derived from `energyRemaining`; a durable pause episode exists only to identify and deduplicate observations.
5. Pending work and pause observations are runtime state, not a task state machine and not a second scheduler.

### 2. Pending dispatch record

1. A task pending entry carries the task id, the effective Bee, and the source event identity. The source identity is used only as the attempt dispatch marker; resume reads the current task snapshot rather than replaying a task.ready event.
2. Pending task entries are deduplicated while active on `task:<taskId>:<bee>`. Re-deferring the same task reaction adds no second entry.
3. Task and direct entries share one creation-order queue, oldest first, with the stable id breaking ties.
4. Task transitions use the same compare-and-set mutation path as direct transitions.
5. The statuses `pending`, `consumed`, `abandoned`, and `unresolved` are inherited from 044a.

### 3. Deferral

1. The task and direct admission gates are separate. Both consume one honey per admitted dispatch; neither lets energy write a task status.
2. When task admission fails for energy, the runtime records a pending entry and returns without starting the adapter. It does not change the task's `ready` or other lifecycle status.
3. When admission fails for a different reason, the runtime does not record pending energy work. Configuration admission, attempt closure, kill, duplicate dispatch, and publisher-is-same-Bee checks retain their existing outcomes. A configuration-barrier hold keeps its existing distinct reason and is never drained by the energy path.
4. A task deferral records the task id, effective Bee, and source event identity, keyed by `task:<taskId>:<bee>`.
5. A killed trail records no new pending work.

### 4. Resume

1. Only an `energy.add` increase makes pending task work runnable, including an increase applied to the ledger while the reactor was down. `energy.stipend` supersedes task pending work instead of draining it.
2. Task pending work resumes through the normal task dispatch path with its in-flight, attempt, configuration, and kill guards intact.
3. If a task has closed or changed to a terminal state while pending, the entry becomes `abandoned` rather than dispatching stale work.
4. A successful resume marks the entry `consumed`. At-least-once recovery is acceptable only when the normal task dedupe cannot prove completion; the runtime must not create duplicate live dispatches in the same process.
5. A dispatch that re-defers because the reserve ran out during the drain leaves the entry pending. The drain does not report a false resume.
6. `system.kill` abandons pending task work for the trail in the same ledger mutation that marks the trail killed. A later top-up cannot revive it.
7. The startup condition and trail enumeration from 044a apply to task entries unchanged: an active entry plus a positive reserve is resumable, and killed or zero-reserve trails are skipped.

### 5. Standing trails and `skipped`

1. `energy.stipend` never drains pending work. It starts a new allocation for a new tick.
2. As the stipend is applied, active pending work from the previous tick is closed:
   - task pending work becomes an `abandoned` pending entry and its task becomes `skipped` with a summary naming the superseded allocation;
   - direct pending work becomes `abandoned`.
3. The standing overlap check treats a task with an active energy-deferred task entry as not occupying a tick, so the previous tick no longer refuses the next one. A configuration-barrier hold is not an energy deferral and keeps the tick open. The check still runs before the stipend is published.
4. Supersession is performed by the ledger reducer for `energy.stipend`, in the same compare-and-set mutation that applies the stipend. A crash cannot leave a resumable previous-tick entry behind, and a duplicate stipend finds nothing left to abandon.
5. `skipped` is terminal for standing overlap: it does not occupy a tick and does not prevent the next tick.
6. `skipped` is not completed work: it does not satisfy the work or pre-merge completion predicates, does not open a final gate, and does not count as a task execution or retry.
7. `skipped` is not in flight for rework: it cannot block request-changes eligibility.
8. A first standing seed behaves like a normal initial allocation; there is no previous tick to abandon.
9. A killed standing trail still refuses stipend and ingress.

### 6. Pause and resume observations

1. `SIGNAL/energy.paused` is a platform observation. It is emitted once when the first energy-caused dispatch of an episode is deferred, and carries a stable episode identity, reason, remaining/budget, and current pending count.
2. `SIGNAL/energy.resumed` is emitted whenever an active pause episode closes after an energy increase — even when no pending item became runnable. It carries the episode identity, source (`energy.add` or `energy.stipend`), remaining/budget, pending counts before and after, dispatched count, and abandoned count when a stipend superseded work.
3. A stipend closes the previous pause episode by abandoning pending work, even though it does not resume that work. The resumed observation must say that explicitly through its source and abandoned counts.
4. These observations never seed, add, consume, defer, or resume anything themselves. Replaying or receiving one is observational only.
5. They are live-only platform signals: excluded from direct dispatch, from deferred-event buffers, and from auto-invite routing.
6. The active episode is a field in the trail snapshot (`EnergyPause`: episode identity, start time, remaining, pending count, and a published flag), written through the ledger's compare-and-set path. The field is opened unpublished in the same mutation as the first energy-caused deferral; a follow-up compare-and-set marks it published before the send, so a crash between the two leaves it recoverable by the startup rule. The field is cleared when the episode closes. The paused observation is marked published before it is sent, and the resumed observation is sent after the field is cleared, so a restart can lose at most one notification and never duplicates one. On startup, an open episode that was never published is published before draining.
7. If the reserve remains exhausted after a partial drain, the pause episode stays active and no false `energy.resumed` is published.

### 7. Surfaces

1. Queen Console task rows show the task's real status. A trail-level paused badge and pending count appear beside the trail, not as a fake task status.
2. `paseka energy show` prints whether the trail is paused, the pending count, and the reason; `paseka status` attention uses the same projection.
3. Telegram subscribes to `energy.paused` / `energy.resumed` for its energy category and stops synthesizing an energy alert from a task status.
4. Metrics and exports distinguish pending, `skipped`, and legacy `blocked` work. A skipped task contributes no execution or retry. The metrics half depends on the trail metrics surface owned by [043](043-trail-observability.md); until that lands, existing exports and the Console/CLI projections carry the distinction.
5. Existing legacy honey-blocked cards remain readable until purge, but new energy deferrals do not create them.

### 8. Compatibility and rollout

1. Snapshot and event changes are additive. Older snapshots without pending or pause fields read as empty, unpaused state.
2. No ledger migration rewrites `blocked` tasks. Existing honey-blocked tasks keep the current unblock-on-`energy.add` behavior until they are purged or naturally resolved.
3. This spec depends on [044a](044a-direct-pending-dispatch.md). Task deferral, `skipped`, standing supersession, and pause observations land here and do not require a protocol version bump.
4. Eval expectations that currently assert "energy exhaustion makes a task blocked" move to pending/`skipped` expectations for new trails; legacy fixtures may keep `blocked`.
5. Documentation updates replace the task-status story with the admission/pending story and cross-link the legacy note.

### 9. Resolved implementation decisions

1. **Standing overlap.** The overlap predicate ignores a task with an active energy-deferred task entry; a configuration-barrier hold still keeps the tick open. The check stays before the stipend, so supersession remains owned by the stipend.
2. **Stipend supersession.** Performed atomically by the ledger reducer for `energy.stipend`, including abandoned pending entries, `skipped` tasks, and the pause episode close. Direct pending abandonment is inherited from 044a.
3. **Startup resume.** Inherited from 044a: an active entry plus a positive reserve is the resumability condition because stipend abandonment is atomic.
4. **Pause episode.** Stored in the trail snapshot as `EnergyPause`, opened with the first energy-caused deferral, and cleared on close. Publication is mark-before-send for `energy.paused` and clear-before-send for `energy.resumed`.
5. **Task resume identity.** A task pending entry keeps the source event identity only as the attempt dispatch marker; the task payload comes from the current snapshot, and the entry dedupes on `task:<taskId>:<bee>`.

## Testing Decisions

Tests assert external behavior, persisted ledger state, published observations, and adapter calls. They do not assert private helper structure. Direct-only cases live in 044a.

- **Task deferral:** a ready task with no honey stays `ready`, has one pending entry, starts no adapter, and becomes `running` only after `energy.add`. Repeated deferral creates no duplicate.
- **Task resume identity:** a resumed task dispatch reuses the stored source event identity as its attempt marker; a duplicate `task.ready` while pending adds no entry and keeps the first identity.
- **Combined ordering:** task and direct entries added out of chronological order drain oldest-first with a stable id tie-break.
- **Durability and restart:** pending task work survives a new reactor over the same ledger; add-approved work resumes; stipend-superseded work does not.
- **Partial top-up:** enough honey for one entry resumes one and leaves the rest pending without a false resume observation.
- **Kill:** `system.kill` abandons pending task work; later `energy.add` does not dispatch it.
- **Standing:** a previous tick's deferred task becomes `skipped` after stipend; the next tick starts; direct pending from the old tick is abandoned; a task `skipped` by one tick is not counted as completed, failed, cancelled, or in-flight.
- **Standing overlap:** a previous tick's energy-deferred `ready` task does not refuse the next tick; a configuration-barrier-held task still does, and the stipend is published only after a clean overlap check.
- **Atomic supersession:** a stipend abandons active pending work in the same ledger mutation; a duplicate stipend finds nothing to abandon, and no crash can leave superseded work resumable.
- **Legacy:** an old `blocked` + `Honey reserve exhausted` task still unblocks on `energy.add`; no new energy path writes `blocked`.
- **Observations:** exactly one paused observation per episode and one resumed observation per closing increase, including after restart; an unpublished pause is published once on startup; the observations do not wake Bees and do not change reserve.
- **Surfaces:** Console, CLI, Telegram, and metrics show paused/pending/`skipped` without presenting a false task `blocked`; legacy cards still render.

Prior art: task-ledger reducer and energy tests, runtime energy block/unblock tests, pending-dispatch persistence tests, standing-tick overlap tests, review final-gate and rework tests, Telegram task-status tests, and Console/hiveview task projection tests.

## Out of Scope

- The direct pending record, statuses, startup resume, kill abandonment, and direct resume behavior — [044a](044a-direct-pending-dispatch.md) owns them.
- Per-task honey budgets, changing the one-token dispatch cost, or charging for work that was never admitted.
- Automatic top-ups, scheduled stipend changes, or a new queue service outside the ledger and reactor.
- A manual task-block UI or a taxonomy of task-level block reasons. `blocked` stays available but is not redesigned here.
- Migrating historical ledger snapshots or rewriting old `blocked` tasks.
- Replacing the task scheduler or changing which task is eligible next.
- New delivery, merge, or pull-request semantics.
- A full pending-work browser beyond counts and diagnostics; the Console may show the queue minimally.

## Further Notes

- **Relationship to 044a.** [044a](044a-direct-pending-dispatch.md) defines the durable pending record, the direct deferral/resume path, the `abandoned`/`unresolved` statuses, kill abandonment, startup resume, and the direct stipend abandonment this spec extends to task entries. This spec must not duplicate or diverge from that record.
- **Relationship to existing specs.** [013-system-kill](013-system-kill.md) owns the hard stop; this spec only abandons pending work on kill. [028-standing-trails](028-standing-trails.md) owns stipend and tick overlap; this spec replaces its reliance on `blocked` with `skipped`, pending abandonment, and an overlap predicate that ignores energy-deferred tasks. [031-task-scheduler](031-task-scheduler.md) owns task eligibility order; pending drain uses creation order. [043-trail-observability](043-trail-observability.md) owns honey metrics and completion truth; this spec supplies the pending/skipped vocabulary it needs. [003-hive-evals](003-hive-evals.md) owns eval cases and must update energy expectations. [002-queen-console-mvp](002-queen-console-mvp.md), [010-telegram-human-gateway](010-telegram-human-gateway.md), and [018-cli-colony-status](018-cli-colony-status.md) own the operator surfaces.
- **Staging.** 044a lands first with direct work and the legacy `blocked` task path intact. This spec then flips task dispatch to the pending model, adds `skipped`, and adds pause/resume observations together, so energy waiting never becomes silent and the standing overlap change lands with the supersession that makes it safe.
- **Why not keep `blocked`.** A task-level status cannot represent a shared-reserve condition, cannot carry a direct reaction, and cannot preserve `waiting_review`. Keeping it would require storing prior state and source context anyway; at that point `blocked` is a deferred-work record wearing a task-status name.
