# Spec 050: Trail attention state from unresolved failures

## Status

**Approved**
Chosen direction "B" from the trail-state discussion: a failure stays open only until a later successful run of the same task supersedes it. Amends the trail-state input described in [045-trail-closing-phase](045-trail-closing-phase.md); the axis itself keeps its meaning.

## Problem Statement

A Flight Trail is a long-lived container for many attempts. Retries, rework rounds, closing rounds, and same-trail analysis are all normal, but the Queen Console decides whether a trail reads `failed` from a flag that says "this trail has ever had a bad run". Any single failed, cancelled, contract-failed, or record-damaged run paints the trail red permanently, even when a later attempt on the same task succeeded and the work moved on.

From a Beekeeper's perspective this fails in three ways:

- The badge stops discriminating. On a busy colony most trails accumulate at least one bad attempt, so `failed` becomes the normal state and no longer says where to look.
- A healed trail cannot be distinguished from a broken one. The Dashboard row, the Traces list, the trail header, and Telegram `/traces` all keep reporting a problem that was already fixed.
- The wording is not even consistent: the Traces list says `failed`, while the Dashboard row and the trail header say `failures`, for exactly the same condition.

The flag also over-reports in both directions. A deliberate stop or a timeout is projected as `cancelled`, which counts as a failure and reads like an error. A failed analysis run — which the trail metrics deliberately keep out of the work baseline — also paints the trail failed, so studying a trail can make it look worse. Meanwhile a trail whose newest work attempt failed but whose older attempt succeeded is indistinguishable from a trail that recovered, because there is no ordering at all: the flags are accumulated booleans with no timestamps and no notion of "latest".

The result is a badge that is historically true, currently misleading, and impossible to act on: it answers "did anything ever go wrong here", when the operator is asking "does this trail need me now".

## Solution

The trail's attention state becomes current rather than historical. A terminal failure counts until a later successful run of the same task supersedes it. Record-integrity problems never resolve, because a later success does not repair a damaged outcome pair. Analysis runs neither raise nor clear attention.

In practice:

- A trail that failed an attempt and then completed the same task successfully reads `idle` again, and its list row goes quiet.
- A trail whose newest attempt on a task failed reads `failed`, with the number of open failures beside it.
- A trail with two open failures on two different tasks reads `2 failures`; a success on one task clears only that one.
- A trail with a conflicting or incomplete outcome pair keeps a distinct `record error` badge even if every later run succeeded.
- A trail with only healed failures still records them: the trail detail carries a quiet `resolved failures` hint, so the history is not erased, only demoted.
- A cancelled run is treated as unfinished work rather than as an error: it can supersede nothing on its own and is cleared by a later success of the same task.
- A failed analysis run does not paint the trail, and a successful analysis run does not clear a failed work attempt.

All surfaces read one server-side projection: the Traces list, the Dashboard trail rows, the trail detail header and flags, Telegram `/traces`, and the legacy Console. The client keeps rendering the state, but it no longer decides which failures count.

## User Stories

