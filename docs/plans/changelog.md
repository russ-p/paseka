# Changelog

Shipped features worth calling out. Design records live under `docs/specs/` in the repo (not published on the docs site) — see [Specs index](specs-index.md).

## 2026-10 — A trail now closes itself before the merge gate

A trail used to end with its last work task and a generic `_review` gate. The pull-request description was written by whichever task happened to be last, so a multi-task trail shipped a body describing one slice; nothing ever looked at the accumulated diff; and `review: final` had to mean both "this is the end" and "a human must approve it".

- **Two new ledger phases, runtime-owned.** `closing` and `merge` join the default `work`. Phase is recorded on `task.plan` and persisted, so replay, fork, `paseka verify`, and the Console read it from the ledger instead of colony configuration. A `closing` plan is honoured only from the reserved runtime agent; `merge` is derived from `review: final`; anything else a Bee or operator writes is `work`.
- **The wrap-up is guaranteed and bounded.** When every work task is completed and the colony's pinned `closing` policy matches, the runtime seeds exactly one AFK closing round (`closing-1`, `closing-2`, …) and kicks it through the ordinary scheduler. The closing Bee receives the whole trail — id, title, phase, intent, status, commit, summary, dependencies per task — writes the authoritative `pr.body` and `trace.summary`, and may plan follow-up work on the same trail. When that work completes the runtime reviews again, up to `max_rounds`.
- **Policy is authored once and pinned.** `closing.enabled`, `when.min_work_tasks`, `when.has_isolated_proposal`, `closing.task` (bee, intent, title, review, params), and `max_rounds` are validated at colony load and frozen into the trail's configuration snapshot at start. `task.params` overlay the closing Bee's params at dispatch, so a stronger model reviews large change sets without changing the bees that built them. Editing `colony.yaml` mid-trail changes nothing.
- **Silence is explained, and only once.** A round skipped by the predicates or by the cap writes a single `context.note`, on the boundary that opens the merge gate, instead of repeating on every approval or replayed completion. A closing task that ends failed or blocked keeps the gate closed and is never silently replaced.
- **The gate is a phase with authorable copy.** The runtime ensures exactly one `_review` task as `phase: merge`, rendered from `merge.task.title` / `merge.task.body` with the same trail inventory. An explicit `review: final` gate still wins and no second one is created. Approval is refused while any other work or closing task is incomplete, so a change set cannot be merged underneath a running round or unfinished rework. A delivered trail seeds no further rounds.
- **The truth is visible.** Cards and the detail header carry a quiet `closing` / `merge` tag beside the status, the trail detail gains a derived stage (`work` / `closing` / `gate` / `delivered`) computed server-side, `task.md` carries `phase`, the CLI prints it beside the status, and `paseka doctor` reports a closing seed that cannot be built, a stuck closing round, a cap reached with follow-up work, or a duplicate merge gate.

