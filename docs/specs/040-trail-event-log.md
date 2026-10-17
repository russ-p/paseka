# Spec 040: Trail event log

## Status

**(Approved)**
A trail gains one append-only event log of its own, written by the colony's single publish path, that answers the question no current surface can: what happened on this trail. It records every domain event, whoever produced it, without requiring a live broker. Related: [039-proposal-patch-records](039-proposal-patch-records.md) depends on this to place its patches in a trail's story.

## Problem Statement

A Flight Trail's history is asked two different questions, and the product can answer neither completely.

**What happened?** A Beekeeper opening a trail, exporting it, or reading a trail report sees roughly a third of what the colony did. The event history is assembled from one file per Bee run, and only a Bee that published through the event CLI ever reaches it. Everything the runtime, the task ledger, the honey reserve, the review gate, the cue layer, and the console decide goes straight to the event bus and exists nowhere else. On a real audited trail, 28 events read as 11: the operator's own intake request, both synthesized code proposals, the ready signal, all six honey charges, three task status transitions, and the auto-generated trail summaries were all absent.

The missing half is not a rare event. On that trail, every run of both builders showed no proposal in its history — so the record of the trail says the flight proposed nothing, while the change it produced had been merged.

**Did it reach the colony?** The opposite question has the opposite failure. Only the broker holds the complete stream, and only while the broker lives. `paseka replay` reads it and prints event kinds and producer identities without payloads, so even the complete half is unreadable. Purging the bus removes the last copy of those events. Every trail surface — timeline, export, prompt memory, trail titles and summaries, worktree branch and PR body resolution — is fully offline-capable today, and that is a real property worth keeping; it is also why nothing noticed that the offline copy was the incomplete one.

Neither surface is authoritative. The bus is complete and fragile; the filesystem is durable and partial. Audit tooling is stuck choosing.

The per-run event log cannot become the answer on its own. It is a per-attempt file that the run launcher deletes and recreates on every dispatch, so a retried or explicitly identified run overwrites the previous attempt's history. And many producers have no run directory at all — an operator signal, a console action, a config read, an explain run — so an audit log placed per run has nowhere to put them.

## Solution

Give each trail one append-only log of its own domain events, written once by the colony's single publish path, beside the run directories rather than inside any of them. Every domain event the colony publishes — synthesized or hand-authored, from a Bee, the runtime, the console, the Human Gateway, or the CLI — lands there before it goes on the bus.

Every trail surface reads that one log and stops guessing. The timeline shows a trail's real history in order; export includes the same rows the timeline shows; prompt memory, trail titles, trail summaries, worktree branch and PR body resolution read the same complete history; `paseka replay` keeps its own question and keeps asking the broker.

Three properties make this the honest choice rather than a second partial copy:

- **The trail log is what happened, not what reacted.** It records intent and outcome locally, so it survives a broker that is down, misconfigured, or purged. A run that changed nothing leaves no record — this is a record of decisions, not of elapsed time.
- **Every event carries a stable identity.** An event's identity is minted once, where the event is created, so the same event found in a per-run audit log and in the trail log is recognizably the same event and appears once. This is what makes merging the two sources safe instead of a doubling.
- **A producer that fails to reach the bus still leaves a local record.** The local write happens first, so the window between "published" and "recorded" — the window that loses events today — disappears.

This does not replace the bus and does not make the bus unnecessary. Choreography still runs on JetStream; the reactor still consumes domain events to drive work; `paseka replay` still answers "what did the colony broadcast". The trail log is the record, not the transport.

## User Stories