1. As a Beekeeper, I want a trail that recovered to stop reading `failed`, so that I can trust the badge as a signal of current work.
2. As a Beekeeper, I want a trail whose latest attempt on a task failed to read `failed`, so that I know where to intervene.
3. As a Beekeeper, I want retries within one task to collapse into one open failure, so that a trail is not counted several times for the same problem.
4. As a Beekeeper, I want a success on task A not to clear a failure on task B, so that a healthy part of the trail cannot hide a broken part.
5. As a Beekeeper, I want the badge to show how many failures are open, so that I can tell one broken task from a trail falling apart.
6. As a Beekeeper, I want the number to agree in grammar (`1 failure`, `2 failures`), so that the badge reads like the rest of the Console.
7. As a Beekeeper, I want a trail with only a conflicting or incomplete outcome pair to read as a record problem, not as a plain work failure, so that I know the evidence itself is damaged.
8. As a Beekeeper, I want a record problem to stay visible after a later successful run, so that a damaged outcome pair is not silently forgotten.
9. As a Beekeeper, I want a resolved failure to remain visible on the trail detail as a quiet hint, so that I can still see the trail's history.
10. As a Beekeeper, I want a resolved failure to leave the Traces list State column empty, so that the eye lands only on rows that need me.
11. As a Beekeeper, I want a cancelled run to count as unfinished work rather than as an error, so that a timeout or a deliberate stop reads differently from a failed contract or gate.
12. As a Beekeeper, I want a cancelled attempt to be cleared by a later successful attempt of the same task, so that recovery after a stop works like recovery after a failure.
13. As a Beekeeper, I want a failed analysis run not to paint the trail, so that studying a trail does not make the work baseline look worse.
14. As a Beekeeper, I want a successful analysis run not to clear a failed work attempt, so that analysis cannot mask a real problem.
15. As a Beekeeper, I want a failed completion contract to count as an open failure, so that a run that produced no acceptable result is treated as unfinished.
16. As a Beekeeper, I want a contract failure to clear when the same task later completes, so that rework closing the gap removes the alarm.
17. As a Beekeeper, I want a direct run on a trail with no task to be cleared by a later successful direct run, so that an ad-hoc retry behaves like a task retry.
18. As a Beekeeper, I want the open-failure answer to be independent of directory read order, so that two reads of the same trail never disagree.
19. As a Beekeeper, I want the open-failure count to be stable under parallel runs, so that overlapping attempts do not make the badge flip between polls.
20. As a Beekeeper, I want a trail with a running or queued retry to read `active` rather than `failed`, so that the in-flight work is the primary fact.
21. As a Beekeeper, I want a queued attempt neither to resolve nor to add to the open failures, so that a retry in the queue does not erase the evidence of the attempt it replaces.
22. As a Beekeeper, I want the Traces list, Dashboard, trail detail, and Telegram to agree on the state, so that the same trail does not read differently per surface.
23. As a Beekeeper, I want Telegram `/traces` to show the open count instead of an eternal `failed`, so that the mobile summary matches the Console.
24. As a Beekeeper, I want the legacy Console to follow the same rule while it is still served, so that the two consoles do not contradict each other during the cutover.
25. As a Beekeeper, I want a standing trail, a paused trail, and an experimental fork to keep their own independent badges, so that this change does not fold separate axes into one.
26. As an interface bee reading the trail API, I want the historical failure flag to keep its current meaning, so that existing automation that counts "has this trail ever failed" continues to work.
27. As an interface bee, I want the new count and recency fields to be additive, so that an older client that ignores them still decodes the payload.
28. As a Beekeeper, I want the badge to refresh through the existing poll, so that a trail stops reading `failed` after a successful retry without a manual reload.
29. As a Beekeeper, I want a trail with only events but no runs to stay `idle`, so that activity alone is not mistaken for failure.
30. As a Beekeeper, I want an unreadable or skipped run directory not to crash the trail projection, so that one damaged directory does not hide the whole list.
31. As a maintainer, I want the trail summary to stay one filesystem walk, so that sharper semantics do not cost an extra read per trail.
32. As a maintainer, I want the resolution rule stated in one place and shared by every surface, so that the Console cannot drift from Telegram or from an export consumer.
33. As a maintainer, I want the reason a run failed to stay on the run row, so that clearing the trail badge does not lose why it happened.
34. As a maintainer, I want the change to be computed on read with no data migration, so that existing trails need no rewrite or backfill.
35. As a maintainer, I want the trail detail's quiet historical hint to be worded as resolved, so that an operator can tell it apart from an open failure.
36. As a maintainer, I want the trail's attention state to remain distinct from the closing and merge stage, so that this change does not reopen the trail-stage design.

## Implementation Decisions

**Where the truth lives.** The open-failure projection is computed in the runs read model, during the existing trail walk, so a summary costs no extra read. Every surface consumes the shared trail read model; the Queen Console does not re-derive which failures count from the run list, and Telegram and the legacy Console do not get their own copy of the rule.

