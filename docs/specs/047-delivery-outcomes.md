# Spec 047: Delivery outcomes

## Status

**(Draft)**
Opens the D6 design record deferred by [043b](043b-trail-metrics-and-work-completion.md) and [045](045-trail-closing-phase.md). A trail-owned delivery episode with an attempt history, a closed state vocabulary, and a handover contract derived from the selected delivery mode. The state vocabulary and the record shape below are proposals; the open decisions in Further Notes are not settled.

Related: [024-pull-request-delivery](024-pull-request-delivery.md) is the shipped delivery mechanism this spec records; [043b-trail-metrics-and-work-completion](043b-trail-metrics-and-work-completion.md) defined work completion one step earlier and left delivery to D6; [045-trail-closing-phase](045-trail-closing-phase.md) made merge an explicit phase and derives a trail stage from the ledger; [039-proposal-patch-records](039-proposal-patch-records.md) is the prior art for an attempt-shaped record with publication states; [033-cli-prune](033-cli-prune.md) owns retention of trail records.

## Problem Statement

A trail knows when its work finished. It does not know, in any trail-owned way, what happened to the result afterwards.

The colony has four delivery mechanisms and records each of them differently:

| Mode | What approve does today | What the trail records |
| --- | --- | --- |
| `local_merge` | merges the trace worktree into the clone's default branch | merge commit SHA in `task.completed.commit` |
| `pull_request` | pushes the head, upserts a PR, leaves the gate at `waiting_review` | **nothing** |
| `ack_only` (root proposal) | acknowledges in place, no branch delivery | `task.completed` only |
| `none` (pinned experimental fork) | accepts the work, forbids delivery | `task.completed` only |

The consequences are concrete:

**Publishing a pull request leaves no trail record at all.** The forge identity — URL, number, head, base, state, draft — is written to the machine-local home state file (`~/.config/paseka/<slug>/state.json`). The trail log, the task ledger, and `task.md` learn nothing. A second machine that opens the same colony sees a trail `waiting_review` and cannot tell whether a PR exists, whether one was ever attempted, or whether the last attempt failed.

**Approval is not recorded for PR-backed trails.** The merge task completes only when the forge reports `merged`, so its completion means "delivered", not "a human approved". Between publish and merge the only trace of the human decision is whatever the operator remembers. Re-approving an open PR is a supported, idempotent operation that overwrites the copy with no audit.

**A closed-without-merge PR is indistinguishable from still waiting.** The gate stays `waiting_review` forever, and the forge state `closed` lives only in the machine-local cache. The operator sees an open review that no longer has anything behind it.

**Delivery failure reads as work failure.** A rejected push, a not-logged-in forge CLI, a non-JSON forge response, or a missing origin fails the approve call and leaves no record. The trail still looks like a trail whose result is fine and whose gate is open; the export, the Console, and the metrics have no vocabulary for "the work succeeded, handing it over did not".

**Retries are indistinguishable from first attempts.** Publish is an upsert, approve may be repeated, reconcile polls every 30 seconds — and none of it has an identity. Nothing answers "was this delivered once or three times", "when was it first published", or "is this the same episode or a new one".

**The trail stage cannot express the delivery it depends on.** The derived stage reports `delivered` from merge-task completion. That is the correct answer for a local merge and a merged PR, but not for a published-but-unmerged PR, not for a closed PR, and not for an experimental trail whose work was accepted without delivery.

**Nothing is portable.** Copy the repository, move the apiary, or delete the home state, and delivery history is gone. The one record that is durable — the merge commit in `task.completed` — covers exactly one of the four modes.

[Spec 043b](043b-trail-metrics-and-work-completion.md) deliberately stops at work completion and says the delivery record is D6. This is D6.

## Solution

