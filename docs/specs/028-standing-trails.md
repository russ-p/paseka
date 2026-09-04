# Spec 028: Standing Trails

## Status

**(Draft)**
Captures the design conversation: long-lived named Flight Trails for recurring procedures (daily triage and similar), sidecar cron via Forage Cues, comb checkpoints, and a per-tick honey stipend. Not implemented.

## Problem Statement

Some colony work is not a bloom. Daily triage, dependency hygiene, and similar procedures must run on a schedule, remember what they already looked at, and avoid redoing the same pass. Today every omitted `--trace` starts a **new** Flight Trail: empty comb, fresh honey seed, no checkpoints. Reusing a manual `traceId` by hand almost works, but the rest of the model still treats a trail as one feature from scent through merge.

Honey is the wrong shape for a procedure that should live for months. `energy_budget` seeds once and then freezes. `energy.add` only increments, so a cron top-up **accumulates** leftover tokens until the anti-loop reserve is meaningless. `system.kill` is sticky: an accidental kill retires the identity; `energy.add` will not revive it. Prompt memory (`{{.Insights}}`) caps at a handful of narrative lines, so it cannot be the checkpoint. Isolated worktrees and `review: final` assume the trail will end in a merge.

Beekeepers do not want a loop orchestrator inside Hive Runtime. Timers already belong on the apiary ([016](016-cue-layer.md)). They want a **standing identity**: the same trail, the same comb, a ration of honey per tick, and a clear rule that product work is a **new** bloom trail — not another task on the procedure.

## Solution

Introduce a **Standing Trail**: a long-lived Flight Trail bound to a Forage Cue. The cue declares a stable `traceId` and a per-tick **stipend**. Sidecar cron (systemd timer, or any host wrapper) still calls `paseka cue run`; Paseka still does not ship a scheduler. Each successful cue run is one **tick**: refill honey **to** the stipend (replace remaining, do not add), refuse if a tick is already in flight, then publish the cue’s signal or task as today.

Checkpoints live in the trail comb ([014](014-artifacts-protocol.md)). Bees read and update structured files under `{{.ArtifactsDir}}` so the next tick does not re-triage the same items. Narrative insights stay short; they are not the memory.

A standing tick **observes and records**. If the procedure discovers work that needs a builder, it **spawns a bloom trail** (a normal cue or signal with a **new** `traceId`). The standing trail does not open an isolated merge gate or accumulate product diffs in its worktree.

Honey remains the only runaway brake: a noisy tick burns the stipend and stops. Kill remains the way to **retire** the standing identity. Pausing the procedure is an apiary concern (stop the timer); there is no unkill in this spec.

## User Stories