1. As a Beekeeper, I want a trail's timeline to show every domain event of that trail, so that I can see what the colony actually did rather than a third of it.
2. As a Beekeeper, I want the intake request that started a trail to appear on its timeline, so that the trail's own first question is visible.
3. As a Beekeeper, I want every synthesized code proposal to appear on the trail timeline, so that I can tell what was proposed without reconstructing it from git.
4. As a Beekeeper, I want honey charges to appear on the trail timeline, so that I can audit what a flight cost.
5. As a Beekeeper, I want task status transitions to appear on the trail timeline, so that a task's progress through the colony is readable in one place.
6. As a Beekeeper, I want trail titles and summaries synthesized at runtime to be visible in the trail itself, so that the name I gave a trail and the summary under it are not one step ahead of its history.
7. As a Beekeeper, I want a run whose Bee published nothing still to have its proposal recorded, so that a trail that changed code never reports that nothing was proposed.
8. As a Beekeeper, I want a trail export to contain the same events the timeline shows, so that an offline report cannot disagree with the screen I read it from.
9. As a Beekeeper, I want an event published by an operator, the console, or the CLI to appear on the trail timeline, so that human actions are part of the trail rather than invisible bus traffic.
10. As a Beekeeper, I want a human producer's event to be visibly attributed to a human rather than shown as an unknown Bee, so that I can tell my own actions from a Bee's.
11. As a Beekeeper, I want a trail readable with no broker reachable, so that audit never depends on bus availability.
12. As a Beekeeper, I want a trail's history to survive purging the bus, so that removing bus state does not silently destroy what the colony did.
13. As a Beekeeper, I want a trail with no runs but with events — an intake and nothing more — to appear in the trail list, so that a trail I started is not invisible until a Bee flies.
14. As a Beekeeper, I want a trail's position in the trail list to reflect its latest event, so that a trail that was charged honey or killed after its last dispatch does not sort as stale.
15. As a Beekeeper, I want events ordered deterministically even when several share a timestamp, so that paging through a trail never repeats or skips a row.
16. As a Beekeeper, I want a retried or explicitly identified run to keep the history of its earlier attempt, so that a rework cycle shows both attempts.
17. As a Beekeeper, I want a Bee's own event log to keep recording what that Bee published, so that a per-run view of a run is unchanged by this feature.
18. As a Beekeeper, I want a script Bee's documented event-log path and environment variable to keep working unchanged, so that existing colonies do not break.
19. As a Beekeeper, I want an event I published to appear once in a merged trail view, not twice, so that the record is not inflated by having two storage locations.
20. As a Beekeeper, I want the same event retried or flushed twice to appear once, so that an at-least-once bus cannot inflate the trail.
21. As a Beekeeper, I want a single unreadable event log to cost only its own trail its events, so that one damaged file cannot blank the colony's timeline.
22. As a Beekeeper, I want a read-only colony to keep working with no broker configured, so that reading a trail never becomes a broker dependency.
23. As a Beekeeper, I want a file-only colony with no broker at all to still record events locally, so that losing the broker does not lose the record.
24. As a Beekeeper, I want the trail log written before the event goes on the bus, so that a crash between the two cannot lose the event.
25. As a Beekeeper, I want to tell that an event the broker refused is not a confirmed broadcast, so that a saved record is not read as proof the colony acted on it.
26. As a Beekeeper, I want a colony-wide timeline read to cost one file per trail rather than every run's file in every trail, so that a wide timeline window stays cheap.
27. As a Beekeeper, I want a trail's timeline to include a proposal's patch reference inline with the event, so that I can move from "a proposal happened" to its bytes without a second lookup.
28. As a Beekeeper, I want a run's own event log to stay a per-attempt file rather than becoming the trail's history, so that a retry does not erase the attempt it replaced.
29. As a Beekeeper, I want a trail whose history is only on the bus to be reported as such, so that a missing local record is visible rather than read as "nothing happened".
30. As a maintainer, I want one writer rather than one per publishing site, so that a new event producer cannot forget to record itself.
31. As a maintainer, I want a new producer to be covered automatically by writing the record, so that adding a producer does not create a new gap.
32. As a maintainer, I want every trail read to go through one reader, so that timeline and export cannot drift apart.
33. As a maintainer, I want event identity minted in one place, so that identity cannot be assigned inconsistently by producers.
34. As a maintainer, I want the publisher seam to remain the single point that turns an event into colony history, so that the layering stays understandable.
35. As a maintainer, I want a non-domain run-lifecycle record to stay out of the trail log, so that the trail's history is not padded with tool-call diagnostics.
36. As a maintainer, I want the trail log removed by the same retention that removes a trail's run data, so that cleanup is one mechanism.
37. As a maintainer, I want the trail log to not be mistaken for a Bee run directory by any projection, so that run counts and listings stay correct.
38. As a maintainer, I want `paseka replay` to keep reporting what reached the bus, so that the two records can be compared rather than conflated.
39. As a Beekeeper, I want to compare a trail's local record against what reached the bus, so that an uncertain publish outcome is answerable.
40. As a Beekeeper, I want the event envelope to carry an identity that is not derived from its position or timestamp, so that the same event is recognizable wherever it is stored.
41. As a Beekeeper, I want prompt memory to draw on the trail's complete history, so that a dispatched Bee is briefed from what actually happened.
42. As a Beekeeper, I want worktree branch and pull-request body resolution to read the complete history, so that a branch name or body written by a step is found regardless of which component published it.
43. As a Beekeeper, I want event identity to be readable in the raw envelope shown on a timeline row, so that I can correlate a row with a bus message.
44. As a colony author, I want publishing to a colony with no configured broker to still record locally, so that a laptop colony is not a second-class record.
45. As a colony author, I want an explicitly published proposal of mine to appear on the timeline once, so that adding this feature does not dispatch or record anything twice.