1. **Delivery is its own trail-owned record.** One delivery episode per trail handover, written under the trail with an atomic replace, readable without the bus, the forge, or the home state.
2. **The episode has an attempt history.** A stable delivery identity, a monotonic attempt number, and one recorded outcome per attempt. Publish retries, reconcile re-polls, and repeated approvals are distinguishable without inventing a new identity.
3. **The state vocabulary is closed and versioned.** Mode, state, action, result identity, target references, actor, timestamps, failure, and supersession. No free strings where a reader thresholds.
4. **The handover contract is derived from the mode.** What counts as delivered — a merge, an in-place acknowledgement, or nothing at all — follows from the delivery mechanism the colony already selected. It is not a second policy knob.
5. **A failure to deliver is not a failure of the work.** Push, forge, network, and credential failures are recorded as delivery attempts that failed, with their own reason, while the work and the gate stay exactly as they were.
6. **Closed-without-merge becomes visible and actionable.** A PR closed without merging is a terminal delivery state, not silence. The operator can retry, request changes, or accept that the work will not be delivered — each of them a recorded decision rather than a stuck review.
7. **The ledger keeps its gate fact; the delivery record says who approved and what happened next.** Approval, delivery, and delivery outcome stop sharing one boolean. No task status is added.
8. **The observation is an event.** One `SIGNAL` kind carries each transition, so Queen Console, Telegram, and any methodology reader react to a delivery change instead of polling the forge.
9. **The machine-local forge cache becomes a cache.** The trail record is the source of truth; the home state file keeps looking names up for compatibility until it can be dropped.

## User Stories

1. As a Beekeeper, I want a trail to record that its pull request was published, so that a trail parked at the merge gate tells me the handover already happened.
2. As a Beekeeper, I want to see whether a trail was merged locally, published as a PR, published and later merged, closed without merge, accepted in place, or deliberately not delivered, so that I do not have to reconstruct it from forge history.
3. As a Beekeeper on a second machine, I want delivery identity to come from the repository, so that copying the colony does not erase what was delivered.
4. As a Beekeeper, I want the commit and branch that were handed over recorded, so that I can verify later that the thing which landed is the thing the trail produced.
5. As a Beekeeper, I want a failed publish to say it was a publish failure with its reason, so that I fix a credential instead of treating the work as defective.
6. As a Beekeeper, I want a rejected push and a failing forge script to remain distinguishable, so that I know which layer refused.
7. As a Beekeeper, I want a pull request closed without merging to be visibly terminal, so that a review queue does not carry a row that can never be approved.
8. As a Beekeeper, I want to choose what to do about a closed PR — retry the handover, ask for changes, or abandon delivery — so that the decision is mine and it is recorded.
9. As a Beekeeper, I want repeated publish and reconcile passes to be idempotent, so that polling does not multiply attempts or notifications.
10. As a Beekeeper, I want a Reactor restart during delivery to leave a recoverable record, so that I never see a half-recorded handover.
11. As a Beekeeper, I want to know who approved a trail and when, even for a PR-backed trail, so that the human decision is auditable.
12. As a Beekeeper, I want several approvals of the same episode to be one episode with several attempts, so that a retried publish is not mistaken for a second delivery.
13. As a Beekeeper, I want an experimental trail to record that its work was accepted and delivery was prohibited, so that "not delivered" is a fact rather than an absence.
14. As a Beekeeper, I want a root-proposal trail to record that acknowledgement fulfilled its handover, so that the single adopting mode is not read as a gap.
15. As a Beekeeper, I want a trail with nothing to merge to record a completed handover, so that a documentation-only trail is not permanently `waiting_review`.
16. As a Beekeeper, I want a trail's stage to distinguish "published, waiting for the forge" from "delivered", so that the trail header stops making me open the Reviews tab.
17. As an operator, I want `paseka delivery show --trace` to print mode, state, attempts, references, and timestamps, so that I diagnose from a shell.
18. As an operator, I want `paseka proposal approve` to print the recorded episode, so that I see what my action recorded, not just an error or silence.
19. As an operator, I want `paseka doctor` to name a delivery stuck in publishing or merging, a closed-without-merge episode, and a trail record that contradicts the forge, so that stuck handovers surface without a manual audit.
20. As an operator, I want Telegram to send one card per delivery transition, so that I learn about a publish or a merge without polling.
21. As an operator, I want kill to leave an already published PR alone and not pretend the delivery was aborted, so that the hard stop does not lie about the host.
22. As an operator, I want rework after a rejected gate to be a new attempt on the same episode, so that the trail shows one handover that took several tries.
23. As a Beekeeper, I want substantive new work accepted after a delivery to open a new delivery episode that names what it supersedes, so that a reopened trail does not overwrite its delivery history.
24. As a metrics reader, I want a delivery block in trail metrics, so that "work that reached the gate and did not land" is a countable outcome.
25. As a metrics reader, I want delivery failure counts separate from work failure counts, so that an aggregate does not blame the builders for a missing credential.
26. As a metrics reader, I want delivery timing — approved, published, merged — so that waiting on the forge is distinguishable from waiting on a person.
27. As a Queen Console user, I want Reviews to read delivery state from the trail, so that the badge and the button agree with the server.
28. As a Queen Console user, I want a published PR to show its attempt count and last state change, so that I can tell a fresh publish from a retried one.
29. As a colony author, I want no new delivery configuration in v1, so that enabling this spec changes no colony's behaviour.
30. As a colony author, I want an old trail without a delivery record to read as unknown, so that a missing record is never presented as "never delivered".
31. As a maintainer, I want delivery records to be removed with the trail's run data and kept by worktree-only cleanup, so that retention has one rule.
32. As a maintainer, I want delivery to stay out of the task lifecycle, so that the same mistake that made honey look like a task defect is not repeated for the forge.
33. As a maintainer, I want no new forge operations, so that this spec records the existing push/upsert/get contract rather than extending it.