**Shape of the projection.** The trail summary gains, alongside the existing flags:

- an open-failure count,
- the instant of the newest open failure,
- a record-integrity flag.

The existing historical failure flag keeps its exact current meaning (any run in the trail ever failed) and is not repurposed. It feeds the trail detail's historical hint and stays available to external readers. The new fields are additive on the wire, in the runs summary, the trailview trail, and the hiveview trail summary.

**What counts as a failure.** A terminal run is a failure when its projected state is `failed` or `cancelled`, or when its completion contract is `failed` or `unavailable`. A run with a conflicting or incomplete outcome pair is a *record problem* and is also an open failure, but it is tracked separately because it never resolves.

**What counts as a success.** A terminal run that projects `completed`, carries no record-integrity marker, and whose contract is not `failed` or `unavailable`.

**Resolution rule.** A failure is open unless a later successful run of the same task exists. Runs are grouped by the task id recorded on the run; the empty task id forms one trail-level group for direct runs, because a direct run has no finer identity than the trail. Closing tasks group by their own task id, so a closing success cannot clear a work failure. Runs are ordered by completion instant (the start instant is the fallback for a record that never finished), with the agent id as a tie-break, so the answer is a total order independent of directory read order.

**Record problems never resolve.** A conflicting or incomplete outcome pair stays open for the life of the trail, even when every later run of that task succeeded. A later success proves the new attempt is sound; it does not repair the old evidence. This preserves the truthful-record guarantee of the trail.

**Analysis is excluded from attention, in both directions.** An analysis run is filtered out before grouping: its failure does not raise the trail, and its success does not clear a work failure. The classification is the execution purpose recorded on the admission — a closed vocabulary of work, closing, and analysis, written onto the run record before the adapter starts — not the closing phase, not the trail's derived stage, not the Bee's name, and not `analysisOf`. A closing run is therefore not analysis and is never excluded, even though closing is derived from the task phase on the runtime side. This keeps the separation between work and analysis that the trail metrics already make, while the run row still shows the analysis outcome and the metrics still count it. Legacy runs with no recorded purpose participate as work-like, because excluding an unknown purpose would hide the only evidence a legacy trail has.

**Cancelled stays a failure, but reads as unfinished work.** The state vocabulary is unchanged: a cancelled run still means the run did not deliver. It is resolvable by a later success of the same task, so a deliberate stop or a timeout followed by a successful retry leaves no open failure. The run's recorded reason remains on the run row; the trail badge does not attempt to guess intent.

**In-flight runs.** A running or queued run sets the existing active flag and otherwise does not participate: it neither resolves nor adds an open failure. Because active outranks failed in the trail state, a trail with a queued retry reads `active` and the open failure reappears if the retry fails again.

**Trail state and label.** The trail state derivation becomes active, then failed when the open-failure count is positive or a record problem exists, then idle. The label is the trail's open count (`1 failure`, `N failures`); when record problems exist without an open failure the label names the integrity problem instead, and when both exist the count is shown with the record problem as a separate detail flag. This removes the current `failed` / `failures` split between the Traces list and the Dashboard row: one state, one label, every surface.

**Historical hint on the detail.** When the historical failure flag is set but nothing is open, the trail detail flags show a quiet `resolved failures` entry. The Traces list State column and the Dashboard badge stay quiet for that trail, which is the point of the change. The hint is not a badge and never takes the failure colour.

**Independent axes stay independent.** Standing, fork, pinned, paused, admission state, and the derived closing/merge stage are untouched. A paused trail still shows paused beside its state, and a standing trail still shows standing. Only the failure input to the trail state changes.

**Surfaces that must move together.** The trails list, the Dashboard trail rows, the trail detail header and flags, Telegram `/traces`, and the legacy Console read the new projection in the same change. The export and inspect surfaces keep using the historical flag unless a later spec says otherwise.

**No migration, no new endpoint.** The projection is computed on read from the run directories the walk already visits. Polling picks up the new state with the existing refresh interval, and no backfill, index, or stored derived file is introduced.