## Implementation Decisions

1. **One append-only log per trail, beside the run directories.** The trail log lives directly under a trail's run root as a file named `trail-events.ndjson`, not as another per-run directory. That placement is the point: it is the level that survives a run being retried, and per-run directories are per-attempt — a launcher deletes and recreates a run's event log on every dispatch, so any audit log placed there is overwritten by the next attempt under the same run identity. Run directories also do not exist for producers that are not dispatches at all, such as an operator signal or a console action, so a per-run log has nowhere to put them. Trail-level storage treats producer identity as a field of the record rather than as a directory name.

   **The name is not the same as the per-run log's, deliberately.** A Bee run directory holds `events.ndjson`; the trail root holds `trail-events.ndjson`. Reusing `events.ndjson` at both levels would follow the project's content-noun convention, and it is harmless to the code — nothing globs by this name, and the trail reader already skips anything that is not a directory by construction — but the two files carry different rights. One is a per-attempt audit of what a single run published; the other is the whole-trail record. Naming both the same makes the wrong file readable without complaint, and an audit tool that quietly answers with the wrong half is the worst kind of wrong. A distinct name fails loudly instead: "what is `trail.ndjson`" is answered once by the layout documentation. The name carries both scope and content because a path fragment gets quoted on its own — in an error message, in documentation, in a console action — where the directory it came from is no longer in view. A Bee's own event log keeps its name, its documented path, and its environment variable unchanged; only the trail-level file is new.

   The two files are described in separate paragraphs of the run-tree layout documentation, never in one sentence without a path, and the trail-level one is stated as the record rather than as an audit log.

2. **The per-run event log keeps its meaning.** A run's own event log remains the audit of what that run published, keeps its per-attempt lifecycle, keeps its documented path and environment variable for script Bees, and keeps answering the run-scoped "which events did this run emit" question. This feature is additive on the write side and changes no existing file's semantics: it does not migrate, repoint, or regenerate any per-run log, and it does not derive per-run logs from the trail log. The two overlap by design — the same event object is stored in both when a run published it — and the merge below is what makes the overlap invisible.