1. As a Beekeeper, I want a named standing trail for daily triage, so that checkpoints and honey accounting survive from one scheduled run to the next.
2. As a Beekeeper, I want the standing identity declared on the Forage Cue, so that I do not have to remember `--trace` on every cron line.
3. As a Beekeeper, I want `paseka cue run daily-triage "tick …"` without `--trace` to reuse that cue’s standing `traceId`, so that omitting `--trace` does not accidentally mint a disposable bloom trail.
4. As a Beekeeper, I want an explicit `--trace` that does not match the cue’s standing id to fail closed, so that a typo cannot attach the procedure to the wrong trail.
5. As a Beekeeper, I want an explicit `--trace` that matches the standing id to succeed, so that scripts can still be verbose.
6. As a Beekeeper, I want Queen Console Run cue to use the standing id when I omit `traceId`, so that a manual extra tick from the dashboard lands on the same comb.
7. As a Beekeeper on Telegram, I want `cue: daily-triage` to target the standing trail, so that a phone-triggered extra tick shares checkpoints with cron.
8. As a Beekeeper, I want Nuc export/import to carry standing cue fields, so that a procedure pack moves with bees and prompts.
9. As a colony author, I want standing cues to stay publish-only (`emit: signal` or `emit: task`), so that cues do not become a DAG or `bee run` launcher.
10. As a colony author, I want timers to remain outside Paseka, so that homelab systemd (or CI on the hive host) owns the schedule as in [016](016-cue-layer.md).
11. As a Beekeeper, I want the docs to stop recommending scheduled cues without `--trace` / standing as the default for recurring procedures, so that `nightly-deps`-style examples do not silently create a new trail every night.
12. As a Beekeeper, I want each tick to be a new ledger task (or a new direct-dispatch event), so that reactor dedupe does not swallow the second day.
13. As a Beekeeper, I do not want cron to `task retry` yesterday’s task, so that a failed tick does not become an infinite retry of stale intent.
14. As a Watch Bee / Medic Bee, I want `{{.ArtifactsDir}}` to be the same directory every tick, so that I can read yesterday’s checkpoint without inventing paths.
15. As a Watch Bee, I want to write a structured checkpoint file (issue keys, SHAs, skip list), so that the next tick can skip work already done.
16. As a Watch Bee, I want an optional human journal file beside the checkpoint, so that the Beekeeper can read what happened without using the checkpoint as prose memory.
17. As a Watch Bee, I want runtime to announce checkpoint changes via `artifact.written` only when the file content actually changed, so that a no-op tick does not wake Messenger.
18. As a Messenger Bee subscribed to `artifact.written`, I want to notify the Beekeeper only when triage produced a new handoff, so that quiet days stay quiet.
19. As a downstream bee on the **same** standing trail, I want to read `artifactKind` from the payload or filename, so that I can ignore journal files and honor `checkpoint`.
20. As a Beekeeper, I want prompt partials to say “read the checkpoint first; do not re-triage keys already listed,” so that LLM bees do not rely on `{{.Insights}}` (capped, narrative) as procedure memory.
21. As a Beekeeper, I do not want comb bodies auto-inlined into prompts, so that token use stays under the bee’s control (same as 014).
22. As a Beekeeper, I want the comb to remain gitignored with runs, so that working memory stays machine-local on the apiary.
23. As a Beekeeper, I want a documented warning that `paseka purge --runs` on the standing `traceId` deletes checkpoints, so that I do not wipe procedure memory by habit.
24. As a Beekeeper, I want knowledge that must survive apiary rebuilds to live in git (or a future Honeycomb), so that comb is not mistaken for colony source of truth.
25. As a Beekeeper, I want each tick to start with a **stipend** of honey, so that a runaway tick burns out instead of looping until I notice.
26. As a Beekeeper, I do not want leftover honey from a cheap tick to stack with tomorrow’s stipend, so that weeks of quiet days cannot accumulate an unbounded reserve.
27. As a Beekeeper, I want stipend to **set** `energyRemaining` to the declared amount at tick start, so that the semantics are “ration for this tick,” not `energy.add`.
28. As a Beekeeper, I want the first standing tick to seed `energyBudget` from the stipend (ledger `SeedEnergy`), so that Console low-honey thresholds still have a seed to compare against.
29. As a Beekeeper, I want later ticks **not** to change `energyBudget`, so that seed stays frozen like other trails.
30. As a Beekeeper, I want stipend to be a bus-visible platform SIGNAL (not a silent KV poke), so that replay and `paseka energy show` explain why remaining jumped.
31. As a Beekeeper, I want stipend **not** to unblock leftover honey-blocked tasks from a previous tick the way `energy.add` does, so that a new tick is a new task rather than a resurrection of a drained one.
32. As a Beekeeper, I want `energy.add` still available for a **live** tick that needs a one-off extra token, so that I can unstick today without changing the cue stipend.
33. As a Beekeeper, I want that extra add to be wiped on the **next** stipend (remaining set to stipend again), so that mid-tick generosity does not become a new baseline.
34. As a Beekeeper, I want a standing cue to reject `energy_budget` when stipend is declared, so that two honey numbers cannot disagree.
35. As a Beekeeper, I want stipend to be a positive integer, so that `0` cannot disable the anti-loop by accident.
36. As a Beekeeper, I want a killed standing trail to refuse cue run (including stipend), so that kill stays a terminal retirement of that identity ([013](013-system-kill.md)).
37. As a Beekeeper, I want `energy.add` after kill to still not redispatch, so that standing trails do not invent an unkill path via stipend or add.
38. As a Beekeeper, I want pausing the procedure to mean stopping the apiary timer, so that this spec does not add `standing.pause` / unkill.
39. As a Beekeeper, I want a second cue run to fail closed while a standing tick is in flight (`running`, `ready`, `planned`, or live AFK on that trail), so that overlapping medic bees do not race the same checkpoint.
40. As a Beekeeper, I want a new tick to be allowed when the previous tick task is `completed`, `failed`, or honey-`blocked`, so that a bad day does not permanently jam the procedure.
41. As a Beekeeper, I want `waiting_review` on a standing trail to fail closed on the next cue run, so that a misconfigured review policy cannot silently queue forever behind a HITL gate.
42. As a colony author, I want standing cues that `emit: task` to require `review: none` (or omit review), so that ticks do not open `required` / `final` gates.
43. As a colony author, I want the standing cue’s bee to run with `worktree: false`, so that months of observation do not pin an isolated worktree to `HEAD` from the first tick.
44. As a Beekeeper, I want doctor / cue validation to name the bee and the field when those constraints fail, so that misconfig is obvious before cron fires.
45. As a Watch Bee, I want product work (features, hotfixes) to run on a **new** bloom `traceId`, so that the standing trail never becomes the merge candidate.
46. As a Watch Bee, I want to spawn that bloom by running another Forage Cue **without** a standing id (or with a generated id), so that I reuse existing ingress instead of a new orchestrator.
47. As a Watch Bee, I want honey for the bloom trail to seed on **that** trail, so that a spawned feature does not spend the standing stipend beyond the observing dispatch already consumed.
48. As a Watch Bee, I do not want `task.plan` of a builder on the standing `traceId` to be the happy path, so that prompts treat same-trail implementation as a mistake.
49. As a Beekeeper, I want an isolated `code.proposal` that appears on a standing trail to be treated as a colony smell (doctor/warn; no synthesized `_review` if there is no merge candidate — already true for empty diffs), so that standing identity stays observational.
50. As a Beekeeper in Queen Console, I want standing trails visually distinct from bloom trails (badge or filter), so that a years-old triage identity does not look like a stuck feature.
51. As a Beekeeper, I want `paseka status` / Telegram traces to show the standing badge and stipend remaining, so that SSH and phone views match Console.
52. As a Beekeeper, I want `trace.title` set from the cue description (or cue id) on first standing seed when no title exists, so that lists show “Daily triage” instead of a raw id.
53. As a Beekeeper, I want later ticks not to overwrite a human or bee-updated `trace.title`, so that a refined name sticks.
54. As a Beekeeper, I want `trace.summary` optional and last-write-wins as today, so that a tick may refresh the subtitle with the latest outcome.
55. As a Beekeeper, I want export `--include artifacts` to still dump the standing comb, so that I can share a triage checkpoint without NATS.
56. As a Beekeeper, I want overlapping wrapper retries (double timer) to hit the in-flight refuse rather than burn a second stipend, so that 016’s “wrapper owns idempotency” has a platform backstop for standing cues.
57. As a Beekeeper, I want `paseka status --check` before cue run to remain the wrapper’s job, so that a timer does not publish into a dead hive and assume bees ran.
58. As a Beekeeper, I want cue success to remain “publish (+ stipend) succeeded,” so that standing does not wait for the AFK bee inside `cue run`.
59. As a platform, I want standing stipend events on the live-only deny-list (with `energy.add` / `system.kill`), so that bees cannot defer or spoof a refill.
60. As a platform, I want duplicate JetStream delivery of the same stipend event to be idempotent per tick identity, so that remaining is not applied twice for one cue run.
61. As a Medic Bee, I want the tick body to include the wrapper’s clock text (`tick 2026-09-04`), so that journal filenames and summaries have a stable date hint.
62. As a Beekeeper with two procedures, I want two standing cues with two `traceId`s, so that triage and deps hygiene do not share a comb or stipend.
63. As a Beekeeper, I do not want two standing cues to declare the same `traceId`, so that doctor/load fails closed on identity collision.
64. As a Beekeeper, I want a non-standing cue to keep today’s semantics (new trail when `--trace` omitted), so that `feature` / `hotfix` stay blooms.
65. As an eval author, I want a scripted standing tick twice on one fixed id to prove checkpoint reuse + stipend replace + overlap refuse, so that this is not only a homelab story.
66. As a documentation reader, I want **Standing Trail** in the glossary mapped to a long-lived `traceId`, so that copy does not say Loop (builder↔guard loops and “Bees Flying in Circles” already own that word).
67. As a Beekeeper, I want Hivewright changes to standing prompts/cues to stay `code.proposal.root` on a **bloom** hivewright trail, so that the procedure identity is not the place we edit the colony.
68. As a Beekeeper, I want HITL `bee chat` on a standing trail to remain possible but not consume stipend until/unless it goes through energy-gated session accept (existing invite honey rules), so that an interactive inspect does not look like a tick.
69. As a Beekeeper, I want ad-hoc `bee run` on the standing id without a cue tick to still see the same comb, so that I can debug a checkpoint without waiting for cron.
70. As a Beekeeper, I want that ad-hoc run **not** to apply stipend (stipend is cue-tick scoped), so that debug sessions do not reset honey.
71. As a Beekeeper, I want failed/cancelled bee runs not to flush `artifact.written` (014), so that a crashed tick does not announce a half-written checkpoint as truth; files may still sit on disk for recovery.
72. As a Beekeeper inspecting a failed tick, I want the previous successful checkpoint still readable if the failed run did not overwrite it, so that I can restore by hand.
73. As a Watch Bee, I want to update the checkpoint atomically from the bee’s point of view (write a temp file then replace — prompt guidance), so that a crash mid-write is less likely to leave truncated JSON.
74. As a Beekeeper, I want no built-in `report_to` callback to cron/GitHub, so that standing trails do not reverse 016’s “no webhook listener / no result callback.”
75. As a Beekeeper, I want machine-origin `source`/`agentId` for cron to remain optional later work (016 leftover), so that MVP scripted `cue run` may stay `cli`.
76. As a Beekeeper, I want ledger growth (a year of tick tasks) to be acceptable for MVP, so that compact/archive is not a ship gate.
77. As a Beekeeper, I want Console task boards on a standing trail to remain usable in the first weeks, even if they get noisy after months, so that MVP can ship without a rolling window.
78. As a Beekeeper, I want kill of a standing trail to cancel in-flight AFK as today, so that a runaway tick can be emergency-stopped.
79. As a Beekeeper, I want a **new** standing `traceId` (new cue field or edited YAML plus doctor) after a kill if I need the procedure again, so that resurrection is an explicit new identity, not unkill.
80. As a Beekeeper, I want spawned bloom trails to use ordinary `defaults.energy_budget` or that cue’s `energy_budget`, so that a spawned hotfix can still be cheap.
81. As a Scout Bee on a spawned bloom, I want no access requirement to the standing comb, so that feature intake stays a normal trail; the Watch Bee copies only what belongs in the cue Text.
82. As a Beekeeper, I want the standing bee’s dispatch to consume **one** stipend token like any AFK dispatch, so that accounting stays uniform.
83. As a Beekeeper, I want direct-dispatch standing ticks (`emit: signal` + bee `subscribes`) to consume honey the same way, so that choreography style does not bypass the ration.
84. As a Beekeeper, I want overlap refuse for `emit: signal` standing cues to key off live/in-flight bees on that `traceId`, not only ledger task status, so that direct ticks cannot pile up.
85. As a platform, I want cue validation to require `standing.trace` non-empty and a legal trace id, so that empty ids cannot collide with generated blooms.
86. As a Beekeeper, I want recommended ids like `trail-daily-triage` documented, without forbidding other manual id shapes already allowed by the ledger.
87. As a Beekeeper, I want profiles ([027](027-config-profiles.md)) not to silently change standing stipend or standing `traceId`, so that adapter-test overlays cannot retarget production procedure memory (follow 027’s “do not overlay energy/cues” spirit if 027 ships first; otherwise standing fields stay colony-canonical).
88. As a Beekeeper, I want Telegram Confirm to still apply for standing cues, so that a pocket tap cannot stipend+publish without the existing gate.
89. As a Beekeeper, I want CLI standing cue run to stay immediate (no confirm), matching 016.
90. As a Beekeeper reading Queen Console Artifacts, I want the checkpoint file previewable like any comb Markdown/JSON, so that I can audit skip lists without SSH.