## Implementation Decisions

### 1. Three axes stay separate

- **Approval** is a human decision on the merge gate. It is a fact about a person, not about a branch.
- **Delivery** is the act of handing the result over by the mechanism the colony selected.
- **Work completion** is the earlier milestone owned by [043b](043b-trail-metrics-and-work-completion.md).

Today the merge task's `task.completed` carries all three: for `local_merge` it means approved-and-landed, for `pull_request` it means the forge reported merged, for a root proposal it means acknowledged, and for an experimental fork it means accepted-without-delivery. This spec keeps `task.completed` as the ledger's "this gate is closed" fact — every existing consumer, route, and document already reads it that way — and records approval and delivery in the delivery episode instead.

Consequences:

- The gate still completes exactly when it completes today. No board column, count, filter, or route changes meaning.
- Approval becomes durable for PR-backed trails, which it is not today.
- A repeated approval is an attempt on the same episode, not a second episode.

### 2. Closed vocabulary

A delivery episode is one trail handover. Its recorded fields are a field contract, not a promise that legacy trails have them:

| Field | Meaning |
| --- | --- |
| `schemaVersion` | `1`. Independent of the protocol version; additive fields do not bump it. |
| `deliveryId` | Stable identity of the episode. Minted once, preserved across retries, restarts, and duplicate delivery. |
| `traceId` | The trail. The record is trail-scoped; there is no colony-wide delivery store. |
| `mode` | How the colony selected to hand the result over: `local_merge`, `pull_request`, `ack_only`, `none`. Recorded at approval, not read from living configuration later. |
| `state` | Terminal or in-flight state of the latest attempt (see the state machine). |
| `attempts` | Ordered attempt records. Each carries `attempt`, `action`, `state`, `startedAt`, `finishedAt`, `actor`, `failure` when failed, and the result and target snapshots that attempt observed. |
| `result` | What was handed over: `baseSha`, `headSha`, `commitSha` when known, `branch`, and the proposal reference when one exists. |
| `target` | Where it went: `kind`, `url`, `number`, `head`, `base`, `origin`. Empty for modes with no target. |
| `approvedAt`, `approvedBy` | The first approval that started the episode. Later attempts record their own actor and time. |
| `supersedes` | The previous `deliveryId` this episode replaces, when it replaces one. |
| `coverage` | `complete`, `partial`, or `unknown`, with reasons for missing, legacy, or conflicting evidence. |