**Compatibility.** Older clients that ignore the new fields keep decoding the payload and keep their historical behaviour; the Console and Telegram are both updated in this change so the operator sees one answer. A run directory that cannot be read is skipped exactly as today, and a trail with no runs but with events remains idle.

## Testing Decisions

Good tests here assert the externally visible answer — which open failures a trail reports and what badge each surface renders — not how the walk stores its intermediate grouping. The interesting cases are order and identity, so the tests are table-driven over a handful of run projections and read like a specification of the rule.

**Runs read model.** Table-driven tests over synthetic run directories covering: failure then completion of the same task (resolved); completion then failure (open); two failures on one task followed by one success (both resolved); failure on one task and success on another (still open); a cancelled run (open, then resolved by a later success); a contract failure and a contract-unavailable run (open, then resolved); a conflicting pair and an incomplete pair (open, never resolved by later success); a failed analysis run (not open); a successful analysis run after a work failure (does not resolve); a direct run with an empty task id (resolved by a later direct success); a closing task's success after a work failure (does not resolve); a queued retry after a failure (active and open); an in-flight run with no terminal record (active, not open); a trail with only events (idle). A shuffle test feeds the same runs in different directory order and asserts an identical projection, and a timestamp tie test asserts the agent-id tie-break. Prior art: the existing outcome-projection and run-record tests.

**Shared projection.** Trailview and hiveview pass-through tests assert that the new fields survive enrichment and that a summary built from runs and a detail built for the same trail agree. Prior art: the existing trailview and hiveview trail tests.

**Frontend state and labels.** Unit tests for the state and label helpers cover active, open count singular and plural, record-problem-only, healed trail, and idle. Route and component tests assert: the Traces list row carries no state badge for a healed trail and shows the count for an open one; the Dashboard row shows the same badge; the trail detail header hides the badge for a healed trail and shows the resolved hint in its flags; a record-problem trail shows the integrity label. Prior art: the existing formatting, traces-route, trace-detail, and dashboard tests.

**Telegram.** The `/traces` formatter test asserts an open failure renders the count, a healed trail renders its run count, and an active trail still renders active. Prior art: the existing Telegram traces test.

**Fixtures.** The shared fixture trail used by the Console tests gains the new fields, and the existing expectations that relied on a historical failure badge are updated to the resolved/open distinction rather than deleted.

## Out of Scope

- Reading the task ledger for trail state. The trail state stays a run projection; the derived closing and merge stage remains the separate ledger-based answer.
- Representing the merge gate, delivery, or a closed-without-merge trail in the trail state.
- Changing the cancelled vocabulary, splitting timeout from a deliberate stop, or renaming cancelled to stopped.
- Delivery failures and delivery outcome accounting.
- Retry budgeting, automatic retry, or notification rules beyond the badge.
- Reworking run-level pages, run filters, or run metrics.
- Changing `paseka status` attention, which is task-based rather than trail-based.
- New historical-failure analytics, dashboards, or stored aggregates.
- Removing the legacy Console or completing the `/next/` cutover.

## Further Notes

This spec amends the statement in 045 that the trail-state axis is "left alone". The axis keeps its meaning — does this trail need attention now — but its input changes from "any failure ever" to "failures with no later success". Because every surface moves to the same input in one change, the badge continues to mean one thing on the Dashboard and on the trail page, which is the property 045 was protecting.

The analysis exclusion follows 043b: analysis cost and failures stay visible in the metrics and on the run rows, but they never become part of the work baseline, and the trail badge is a work-baseline signal.

Two alternatives were considered and rejected. Keeping the historical flag as the badge and adding a second "latest attempt" badge leaves two failure signals for the operator to reconcile on a row that is already dense. A purely cosmetic count-and-age on the historical flag is cheaper but leaves the central defect in place: a healed trail still reads failed.

The open question left to the operator, not to this spec, is whether a resolved failure should ever be resurfaced as a notification. Today it is not: the badge goes quiet, and the trail detail keeps the history.