## Implementation Decisions

### 1. Naming

- Technical: **standing trail** (stable `traceId` used as a procedure identity).
- Experience layer: **Standing Trail** (a kind of Flight Trail). Do **not** call this Loop in product copy, CLI, or prompts.
- Tick: one cue-run of a standing cue that applies stipend and publishes ingress. Not a new `EventType`.
- Bloom trail: any non-standing Flight Trail (today’s default). This spec does not add a `bloom` flag; absence of standing binding is bloom.

### 2. Where standing is declared

- Standing is a **cue** concern, not a free-floating colony list and not an `INSIGHT/trace.kind` MVP.
- Optional block on a cue file:

  - `standing.trace` — stable trail id (required if `standing` is present).
  - `standing.stipend` — positive integer (required if `standing` is present).

- Two standing cues with the same `standing.trace` → load/doctor error.
- Non-standing cues unchanged: omitted `--trace` generates a new id ([016](016-cue-layer.md)).
- `energy_budget` is **forbidden** on a standing cue (stipend is the only honey number). Non-standing cues keep `energy_budget` as today.

### 3. Default trace resolution

Order for a standing cue run (CLI, Console API, Telegram `cue:`):

1. If the caller passed a trace id and it equals `standing.trace` → use it.
2. If the caller passed a different trace id → **error** (fail closed).
3. If the caller omitted a trace id → use `standing.trace`.