Closed sets:

- `mode` ∈ `{local_merge, pull_request, ack_only, none}`.
- `action` ∈ `{merge, publish, accept, reconcile, retry, abandon}`.
- `state` ∈ `{publishing, published, merging, merged, accepted, closed_unmerged, not_delivered, failed, interrupted}`. An episode has no state before its first attempt: the record is created together with that attempt.
- `failure.reason` is an open string naming the layer — `git_push_failed`, `forge_command_failed`, `forge_rejected`, `origin_missing`, `worktree_missing`, `conflict`, and so on — because the next failing layer is not predictable. `failure.message` is the operator-facing text.

`handoverFulfilled` is **derived**, not stored: a reader applies the mode's contract to the state. Storing it would create a second source of truth that a mode edit could contradict.

### 3. Episode identity and attempts

- One episode per handover. The episode is created by the first approval that starts a delivery.
- `deliveryId` is minted once and reused by every retry, restart, and duplicate bus delivery. It is the dedupe key for observation events, exactly as episode identity works for the energy pause observations ([044b](044b-standing-supersession-and-energy-observations.md)).
- `attempt` is monotonic per episode. Reconciliation that observes no change writes nothing and publishes nothing, so polling cannot inflate the count.
- A retry after a failed attempt reuses the episode and increments the attempt. A re-publish of the same head is the same episode: the forge upsert is already idempotent, and the record must not pretend otherwise.
- A new episode with `supersedes` is created only when the trail's result changes after a terminal delivery — a reopened trail that produces new substantively accepted work. This keeps the earlier handover readable instead of overwriting it.
- Duplicate suppression is on `(deliveryId, attempt, action, state)`. A redelivered event or a concurrent reconciler therefore converges on one recorded outcome.

### 4. Handover contract per mode

`handoverFulfilled` is derived from the mode the episode recorded:

| `mode` | Fulfilled when | Notes |
| --- | --- | --- |
| `local_merge` | `merged` | The merge commit is the result. |
| `pull_request` | `merged` | Publication alone is not delivery: the gate stays open until the forge reports the merge, as today. |
| `ack_only` | `accepted` | The root-proposal mode adopts the work in place; there is nothing to land. |
| `none` | never | Pinned experimental work is accepted internally and must not be delivered. The terminal state is `not_delivered`. |

This is deliberately **not** configurable in v1. Publication-versus-merge is a property of what the selected mechanism guarantees, not a colony preference; a colony that wants "publishing counts as delivered" would be asking for a different delivery mechanism, not a different reading of this one. If someone asks for it, the extension point is a per-mode contract value, not a per-colony boolean.

The empty-gate case — a trail whose gate auto-completes because there is nothing to merge — records `mode` normally and `state: accepted` with the reason `nothing to deliver`. It is a fulfilled handover, not a gap.

### 5. State machine

```text
(approval or gate auto-completion creates the episode together with its first attempt)

local_merge:
  merging → merged
  merging → failed       (merge refused, conflict, worktree missing; retryable)
  merging → interrupted  (process died mid-merge)

pull_request:
  publishing → published → merged
  publishing → failed            (push or forge layer; retryable)
  publishing → interrupted
  published  → closed_unmerged   (host closed it without a merge)
  published  → failed            (a later re-publish attempt failed)

ack_only:          accepted
none:              not_delivered
nothing to land:   accepted

retry:    failed | closed_unmerged → same deliveryId, attempt++
abandon:  failed | closed_unmerged → recorded decision, no further attempts
```

- `published` is not terminal and not fulfilled. It is the honest description of a PR that exists and has not merged.
- `closed_unmerged` is terminal for the attempt and explicitly not a work failure. It is retryable and abandonable.
- `interrupted` exists for a crash between an attempted action and its recorded outcome. It is never silently promoted to success: a reconciler may resolve it by inspecting git or the forge, and the resolution is a new recorded attempt outcome.
- `merged`, `accepted`, and `not_delivered` are terminal. `merged` after `published` is a transition on the same episode, not a new episode.