3. **One writer, at the publisher seam.** The record is written where the colony turns an event into history, not at each publishing site. Publishing happens through a narrow publisher seam from roughly two dozen call sites across the runtime, task ledger, honey reserve, review gate, cue layer, invites, Human Gateway, artifact announcements, deferred flush, and CLI, and each of them currently remembers the bus. Adding the record to that same seam means a producer that can publish can record, with no per-site change and no way for a new producer to forget.

4. **Only domain events are recorded.** The same test that gates the bus gates the trail log: trigger and platform-control signals, insights, mutations, and verifications are recorded; run-lifecycle records such as tool calls and assistant text are not. This keeps the trail's history free of stream diagnostics that a provider adapter may write to the same per-run file today.

5. **Write locally first, then publish.** The local write precedes the broker attempt, so the gap between "published" and "recorded" is closed rather than widened. Validation of the event shape already happens before the publisher is reached, so the record is not a place where rejected events accumulate; what remains is broker unavailability, which is exactly when a local record matters most. The consequence is deliberate and stated wherever the trail log is described: the trail log is the colony's account of what it decided to do, and an event it holds may still have failed to reach the bus.

6. **The log is not marked with publication state.** A trail log entry carries no indication of whether the broker accepted its event, and nothing rewrites a recorded line. Rewriting is what makes an append-only log unappend-only, and the state machine it would require would sit on every publish path — including the reactor's hot path — to serve a state whose realistic failure is a broker outage, and a broker outage means nothing reacted either, so the practical loss is zero while the cost is permanent. The distinction is observable instead by comparing the trail against the bus (see Further Notes). Publication state where it is already paid for stays where it is: the proposal record in 039, whose archived bytes are the artifact being claimed and where "retained but unconfirmed" is a real ambiguity about the artifact itself. There is no operator-visible publication-failure surface outside those proposal records today, and this feature does not add one.

7. **Event identity is minted where the event is created.** Every event envelope carries a stable identity assigned in the event constructor, not derived from its sequence number, timestamp, or position in a file. This is what makes storing the same event in two places safe. Today a published event reaches the bus with a zero sequence and reaches disk with a sequence number allocated at write time, so no field can currently distinguish "the same event in two stores" from "two events" — sequence identity is unusable across the boundary, and allocating a shared counter across processes would introduce a race for no gain.

8. **Merge on read, deduplicate by identity.** A trail's events are the union of its per-run logs and its trail log, ordered by creation time with identity as the tie-break so the order is total and stable for paging. Where an event carries an identity it is deduplicated on it, so an event stored in both a per-run log and the trail log appears once, and a retried or re-flushed event appears once. Records written before identities existed are kept and ordered by their previous key; no backfill is performed, because an event that predates this feature exists in only one store and therefore cannot collide with anything.

9. **Order by creation time, not by sequence.** Trail events carry no usable sequence — a per-trail counter would be a read-modify-write across processes, and a zero value would make the old tie-break meaningless. Ordering is by creation time with identity as tie-break, which is total and does not depend on a counter any writer can collide with.

10. **Colony-wide reads use the trail log only.** A colony-wide timeline page reads one file per trail rather than every run log in every scanned trail. It is the same projection the timeline should have been using, it is bounded per trail instead of per run, and it also means the colony-wide feed survives a partially cleaned trail. A trail-scoped view merges both sources, because only there do the per-run logs matter and only one trail is in play.

11. **Trail listing counts trail-log activity.** A trail's summary takes its last activity from its events as well as from its run metadata, and a trail with events but no runs is listed rather than dropped. Without this, the trail list would contradict the timeline: a trail whose newest fact is a honey charge or a kill would sort as stale, and a trail that only received an intake request would not appear at all — which is the same class of absence this feature exists to remove.

12. **One unreadable log costs one trail.** A run log that cannot be read is skipped with a diagnostic naming the file, matching what a trace summary already does for a run whose metadata will not parse. Readers behind this one feed the timeline, export, and prompt memory, so failing the whole read hides every trail along with the damaged one.