Do not generate a mini-ULID for standing cues.

### 4. Honey stipend

- First time the standing `traceId` is seen with `energyBudget == 0`: ledger `SeedEnergy(traceId, stipend)` (same primitive as cue `energy_budget` on a **fresh** bloom). Then publish ingress.
- Every standing cue run after seed, if the trail is **not** killed: publish platform `SIGNAL` / `energy.stipend` with `{ "kind": "energy.stipend", "amount": <stipend> }` **before** ingress publish.
- Reducer: set `energyRemaining = amount`. Do **not** change `energyBudget`. Do **not** increment `energyAdded` (added remains operator top-up of blooms / live ticks). Display for standing trails: remaining vs stipend (seed still shown as budget = stipend).
- **Not** `energy.add`: add increments and unblocks honey-blocked tasks. Stipend must **not** run that unblock path.
- Previous tick `blocked` (honey exhausted) stays `blocked`; the new tick is a new task (or new signal). Operator `energy.add` during a **live** tick still unblocks that tick’s tasks as today; the **next** standing cue run sets remaining back to stipend.
- Killed trail: do not stipend, do not publish ingress; return a clear error naming `system.kill`.
- `energy.stipend` is live-only (defer deny-list). Bees are not the intended publisher; cue runtime is.
- Idempotency: one cue-run attempt applies stipend at most once (local apply-before-publish + reactor skip of own echo, same family as other ledger energy events). A **later** cue run always sets remaining to stipend again even if remaining was already equal (harmless).

