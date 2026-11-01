# Spec 044a: Direct pending dispatch

## Status

**(Implemented)**
The direct admission half shipped: pending direct dispatches now carry `abandoned` and `unresolved` alongside `pending` and `consumed`, a missing source event becomes visibly `unresolved` instead of retrying, `system.kill` abandons active pending direct work and `energy.stipend` abandons the energy-deferred part of it, each in the same ledger mutation that applies them, and reactor startup enumerates trails from the ledger and resumes work an `energy.add` already approved. Task dispatch and the standing tick lifecycle stay on the legacy `blocked` path; [044b](044b-standing-supersession-and-energy-observations.md) owns that half.

## Problem Statement

A direct dispatch is a reaction to an event. It may run while its task is `waiting_review` or `running`, so it is not a task lifecycle transition. Writing honey exhaustion into `task.status` destroys the real review or execution state: a later `energy.add` resumes the task's assigned Bee instead of the blocked guard, receiver, or fix-up Bee, and the source event is lost.

The shipped direct path already records a durable pending entry without touching the task, but it leaves four gaps:

- a pending entry whose source event is gone is retried forever instead of becoming unresolved;
- `system.kill` never closes pending work;
- an `energy.add` applied while the reactor is down is never drained;
- `energy.stipend` can let a later top-up resurrect a previous allocation's direct reactions.

Task dispatch and the standing tick lifecycle still use the legacy `blocked` path; [044b](044b-standing-supersession-and-energy-observations.md) owns that half.

## Solution

1. **Direct reactions defer, never write task status.** When honey cannot pay for a direct dispatch, the runtime records a durable `PendingDispatch` and returns.
2. **The trail owns deferred direct work.** `energy.add` drains pending direct dispatches in creation order, oldest first.
3. **Top-up and restart drain the queue.** A top-up applied while the reactor was down is resumed on the next reactor start; deferred work survives a restart.
4. **Missing evidence is visible.** A pending direct dispatch whose source event cannot be recovered becomes `unresolved`; it is not silently deleted and not retried as if it were runnable.
5. **Stipend and kill close the queue.** `energy.stipend` abandons active energy-deferred direct pending work because it starts a new allocation; `system.kill` abandons all active direct pending work because kill is a hard stop. Neither a later top-up nor a restart revives it. Configuration-held work belongs to its pending transition, not to an allocation, so a stipend leaves it alone.
6. **Legacy stays readable.** Existing honey-blocked tasks keep their current unblock-on-`energy.add` behavior until purge. This slice does not change task dispatch.

## User Stories

1. As a Beekeeper, I want a task in `waiting_review` to stay in `waiting_review` when its direct reviewer cannot run because honey is exhausted, so that the ledger still tells me what stage the work reached.
2. As a Beekeeper, I want `energy.add` to resume the guard, receiver, or fix-up Bee that was actually blocked, so that a top-up continues the choreography instead of restarting the builder.
3. As a Beekeeper, I want the blocked source event to be recoverable from the trail record, so that a resumed direct dispatch is the same reaction, not a fabricated one.
4. As a Beekeeper, I want deferred direct work to survive a reactor restart, so that restarting the hive does not silently forget blocked work.
5. As a Beekeeper, I want pending direct work drained oldest-first, so that a top-up continues the queue in the order the trail attempted it.
6. As a Beekeeper, I want repeated deferrals of the same direct reaction to produce one pending entry, so that duplicate bus delivery cannot multiply dispatches.
7. As a Beekeeper, I want a repeated `energy.add` not to dispatch the same pending work twice, so that a retried top-up is safe.
8. As a Beekeeper, I want an `energy.add` that is still too small to leave the remaining direct work pending, so that the trail does not claim to have resumed everything.
9. As a Beekeeper, I want a direct dispatch whose source event can no longer be recovered to become visibly unresolved, so that missing evidence is not mistaken for successful work.
10. As a Beekeeper, I want the reactor to resume add-approved direct pending work after a crash, so that a top-up committed to the ledger is not lost between apply and dispatch.
11. As a Beekeeper, I want a killed trail to abandon pending direct work and never resume it after a top-up, so that kill remains the hard stop.
12. As a Beekeeper, I want a standing stipend to abandon the previous tick's pending direct reactions, so that a new tick cannot accidentally run an old guard or receiver.
13. As a Beekeeper, I want `paseka energy add` to recover direct pending work, so that the recovery rule does not depend on which process applies the top-up.
14. As a Beekeeper, I want an existing honey-blocked task from an older ledger to keep working until purge, so that upgrading the runtime does not strand old trails.
15. As a Beekeeper, I want `blocked` to remain available for genuine task-level blockers, so that a future manual or dependency block does not need another status name.
16. As a Bee, I want deferred direct work to resume the same reaction target, so that a guard is never replaced by the task's builder.
17. As an implementer, I want older snapshots without pending fields to decode unchanged, so that the protocol change is additive.
18. As an implementer, I want pending transitions to use the ledger's compare-and-set path, so that concurrent deferral, top-up, and kill cannot overwrite each other.
19. As a colony author, I want this to remain ledger and reactor behavior, without a new orchestrator or queue service, so that choreography stays event-driven.