- Spec: [045-trail-closing-phase](../specs/045-trail-closing-phase.md)
- Canonical: [task ledger](../reference/task-ledger.md#task-phase-phase), [event contracts](../reference/event-contracts.md#task-plan-phases), [colony layout](../guide/colony-layout.md), [CLI](../guide/cli.md#paseka-task)

Not in this ship: publishing the pull request, any merge behaviour beyond copy (automatic approval, delivery-mode changes), colony-defined phase names, per-severity model routing, multiple closing Bees per trail, closing on standing trails, and the `trail.work.completed` trigger from [spec 043](../specs/043-trail-observability.md) — the v1 "new work completed after the last round" comparison stands in for it.

## 2026-10 — A typo in `dependsOn` can no longer close a trail

A dependency is an id, and an id typed wrong used to be published as-is. The task it belonged to then waited on something that would never complete: it stayed `planned` forever, and because the final gate opens only when every task but the merge gate has completed, the trail's review gate could never open — silently, with no event and no way to repair the task in place.

- **The producer refuses it.** `paseka task create` — and the Console create route that shares it — validates `dependsOn` before publishing: an unknown task id, a self dependency, an empty entry, or a cycle fails the create with the offending ids named. A plan may still depend on a sibling registered in the same `task.plan`, and a re-plan for an id that already has a status is judged by the stored edges, because the reducer would ignore the new ones.
- **The reducer stays tolerant.** Replaying an older trail still never fails on a dependency it cannot resolve; validation belongs to whoever is about to publish, not to the event log.
- **`paseka doctor` finds the trails that are already stuck.** It scans every trace in the task ledger and warns on a planned task whose dependency is unknown, cancelled, or blocked, or on a dependency cycle — naming the trace and the task, instead of leaving an operator to wonder why the gate never opened.

- Canonical: [task ledger](../reference/task-ledger.md), [CLI](../guide/cli.md)

## 2026-10 — A trail remembers the procedure it ran, and a fork tries another one

Every trail used to run whatever `.paseka/` happened to say at dispatch time. Editing a prompt, a bee definition, a skill list, or a model alias silently changed the next task, and afterwards no record could say which procedure an earlier run actually used — the files had already moved on. Testing a different procedure meant a second colony checkout with its own Reactor, home, and bus, whose results were hard to place beside the original. A trail now captures its configuration as an immutable, content-addressed snapshot, applies later versions only on an explicit operator action at a safe boundary, and can start a pinned experimental fork that inherits one source task attempt's evidence and changes nothing else.

- **A trail owns its configuration.** A new trail captures revision 1 before its first configuration-dependent validation or routing: bee and cue definitions, prompts and partials, the selected profile with its local contributions, model aliases, and the skills the configuration makes available. `initialConfigRef` and `activeConfigRef` are content identities (`ConfigRef`), not paths, so identical procedure in two trails compares equal while each trail keeps its own copy under `.paseka/runs/<traceId>/config/`.
- **Applying a version is explicit and safe.** `paseka trail config apply` takes exactly one source — current colony configuration, a retained snapshot for rollback, or validated `--set bee.<role>.<param>=<value>` overrides — previews the resolved change, and revalidates the remaining unstarted work. If anything is still executing under the old context, the action is recorded `pending` with its blockers instead of racing the cutover. Only the active reference moves; past runs keep their recorded `ConfigRef`.
- **An idle chain no longer parks the cutover.** A direct-dispatch chain that is open but not executing used to count as a permanent blocker, so an apply could wait forever on work that would never finish. Blocking is now decided by evidence: an attempt blocks only when nothing was ever dispatched or a run it owns is still non-terminal. Idle attempts stay open and keep their version; they just stop holding the gate.
- **A fork is a pinned trail in the same colony.** `paseka trail fork` copies one recorded source task input, starts a clean worktree at the source trail's initial code commit, applies a recorded proposal patch or nothing at all (a control arm), and seeds its own capped honey reserve. It runs on the same Reactor, home, bus namespace, and trail collection; its `fork.json` lineage names the source task, attempt, configuration revision, base, delta, workspace, branch, and budget.
- **Pinned means pinned.** Fork dispatch uses only the candidate snapshot. Editing the live colony or the fork's own checkout changes nothing, and `trail config apply` refuses a pinned trail with a clear reason. Internal proposals and verification still run, but every merge, branch-push, and pull-request entry point calls the review gate's experimental check, which reads the pinned policy and lineage, so a fork cannot deliver its branch even if its binding is missing or replaced.
- **Agents may ask; only an operator admits.** A bee with an explicit canonical grant can publish `SIGNAL/trail.fork`, which records a durable pending request — no trail, snapshot, worktree, honey, or task plan. The matching `approve`, `reject`, and `cancel` signals are accepted only from the reserved operator sender and perform creation (or refusal) exactly once under the request identity. Pending requests are quota-bound (20) and expire after 24 hours; CLI creation is operator approval in the same invocation.
- **The surfaces tell the same story.** `paseka trail list --forks`, `trail show`, `trail config show/capture/apply/cancel`, `trail fork pending/approve/reject/cancel`, and `trail drop` read the shared collection, and Queen Console shows the same configuration panel, fork page, and pending requests with approve/reject/cancel actions. Dropping a fork stops dispatch first, removes only its worktree and branch, and keeps configuration, lineage, runs, and comb readable.
- **A decision is recorded once and honoured.** `trail fork pending` also shows creations that have not finished, with their last error, and approval resolves the source attempt recorded on the request rather than whichever attempt the source has since gained. A retry between submission and approval neither changes the selected evidence nor turns the decision into a refusal, and a duplicate approval waits for the in-flight creation and returns the recorded outcome. A fork persists its lineage as `creating` and becomes dispatchable only after its initial plan and ready signal are published, so an interrupted creation is visible rather than running unpublished work.
- **Continuations stay on the procedure they started with.** Resuming a finished HITL session runs under the configuration revision its `session.json` captured; a revision that is no longer retained refuses the resume with `config_unavailable` instead of opening the chat on the active version, and sessions recorded before capture keep the active-version path. While a `trail config apply` is pending, a new independent direct reaction is recorded as durable pending dispatch work instead of being dropped, then resumes under whichever version remains active when the action is activated or cancelled; ready tasks resume once the boundary clears.
- **Honest gaps stay visible.** A trail created before capture is unbound rather than backfilled: `config capture` can establish revision 1 for future work but cannot claim a snapshot for earlier runs. A fork refuses an ambiguous root task, a dependency-bearing task, a synthetic final-review task, an unrecorded dirty source base, or missing source evidence. Applying configuration never retries, clones, or replenishes anything.

- Spec: [042-trail-fork](../specs/042-trail-fork.md)
- Canonical: [CLI](../guide/cli.md#paseka-trail), [event contracts](../reference/event-contracts.md#forked-trails-and-consumer-scope), [interactive sessions](../guide/interactive-sessions.md), [glossary](../idea/glossary.md), [colony layout](../guide/colony-layout.md)

Not in this ship: a configurable active-fork cap, per-source fan-out limit, or cumulative experiment allowance; agent-initiated configuration application; whole-plan or dependency-carrying forks; a shared snapshot store with cross-trail dedupe, reference counting, or GC; broker-level or OS-level isolation for experimental work; and metrics `forkOf` reads, which belong to [spec 043](../specs/043-trail-observability.md) (Draft).

## 2026-10 — OpenCode upgrades stop breaking the colony

Every colony on `adapter: opencode` broke the moment OpenCode 2 landed: `run --dir` was gone, so isolated worktrees silently stopped isolating; `--variant` was gone, so reasoning effort and cost accounting drifted; the TUI rejected `--model`, `--agent`, `--port`, and `--hostname`, so `paseka bee chat` could not start at all; and the V1 pre-create path retried a `405` until its timeout, hanging the launch before losing the provider session id. The two majors cannot be installed side by side under the one command Paseka shells out to, and a Beekeeper may have either — or both under different names.

- **One name, three adapters.** `opencode` is now a thin proxy that detects the installed major and delegates to the frozen `opencodev1` line or the new `opencodev2` line. All three names are valid `adapter:` values in bee YAML and profiles; the proxy still reports itself as `opencode`, so trails, Console grouping, status, and existing colonies keep working.
- **Detection is cheap and fail-closed.** One `opencode --version` call, default five seconds, cached in-process by binary path plus modification time and size; a replaced binary re-detects and a swarm detects once. Unparsable output is an actionable error, never a silent fallback to V1 flags; a missing binary names the install route (`npm install -g opencode-ai@1` or `@2`). A version pin skips detection and is verified against the binary when it answers; a disagreement is an explicit mismatch error.
- **Per-bee or machine-wide pinning.** The machine-local `adapters/opencode.yaml` gains `version` (`auto` \| `1` \| `2`), `v1_binary`, `v2_binary`, and `detect_timeout`. Bee `params.options` carries the same keys and wins, so one bee can pin a major without changing the colony. With both majors installed under different names, `v1_binary` / `v2_binary` drive both from one colony.
- **V2 AFK is scoped by cwd, not a flag.** `opencode run` now runs from the workspace directory; the adapter asserts that directory before dispatch and warns if the created session's location does not match. The reasoning variant rides on the model as `provider/model#variant`, plan mode still selects the `plan` agent, and auto-approval keeps unattended bees from stalling. A custom `command:` override is passed through untouched.
- **V2 accounting reads the session, not the stream alone.** A single-step V2 run may emit no token event, so usage falls back to the session record over the API. Reasoning tokens and reported dollar cost are recorded, and a provider outcome of `failed` or `interrupted` is surfaced as a run warning instead of passing as a healthy zero-token run. New labels `opencode.v2-run-stream` and `opencode.v2-session-record` keep the V1 labels (`opencode.run-json`, `opencode.server-json`) meaningful in old files.
- **V2 HITL owns its server and its session id.** Paseka starts `opencode serve` for the session lifetime, mints a `ses_*` id, creates the session over `/api/session` with workspace, title, agent, and model variant, and launches the TUI with only `--server`, `--session`, `--prompt`, and `--auto`. If the provider rejects the id, the client retries without one and reads the assigned id back. The V1 control-server and prompt-injection path is not ported; it had no reason to exist once `--prompt` works with `--session`.
- **Identity and Resume stay honest across an upgrade.** Run meta and `session.json` keep the configured adapter name and add the concrete `implementation` that ran. Resume remains eligible on name grounds after an upgrade, but a recorded V1 session continued while a V2 binary is installed (or the reverse) is refused as ineligible with `generation_mismatch`; sessions recorded before the split carry no implementation and stay eligible. A pinned source continued through a differently pinned bee is still `adapter_changed`.
- **Scaffolding follows the choice.** `paseka init --adapter opencode` keeps writing version-agnostic `adapter: opencode` bees and the one adapter file, now documenting the new keys. `--adapter opencodev1` and `--adapter opencodev2` scaffold pinned bees and print the matching install command. The Settings adapter list keeps a single `opencode` row.

- Spec: [037-opencode-v2-proxy](../specs/037-opencode-v2-proxy.md)
- Canonical: [adapter launch resolution](../reference/adapter-launch-resolution.md#opencode-version-proxy), [bee config](../guide/bee-config.md), [interactive sessions](../guide/interactive-sessions.md), [architecture overview](../architecture/overview.md), [colony layout](../guide/colony-layout.md), [CLI](../guide/cli.md)

Not in this ship: continuing a session across OpenCode majors (refused, not attempted), Console live streaming or transcript hydration from the OpenCode store, per-turn diff views, V2-native worktree management, ACP/plugins/Code Mode, and answering permission requests through the API.

## 2026-10 — An exhausted honey reserve no longer rewrites a direct reaction

When honey ran out while a guard, receiver, or fix-up Bee was reacting to an event, the runtime wrote `task.status=blocked` over the task's real state. A task in `waiting_review` lost its review state, and a later `energy.add` restarted the task's builder instead of the blocked reaction.

- **Direct reactions defer without touching the task.** The runtime records a durable pending direct dispatch and leaves `task.status` unchanged.
- **Top-up resumes the same reaction.** `energy.add` recovers the source event by identity and drains pending direct work in creation order, without re-applying the event to the ledger.
- **The queue survives restart.** Pending work lives in the trail ledger, not in the in-memory dispatch maps.
- **Task dispatches keep the old path for now.** A deferred task dispatch still uses `blocked` and the existing unblock path until [spec 044](../specs/044-energy-admission-and-pending-dispatch.md) completes the admission/pending split.

- Canonical: [Task ledger](../reference/task-ledger.md), [CLI](../guide/cli.md), [Forage Cues](../guide/cues.md)

## 2026-10 — A proposal outlives the review that produced it

Once a trail merged, the diff a run proposed was gone: the review worktree was removed, the synthesized `MUTATION` inlined the diff into an event payload nobody archived, and above 64 KiB the runtime stashed the bytes in a JetStream object store that no surface could read. On a real trail an 18-file, +1433/−14 reviewed change left nothing behind. Every proposal attempt now writes an immutable patch and a metadata sidecar to the trail's run tree, and the `MUTATION` points at it.

- **A new first-class record, not a comb artifact.** Patches live at `.paseka/runs/<traceId>/proposals/<agentId>.patch` with a `<agentId>.json` sidecar. `proposals/` is a reserved trace subdirectory — never read as a Bee run directory, never merged into trail event reads — and it is deliberately **not** the comb: a patch is captured code history the runtime owns, not a handoff a Bee wrote, so `artifact.written` and comb listings keep meaning handoffs.
- **The runtime writes it once, for AFK and HITL alike.** `internal/patches` owns the store (atomic install, immutable body, sidecar as the visibility marker); `internal/proposal` owns the shared gate, collector, writer, and publisher. A successfully completed AFK run or interactive session that auto-publishes a `code.proposal*` and whose collector returns a non-empty diff gets one record. A failed, cancelled, or completion-contract-rejected run gets none, and a session that already emitted its own proposal is passed through untouched so a Guard is never dispatched twice.
- **The payload carries a ref, not a diff.** A synthesized proposal sets `ref` to a colony-root-relative path and leaves `diff` empty, so mutation payloads are small and uniform and a consumer no longer branches on diff size. The reader resolves the ref against `ColonyRoot`, not a sector or worktree. Explicit inline diffs and legacy object-store refs keep their own meaning — the archive path shape is the only thing reclassified.
- **The Guard still reviews the working tree.** For a new archive ref the task text carries the summary and the workspace instruction, and neither the patch bytes nor the pointer. Saving a proposal asserts nothing about the code a Guard approved, and handing over the archived bytes would imply a correlation this record does not make.
- **The object-store offload is gone.** The 64 KiB threshold, the runtime's artifact-store dependency, and the silent `ref`-for-`diff` swap all go away; a missing object store can no longer affect a proposal. `paseka purge --bus` still deletes objects written by earlier versions.
- **Publication state is honest.** A record installs as `unpublished`, flips to `unconfirmed` immediately before a real broker attempt, and to `published` only after an acknowledgement. A nil, file-only, or no-op publisher cannot confirm anything and leaves it `unpublished`; an error or interruption after an attempt leaves `unconfirmed` with the last error, and never deletes bytes a published event may already point at.
- **Two read surfaces.** `paseka export --include patches` adds a self-describing slice — run identity, bee, AFK/HITL mode, kind, workspace, base SHA, summary, size, file count, capture time, publication state — inlined within a 512 KiB per-patch and 8 MiB aggregate budget, with oversized, unsuitable, or unreadable bodies listed by metadata and an explicit reason. The Queen Console grows a per-trail `Proposal patches` block beside the live merge-gate preview, with an on-demand bounded preview that never presents a truncated body as the original.
- **Retention is one mechanism, not two.** Patches ride on the existing whole-trail run retention: `purge --runs` and `prune --runs` remove the store with the trail, and `purge --bus` leaves the bytes and their last-known publication state on disk. An active Standing Trail can accumulate records until it is explicitly cleaned — there is no individual-patch TTL and no storage quota.
- **What the archive is not.** It preserves exactly what the collector returned, with its documented limits: earlier edits inside a changed file can be included, untracked files and changes committed during the run can be omitted, deletions can fail collection, and `baseSha` is context rather than a promise that applying the patch reconstructs the workspace. It is a history of proposals, not a backup and not proof of reviewed state. Historical trails have no proposals directory and read as empty; nothing is migrated.

- Spec: [039-proposal-patch-records](../specs/039-proposal-patch-records.md)
- Canonical: [Architecture overview](../architecture/overview.md), [event contracts](../reference/event-contracts.md), [interactive sessions](../guide/interactive-sessions.md), [CLI](../guide/cli.md), [Queen Console design system](../architecture/queen-console-design-system.md)

Deferred from that work: a streamed original-download endpoint or offline package for retained bodies, and the side-by-side per-run diff UI — see [Backlog](backlog.md).

## 2026-10 — A trail keeps its own record of what happened

A Flight Trail's history used to be assembled from one file per Bee run, and only a Bee that published through the event CLI ever reached it: the operator's intake request, both synthesized code proposals, the ready signal, the honey charges, the task status transitions, and the runtime's own trail summaries were all absent from the surface an audit reads. On one audited trail, 28 events read as 11. Every trail now keeps one append-only record of its own domain events — and every trail surface reads it.

- **The record is written by the publisher seam, before the bus.** `bus.Publisher` is the colony's event sink: a client records each domain event in its trail and then publishes it, and `bus.LocalPublisher` records without a bus at all, so a colony configured without NATS keeps a first-class record. One writer means a new producer cannot forget to record itself, and writing first closes the window where a crash between publish and record lost the event.
- **Producers with no run directory finally land somewhere.** The runtime, the task ledger, the honey reserve, the review gate, the cue layer, the console, the Human Gateway, and the CLI all go through that one seam — including an operator signal, a console action, and an explain run, which have no run directory of their own. A run whose Bee published nothing now records its synthesized proposal, so a trail that changed code never reports that nothing was proposed.
- **Every event carries an identity, minted where the event is created.** Storing the same event in a run's audit log and in the trail record is safe because a merged read collapses it to one row, which also keeps an at-least-once bus from inflating a trail. The identity is readable in the raw envelope on a timeline row, so a row can be correlated with a bus message. It is colony-minted: no producer supplies it, no bee sees it, and the protocol version is unchanged.
- **The two files stay different things.** A run's `events.ndjson` is still the per-attempt audit of what that run published, with its documented path, its `PASEKA_EVENT_LOG`, and its lifecycle; `trail-events.ndjson` at the trail root is the whole-trail record. They are named differently on purpose — a path fragment gets quoted on its own, and an audit tool that answers with the wrong half is the worst kind of wrong.
- **Reading needs no broker.** The timeline, the export, prompt memory, trail titles and summaries, and worktree branch and pull-request copy read the trail's history through one reader, so they cannot drift apart. A colony-wide feed costs one file per trail instead of every run log in every scanned trail, and a trail with events but no runs is listed rather than dropped.
- **The colony-wide Timeline reads thinner for trails that predate this record — nothing is backfilled.** A trail that ran before this feature has no trail record of its own, and the colony-wide feed reads one file per trail, so **its events no longer appear in the colony-wide Timeline or in the dashboard's recent insights**. Open that trail and nothing is missing: its own Timeline, its export, its prompt memory, its title and summary, and its branch and pull-request copy all still read the older per-run logs, and the trail still appears in the trail list. The gap is the colony-wide view, and only for history written before this ship.
- **A trail's newest fact is not always a dispatch.** Last activity now includes honey charges, kills, and runtime-synthesized events, so a trail that was charged or killed after its last run no longer sorts as stale.
- **A human action reads as a human.** An event from the CLI, the gateway, or a Console write is attributed to the beekeeper, and a runtime-synthesized one to the runtime, rather than appearing on the timeline as an unknown bee.
- **What the record is not.** It is what the colony decided to do, not proof of what was broadcast: nothing marks a publish failure on it, and rewriting a line to do so would make an append-only log something else. `paseka verify <traceId>` is the answer — it compares a trail against the stream and reports what is only local, only on the bus, or in both. It waits for the stream to be drained rather than for it to go quiet, and reports **inconclusive** if that read does not finish; with no broker reachable it reports that verification was not possible. Neither is ever printed as a divergence.
- **A torn line costs one event.** Both event logs are appended a line at a time, so a process that dies mid-write leaves half a line behind; the reader skips that line with a diagnostic naming the file and the line, and returns the rest of the history. A crash mid-append no longer costs a trail its whole record.
- **A colony with no broker can still emit.** `paseka event emit` records the event in its trail instead of reporting `nats_not_configured`, matching every other publishing site. The result carries no subject, which is how a caller can tell it was recorded rather than broadcast.

- Spec: [040-trail-event-log](../specs/040-trail-event-log.md)
- Canonical: [Architecture overview](../architecture/overview.md), [event contracts](../reference/event-contracts.md), [colony layout](../guide/colony-layout.md), [CLI](../guide/cli.md)

Deferred from that work: the duplicate `trace.summary` still cannot be explained, and a trail-list index that would carry last activity and event count instead of walking every trail — see [Backlog](backlog.md).

## 2026-10 — A run records the model it answered on

Which model produced a run used to be recoverable only by re-deriving it from a bee's `params`, the machine-local adapter layer, the selected profile, and the alias table — all live config, so the answer described the colony as it is now rather than as it was when the run flew. Every run now records the model it actually launched with, at the moment that model was resolved.

- **The launch is recorded once, where it is resolved.** `request.json` carries `model` (the id after alias substitution, before the per-adapter argv split), `modelAlias`, `profile`, and `launchForm`. The runtime writes it, so no adapter changed and the record cannot drift from what was launched.
- **The Console states it.** The Runs list stays five columns and takes the model as a filter term beside the adapter; the run page leads with `{bee} on {adapter} · {model}` and adds `Model` and `Profile` identity rows.
- **An export carries it.** Markdown and HTML run blocks name the model and profile, because neither the bee's config nor the alias table travels with an archive.
- **An empty model means something specific.** A `command` or `script` bee takes its argv from the bee and named no model; `launchForm` distinguishes that from a fallback to the adapter's default.
- **What is deliberately absent.** `modelAlias` is recorded but not projected, and nothing reports what a provider *actually* answered on — only what the launch resolved to.

- Canonical: [Architecture overview](../architecture/overview.md), [bee config](../guide/bee-config.md), [Queen Console design system](../architecture/queen-console-design-system.md)

## 2026-10 — Launch params describe the work, not the vendor

A bee's `params` block now carries only colony-level decisions, and one resolver turns them into a process. Before, four code paths merged the same layered config and each adapter re-assembled its own argv and environment, so a committed `params.binary` never took effect, a profile could not turn a colony boolean off, and a param typo was a silent no-op. Resolution now happens once, in a `LaunchPlan` shared by AFK runs and interactive sessions alike.

- **Semantic keys, adapter-owned flags.** A bee says `yolo`, `plan`, `reasoning`, `model`, `binary`, `env`, `env_mode`, or `timeout`; each adapter maps them to its own flags. Output format is runtime-owned, with `paseka bee run --raw` as a per-invocation debugging escape hatch.
- **The vendor rides in the model id.** Alias values are full ids (`high: anthropic/claude-sonnet-4`), so switching a profile's `model_aliases` moves a bee's vendor and model in one atomic edit. Pi splits the first `/` and keeps the rest intact, so OpenRouter-style ids survive the trip.
- **A planned environment.** The child env is assembled explicitly: inherited base, the resolved credential (recorded for log redaction), and declared `params.env` pass-through; `params.env_mode: strict` opts into denying undeclared secrets. `post_exec` now inherits the same environment.
- **AFK runs can time out.** `params.timeout` bounds a headless run and reports `cancelled` with reason `timeout`; interactive sessions stay unbounded.
- **The launch is explainable.** `paseka doctor --explain <bee>` prints the plan and the layer that set each field.
- **The migration window is still open (one release).** The retired keys keep working with a deprecation warning: `trust`/`force` → `yolo`, `thinking` → `reasoning`, `provider` → fold the vendor into `model` (`model: <vendor>/<model>`), and `output_format` → runtime-owned. A bee or profile that still declares one loads; `paseka doctor` names the replacement. **Removal is deferred to the next release**, so operators get a full release to migrate.

- Spec: [038-adapter-launch-resolution](../specs/038-adapter-launch-resolution.md)
- Canonical: [Adapter launch resolution](../reference/adapter-launch-resolution.md), [bee config](../guide/bee-config.md), [colony layout](../guide/colony-layout.md), [interactive sessions](../guide/interactive-sessions.md)

## 2026-10 — A review can be closed without merging

A review-gated task now has a third outcome beside approve and reject: **discard**. It is the deliberate "not doing this" close — the terminal, non-completed end of the queue that approve (merge or publish) and reject (feedback, and rework on a final gate) both left uncovered. A task waiting on a human could be accepted or sent back, but there was no way to simply stop, so work nobody wanted kept the queue occupied.

- **Discard does nothing but close the task.** No worktree merge, no pull-request publish, no `human.feedback`, and no rework task: the task lands `cancelled` with an optional reason as its summary. The worktree is kept, not pruned, so a discarded result can still be revived.
- **One verb, three surfaces.** `paseka proposal discard --trace --task [--reason]` mirrors the existing approve/reject subcommands; `POST /api/traces/:traceId/tasks/:taskId/discard` carries `CanDiscard` to the console review queue; and the review action panel grows a Discard button with a short confirmation. Unknown or non-`waiting_review` targets fail with the same errors as approve and reject.
- **It reuses the shared review gate.** `internal/review.Discard` runs the same `waiting_review` + `IsReviewGate` validation and writes a `task.status` event with `cancelled` — not `task.completed`, so it does not promote the next planned task or open a merge gate.

- Canonical: [CLI](../guide/cli.md), [Queen Console spec](../specs/002-queen-console-mvp.md)

## 2026-10 — A task says when it was created

`task.md` frontmatter now carries **`createdAt`** next to `updatedAt`, so "how long has this task been sitting here?" is a subtraction instead of an inference. Until now only `updatedAt` was on disk, and it moves on every status write — so an offline read of a trail could not tell a task that waited a day from one that ran straight through, which is exactly the question a trail audit asks.

- **The stamp is written once and never moves.** The ledger sets it the first time it sees the task — normally on `task.plan`, and on `task.status`, `task.completed`, or a code proposal for a task that arrives before its plan — and every later transition leaves it alone.
- **Tasks now start in the order they were planned**, as [spec 031](../specs/031-task-scheduler.md) has always said: when several tasks are eligible in the same tick, the oldest goes first, with the task id breaking ties. Until now the order was lexicographic, so `task-10` ran before `task-2`. A task with no `createdAt` — a trail planned before the ledger stamped one — sorts last rather than pretending to be the oldest.
- **Older trails still read.** The field is omitted when unknown rather than written as a zero time, so a `task.md` written before it existed parses as before, and a blank `createdAt` means unknown rather than "created at the epoch".

- Canonical: [task ledger](../reference/task-ledger.md)

## 2026-10 — A gated task links to its own review

A task stopped at a review gate could be **approved** or **rejected** from its own page, but not **read**: the diff lives on the review route, and the only way onto it from there was to open the review queue and find this trail's row among the others by eye — so approving a change meant approving it unread. The Review section now carries **Open review** beside the buttons it belongs with, under the same gate that renders them, and it lands on the proposal rather than the full-screen preview below it — that is where a missing branch or an empty diff is explained, and where the way back to the task is. The label and its chevron name the destination; what is behind it — the diff and the notes on a final gate, the proposal on a mid-trail one that merges nothing — is said in the hint beside it.

- Canonical: [Queen Console design system](../architecture/queen-console-design-system.md), [Queen Console](../guide/queen-console.md)

## 2026-10 — A bee can be run from the console

The **Bees** page now has a **Run bee** button beside **Refresh** — a headless run of a bee you pick, the browser equivalent of `paseka bee run <role> --body "…"`. Until now the console could only *watch* bees work: starting one meant an interactive session, and the session picker cannot offer a `script` bee at all, so a colony of scripts had no way in from the browser.

- **The launch is a header button, shaped like the session one.** `Run bee` stands where `Launch session` stands on Sessions, and the roster keeps no per-row control: which bee to run is the form's question, not the table's. The form takes the roster from the page when the page has one and reads the colony roster itself when it does not — **every** bee, `script` rows included, because this is the only launch those rows have — and the bee it lands on rebuilds the intent list.
- **It does not need `paseka run`.** The console dispatches the adapter itself, exactly as the CLI does, so the button works with the Hive Runtime stopped. That is why it is deliberately neither **Run cue** nor **New task**: both publish an event for the runtime to pick up, and neither does anything while it is down. Honey is not spent on a run started this way, the same as `paseka bee run` — only work that goes through the runtime is metered.
- **The form is the colony's, not the launcher's.** Intent, task, and an optional trail id, plus **Advanced: write the prompt myself** for a prompt sent verbatim, with no request of its own: the roster the operator is reading is the one the picker offers. For a `script` bee the task is optional, because the adapter runs the command from its own YAML.
- **A trail can start work too.** The trail page carries **Run bee** beside *Open timeline*, opening the same drawer with the trail prefilled — so continuing a trail's work is one click and no typed id. The toast there has no **Open trail**: the operator is already standing on the page that action would navigate to. The form is one component in `lib/components`, used by both routes, because two forms for one verb is how they drift.
- **The answer arrives before the agent does, so it is a trail and a toast.** `POST /api/bees/:role/run` returns the trace id and nothing else — no agent id, no output — because a run lasts as long as its agent and a browser does not. **Open trail** is offered rather than taken, and the roster is deliberately not refreshed: a refresh in the same second would report a Last run that has not been written yet. The run is watched under **Runs**.
- **What can be refused is refused in the form.** An unknown bee, a bee that reads a prompt with no task, and a bee with nothing configured to render that task through are all answered before the run starts. Past those checks a failure — a missing adapter binary, a deleted template file — can only reach the terminal running `paseka console`, and the guide says so rather than implying the page would show it.

- Canonical: [Queen Console](../guide/queen-console.md), [Queen Console design system](../architecture/queen-console-design-system.md), [CLI](../guide/cli.md)

## 2026-10 — Token counts for Pi and OpenCode sessions

Bee runs now say what they cost in tokens, not just that they finished. `paseka inspect`, trace totals, and the console read the same numbers for headless and interactive sessions.

- **Pi reports usage on every path.** In `--mode json` the adapter sums each assistant turn's final usage from the event stream, skipping the streaming deltas that repeat the same totals and the tool results that carry tool-level rather than model tokens. When stdout has nothing to count (`text`, `rpc`), it reads the run-scoped session file instead — so a bee switched to plain text output does not silently lose its token totals.
- **Interactive Pi and OpenCode sessions report usage too.** A TUI prints nothing machine-readable and its control server dies with the process, so after exit the runtime asks the adapter what the session spent: Pi from its session file, OpenCode by starting a throwaway `opencode serve` and asking for the session's totals. The answer lands on `session.json` as `usage`, which is where trace totals now look when a run has no `result.json`.
- **Adapters that cannot answer leave the field absent**, and a provider that is slow or unreachable costs nothing but a skipped number — session teardown never waits on it and never fails because of it.
- **Honey does not move.** Honey stays dispatch-count based; tokens are observability, not billing, until someone writes that design down.

Deferred from that work: Cursor interactive sessions still have no token count — see [Backlog](backlog.md).

- Canonical: [architecture overview](../architecture/overview.md), [interactive sessions](../guide/interactive-sessions.md), [bee config](../guide/bee-config.md)

## 2026-10 — Paseka can now name itself

Every binary reports which build it is, and the console shows it. An operator running a release, a build from `main`, and a `go install` at the same time could not tell three consoles apart from a screenshot, and a bug report had nowhere to put the one thing that made it actionable.

- **`paseka version`** prints the version, the full commit, the commit date, whether the build is a tagged release, and the Go toolchain — and answers outside a colony, so it still works when the checkout is the thing that is broken. `paseka version --json` is the machine contract; `paseka --version` prints the same one line.
- **A build from `main` names its commit.** Release builds are stamped at link time, and everything else falls back to the VCS data the Go toolchain already embeds — so a plain `go build` in a worktree reports `dev+67730c4` with no flags at all. A dirty tree says so, because a sha with uncommitted work behind it is a starting point rather than a claim.
- **`GET /api/version`**, shown in the side menu's foot on every page. The stamp is read once and kept — no timer, because a build cannot change under an open page — and it is absent rather than dashed when the console cannot read its own build.
- **The build is on the console's startup banner.** `paseka console` prints `Build: dev+67730c4.dirty (development build)` under the listening URL, because the process about to hold a port is the one you will want to name in a bug report an hour later, and scrollback is the last place anybody looks.
- **The version in the side menu is the link.** Clicking `0.6.0+67730c4` opens this build's commit on GitHub, or the repository when the build has no commit to show — a sha with nowhere to go is half an answer. The URL comes from the server, not the bundle, so a console built from a fork opens that fork rather than upstream.
- **The container image takes the stamp as build args.** Its context excludes `.git`, so the toolchain has no commit to read; `PASEKA_BUILD_COMMIT` and friends in `docker/dev/.env` name the build instead of leaving it a bare `dev`.

A module pseudo-version (`go install …@main`) contributes a commit but deliberately **no** version: its base is the version the *next* release would carry, and reporting it would put an unreleased build on the same line as a shipped tag.

- Canonical: [CLI](../guide/cli.md), [Queen Console](../guide/queen-console.md), [Homelab deployment](../guide/homelab-deployment.md), [architecture overview](../architecture/overview.md), [Queen Console design system](../architecture/queen-console-design-system.md)

## 2026-09 — Queen Console redesign, complete under `/next/`

The redesigned console is no longer a preview of a few routes: **every page under `/next/` is real now** — Dashboard, Traces, Git, System, Timeline, Topology, Runs, Tasks, Reviews, Sessions, Worktrees, Bees, Settings — and a "migration pending" card anywhere under it is a bug. The legacy console still serves `/` until the explicit cutover, and both read the same root-relative `/api/*` endpoints.

The new UI ships with a contract written for agents in the [Queen Console design system](../architecture/queen-console-design-system.md): layout shell, theming, one component inventory, one `DataTable`, one status→color mapping, and hard rules for what is always true. New console pages are written against that contract rather than against the legacy markup.

Three surfaces have no legacy equivalent:

- **Worktrees** — every isolated checkout with its branch and its trail, plus the one confirmed sweep for the leftovers the branch sweep skips.
- **Bees** — the whole roster (role, adapter, sector, intents, worktree or colony root, live processes, last run), including headless `script` bees that no launch form can start. `GET /api/bees` gained `?scope=colony`; the default still returns the launch picker, so the launch forms and the legacy console read exactly what they read before.
- **Settings** — what the colony is configured with *and what decided each value*: the env var actually in force over the one in `config.yaml`, a code default distinguished from a declared one. Read-only, on the new `GET /api/config`, and no secret crosses it — an adapter reports the *name* of its key variable and whether that resolves.

- Spec: [035-queen-console-redesign](../specs/035-queen-console-redesign.md)
- Canonical: [Queen Console](../guide/queen-console.md), [Queen Console design system](../architecture/queen-console-design-system.md), [Bee config](../guide/bee-config.md)

Deferred from that work: user story #9 also asks for these settings to be editable "without editing files", and the console still writes no configuration — see [Spec 036](../specs/036-console-config-write.md).

## 2026-09 — What changed in the console, and the keys it gained

Migrating the pages was also a chance to fix what the legacy console did badly. What an operator can feel:

- **Every list keeps its filter and its page in the URL** (`?q=`, `?page=`) — a narrowed list is a link you can paste, and **Back** from a trail, run, or task returns to the page of the list you left. Filters write with `replaceState`, so Back never walks you backwards through the letters of a word.
- **Traces pages by cursor, all the way down.** The old cursor stopped *Load older trails* at 500 trails and announced that history ended there, because the cursor was applied after a scan that had already truncated the rows. There is no ceiling now, and the client pager and its `1–15 of 50` label are gone — the server page is the only depth the list has.
- **The shell answers the keys an operator reaches for.** **Escape** walks a trail, run, task, or session back to its list — yielding to dialogs and to half-written notes — and **`/`** focuses the list's filter and selects what is in it. The **Live bees**, **Host**, and **Git** plaques are now links to the pages behind them, and a trail's worktree **Path** and **Base SHA** are on the clipboard.
- **The event feed can be watched while it is being read.** The Timeline header carries an **Auto-refresh** selector — Manual, 5s, 10s, 15s, 60s — arriving on Manual, and a hidden tab is not somebody waiting on a feed.
- **A published cue waits instead of 404ing.** Nothing reaches disk until a bee picks the signal up, so the trail page says the trail will appear, bounded at 30 seconds and naming both dead ends: a wrong id, or no bee took the cue. The publish toast offers **Open trail** instead of navigating you away.
- **The Git plaque says the two facts apart.** The badge answers the colony root against origin (`in sync`, `↑3`, `↓2`, `fetch`) and a dirty tree is a word beside it rather than a tone — amber on a dirty root would be amber for most of every working session, wearing the same tone as the review badges two panels away. A branch the sweep's name filter skips offers **Delete**, with the same guards the sweep uses.
- **A refused artifact says how large it is**, on the trail and beside the refusal in the preview, so "raise the cap, page it, or look elsewhere" is a decision made on a number.
- **A raw event reads as structure.** The folded **Raw event** view on the Timeline feed and on a run's events colours field names, values, numbers, keywords, and punctuation, so a payload can be scanned for the field it is about instead of read line by line. The text is byte-for-byte what was published — nothing is reformatted, dropped, or escaped away — and a payload that is not JSON shows as itself, because the envelope is whatever the publishing bee put in it.

- Spec: [035-queen-console-redesign](../specs/035-queen-console-redesign.md)
- Canonical: [Queen Console](../guide/queen-console.md), [Queen Console design system](../architecture/queen-console-design-system.md)

## 2026-09 — The console bundle is a build artifact

The console now builds from a frontend bundle that the Go binary embeds at compile time, and that bundle is **not committed** — it is produced at build time. A `go build` without the frontend step still compiles and still serves the legacy console; `/next/` answers with a page saying the preview was not built.

**How you install changes.** Release archives and the container image build the frontend first and therefore carry the redesign; `go install` never does, and says so. Building from the repo is `pnpm --dir web install --frozen-lockfile`, `pnpm --dir web build`, then `go build -o paseka ./cmd/paseka`.

- Canonical: [CLI](../guide/cli.md), [Queen Console](../guide/queen-console.md), [Homelab deployment](../guide/homelab-deployment.md#rebuild-paseka-inside-the-container)

## 2026-09 — Age-based prune

`paseka prune` adds an age-aware sibling to `paseka purge`: it removes `.paseka/worktrees/` and `.paseka/runs/` trace directories whose last activity predates a retention period (default **14 days**, `--older-than 14d|2w|336h`) instead of wiping every trace. Filesystem flags mirror purge (`--runs`, `--worktrees`, `--all`, `--yes`, `-C`), and a plan is shown before deleting. Worktrees with uncommitted changes are never auto-removed. With `--bus`, prune also removes task-ledger KV, stream events, and artifacts for correlatable traces, using ledger task activity to protect traces whose files have gone quiet while their tasks were touched recently, and cleaning up stale ledger-only traces.

- Spec: [033-cli-prune](../specs/033-cli-prune.md)
- Canonical: [CLI](../guide/cli.md) (`paseka prune`)

## 2026-09 — Clean shutdown and serialized runtime state

Runtime writes to home `state.json` are serialized so a stopped bee can never flip back to running from a stale write, and `paseka run` treats an OS signal (Ctrl+C) as a clean exit instead of a failed run — operators can shut the hive down without false error states. `paseka console` prints a shutdown notice on Ctrl+C.

- Canonical: [CLI](../guide/cli.md) (`paseka run`, `paseka console`)

## 2026-09 — Resume OpenCode HITL sessions

A new OpenCode HITL session now pre-creates its provider chat before the TUI starts — `opencode serve` + `POST /session`, the OpenCode analog of Cursor's `create-chat`, with **no model turn and no cost** — and launches the TUI with `--session <ses_*>`, storing `ses_*` as `providerSessionId`. Because the OpenCode TUI ignores `--prompt` when `--session` is set, the kickoff (and any Resume continue line) is delivered out-of-band through the TUI's own loopback control server (`--port` + `/session/<id>/prompt_async`), so the first turn actually lands in the pre-created chat. A finished OpenCode HITL session can then **Resume** in Queen Console or via `paseka session resume`: a new Paseka session on the same Flight Trail that copies the id, records `resumedFrom`, skips honey, and fails closed when ineligible. Pre-create failure degrades to a normal TUI launch; Pi/Claude/script sources stay ineligible (the ineligible reason is now `not_resumable`).

- Spec: [030-opencode-session-resume](../specs/030-opencode-session-resume.md)
- Canonical: [Interactive sessions](../guide/interactive-sessions.md), [CLI](../guide/cli.md) (`paseka session resume`), [Queen Console](../guide/queen-console.md), [Architecture overview](../architecture/overview.md)

## 2026-09 — Pull-request delivery

Isolated final-gate approve can **publish a pull request** instead of merging into the colony clone. Colony `defaults.delivery: pull_request` is opt-in; empty still means local merge. The apiary points `forge.command` at a `tea` / `gh` / custom script (JSON stdin/stdout). Approve pushes the worktree head, upserts the PR, and keeps `waiting_review` until the host reports `merged`. Queen Console Reviews grows Open/Update PR; `paseka proposal approve` takes `--pr-title` / `--pr-body` / `--draft`. Telegram cards show the URL. The Git tab still only pushes default.

- Spec: [024-pull-request-delivery](../specs/024-pull-request-delivery.md)
- Canonical: [Pull-request delivery](../guide/pull-request-delivery.md), [Colony layout](../guide/colony-layout.md), [Queen Console](../guide/queen-console.md), [CLI](../guide/cli.md) (`paseka proposal approve`), [INSIGHT kinds](../reference/insight-kinds.md) (`pr.body`)

Deferred from that work: per-cue delivery, auto-detect forge, a dedicated `pr_open` task status — see [Backlog](backlog.md).

## 2026-09 — Config profiles

A Beekeeper can run the same colony through a named overlay (`paseka --profile pi`, `PASEKA_PROFILE`, or sticky `profile:` in home `config.yaml`) without rewriting committed bee YAML. Global `adapter:` replaces LLM bees (script and `command:` roles stay put unless a per-bee exception says otherwise). Unknown names fail closed and list what exists. `--no-profile` ignores a sticky default. Queen Console inherits the process overlay; there is no in-UI switcher. The Pi adapter ships a committed `.paseka/profiles/pi.yaml` (`--profile pi`) and no longer swallows or mispasses its prompt argument.

- Spec: [027-config-profiles](../specs/027-config-profiles.md)
- Canonical: [Colony layout](../guide/colony-layout.md), [Bee config](../guide/bee-config.md), [CLI](../guide/cli.md) (global `--profile`), [Architecture overview](../architecture/overview.md)

Deferred from that work: concurrent processes with different profiles still share worktrees and home `state.json` — see [Backlog](backlog.md).

## 2026-09 — Resume Cursor HITL sessions

A finished Cursor HITL chat can continue in the same Cursor conversation without `create-chat`. Queen Console **Resume** (optional one-line continue) and `paseka session resume` start a **new** Paseka session that copies `providerSessionId`, records `resumedFrom`, skips honey, and fails closed when the source is ineligible or still live. New launches still create a fresh chat.

- Spec: [025-cursor-session-resume](../specs/025-cursor-session-resume.md)
- Canonical: [Interactive sessions](../guide/interactive-sessions.md), [CLI](../guide/cli.md) (`paseka session resume`), [Queen Console](../guide/queen-console.md)

## 2026-09 — Standing trail checkpoints and doctor smells

Standing ticks keep procedure memory in the trail comb (`checkpoint.json`, optional `journal/`) and prompt partials tell bees to read that file first, write it atomically, and spawn bloom work on a **new** `traceId`. `paseka doctor` warns when a standing SIGNAL kind has only isolated worktree subscribers, when a standing tick bee publishes isolated `code.proposal`, or when an isolated proposal already sits on a standing trail. `purge --runs` is documented as wiping that memory.

- Spec: [028-standing-trails](../specs/028-standing-trails.md)
- Canonical: [Forage Cues](../guide/cues.md) § Checkpoints, [Prompt templates](../guide/prompt-templates.md), [CLI](../guide/cli.md) (`paseka doctor`, `paseka purge`)

Deferred from that work: live eval-colony standing case, ledger compact after a year of ticks — see [Backlog](backlog.md).

## 2026-09 — Queen Console chrome stream

Always-visible header plaques (Hive runtime, Live bees, Host, Git) and Reviews/Sessions tab badges now update from one Server-Sent Event stream instead of a bundle of timed GETs. System and Git tabs still poll their full snapshots while open. Git status never fetches remotes from the header clock.

- Spec: [029-console-chrome-stream](../specs/029-console-chrome-stream.md)
- Canonical: [Queen Console](../guide/queen-console.md)

Deferred from that work: domain Timeline SSE (`/api/events/stream`) — see [Spec 002](../specs/002-queen-console-mvp.md).

## 2026-09 — Standing trail badge in Console, status, and Telegram

Flight Trails bound to a standing Forage Cue are now visually distinct from bloom trails on every operator surface, so a months-old triage identity no longer reads as a stuck feature.

- Queen Console: **standing** badge in Dashboard and Traces lists, `standing` flag in trail detail, and honey shown as `remaining / stipend` for those trails.
- `paseka status`: `· standing` marker on honey, low-honey, and recent-trace lines; `--json` adds `standing: true` to `energy.traces`, `attention.lowEnergyTraces`, and `recentTraces`.
- Telegram `/traces`: `standing` plus remaining honey on standing trail lines.

The badge is derived from colony cue YAML (`standing.trace`), not a ledger flag — delete the cue and the trail reads as an ordinary leftover trail.

- Spec: [028-standing-trails](../specs/028-standing-trails.md)
- Canonical: [Queen Console](../guide/queen-console.md), [CLI](../guide/cli.md) (`paseka status`), [Forage Cues](../guide/cues.md), [Telegram gateway](../guide/telegram-gateway.md)

## 2026-09 — Standing trail first title

The first standing `cue run` that seeds honey also publishes `INSIGHT` / `trace.title` when the trail has none yet: cue `description` (trimmed, 120-character cap), or the cue id if description is empty. Later ticks and a human/bee title already on the trail are left alone, so lists show “Daily triage” instead of a raw id without clobbering a refined name.

- Spec: [028-standing-trails](../specs/028-standing-trails.md)
- Canonical: [Forage Cues](../guide/cues.md), [INSIGHT kinds](../reference/insight-kinds.md) (`trace.title`)

## 2026-09 — Console lib assets in `go install` binaries

Queen Console third-party files (xterm, Diff2Html, cytoscape, fonts) live under `/lib/...`. A directory named `vendor` is stripped from Go module zips, so `go install` used to ship a binary that answered those URLs with the SPA `index.html`. Missing `.js`/`.css` requests now 404 instead of falling back to HTML.

- Canonical: [Queen Console](../guide/queen-console.md)

## 2026-09 — Standing trail overlap refuse

A second standing `cue run` fails closed while the trail already has an open tick (`planned`, `ready`, `running`, `waiting_review`) or a live AFK adapter on that `traceId`. The error says the trail **is busy** and names the blocking task status or bee. No stipend and no ingress. `blocked` / finished tasks and interactive sessions do not block, so a drained tick can start the next ration and a HITL inspect is not a tick.

- Spec: [028-standing-trails](../specs/028-standing-trails.md)
- Canonical: [Forage Cues](../guide/cues.md), [CLI](../guide/cli.md) (`paseka cue`)

## 2026-09 — Standing trail stipend

Later ticks of a Standing Trail **replace** remaining honey with the cue stipend (`SIGNAL` / `energy.stipend`) instead of leaving leftover tokens or stacking `energy.add`. The event does not change seed budget or `energyAdded`, and it does not unblock honey-blocked tasks from a previous tick. A killed standing trail refuses `cue run` (error names `system.kill`). First tick still seeds via `SeedEnergy` as in the identity slice.

- Spec: [028-standing-trails](../specs/028-standing-trails.md)
- Canonical: [Forage Cues](../guide/cues.md) § Honey, [Task ledger](../reference/task-ledger.md) § Honey reserve, [Event contracts](../reference/event-contracts.md)

## 2026-09 — Standing cue identity

A Forage Cue may declare `standing.trace` and `standing.stipend`. Omitting `--trace` / API `traceId` / Telegram `cue:` then publishes on that stable Flight Trail instead of minting a disposable bloom id; a mismatched explicit id fails closed. First tick seeds honey from stipend. Standing task cues require `review: none` and `worktree: false`. Nuc export/import carries the YAML as today.

- Spec: [028-standing-trails](../specs/028-standing-trails.md)
- Canonical: [Forage Cues](../guide/cues.md), [CLI](../guide/cli.md) (`paseka cue`), [Glossary](../idea/glossary.md) (Standing Trail)

## 2026-09 — Cursor Agent log from local transcripts

`paseka export --include agent-logs` now fills Cursor **Agent log** tool calls from `~/.cursor/projects/<slug>/agent-transcripts/<uuid>/<uuid>.jsonl` (same UUID as `providerSessionId`). Files are read in place, never copied into `.paseka/runs/`. Missing or unreadable transcripts omit with `store not found` / `parse error`; a conversation with no tools is an empty table. Pi stays stubbed.

- Spec: [021-provider-session-logs-export](../specs/021-provider-session-logs-export.md) (Draft; Cursor jsonl reader)
- Canonical: [CLI](../guide/cli.md) (`paseka export`), [Architecture overview](../architecture/overview.md) (adapter result collection)

## 2026-09 — Export Agent log (adapter stubs)

`paseka export --include agent-logs` adds a per-run **Agent log** subsection (tool-call table, or a one-line omit reason). Enrichment is best-effort: missing `providerSessionId`, adapters without a resolver, stub readers, and resolve errors never fail the command. Cursor and Pi implement the resolve seam as stubs (`not implemented`) until vendor session stores are read in place. Provider logs are not copied into `.paseka/runs/`.

- Spec: [021-provider-session-logs-export](../specs/021-provider-session-logs-export.md) (Draft; export plumbing)
- Canonical: [CLI](../guide/cli.md) (`paseka export`), [Architecture overview](../architecture/overview.md) (adapter result collection)

## 2026-09 — HITL provider session id

Interactive Cursor and Pi sessions record the provider’s native session id as `providerSessionId` on `session.json` and `meta.json` before the PTY starts. Cursor HITL runs `agent create-chat` and launches the TUI with `--resume`; a failed create-chat still starts the session without a pointer. Pi HITL stores the pinned `--session-id` (`agentId`, or the flag already on a `command:` override). Queen Console session detail shows the id when set. Export Agent log and `events.ndjson` cleanup stay out of this slice.

- Spec: [021-provider-session-logs-export](../specs/021-provider-session-logs-export.md) (Draft; HITL association)
- Canonical: [Interactive sessions](../guide/interactive-sessions.md), [Architecture overview](../architecture/overview.md) (interactive sessions)

## 2026-08 — AFK provider session id

Headless Cursor and Pi runs record the provider’s native session id as `providerSessionId` on `result.json` / `meta.json`. Cursor parses `session_id` from stream-json (prefer `system`/`init`). Pi AFK pins `--session-dir` / `--session-id` like interactive sessions, then prefers the JSON session header when present. Queen Console run detail and `paseka inspect usage --agent` show the id when set. Missing id does not fail the run. Export enrichment and HITL Cursor resume are not in this slice.

- Spec: [021-provider-session-logs-export](../specs/021-provider-session-logs-export.md) (Draft; association slice)
- Canonical: [Architecture overview](../architecture/overview.md) (adapter result collection), [CLI](../guide/cli.md) (`paseka inspect usage`)

## 2026-08 — Queen Console Git

Queen Console header adds a **Git** plaque (ahead/behind `origin`, dirty, or synced) next to Host. Click or Enter/Space opens a **Git** tab: colony root vs origin with explicit **Fetch** (remote-tracking only), **Push** of the default branch (never `--force`; hooks skipped by default), and fast-forward-only **Pull** as a backup when a webhook sidecar did not update the clone. The tab lists colony-managed worktrees (with prune of orphans) and leftover merged branches (`paseka/*`, `feature/` / `hotfix/` / `fix/`) with safe `git branch -d`. Reviews warn when origin is ahead of the local default branch; Approve still does not push. Git ops do not depend on NATS. Credentials stay in the system git helper (`HOME`/`PATH` of `paseka console`).

- Spec: [023-console-git](../specs/023-console-git.md)
- Canonical: [CLI](../guide/cli.md) (`paseka console`), [Homelab deployment](../guide/homelab-deployment.md)

Deferred from that work: autostash list, porcelain file list, incoming log vs origin, mutation lock while bees run — see [Backlog](backlog.md#console-git).

## 2026-08 — Queen Console System Info

Queen Console header adds an observe-only **Host** plaque (CPU percent and memory used/total, optional 1-minute load) next to Hive runtime and Live bees. Click or Enter/Space opens a **System** tab with hostname/kernel, OS/arch, CPU count, uptime, console PID, load averages, memory, optional colony-root disk, and a capped top-process table. Live bee PIDs are highlighted client-side. `GET /api/system` reads the OS view of the `paseka console` process (container PID namespace in Docker); it does not call Docker APIs and does not depend on NATS or Hive runtime. Linux uses `/proc`; other OSes keep identity fields and leave load/processes empty. `/proc/meminfo` may still describe the **host** when cgroups do not hide it.

- Spec: [022-console-system-info](../specs/022-console-system-info.md)
- Canonical: [CLI](../guide/cli.md) (`paseka console`), [Homelab deployment](../guide/homelab-deployment.md)

## 2026-08 — Honey remaining / allocated

Honey UIs (CLI, Queen Console, Telegram, hive status, export) show **remaining / allocated**, where allocated is the frozen seed plus post-seed `energy.add` totals (`energyAdded` on the task ledger snapshot). `energyBudget` stays the initial seed so Forage Cue overrides and `budget == 0` seeding still work. When the trail has been topped up, a second line shows `seed N · topped M`. Console **low** still uses remaining vs seed (`budget/4`).

`paseka energy show` still prints parseable `budget:` and `remaining:` integers; `added:` and the remaining/allocated fraction appear only after a post-seed top-up. Unseeded trails print remaining without a `/ 0` denominator. Ledger snapshots from before `energyAdded` can still show remaining above seed until a new top-up.

- Canonical: [Task ledger](../reference/task-ledger.md) (Honey reserve), [Forage Cues](../guide/cues.md) § Honey, [CLI](../guide/cli.md) (`paseka energy`), [Telegram gateway](../guide/telegram-gateway.md)

## 2026-08 — Worktree branch name (`worktree.branch`)

Planner bees can emit `INSIGHT/worktree.branch` so isolated Flight Trail worktrees use readable git refs (`feature/…`, `hotfix/…`) instead of only `paseka/<traceId>`. Runtime applies names on worktree ensure and renames immediately when a later insight lands; collisions fail closed (no detached HEAD fallback). Merge, merge-diff, Queen Console trace detail, and `state.json` follow the live branch name. Path stays `.paseka/worktrees/<traceId>/`.

- Spec: [020-worktree-branch](../specs/020-worktree-branch.md)
- Canonical: [Insight kinds](../reference/insight-kinds.md), [Architecture overview](../architecture/overview.md), [Prompt templates](../guide/prompt-templates.md), [Interactive sessions](../guide/interactive-sessions.md)

## 2026-08 — Final-gate Request changes (Slice C)

On `review: final` / `_review`, annotated Submit on the merge preview is **Request changes**: it writes the comb packet as before, keeps the merge gate in `waiting_review`, and plans+readies a new AFK rework task for the last isolated-proposal Bee on the same Flight Trail (honey consumed on dispatch). Plain reject remains abandon-only (feedback, no rework). Duplicate Request changes is rejected while non-final work is still in flight. Approve stays the only merge path. CLI `paseka proposal reject --comments-file` on a final gate uses the same rework path.

- Spec: [017-console-diff-review](../specs/017-console-diff-review.md)
- Canonical: [Queen Console](../guide/queen-console.md) (Reviews), [CLI](../guide/cli.md) (`paseka proposal reject`), [Task ledger](../reference/task-ledger.md)

## 2026-08 — Session deferred flush uses home NATS config

Interactive session (and other ColonyRoot-only) deferred flush loads `~/.config/paseka/<slug>/config.yaml` before connecting. Previously a partial in-memory context treated NATS as unset, flushed pending events to the run audit log with a no-op publisher, and never reached JetStream or the task ledger.

- Canonical: [Interactive sessions](../guide/interactive-sessions.md), [CLI](../guide/cli.md) (`paseka event emit --defer`)

## 2026-08 — Queen Console annotated review comments (Slice B)

Queen Console merge preview supports line-anchored comment drafts (click added/context lines; Shift-click for a range). Submit writes `review-comments.md` to the trail comb, publishes `SIGNAL/artifact.written` (producer `console`), then a short `INSIGHT/human.feedback` with optional `ref`. Fail closed if the comb write fails. `review: required` still returns the task to `ready`. CLI: `paseka proposal reject --comments-file` copies an existing Markdown packet into the same comb ref. Final-gate rework shipped separately as Slice C.

- Spec: [017-console-diff-review](../specs/017-console-diff-review.md) (Slice B)
- Canonical: [Queen Console](../guide/queen-console.md) (Reviews), [CLI](../guide/cli.md) (`paseka proposal reject`), [Insight kinds](../reference/insight-kinds.md), [Prompt templates](../guide/prompt-templates.md)

## 2026-08 — Model aliases (`params.model`)

Colony-owned `model_aliases` in `.paseka/colony.yaml` map stable names to vendor model ids; home `config.yaml` overlays the same keys per machine. Bees keep `params.model` as alias or raw id; runtime resolves once before `--model` is passed to the adapter.

- Spec: [019-model-aliases](../specs/019-model-aliases.md)
- Canonical: [Colony layout](../guide/colony-layout.md), [Bee config](../guide/bee-config.md), [Architecture overview](../architecture/overview.md)

## 2026-08 — Trail artifacts protocol (comb + `artifact.written`)

Trace-scoped comb under `.paseka/runs/<traceId>/artifacts/` with `{{.ArtifactsDir}}` prompt injection. Runtime captures a per-run SHA-256 baseline and publishes one batched `SIGNAL/artifact.written` on successful AFK or interactive exit (added/changed files only). Coexistence with deferred `artifact.written` skips duplicate scan flush. Queen Console lists comb files (staged vs announced) with Markdown preview. `paseka export --include artifacts` inlines comb bodies in reports. Human `artifacts.WriteAndAnnounce` helper publishes with producer `console` (for 017 Slice B).

- Spec: [014-artifacts-protocol](../specs/014-artifacts-protocol.md)
- Canonical: [Architecture overview](../architecture/overview.md) (runs comb), [Prompt templates](../guide/prompt-templates.md) (`ArtifactsDir`), [Bee routing](../reference/bee-routing.md), [CLI](../guide/cli.md) (`paseka export --include artifacts`)

## 2026-08 — Queen Console merge-diff viewer (Slice A)

Queen Console Reviews final merge gates show merge-diff summary on the detail panel; **Open merge preview** opens a dedicated full-page viewer (sticky file list, path filter, jump-to-file, per-file hunks via vendored Diff2Html, unified or side-by-side format). Line-number gutters stay clipped to each file pane instead of overlaying scrolled hunks. Approve/reject stay on Reviews. Queue polling still skips re-fetch when the same gate stays selected.

- Spec: [017-console-diff-review](../specs/017-console-diff-review.md) (Slice A only; B/C follow)
- Canonical: [Queen Console](../guide/queen-console.md) (Reviews)

## 2026-08 — Queen Shell colony status

`paseka status` is a read-only colony snapshot for Beekeepers and interface bees: runtime liveness, live bees, task counts, honey for recent Flight Trails, attention items (reviews, invites, failures, exhausted honey), and recent traces. Default text output; `--json` emits `schemaVersion` 1 for agents. `--check` exits non-zero only when the hive substrate cannot choreograph (runtime down or configured NATS unreachable) — pending HITL work is not treated as an outage.

- Spec: [018-cli-colony-status](../specs/018-cli-colony-status.md)
- Canonical: [CLI](../guide/cli.md) (`paseka status`)

## 2026-08 — `task.ready` race fix (prompts + ledger)

Scout and Drone breakdown prompts now defer both `task.plan` and post-plan `task.ready` (FIFO flush), with slim ready payloads (`taskId` only). The task ledger parks unmatched `task.ready` kicks on `pendingReady` until the matching `task.plan` registers the task, then promotes on the plan event — so live-ready-before-plan no longer loses autostart. Cleared on `system.kill`.

- Canonical: [Task ledger](../reference/task-ledger.md), [Prompt templates](../guide/prompt-templates.md)

## 2026-08 — Export `--include` (richer payload)

`paseka export` accepts composable `--include` slices independent of `--format`: `usage` (trace aggregate + per-run tokens), `durations` (wall-clock per run), `bees` (committed `.paseka/bees/*.yaml` for trail roles), `colony` (`.paseka/colony.yaml`), `cues` (all colony cues), and `artifacts` (trail comb file bodies). Default export stays trail-only with no config snapshots.

- Canonical: [CLI](../guide/cli.md) (`paseka export`)

## 2026-08 — Export `--format` (HTML | Markdown)

`paseka export` now accepts `--format html` (default) or `--format md`. Both renderers share the same `TraceExportData` (overview, tasks, runs, event timeline); the output filename extension matches the format. Markdown keeps run and event summaries verbatim and fences raw event JSON for agent-friendly trail dumps.

- Canonical: [CLI](../guide/cli.md) (`paseka export`)

## 2026-08 — Forage Cues (cue layer)

Named colony ingress shortcuts (`.paseka/cues/<id>.yaml`) publish `signal` or `task` choreography without hand-writing emit JSON. One definition drives Queen Shell (`paseka cue list|run`), Queen Console **Run cue** (`GET/POST /api/cues`), and Telegram `commands.custom` with `cue: <id>`. Optional per-cue `energy_budget` seeds a smaller initial honey reserve on fresh trails; `paseka init` scaffolds `feature` and `hotfix`. Nuc export/import includes cues with `--cues` filter.

- Spec: [016-cue-layer](../specs/016-cue-layer.md)
- Canonical: [Forage Cues](../guide/cues.md), [CLI](../guide/cli.md) (`paseka cue`), [Telegram gateway](../guide/telegram-gateway.md), [Colony layout](../guide/colony-layout.md), [Nuc packs](../guide/nuc.md), [Task ledger](../reference/task-ledger.md)

## 2026-07 — Deferred event emit buffer

Bees can stage bus events until a run or session completes successfully. `paseka event emit --defer` validates and appends to per-run `pending.ndjson`; runtime flushes FIFO on success (before `run.summary` synthesis). Operators inspect with `paseka event pending` and recover with `paseka event flush` or `--discard`. Platform control kinds (`system.kill`, `energy.*`, `session.invite`, `beekeeper.ready`, `task.status`) are live-only.

- Spec: [015-deferred-event-emit](../specs/015-deferred-event-emit.md)
- Canonical: [CLI](../guide/cli.md) (`paseka event emit`, `pending`, `flush`), [Prompt templates](../guide/prompt-templates.md), [Interactive sessions](../guide/interactive-sessions.md)

## 2026-07 — Hard trace kill (`system.kill`)

Beekeepers can emergency-stop a trace without waiting for honey to drain. `paseka kill --trace <id>` publishes `SIGNAL/system.kill`: marks the trace `killed`, cancels non-terminal tasks, blocks new AFK dispatch, and cancels in-flight adapter processes. `energy.add` after kill does not redispatch.

- Spec: [013-system-kill](../specs/013-system-kill.md)
- Canonical: [Task ledger](../reference/task-ledger.md), [CLI](../guide/cli.md) (`paseka kill`)

## 2026-07 — `paseka inspect usage`

Operators can dump LLM token usage from the terminal without opening Queen Console. `paseka inspect usage --trace <id>` prints a trace aggregate summed from runs that report `usage` on `result.json`; `--agent` scopes to one run.

- Canonical: [CLI](../guide/cli.md) (`paseka inspect usage`)

## 2026-07 — Colony `defaults.default_bee`

Colonies can set the default AFK task role in `.paseka/colony.yaml` (`defaults.default_bee`). Reactor `task.ready` dispatch, `paseka task create` / `task start` / `task retry`, and review helpers resolve empty `task.bee` through this setting (platform fallback `builder`). `paseka init` scaffolds `default_bee: builder`.

- Canonical: [Colony layout](../guide/colony-layout.md), [CLI](../guide/cli.md), [Bee routing](../reference/bee-routing.md)

## 2026-07 — Queen Console tab attention badges

Sessions and Reviews tabs show pending invite and review counts (1–9, then `9+`) with background polling so counts stay fresh while you are on other views.

- Spec: [002-queen-console-mvp](../specs/002-queen-console-mvp.md)
- Canonical: [Queen Console](../guide/queen-console.md) (Sessions / Reviews)

## 2026-07 — Homelab / server container apiary

Operator-facing `docker/dev/` image (Ubuntu 24.04, Go, git, Cursor Agent CLI, prebuilt `paseka`) with compose volumes for colony repo, paseka home, and Cursor config. Default command is Queen Console on `0.0.0.0:8787`; `PASEKA_NATS_URL` reuses a host or LAN JetStream. Guide covers `colony_root` path matching and trusted-network Console exposure.

- Canonical: [Homelab deployment](../guide/homelab-deployment.md), [`docker/dev/`](../../docker/dev/)

## 2026-07 — `PASEKA_NATS_URL` override

Non-empty `PASEKA_NATS_URL` overrides `nats.url` in home `config.yaml` for runtime and CLI — useful for containers, shared LAN JetStream, and multi-environment setups without editing yaml per host.

- Canonical: [CLI](../guide/cli.md) (NATS dependency), [Colony layout](../guide/colony-layout.md), [Homelab deployment](../guide/homelab-deployment.md)

## 2026-07 — Queen Console topology layout persistence

Colony EDA topology node positions persist in browser `localStorage` per colony slug on drag; layout restores on reload. **Reset layout** clears saved positions and re-runs the default layout.

- Spec: [007-colony-eda-topology](../specs/007-colony-eda-topology.md)
- Canonical: [CLI](../guide/cli.md) (`paseka colony topology`), [Colony EDA topology](../specs/007-colony-eda-topology.md) (Console Topology tab)

## 2026-07 — Flight trail summary (`trace.summary`)

Operational `INSIGHT/trace.summary` sets a human Flight Trail description for Queen Console (muted subtitle) and the default merge-commit **body**. Conventional merge **subject** stays HITL (`mergeMessage` / `--merge-message` / default). The sole incomplete non-final AFK work task gets must-emit guidance via `{{.IsLastWorkTask}}`.

- Spec: [012-trace-summary](../specs/012-trace-summary.md)
- Canonical: [INSIGHT kinds](../reference/insight-kinds.md), [Prompt templates](../guide/prompt-templates.md), [CLI](../guide/cli.md) (approve `--summary` vs `--merge-message`)

## 2026-07 — Queen Console honey top-up

Beekeepers can top up a trace honey reserve from Queen Console without switching to CLI or Telegram. The Trace view Energy section exposes `+1` / `+5` / `+12` controls (aligned with Telegram) backed by `POST /api/traces/:traceId/energy/add`.

- Spec: [002-queen-console-mvp](../specs/002-queen-console-mvp.md)
- Canonical: [CLI](../guide/cli.md) (`paseka energy add`), [Telegram gateway](../guide/telegram-gateway.md)

## 2026-07 — Run log artifact rename (`summary.md`)

AFK and interactive runs now persist the human-readable run log as `summary.md` instead of `result.txt`. Success semantics remain on process exit and `INSIGHT/run.summary`; the file is a log only. Template keys (`{{.ResultFile}}`, `$RESULT_FILE`, `PASEKA_RESULT_FILE`) are unchanged — only the basename changes. Runtime still reads legacy `result.txt` when present for adapter summary preference.

- Canonical: [Architecture overview](../architecture/overview.md), [Colony layout](../guide/colony-layout.md)

## 2026-07 — Flight trail title (`trace.title`)

Operational `INSIGHT/trace.title` sets a human Flight Trail name for Queen Console and planner prompts. Runtime resolves `{{.TraceTitle}}` with fallbacks from `feature.requested` and task ledger titles.

- Spec: [011-trace-title](../specs/011-trace-title.md)
- Canonical: [INSIGHT kinds](../reference/insight-kinds.md), [Prompt templates](../guide/prompt-templates.md)

## 2026-07 — Telegram notify modes

`paseka gate telegram` notify policy now supports per-category **`off` / `silent` / `sound`** modes, splits `waiting_review` into `review_required`, `review_final`, and `commit_gate` (AFK defer), and pushes on live **`task.completed`** events (default silent; not reconciled on gate restart).

- Spec: [010-telegram-human-gateway](../specs/010-telegram-human-gateway.md) §8
- Canonical: [Telegram gateway](../guide/telegram-gateway.md)

## 2026-07 — Telegram custom signal commands

`paseka gate telegram` supports `commands.custom` in `telegram.yaml` — configurable slash commands that publish colony `SIGNAL` events (preview + Confirm). Example: `/feature` → `feature.requested` for Scout intake when `paseka run` is active.

- Spec: [010-telegram-human-gateway](../specs/010-telegram-human-gateway.md) §10
- Canonical: [Telegram gateway](../guide/telegram-gateway.md)

## 2026-07 — SIGNAL direct dispatch

Reactor direct dispatch now supports colony `SIGNAL` events (e.g. `feature.requested` → Scout `intake`). Platform SIGNAL kinds (`task.*`, `energy.*`, invite protocol) remain denylisted for direct AFK runs.

- Canonical: [Bee routing](../reference/bee-routing.md) §4 Direct path

## 2026-07 — Prompt text flag `body`

Hard rename of free-text prompt input to avoid collision with ledger `taskId`:

- CLI: `paseka bee run` / `bee chat` / `invite record` use `--body` / `-b` (removed `--task` / `-t` on those commands)
- Queen Console: session launch form label **Task body**; `POST /api/sessions` and run detail JSON use `body` for prompt text
- Unchanged: `--task` on `paseka task *` and `proposal *` (task id); template variable `{{.Task}}`; protocol `session.invite` payload field `task`

- Canonical: [CLI](../guide/cli.md), [Interactive sessions](../guide/interactive-sessions.md), [Prompt templates](../guide/prompt-templates.md)

## 2026-07 — Telegram Human Gateway

Async phone triage via `paseka gate telegram`: long-poll Bot API, allowlisted chats, bus notify + reconcile dedup, `/status` `/energy` `/task` `/invites` `/help`, invite HITL (local PTY on accept), and proposal reject / soft-mid approve (final-merge Console/CLI only).

- Spec: [010-telegram-human-gateway](../specs/010-telegram-human-gateway.md)
- Canonical: [Telegram gateway](../guide/telegram-gateway.md)

## 2026-07 — Merge autostash on approve

Final merge on isolated proposal approve autostashes a dirty colony root (including untracked files) and restores afterward.

- Spec: [009-merge-autostash](../specs/009-merge-autostash.md)

## 2026-07 — Code proposal workspaces

Dual proposal paths: `code.proposal.isolated` (worktree + AFK merge gate) and `code.proposal.root` (shared workspace + soft human ack). Alias `code.proposal` → isolated. `paseka doctor` wiring checks.

- Spec: [008-code-proposal-workspaces](../specs/008-code-proposal-workspaces.md)
- Canonical: [Architecture overview](../architecture/overview.md) §2, [Bee routing](../reference/bee-routing.md), [Bee config](../guide/bee-config.md), [Task ledger](../reference/task-ledger.md), [CLI](../guide/cli.md)

Deferred from that work: `proposal_paths` allowlist, untracked files in proposal delta, alias removal timeline — see [Backlog](backlog.md).

## Earlier MVP baselines

| Area | Spec | Notes |
| ---- | ---- | ----- |
| Queen Console MVP | [002](../specs/002-queen-console-mvp.md) | `paseka console`, SPA, polling APIs, reviews, sessions |
| Live bees indicator | [004](../specs/004-live-bees-indicator.md) | Header live-agents panel |
| Colony EDA topology | [007](../specs/007-colony-eda-topology.md) | Topology tab + `paseka colony topology` |
| Pi adapter | [001](../specs/001-pi-integration.md) | First-class `adapter: pi` |
| Human gateway invites | [006](../specs/006-human-gateway-invites.md) | `session.invite`, `auto_invites`, `done_when` |
| Feature ideation flow | [005](../specs/005-feature-ideation-flow.md) | Colony reference choreography (classify → grill → breakdown) |