13. **Retention is the existing trail retention.** The trail log lives inside the trail's run tree, so the commands that remove a trail's run data remove it with them, and the age that selects a trail for cleanup already includes it. No new target, no new cadence, and no bus-side lifecycle. Purging the bus leaves it, which is consistent with the principle already fixed for proposal patches: removing bus state should not destroy a filesystem record of what the colony did.

14. **The bus remains a query surface, not a fallback.** Readers keep working with no broker, so there is no bus-to-filesystem fallback path to build, no degraded mode to specify, and no requirement for a live broker on any read surface. `paseka replay` keeps answering "what did the colony broadcast", which is a different question from "what happened", and the difference is what makes a divergence diagnosable rather than confusing.

15. **The publisher seam stays the composition point.** The transport connection is built from colony configuration in one place that every composition root already calls, so the trail's filesystem location reaches the publisher without being threaded through callers that do not know about the run tree. This keeps the layering honest: the publisher is the colony's event sink, bus and local record both, rather than a transport that happens to write files.

16. **Patch references travel with the event.** A synthesized proposal's patch reference is inside its payload, so a trail event row carries it with no extra join. This is what lets 039's patch listing be reachable from the trail's own history; before the trail log, an exported patch had no row explaining when or why it was proposed.

17. **Event identity is an envelope field, deliberately not an author-facing contract.** The field is minted where the event is created and read by the colony; it is absent from the agent-facing input shape, exactly as the sequence number is, and is not documented in the event contracts reference as something a colony author may rely on. No producer is required to supply it, the protocol version is unchanged, and hand-authored events and historical payloads parse unchanged. The pre-existing sequence number is the precedent: it is a colony-minted field that no bee sees or sets. Making identity author-facing is a separate decision to be taken when a producer outside the Go runtime actually needs to deduplicate its own record — until then it would be a compatibility promise that no current consumer requires.

## Testing Decisions

Good tests here assert what a reader can observe about a trail's history. A test that fabricates a producer, calls the publisher, and then reads the trail back is asserting the right shape: the event is in the trail, in order, once, and without a broker.

- **Trail record** — every producer path results in an event in that trail's log; ordering is total and stable when timestamps collide; identity is present and stable across the bus and the local record; an event whose broker attempt failed is still recorded locally; a non-domain record is not; no broker configured still records.
- **Merge and identity** — an event present in both a per-run log and the trail log appears once; a retried or re-flushed event appears once; pre-identity records are preserved and ordered by the previous key without colliding with newer ones; a trail with only a trail log and no runs reads correctly.
- **Read isolation** — one unreadable run log yields the rest of the trail plus a diagnostic and no error; a colony-wide read does not open per-run logs; a trail-scoped read does.
- **Contract preservation** — a run's own event log still records only what that run published; its documented path and environment variable still resolve; its per-attempt lifecycle still replaces the previous attempt's file; the protocol version and payload validation are unchanged for hand-authored events.
- **Trail listing** — a trail with events and no runs is listed with a correct last-activity time; a trail whose newest event is not a dispatch sorts by that event; run counts still count runs.
- **Retention** — trail run-data removal removes the log; bus purge leaves it.
- **Cross-feature** — a synthesized proposal appears once on the timeline and carries a patch reference that resolves in 039's store.

Prior art in-repo: runs package layout and projection tests including the reserved-subdirectory and console-audit cases, publisher fakes used by runtime proposal tests, task-ledger integration tests, console handler tests over trail-scoped endpoints, and export include tests.

## Out of Scope