## Implementation Decisions

### 1. Authority and vocabulary

1. `task.status` describes task lifecycle only. Direct energy exhaustion must not write `ready`, `failed`, `running`, `waiting_review`, or `blocked`.
2. `blocked` remains a protocol status. This slice does not use it for direct dispatch; task dispatch keeps its existing energy-blocked path until 044b, and legacy honey-blocked tasks stay readable.
3. This slice does not add `skipped`; standing supersession owns that status in 044b.
4. Trail energy remains the existing shared reserve fields. Admission is derived from `energyRemaining`. There is no pause episode in this slice.
5. Pending direct work is runtime state, not a task state machine and not a second scheduler.

### 2. Pending dispatch record

1. A pending dispatch is stored in the trail ledger and survives restart. Its durable shape carries:
   - a stable id that also serves as the dedupe key;
   - kind: `direct` in this slice (`task` is reserved for 044b);
   - task id and Bee role when known;
   - source event identity;
   - reason, status, creation time, and update time.
2. Pending statuses are `pending`, `consumed`, `abandoned`, and `unresolved`. `unresolved` means the source event could not be recovered; it is visible and is not retried as if it were runnable.
3. A direct pending entry stores only the source event identity, not a copy of the payload. Resume recovers the event from the trail record or bus replayer and calls the normal direct dispatch path without re-applying the event to the ledger.
4. Pending direct entries are deduplicated while active on the existing direct dispatch key. Re-deferring the same direct dispatch identity adds no second entry.
5. Entries drain in creation order, oldest first, with the stable id breaking ties.
6. Pending transitions are written through the ledger's compare-and-set mutation path, not by rewriting the whole snapshot from the reactor.

### 3. Deferral

1. The direct admission gate consumes one honey per admitted dispatch and never lets energy write a task status.
2. When direct admission fails for energy, the runtime records a pending entry and returns without starting the adapter.
3. When admission fails for a different reason, the runtime does not record pending energy work. Configuration admission, attempt closure, kill, duplicate dispatch, and publisher-is-same-Bee checks retain their existing outcomes. A configuration-barrier hold keeps its existing distinct reason and is never drained or abandoned by the energy path: the pending configuration transition owns it.
4. A direct deferral records the event identity returned by the existing event identity helper, so restart can find the exact source event.
5. A killed trail records no new pending work.

### 4. Resume

1. Only an `energy.add` increase makes pending direct work runnable. Startup resume drains work whose `energy.add` was already applied to the ledger while the reactor was down. `energy.stipend` never drains direct pending work; it abandons the energy-deferred part as a new allocation.
2. The runtime drains pending direct work oldest-first until the reserve is exhausted or the queue is empty. Each admitted dispatch consumes one honey through the existing energy-consume path.
3. Direct pending work recovers the source event by identity and runs the normal direct dispatch path. The event must not be replayed through the general event processor, because it was already applied and some reducers are not idempotent.
4. If the source event is missing, the entry becomes `unresolved` with a diagnostic; it is not silently deleted and not retried forever.
5. A dispatch that re-defers because the reserve ran out during the drain leaves the entry pending. The drain does not report a false resume.
6. A successful resume marks the entry `consumed`. At-least-once recovery is acceptable only when the normal direct dedupe cannot prove completion; the runtime must not create duplicate live dispatches in the same process.
7. An active energy-deferred entry with a positive reserve is resumable. Stipend abandons energy-deferred entries atomically, so anything still awaiting energy belongs to the current allocation; configuration-held entries are outside any allocation and stay with their transition. On reactor startup the runtime enumerates trails from the ledger, skips killed trails and trails without reserve, and drains active work. Stipend-superseded work is already abandoned and never resumes.
8. `system.kill` abandons pending direct work for the trail in the same ledger mutation that marks the trail killed. A later top-up cannot revive it.
9. `energy.stipend` abandons active energy-deferred direct pending work in the same ledger mutation (the stipend reducer) that applies the stipend. A later `energy.add` cannot resurrect the previous allocation's reactions. Configuration-held work is not energy waiting, so a stipend leaves it active for its own transition to release or cancel.

### 5. Compatibility and rollout

1. Snapshot changes are additive. Older snapshots without pending fields read as empty state.
2. No ledger migration rewrites `blocked` tasks. Existing honey-blocked tasks keep the current unblock-on-`energy.add` behavior until they are purged or naturally resolved.
3. This direct slice lands first. Task deferral, standing supersession, and pause observations follow in 044b on the same pending model and require no protocol version bump.
4. Existing tests that assert task-level energy blocking keep their meaning; the new expectations are direct-scoped.
5. Documentation updates describe the direct admission path and cross-link 044b for the task and standing story.

### 6. Resolved implementation decisions