### 6. Where the record is written

- **Approval, local merge:** record `merging` with the resolved base/head before the merge runs, then `merged` or `failed` with the merge commit. A crash mid-merge therefore reads `interrupted` rather than a false success.
- **Approval, pull request:** record `publishing` before the push, then `published` with the forge target, or `failed` with the layer's reason. This is the record that does not exist today.
- **Reconcile:** an observed change writes a transition — `published → merged` or `published → closed_unmerged` — with the forge-reported target. An unchanged observation writes nothing.
- **Root proposal approval:** `accepted`, actor is the human.
- **Gate auto-completion with nothing to merge:** `accepted` with the reason `nothing_to_deliver`, actor is the runtime. The episode is created by the same boundary that completes the gate.
- **Experimental approval:** `not_delivered` with the reason `experimental_delivery_disabled`.
- **Kill:** records no delivery transition. An already published PR is not closed on the host, and the episode keeps its last observed state.
- **Request changes after a rejection:** ordinary work, then a new attempt when the gate is approved again. No new episode.

The record is written by the same code path that performs the action, not by a watcher that might see the result without the cause.

### 7. Persistence, retention, portability

- The record lives with the trail's run data as `delivery.json`, written atomically by temporary file and rename. `schemaVersion: 1` is independent of the protocol version, following the metrics decision in [043b](043b-trail-metrics-and-work-completion.md).
- It is **not** ledger KV and **not** home state. The ledger owns the approval's ledger consequence (`task.completed`), and the home state file owns machine-local cache entries; neither survives a repository copy the way an in-trail record does.
- Reading has no dependency on the bus, the forge CLI, or the network. A trail whose forge is unreachable still reports its last recorded attempts and states.
- Retention follows the trail: removing the trail's run data removes the record; worktree-only cleanup leaves it readable.
- The home state file keeps its `pullRequests[]` entry as a cache for compatibility and for lookups by trail id that have not been migrated. The trail record is authoritative where both exist; a disagreement is reported as `coverage: partial` with a conflict warning rather than resolved silently.
- A legacy trail with a cached PR identity and no delivery record is readable as `coverage: partial`. A first reconcile may seed an episode marked `legacy` in its coverage reasons, but it must not invent approval time, attempt count, or publish time. A trail with neither is `unknown`, never "never delivered".

### 8. Observation event

- One `SIGNAL` kind announces a delivery transition. Its payload carries the trail envelope, `deliveryId`, `attempt`, `action`, `state`, `mode`, `handoverFulfilled`, and the target reference when one exists. State lives in the payload; the kind does not multiply per state.
- The event is a platform observation: excluded from direct dispatch, from deferred event buffers, and from auto-invite routing, following the precedent of the energy observations.
- Publication is mark-before-send. The record is durable first; a pending observation is published by the next pass or on startup, so a crash can lose one notification and never duplicate one.
- One path — a read-only Console merge-diff refresh — has no publisher. It may write the record and leave the observation pending; it must not fail the read because it could not announce.
- Reconcile publishes only on a real transition. A poll that changes nothing is not an event.

### 9. Surfaces

- **Queen Console Reviews** reads the episode instead of deriving a view from the home state cache: mode, state, attempt count, URL, last change, and the failure reason when the last attempt failed. `closed_unmerged` replaces the silent `waiting_review` row with explicit retry / request-changes / abandon actions.
- **CLI:** `paseka delivery show --trace <id> [--json]` prints mode, state, attempts, result and target references, timestamps, and coverage. `paseka proposal approve` reports the episode it recorded.
- **Export:** an optional `delivery` slice, so a trail export carries its handover without the home state.
- **Telegram:** publish, merge, close, and failure transitions become cards in the existing delivery/proposal category instead of polling.
- **Doctor:** a delivery stuck in `publishing` or `merging`, a `closed_unmerged` episode awaiting a decision, and a record that contradicts the forge.
- **Trail stage:** `delivered` is derived from a fulfilled handover, with the ledger's merge-task completion kept as the fallback for trails without a record. Published-but-unmerged reports its own stage rather than `delivered` or `idle`.