### 5. Overlap (one open tick)

Refuse standing cue run (no stipend, no ingress) when **any** of:

- A ledger task on that trace is `planned`, `ready`, `running`, or `waiting_review`.
- Hive Runtime reports in-flight AFK/direct dispatch for that `traceId`.

Allow when all tasks are `completed`, `failed`, `cancelled`, or `blocked`, and nothing is in flight.

`waiting_review` is treated as jammed (fail closed) rather than “start another tick beside the gate.”

Error text must say the trail is busy and name the blocking task status or in-flight bee when known.

Wrapper still should `paseka status --check` and may remember timer slots ([016](016-cue-layer.md)); overlap refuse is the in-process backstop, not a GitHub `delivery_id` substitute.

### 6. Ingress per tick

- `emit: task`: new `task.plan` (+ `task.ready` if autorun). New `taskId` every tick. `review` must be `none` / omitted. `autorun: true` is the expected scheduled path; `autorun: false` is allowed for a parked tick the Beekeeper starts by hand.
- `emit: signal`: one colony SIGNAL as today; bees `dispatch: direct` as today. Dedupe remains per-event fingerprint when `taskId` is absent, so a new publish is a new run.
- Cue success = stipend (if applicable) + publish succeeded. AFK still needs `paseka run`.

### 7. Observational standing vs bloom spawn