- Reading events from the event bus, or any bus-to-filesystem fallback for readers, and any requirement for a live broker on a read surface.
- Making the bus optional for publishing, or queuing events for a broker that is not up; the deferred-emit buffer remains the mechanism for that.
- Changing what goes on the bus, the event contracts, subject layout, the reactor, or any choreography.
- Changing the per-run event log's meaning, lifecycle, path, or producers' contract.
- A global colony-wide event log, or cross-trail queries; this is a per-trail record.
- An index, manifest, or checkpoint file over trails; the existing full-walk listing is unchanged apart from reading trail activity.
- Streaming or tailing events to a live console over WebSocket or SSE; the timeline keeps its polling transport, and a domain event stream remains the separate open item spec 002 already carries.
- Showing payloads for bus events in `paseka replay`, or changing what `paseka replay` reports.
- Deleting or compacting a trail log, event retention inside a trail, or any per-trail pruning of records; a trail log is whole-trail history.
- A background reconciliation service that compares the local record with the bus, or any repair path for a partially written log.
- Migrating historical bus events into a trail log, or backfilling logs for trails that predate this feature.
- Changing proposal patch storage, review truth, or what a Guard reviews; see 039.

## Further Notes

- **Event identity stays internal (decided).** The field is minted in the event constructor and read by the colony; it is not added to the agent-facing input shape and not documented as something a colony author may rely on. The existing sequence number is the precedent — a colony-minted field no bee sees or sets. The alternative was rejected as a compatibility promise no current consumer requires: documenting it would oblige a future protocol version to keep the field, its name, and its meaning. Revisit when a producer outside the Go runtime needs to deduplicate its own record.
- **Publication state is not marked on the log (decided).** The trail log records what the colony decided to do and is not annotated with whether the broker accepted each event; nothing rewrites a recorded line. The comparison against the bus is the answer, and there is deliberately no operator-visible publication-failure surface outside 039's proposal records. Every document describing the trail log must say plainly that it is what the colony decided to do, not proof of what was broadcast. Revisit if a bus rejection ever needs to be visible without running the comparison.
- **Verification is a separate command (decided).** "Compare this trail's local record against what reached the bus" becomes its own verb over the trail log and the stream, reporting events that are only local, only on the bus, or in both. It is deliberately not an export section, because export must keep working with no broker reachable, and not a flag on `paseka replay`, which answers a different question and whose output is intentionally payload-free. With no broker reachable the command must report that verification was not possible, never that a divergence exists. Identity is the prerequisite; the command ships with or after this feature and closes the backlog item on duplicate trail summaries, which is currently unverifiable because payloads cannot be compared from the CLI.
- **Ordering against [039-proposal-patch-records](039-proposal-patch-records.md): this spec first.** The earlier reading of this note ran the other way — that the publisher should record a payload shape 039 is about to change. That argument does not hold up. The trail log is append-only, and a history holding both payload shapes across time is exactly what the per-run logs and the bus already hold and what 039 explicitly keeps readable; recording the current shape first costs one sentence in the documentation, not a rework. Meanwhile the reverse order cannot start at all, because 039 is a draft with two budget decisions still open while this spec is approved and implementable.

  Doing this first also pays part of 039's bill in advance. 039's headline problem is not only that a diff is unrecoverable — it is that a synthesized `MUTATION` reaches no disk surface, so an export cannot mention it. Once this spec records every domain event, a trail export carries the raw event JSON for a synthesized proposal, inline diff included, because both the HTML and Markdown renderers emit the full envelope. Diffs within the current inline budget therefore become recoverable offline with no patch store at all, and 039 afterwards supplies completeness beyond that budget plus bytes, baseline, and publication state — rather than the bare fact of existing.

  It also removes rework in the other direction: with this spec in place, 039's publish path records its trail event for free, including the interactive session path that 039 has to add from scratch.
- **Relationship to the trail-listing index already proposed.** The backlog carries an idea for a per-trail manifest that would turn a trail list read into a page-sized read. Last activity from trail events, and an event count, belong in that manifest when it lands, rather than being computed by a second mechanism.
- **The composition contracts this feature touches are enumerated in the event contracts reference and the architecture overview**: the run-tree layout and its reserved directories, the publisher seam and its composition roots, and the description of the per-run event log as a per-run audit. All three currently describe a single file per run as the trail's history, and each needs the trail log stated alongside it.