### 10. Metrics

[043b](043b-trail-metrics-and-work-completion.md) gains a `delivery` block: mode, state, attempt count, `handoverFulfilled`, approval/publish/merge timestamps, and failure counts by reason. Root aggregates stay total activity; delivery measurements are neither work nor analysis cost, and delivery failures never increment work-failure counters.

### 11. Compatibility and rollout

- No colony configuration changes. `defaults.delivery` keeps deciding the mode; `forge.command` keeps deciding the driver; nothing else is added.
- Old trails decode as partial or unknown. No migration rewrites history and no reader fills a missing approval or publish time.
- The task lifecycle is untouched: no new status, no new phase, and no change to when the merge task completes.
- Existing forge operations are unchanged. The record reads push/upsert/get; it does not add an operation, and it does not change what a failed forge call does to the approve request.

## Testing Decisions

Tests assert recorded outcomes, published events, and CLI/API output, using a fake forge driver and local fixtures rather than a real host. They should not assert private call order.

- **Local merge:** approve records `merging` then `merged` with base, head, and merge commit; a refused merge records `failed` with its reason and leaves the gate open.
- **Publish:** approve records `publishing` then `published` with URL, number, head, and base; the gate stays `waiting_review`; the trail log contains the delivery observation.
- **Publish failure:** a failing forge script and a failing push record distinct `failure.reason` values, dispatch no work, and do not fail the task.
- **Reconcile:** a poll observing `merged` records `merged` and completes the gate once; a poll observing `closed` records `closed_unmerged` and leaves the gate open with an actionable state; an unchanged poll writes no file and publishes no event.
- **Retry identity:** a repeated approve reuses `deliveryId` and increments `attempt`; a redelivered observation or concurrent reconciler produces one recorded outcome; attempt count equals recorded attempts.
- **Crash and restart:** an episode left in `publishing` or `merging` reads `interrupted` and is never reported as fulfilled; a pending observation is published once on startup.
- **No-publisher path:** a read-only Console refresh records the transition without publishing and does not fail.
- **Modes:** root approval records `accepted`; experimental approval records `not_delivered` and never calls push or forge; nothing-to-land records `accepted` with its reason.
- **Supersession:** new accepted work after a terminal delivery opens an episode with `supersedes` and leaves the earlier one readable.
- **Legacy:** a cached PR identity with no record reads partial without invented timestamps; a trail with neither reads unknown, not "never delivered".
- **Retention:** run-data removal removes the record; worktree-only cleanup leaves it readable.
- **Surfaces:** `delivery show` prints the same values `--json` returns; export includes the slice; Telegram sends one card per transition; doctor names each stuck case.
- **Stage:** published-but-unmerged is not `delivered`; merged is; a legacy trail falls back to the ledger.
- **Regression:** existing approve, publish, reconcile, close-without-merge, and experimental refusal tests keep their behaviour; the only new failure surface is the record write.

Prior art: review gate approve/reject/discard tests, pull-request publish and reconcile tests with the fake forge driver, proposal archive publication-state tests, energy pause observation durability tests, run/task record retention tests, and Console review-queue projection tests.

## Out of Scope

- **Publishing or merging differently.** Push ref rules, force-with-lease, upsert semantics, base selection, and the refusal to merge on the host all stay with [024](024-pull-request-delivery.md).
- **Per-cue delivery.** Choosing a mode per cue remains in the backlog; this spec records whichever mode was selected.
- **A `delivering` or `delivered` task status.** Delivery is not a task lifecycle state; see Further Notes.
- **Automatic approval or automatic merge.** A human still decides, and a forge merge still closes the gate.
- **Forge webhooks or a push-based reconcile.** Polling, the Console refresh, and the existing sidecar keep working.
- **PR review comments, checks, labels, or forge-native review state.** The record stores the handover, not the review conversation.
- **A method for deciding what to do about a failed delivery.** The record exposes retry, abandon, and request-changes; which one to pick is judgement.
- **A colony-wide delivery dashboard.** Metrics are trail-local, as in [043b](043b-trail-metrics-and-work-completion.md).
- **Changing `defaults.delivery`, `forge.command`, or the approval guard.**