- Standing ticks must not be the implementation path. Prompt partials for standing intents: read checkpoint; update checkpoint; if work is needed, **spawn** via `paseka cue run <bloom-cue> "…"` with **no** standing binding (new `traceId`), or `paseka signal` / `event emit` with a **new** `traceId`.
- Same-trail `task.plan` for builder/hotfix is documented as incorrect. MVP does not hard-deny arbitrary emits from the standing bee (too easy to break debugging); doctor and partials carry the rule.
- Spawned bloom honey is that trail’s seed, not a second consume on the standing stipend (the observing adapter dispatch already consumed one token).
- No new spawn orchestrator, wait-for-child, or callback into cron.

### 8. Worktree and review

- The bee named by a standing `emit: task` cue must have `worktree: false`.
- Standing `emit: signal` subscribers that are the intended tick consumer should also be `worktree: false`; doctor warns if a direct subscriber with `worktree: true` is the only matcher for that kind (best-effort; colonies can have mixed subscribers).
- Isolated proposals on a standing trail remain technically possible if a bee misbehaves; do not add a special merge path. Empty-diff `_review` skip stays as in the task ledger. Hivewright edits of the cue/prompt belong on a root-proposal **bloom** trail.

### 9. Checkpoints (reuse 014)

- Canonical procedure state: comb file(s) under the standing trail. Recommended names: `checkpoint.json` (source of truth), optional `journal/YYYY-MM-DD.md`.
- `artifactKind` stays basename-stem (`checkpoint`, `journal` heuristics as 014). No new YAML matcher on nested kind.
- Prompts: do not use `{{.Insights}}` as the skip list.
- Comb lifecycle unchanged: gitignored, purged with runs. Docs must state purge destroys standing memory.
- Durable facts the colony must keep → git (`docs/`, tracker), not comb.

### 10. Title and Console

- On first standing seed, if no `INSIGHT/trace.title` exists, cue runtime publishes `trace.title` from cue `description` (trimmed, 120-char cap) or cue id if description is empty. Do not overwrite later.
- Queen Console, `paseka status`, and Telegram trace lists: a **standing** badge when the `traceId` is referenced by a loaded standing cue (colony config, not a ledger flag). If YAML is removed, the trail looks like a normal leftover trail (honest).
- Honey UI for those ids: remaining / stipend (budget equals stipend after seed). Low-honey rule may keep `remaining <= energyBudget/4`.
- Filter or grouping of standing vs bloom is in MVP if the badge is easy; a dedicated Standing tab is not required.

### 11. Kill, pause, debug

- Kill = retire this standing identity (013 unchanged).
- Pause = stop the sidecar timer (and/or stop invoking the cue). No `standing.pause` event in this spec.
- Ad-hoc `bee run` / `bee chat` on the standing id: allowed; **no stipend**. Comb is shared. Interactive honey remains invite-accept rules ([006](006-human-gateway-invites.md)).
- After kill, operators create a new `standing.trace` (or a new cue id) rather than unkill.

### 12. Modules (indicative)

- Cue schema/validate/load: `standing` block, collision detection, forbid `energy_budget`, review/worktree checks for task cues.
- Cue run: resolve id, overlap gate, seed or stipend, optional first `trace.title`, then existing signal/task publish.
- Ledger: `energy.stipend` reducer; skip energy-add unblock.
- Protocol: payload + live-only deny-list.
- Doctor / `paseka cue` errors: collision, killed, busy, validation.
- Console/status/Telegram: badge; Run cue default id.
- Prompt partials + cues guide: checkpoint, spawn bloom, cron example **with** standing cue (no generated id).
- Glossary: Standing Trail.
- Eval: two ticks, stipend replace, overlap, checkpoint file reuse.

### 13. Relationship to existing specs