1. **Startup resume.** The reactor enumerates trail ids from the ledger, skips killed trails and trails with no reserve, and drains active entries. An active entry plus a positive reserve is the resumability condition because stipend abandonment is atomic.
2. **Stipend and kill.** Kill abandons every active direct pending entry; a stipend abandons the energy-deferred ones. Both do it in the same ledger mutation that applies their event, and neither resumes the work.
3. **Unresolved.** A missing source event becomes a visible `unresolved` entry with a diagnostic; it is never treated as runnable.
4. **Identity.** The direct dispatch key is the dedupe key; the source event identity is stored only for recovery.
5. **Deferred scope.** Task resume identity, standing overlap, `skipped`, and pause/resume observations belong to 044b.

## Testing Decisions

Tests assert external behavior, persisted ledger state, and adapter calls. They do not assert private helper structure.

- **Direct deferral:** a `waiting_review` task whose guard, receiver, or fix-up reaction is denied stays `waiting_review`, keeps its proposal/review evidence, records one direct pending entry, and starts no adapter.
- **Resume routing:** after `energy.add`, the same guard/receiver/fix-up Bee runs, not the task's assigned builder. The resumed direct event is not applied twice to the ledger.
- **Ordering:** three pending entries added out of chronological order drain oldest-first with a stable id tie-break.
- **Durability and restart:** pending direct work survives a new reactor over the same ledger; add-approved work resumes.
- **Startup resume:** an `energy.add` applied to the ledger while the reactor was down resumes its pending direct work on the next start; a killed trail and a zero-reserve trail do not.
- **Partial top-up:** enough honey for one entry resumes one and leaves the rest pending.
- **Missing source event:** a direct pending entry whose event is absent becomes `unresolved` and remains visible; it is neither dispatched nor deleted.
- **Kill:** `system.kill` abandons pending direct work; later `energy.add` does not dispatch it.
- **Stipend:** `energy.stipend` abandons energy-deferred pending direct work; later `energy.add` does not resurrect it. A configuration-barrier hold survives the stipend and still resumes when its pending change resolves.
- **Dedupe:** repeated deferral creates no duplicate; a repeated `energy.add` does not dispatch the same entry twice.
- **Legacy:** an old `blocked` + `Honey reserve exhausted` task still unblocks on `energy.add`; no new direct path writes `blocked`.
- **Compatibility:** a snapshot without pending fields decodes as an empty queue.

Prior art: task-ledger reducer and energy tests, runtime direct-dispatch routing and dedupe tests, and pending-dispatch persistence tests.

## Out of Scope

- Task dispatch deferral, `skipped`, standing overlap, pause/resume observations, operator surfaces, and metrics — [044b](044b-standing-supersession-and-energy-observations.md) owns them.
- Per-task honey budgets, changing the one-token dispatch cost, or charging for work that was never admitted.
- Automatic top-ups, scheduled stipend changes, or a new queue service outside the ledger and reactor.
- A manual task-block UI or a taxonomy of task-level block reasons. `blocked` stays available but is not redesigned here.
- Migrating historical ledger snapshots or rewriting old `blocked` tasks.
- Replacing the task scheduler or changing which task is eligible next.
- New delivery, merge, or pull-request semantics.
- A full pending-work browser beyond counts and diagnostics.

## Further Notes

- **Relationship to 044b.** [044b](044b-standing-supersession-and-energy-observations.md) owns task-dispatch deferral, `skipped`, the standing overlap predicate, atomic stipend supersession for task entries, and `energy.paused` / `energy.resumed`. This slice deliberately leaves task dispatch and the standing tick lifecycle on the legacy `blocked` path so existing energy alerts keep working until the replacement observations exist.
- **Barrier-held work and startup.** A configuration-held entry is resumed by its transition, not by the startup drain. When the change resolves while the reactor is down, the hold waits for the next event on the trail instead of resuming on start; tracked in [Backlog](../plans/backlog.md#barrier-held-direct-work-is-not-resumed-at-startup).
- **Relationship to existing specs.** [013-system-kill](013-system-kill.md) owns the hard stop; this spec only abandons pending work on kill. [028-standing-trails](028-standing-trails.md) owns stipend and tick overlap; this slice only stops a stipend from resurrecting old direct reactions. [031-task-scheduler](031-task-scheduler.md) owns task eligibility order; pending drain uses creation order. [043-trail-observability](043-trail-observability.md) owns honey metrics and completion truth. [003-hive-evals](003-hive-evals.md) owns eval cases. [002-queen-console-mvp](002-queen-console-mvp.md), [010-telegram-human-gateway](010-telegram-human-gateway.md), and [018-cli-colony-status](018-cli-colony-status.md) own the operator surfaces.
- **Why split from 044b.** The direct slice closes real defects without changing task dispatch or the standing tick lifecycle: task dispatch still writes `blocked`, so a deferred standing tick still lets the next tick start, and existing blocked cards remain. Removing `blocked` before pause cards exist would make energy waiting silent.
- **Why not keep `blocked` for the direct path.** A task-level status cannot represent a shared-reserve condition, cannot carry a direct reaction, and cannot preserve `waiting_review`. Keeping it would require storing prior state and source context anyway; at that point `blocked` is a deferred-work record wearing a task-status name.