## Further Notes

### Open decisions

- **D6.1 — What fulfils the handover for `pull_request`: publication or merge?** Proposed answer: merge, derived from the mode, because the gate already stays open until the forge reports it and a published PR is not a landable result. The alternative — recording an explicit `handover.fulfilledBy` per mode — is the extension point if a colony ever needs publish-only delivery, but it makes "delivered" mean two different things in one colony.
- **D6.2 — Is approval recorded in the ledger or in the delivery record?** Proposed answer: the delivery record, because approval is the transaction that starts a delivery episode and the ledger's merge task already means "gate closed". The alternative, a `review.approved` ledger fact, would add a second completion-like event to the task state machine for one consumer.
- **D6.3 — What should `closed_unmerged` default to?** Proposed answer: stay open and ask, as today, but with recorded and visible state plus explicit retry/abandon actions. The alternative — automatically reverting the gate to rework — would make a host-side action silently plan work.
- **D6.4 — How does a legacy PR identity become an episode?** Proposed answer: seed an episode on first reconcile, marked `legacy` and partial, with no invented timestamps. The alternative, refusing to read a legacy trail until it is re-approved, loses exactly the trails an operator most wants to audit.
- **D6.5 — Who owns `stage: delivered`?** Proposed answer: the delivery record when it exists, the ledger otherwise. The alternative, leaving the stage ledger-only, keeps a published PR indistinguishable from a quiet trail.

### Why not a task status

A `delivering` or `delivered` status would put a second meaning into a field that already has one, which is precisely the defect [044b](044b-standing-supersession-and-energy-observations.md) removed when honey used to write `blocked`. A delivery is a property of the trail's result, not of the gate task's lifecycle: the task is `waiting_review` while the result is published, and both are true. [045](045-trail-closing-phase.md) reached the same conclusion for the merge phase, and its Further Notes name the delivery vocabulary as the natural next spec.

### Why a file and an event, not one of them

The file makes the record readable without the bus, the forge, or the home state — which is the whole point of a trail-owned record, and the reason [040](040-trail-event-log.md) exists. The event makes the record observable without polling — which is the whole point of the choreography, and the reason the energy pause observations were worth adding. Either alone leaves half the requirement: a file-only design cannot notify, and an event-only design cannot be read after the bus is gone.

### Relationship to existing specs

- [024-pull-request-delivery](024-pull-request-delivery.md) owns the mechanism: push, upsert, get, the fake forge contract, and the refusal to merge on the host. This spec records what that mechanism did.
- [043b-trail-metrics-and-work-completion](043b-trail-metrics-and-work-completion.md) owns work completion and trail metrics. This spec starts where that milestone ends and supplies the `delivery` block.
- [045-trail-closing-phase](045-trail-closing-phase.md) owns the merge phase and the trail stage. This spec refines the stage's `delivered` source without changing the phase.
- [039-proposal-patch-records](039-proposal-patch-records.md) is the prior art for attempt-shaped records with publication state. This spec deliberately reuses the shape rather than inventing a second one.
- [033-cli-prune](033-cli-prune.md) owns retention. This spec adds one file to the same rule set.
- [044b-standing-supersession-and-energy-observations](044b-standing-supersession-and-energy-observations.md) is the prior art for an episode identity, mark-before-send publication, and a closed observation vocabulary.

### Boundaries this spec protects

- **A delivery is not a work verdict.** A trail whose work passed and whose push was rejected has a successful result and a failed handover, and both must be readable at once.
- **A published PR is not a delivered trail.** Conflating them is how a review queue fills with rows nobody can approve.
- **A missing record is not a negative fact.** Absence of delivery evidence never becomes "never delivered".
- **The runtime records; the operator decides.** Retry after a failure, rework after a rejection, and abandoning a closed PR are decisions this spec makes visible, not decisions it makes.