| Spec | Relationship |
| ---- | -------------- |
| [016](016-cue-layer.md) | Extends cues; does **not** add in-process cron, webhooks, or `report_to`. |
| [014](014-artifacts-protocol.md) | Comb is checkpoint storage; no change to scan-flush rules. |
| [013](013-system-kill.md) | Kill still terminal; stipend must not revive. |
| Task ledger honey | Stipend is a third mechanism beside seed and `energy.add`. |
| [011](011-trace-title.md) / [012](012-trace-summary.md) | Title auto-set once; summary optional per tick. |
| [005](005-feature-ideation-flow.md) | Spawned work uses ordinary `feature` / breakdown on a **new** trail. |
| [008](008-code-proposal-workspaces.md) | Standing bees stay on colony root. |
| [015](015-deferred-event-emit.md) | Stipend not deferrable. |

## Testing Decisions

Good tests assert **external behavior**: cue validation, trace resolution, stipend remaining, overlap refuse, kill fail-closed, publish shapes, comb reuse — not helper names.

- Load/validate: standing requires trace+stipend; stipend `0`/negative rejected; `energy_budget` + standing rejected; duplicate `standing.trace` across cues rejected; `review: required` on standing task cue rejected; standing task bee `worktree: true` rejected.
- Resolve: omit trace → standing id; matching `--trace` OK; mismatched `--trace` error; non-standing cue still generates a new id.
- First run: `energyBudget == stipend`, remaining == stipend, ingress published, `trace.title` present when description set.
- Second run after completed tick: remaining **set to** stipend even if leftover was 1 or `energy.add` left remaining above stipend; new task id; `energyBudget` unchanged; title not overwritten.
- Stipend does not complete or redispatch a previous `blocked` task.
- `energy.add` during a running tick still increments remaining (existing tests plus a standing fixture).
- Overlap: second cue run while task `running`/`ready`/`planned`/`waiting_review` fails; after `completed`/`failed`/`blocked` succeeds.
- Killed snapshot: cue run errors; no stipend event; no new task.
- Direct signal standing: two successive publishes dispatch twice when the first run finished; second publish while in-flight skipped/refused at cue run.
- Comb: first tick writes checkpoint; second tick sees file (014 flush still delta-based).
- Console/API: omit `traceId` on standing cue run uses standing id; list/status includes standing badge for ids mentioned in cues.
- Prior art: cue `energy_budget` seed tests ([016](016-cue-layer.md)); energy add/consume ledger tests; 013 kill; 014 scan-flush; eval colony reset/reuse of fixed `--trace`.

## Out of Scope

- Built-in cron, systemd generator, or cue `schedule:` field in colony YAML.
- HTTP webhook receiver, `report_to`, or GitHub delivery-id inside Paseka (wrappers stay in 016).
- `standing.pause` / unkill / resume-after-kill.
- Ledger compact, rolling task window, or auto-purge of old ticks.
- Honeycomb / KV knowledge base as checkpoint storage.
- Cross-apiary replication of comb files.
- Nested `artifactKind` subscribe matchers.
- Hard-deny of same-trail `task.plan` / `code.proposal` from standing bees (prompts + doctor only).
- New spawn protocol or child-trail wait.
- Per-tick stipend that **adds** with a cap (rejected in favor of set-to-stipend).
- Changing bloom `energy.add` accumulation semantics.
- Dedicated `--source cron` (optional later; 016 leftover).
- Standing-specific Telegram preview of checkpoint files.
- Auto-inlining checkpoint JSON into `{{.Insights}}`.
- Windows-specific scheduler docs.
- Treating standing trails as always-on interactive sessions.

## Further Notes

- Convention-only reuse of `--trace` + comb + `energy.add` was rejected as the product answer because add accumulates and omitted `--trace` is a footgun. This spec is the first-class version of that pattern.
- Open product gap (not required to Approve the rest): whether doctor should **warn** or **error** when a standing `emit: signal` kind has only `worktree: true` subscribers. Prefer warn in MVP if mixed topologies exist.
- Follow-up if operators actually keep a year of ticks: compact completed standing tasks in the ledger snapshot, or a Console “ticks” subset. Do not block MVP.
- Durable docs after ship: cues guide (standing + cron example), task ledger honey section, glossary, CLI `paseka cue` / `paseka energy`, Console Run cue. Specs stay unpublished ([specs index](../plans/specs-index.md)).
