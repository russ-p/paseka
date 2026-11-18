# Backlog

Deferred follow-ups of shipped work and durable caveats that are useful soon but not part
of the active change. New ideas live in [Ideas](backlog-ideas.md) and deferred bugs in
[Bugs](backlog-bugs.md) — the `Kind` field decides the file.
Shipped work: [Changelog](changelog.md). Design drafts: [Specs index](specs-index.md).
Console redesign navigation debt lives in [UI migration backlog](ui-migration-backlog.md) — dead links, unbuilt transitions, and placeheld sections.

## Deferred work

### Trail metrics and provenance

Follow-ups from [043b](../specs/043b-trail-metrics-and-work-completion.md). The runtime records and the CLI projects; what is left here is the Console surface. The analysis-admission recovery case from the same spec is now [Bugs](backlog-bugs.md#an-analysis-admission-left-running-after-a-crash-is-never-reconciled).

#### Console does not show a trail's origin or its metrics

- **Kind:** follow-up
- **Source:** [043b-trail-metrics-and-work-completion](../specs/043b-trail-metrics-and-work-completion.md) § Out of Scope (reader surfaces)
- **Summary:** The trail detail reads the same derived stage as `inspect metrics`, but it shows neither the retained origin (`cue` + invocation, `manual`, `other`, `fork`, `unknown`) nor any of the metrics projection — honey by purpose, per-task retries and gate rejections, coverage warnings, the current completion identity. Add an origin line to the trail header and a read-only metrics panel fed by the same `runs.BuildTrailMetrics` call the CLI uses, so the Console and `inspect metrics` cannot disagree.
- **Why deferred:** 043b ships the recorder and the offline read surface; the Console work is a separate UI slice with its own layout decisions, and no runtime data is missing for it.
- **Revisit when:** An operator compares two trails of the same kind, or asks why a trail looked expensive, without leaving the Console.

### Trail attention state

Follow-ups from [050-trail-attention-state](../specs/050-trail-attention-state.md). The projection and every read surface shipped; what is left here is one semantic edge on damaged analysis records. The notification-policy question from the same spec is now an [idea](backlog-ideas.md#resurfacing-a-resolved-failure-as-a-notification-is-an-operator-decision).

#### A record-damaged analysis run still paints the trail

- **Kind:** follow-up
- **Source:** guard review note on task 003 (`01a1b9bf16a7cfb4`), [050-trail-attention-state](../specs/050-trail-attention-state.md) § "Analysis is excluded from attention"
- **Summary:** `recordProblem` is set from the run's conflicting/incomplete outcome pair before the purpose filter, so a damaged analysis record raises the never-resolving `record error` even though ordinary analysis failures and successes are excluded from work attention. That is defensible — a contradictory pair cannot prove what the run was — but it means a trail whose work is healthy can carry a work-looking `record error` because an analysis record is damaged. Decide whether the integrity axis should also be purpose-filtered, and on what evidence, or whether record damage stays trail-wide by definition.
- **Why deferred:** The shipped behavior preserves the truthful-record guarantee, the pair really is damaged, and no surface or test currently contradicts it. Changing it needs a policy on whether record damage is evidence about the run or about the trail.
- **Revisit when:** A damaged analysis record puts `record error` on a trail with healthy work, or analysis records become common enough that their integrity noise matters.

### Adapter catalog

#### The Agent CLI roster is duplicated across the Go/TS boundary

- **Kind:** follow-up
- **Source:** guard review note, deferred from the adapter-probe trail (`trace-01a10d602db8d6f6`)
- **Summary:** `adapterCLINames` (`internal/console/adapters.go:84`) and `agentCLIAdapterNames` (`web/src/lib/api/types.ts:117`) list the same four adapters in the same order with no cross-language guard. Both sides are pinned by tests (`adapters_api_test.go:119` asserts the literal order, `format.test.ts` asserts the literal `not probed, 4 adapters`), so drift fails loudly rather than silently. Serving the roster from `GET /api/system/adapters` without probing would delete the second list.
- **Why deferred:** The block is lazy by contract — arriving on `/system` must issue zero requests — so a roster in the first payload either breaks that rule or adds a second endpoint for four strings.
- **Revisit when:** A fifth adapter is added, or the roster stops being a fixed set.

### Secret redaction

#### Content redaction covers the transcript path only

- **Kind:** follow-up
- **Source:** [048-native-session-transcript](../specs/048-native-session-transcript.md) § 8, after the minimal transcript helper shipped
- **Summary:** A minimal content helper now lives in `internal/redact` and runs once in `adapters.NormalizeTranscript`, so transcript text, reasoning, tool arguments, and tool output are masked before caps for every provider and both export projections. Its pattern set is authorization/bearer headers, well-known token prefixes, private-key blocks, and `KEY=value` assignments whose key looks secret, pinned by table-driven fixture tests. What remains is the cross-surface policy: colony-declared extra patterns, an escape hatch, near-miss handling, redact-vs-drop per surface, and wiring at the other render boundaries (proposal patches, Telegram, logs, Console), so those surfaces stop needing their own pattern list.
- **Why deferred:** 048 needed a minimal pattern set to ship the transcript export safely, not a policy decision; the policy question (redact or drop the whole part, what to do about near-misses, whether redaction is on by default off-machine) belongs in one place with its own review.
- **Revisit when:** Another surface needs content redaction, a colony wants to declare its own patterns, or a real secret is found in an emitted artifact — whichever comes first.

### Adapter launch resolution

#### Project the recorded model alias into the console and exports

- **Kind:** follow-up
- **Source:** run model record (2026-10)
- **Summary:** `request.json` records `modelAlias` — the alias key the bee declared, when `params.model` was one — next to the resolved `model`, and the run/session projections drop it. Carrying it into `runs.RunMeta`, `hiveview.RunView`, and the export run blocks would let a reader see both halves: that a bee asked for `high`, and that `high` meant `cursor-grok-4.6-high-fast` at the time. Today only `request.json` answers the alias half.
- **Why deferred:** The resolved id is what an operator acts on; a second, near-duplicate model value on every run row and export is noise until the alias tables change often enough that "which key was it" becomes a real question.
- **Revisit when:** An alias's target has been edited at least once and a run has been read after that edit, i.e. the resolution genuinely cannot be explained by the current config.

### Task ledger

#### Task retry with edit

- **Kind:** follow-up
- **Source:** [003-hive-evals](../specs/003-hive-evals.md); planning (task ledger / Console)
- **Summary:** Allow changing bee, intent, body, or sector when retrying a failed task (CLI flags or Console form). Today `paseka task retry` and Console Retry reuse the ledger snapshot as-is.
- **Why deferred:** Snapshot reuse was enough for MVP retry; edit-on-retry needs UX and ledger rules. Eval colony has no case that needs a corrected retry.
- **Revisit when:** Operators or eval cases need corrected retries without creating a new task.

#### Compact standing-trail task history

- **Kind:** follow-up
- **Source:** [028-standing-trails](../specs/028-standing-trails.md)
- **Summary:** Compact completed standing tick tasks in the ledger snapshot, or a Console “ticks” subset, after months of daily cue runs.
- **Why deferred:** Spec 028 accepted ledger growth for MVP; boards stay usable in the first weeks without a rolling window.
- **Revisit when:** Operators keep a year of ticks and Console/task list becomes noisy.

### Energy and honey

MVP shipped per-trace honey (`defaults.energy_budget`, `energy.add` / `energy.consume`, reactor gating, `paseka energy show|add`). Loop protection is energy depletion: an exhausted reserve defers a task or direct dispatch and keeps the task's lifecycle status, and the trail publishes one `energy.paused` / `energy.resumed` episode instead of writing a fake task `blocked`. The admission/pending separation is [spec 044a](../specs/044a-direct-pending-dispatch.md) (direct) and [spec 044b](../specs/044b-standing-supersession-and-energy-observations.md) (task and standing), both implemented; legacy honey-blocked tasks stay readable until purged. Follow-ups stay here; the ideas that would expand the MVP live in [Ideas](backlog-ideas.md#energy-and-honey) and its known defects in [Bugs](backlog-bugs.md).

#### New trace from interrupted worktree

- **Kind:** follow-up
- **Source:** planning (`system.kill` / hard kill); [013-system-kill](../specs/013-system-kill.md)
- **Summary:** After a hard kill (or late-stage avalanche), good early work may already live in `.paseka/worktrees/<traceId>/`. Need an operator path to start a **new** `traceId` that reuses that worktree (or grafts its branch/diff) instead of discarding progress and redoing from `HEAD`.
- **Why deferred:** Orthogonal to kill protocol itself (`paseka kill` shipped); needs worktree registry + trace bootstrap design (identity, honey budget, which tasks/events to carry).
- **Revisit when:** Operators hit “early stages were fine, last stage blew up” without a clean continue path. The console surface is settled — `/worktrees` is where the interrupted checkout will be listed, and it will carry a trail link on every row — so the open half is the trace bootstrap, not the place to show it.

#### Energy gate on `paseka bee run` / `bee chat`

- **Kind:** follow-up
- **Source:** planning (energyToken)
- **Summary:** One-shot `bee run` and `bee chat` bypass the reactor today; only paths through `paseka run` consume honey. Gate standalone invocations the same way. The console's **Run bee** (`POST /api/bees/:role/run`) is the same dispatch and carries the same gap — it says so in its own copy rather than pretending otherwise.
- **Why deferred:** Requires adapter-layer changes without a running reactor.
- **Revisit when:** Operators need honey accounting for one-shot/interactive launches, or eval/product rules demand it.

#### Interactive session usage (Cursor)

- **Kind:** follow-up
- **Source:** planning (energyToken / SessionAdapter)
- **Summary:** Surface Cursor token `usage` from `bee chat`. Pi and OpenCode already report interactive usage through the `SessionUsageResolver` seam on `session.json`; Cursor has no equivalent read — its stream-json usage only exists on the AFK path.
- **Why deferred:** Cursor's TUI keeps no provider-side token store to read after exit, so it needs its own capture approach rather than the post-exit accounting the other adapters use.
- **Revisit when:** Console/CLI need Cursor session token usage, or billing/observability work starts.

### Queen Console

API fields for energy and merge-diff exist; per-run proposal preview is still thin.

#### Range reads for oversized comb files

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Traces migration)
- **Summary:** The trace comb modal reports `file too large for inline preview` for anything over 512 KiB, with no way to read the rest. A byte-range or line-range read, or a download link beside the omission, would make a large `checkpoint.json` inspectable without raising the inline cap.
- **Why deferred:** The cap is the server's and predates the redesign; the legacy console had the same dead end. Trail comb files are usually small, and raising the cap trades memory for a case that is rare.
- **Revisit when:** An operator hits an unreadable comb file in a real trail, or comb files start growing past a few hundred KiB.

#### Console redesign parity sweep

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md)
- **Summary:** Every console route is migrated and no page resolves to `PagePlaceholder`, and the explicit root cutover from the legacy bundle — the last step this entry tracked — shipped as [051-console-root-cutover](../specs/051-console-root-cutover.md): the modern console is served at `/`, the legacy vanilla-JS bundle is deleted rather than kept behind a flag, and `/next` permanently redirects to the root. Worktrees shipped as a split rather than a port: its worktree table and the orphan prune left `/git` for `/worktrees`, the data stayed on `GET /api/git`, and `WorktreeCard` was dropped. Bees shipped as the first route to need a **server** answer rather than only markup: `GET /api/bees` grew a `scope` because the endpoint the launch forms read is a picker that hides every non-interactive bee, and `BeeCard` was dropped for the seventh `DataTable`. Settings shipped as the second, on a new `GET /api/config`, and is the only route that is deliberately incomplete — its write half is [Spec 036](../specs/036-console-config-write.md).
- **Closed:** Shipped. The cutover is recorded in [Changelog](changelog.md) and [051-console-root-cutover](../specs/051-console-root-cutover.md) is Implemented; the visual-regression and Playwright gap is not closed by it and stays open as separate work in the [UI migration backlog](ui-migration-backlog.md#no-visual-regression-or-end-to-end-suite).

#### A worktree registry state the console can see

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Worktrees scope)
- **Summary:** The worktree list is built from the home registry, `git worktree list --porcelain`, and the `.paseka/worktrees/` directory, and then every row that is no longer a checkout root is dropped. So the console cannot show registered-but-missing, on-disk-but-unregistered, or a branch that disagrees with the `worktree.branch` insight, and pressing prune is the only way to learn what it would remove. A per-row state (`registered` / `on disk` / `missing` / `orphaned`) plus a preview of what a prune would drop would make `/worktrees` the reconciliation surface its scope was argued to be.
- **Why deferred:** The route ships as a live-checkout inventory and the prune result names what it reconciled, so nothing is lost silently. A row for a checkout that does not exist also changes what the list's empty state means, and the disagreements are rare enough that a state machine is not yet earned.
- **Revisit when:** An operator presses prune to find out what is lying around, or a stale registry row surfaces as a confusing trace error instead of a cleanup.

#### Per-run proposal diff in Reviews

- **Kind:** follow-up
- **Source:** [002-queen-console-mvp](../specs/002-queen-console-mvp.md); planning (reviews)
- **Summary:** Side-by-side preview of per-run `MUTATION/code.proposal.isolated` / `code.proposal.root` for `review: required` tasks. Final merge gate preview (`GET /api/traces/:traceId/merge-diff`) already ships, and [039-proposal-patch-records](../specs/039-proposal-patch-records.md) shipped the storage plus a per-trail proposal-patches list and body preview; what remains is the **side-by-side, per-run diff view** built on those archived bodies. The `Proposal patches` block in the review detail is a list with an on-demand body, not the reviewer's two-column diff.
- **Why deferred:** Final merge gate was enough for MVP; per-run preview is extra UI surface.
- **Revisit when:** Beekeepers need mid-trace proposal diffs without waiting for the merge gate, now that 039 ships the patch listing and only the side-by-side view is left.

#### An index over trail summaries, so a page costs a page

- **Kind:** follow-up
- **Source:** planning (Traces pagination — the removed `cursorPageScanLimit`)
- **Summary:** `runs.ScanRecentTraces` is the whole story of every trail list in the product, and it is a full walk: `os.ReadDir` over `.paseka/runs`, then `loadTraceSummary` per trace — a second `ReadDir` of that directory, a `LoadRunMeta` file read per agent directory, and `ListTraceTaskIDs` when there are tasks — then a sort of everything, and only then a slice to `limit`. **The limit bounds the result, not the work**, so a 15-row page costs exactly what a 200-row page costs, and both cost what the colony's whole history costs. `ScanTracesAfter` shares that walk and filters a cursor over it in memory, so the trail page pays it once per read like everything else. Eight call sites pay it, and several ask for a wide window on purpose: `console/dashboard.go` reads `dashboardTraceLimit*3`, `hiveview.GetTrace` reads `maxEventScanTraces*4` (200) just to find one trail it could have read directly, and the task board and snapshot each read `TaskBoardTraceLimit`. The fix is a small manifest beside `.paseka/runs` — trace id, last activity, run and task counts, bee list, the failure and active flags, and the trail record's event count — written when a run finishes and consulted instead of the walk, with the walk kept as the fallback that rebuilds it when the manifest is absent or stale. That turns a list read into O(page) and makes `GetTrace` a single read.
- **Why deferred:** Nothing is slow yet — this colony keeps 61 trail directories and a page assembles in single-digit milliseconds — and the walk is also what makes the projection honest, since a summary is derived from files a bee may still be writing. It is orthogonal to cursor-versus-rank and was **not** a reason to prefer either: a rank buys the same walk, and the cursor's old `limit * 10` cap bought nothing at all, which is why the cap went and the cursor stayed. Writing a manifest is a new on-disk artifact that a `paseka init`, a worktree move, or a hand-deleted trail directory can desynchronize, and that class of bug is worse than a slow page.
- **Revisit when:** Trail directories reach the low hundreds — the `getTrace` fallback and `defaultTraceScanLimit` are the two places that start guessing, and the daily standing-tick habit in "Compact standing-trail task history" is what will get there — or when a Console read is measurably slow, which is the only signal that distinguishes this from every other scan in the codebase.

### Pull-request delivery

Leftovers from [024-pull-request-delivery](../specs/024-pull-request-delivery.md). v1 is colony-wide `defaults.delivery`, an explicit home `forge.command`, and reconcile via forge `get`. Delivery outcomes — the trail-owned episode, closed-without-merge, and the badge that distinguishes a published PR from an un-reviewed gate — are designed in [047-delivery-outcomes](../specs/047-delivery-outcomes.md), not here.

#### Per-cue delivery

- **Kind:** follow-up
- **Source:** [024-pull-request-delivery](../specs/024-pull-request-delivery.md); named in [047-delivery-outcomes](../specs/047-delivery-outcomes.md) § Out of Scope
- **Summary:** Let a Forage Cue override `defaults.delivery` so one trail can publish a PR while the rest of the colony stays on `local_merge`.
- **Why deferred:** Colony-wide policy was enough for homelab vs laptop; per-cue mixing needs cue schema and review UX.
- **Revisit when:** Operators want a single colony that both merges locally and opens PRs depending on the cue.

### Console Git

Leftovers from [023-console-git](../specs/023-console-git.md). The Git tab MVP covers status vs origin, fetch, explicit push, ff-only pull, worktrees, leftover branch delete, Reviews origin-ahead warn, and skip-hooks defaults.

#### Autostash list on Git tab

- **Kind:** follow-up
- **Source:** [023-console-git](../specs/023-console-git.md); [009-merge-autostash](../specs/009-merge-autostash.md)
- **Summary:** Show `git stash list` entries (especially `paseka: autostash before merge …`) on the Git tab when merge left a stash behind.
- **Why deferred:** Leftover refs and worktrees were the homelab cleanup priority; stash pop/drop is easy to get wrong from a browser.
- **Revisit when:** Operators hit merge-failure stashes on the apiary without SSH, or 009 leftovers show up in real homelab use.

#### Porcelain file list

- **Kind:** follow-up
- **Source:** [023-console-git](../specs/023-console-git.md)
- **Summary:** Git tab dirty flag expands to a short `git status --porcelain` file list (hivewright R1 uncommitted paths). No stage/commit UI.
- **Why deferred:** A boolean dirty flag is enough to know Pull/autostash will fire; a file list is extra UI.
- **Revisit when:** Beekeepers cannot tell *what* dirtied colony root from Console and avoid SSH for that reason.

#### Remote connectivity probe

- **Kind:** follow-up
- **Source:** [023-console-git](../specs/023-console-git.md)
- **Summary:** Cheap probe that origin fetch/push is reachable (without publishing), distinct from running Push and from dumping helper secrets.
- **Why deferred:** Real `git fetch`/`push` errors already surface helper/auth failures; a dedicated probe is extra round-trips to Gitea.
- **Revisit when:** Operators want a green/red “tea/helper OK” before touching Push, or fetch-on-demand is too heavy as the only signal.

#### Incoming log vs origin

- **Kind:** follow-up
- **Source:** [023-console-git](../specs/023-console-git.md)
- **Summary:** Short log of `HEAD..origin/<default>` (what a sidecar pull would fast-forward), alongside the MVP unpublished outbound list.
- **Why deferred:** Behind count plus Fetch is enough to decide Pull vs wait; outbound unpublished list covers Push.
- **Revisit when:** Beekeepers need to see *which* remote commits they are missing before ff-only Pull.

#### Mutation lock while bees run

- **Kind:** follow-up
- **Source:** [023-console-git](../specs/023-console-git.md)
- **Summary:** Disable Push and branch-delete (not Fetch) while Live bees or a merge is in progress. Pull already refuses colony-root bees and in-progress merge in 023.
- **Why deferred:** Push does not rewrite the working tree; isolated worktrees do not need a global lock for v1. Extra coupling to the agents API. The `/git` store serialises its own mutations so two clicks cannot race, but that is a UI guard against one page, not this lock — it sees nothing about live bees.
- **Revisit when:** A Push or branch delete races an in-flight adapter in practice, or operators ask for a hard lock.

#### Gitea (or origin host) commit links

- **Kind:** follow-up
- **Source:** [023-console-git](../specs/023-console-git.md)
- **Summary:** Link HEAD / unpublished SHAs to the origin host commit URL (e.g. Gitea). Not a PR or issues UI.
- **Why deferred:** SHA + Push is enough to publish; hyperlinks are convenience once the tab exists.
- **Revisit when:** Beekeepers copy SHAs into Gitea often enough that Console should deep-link.

### Code proposal workspaces

Leftovers from [008-code-proposal-workspaces](../specs/008-code-proposal-workspaces.md).

#### `proposal_paths` allowlist

- **Kind:** follow-up
- **Source:** [008-code-proposal-workspaces](../specs/008-code-proposal-workspaces.md)
- **Summary:** Restrict which paths may appear in a code proposal.
- **Why deferred:** Not required for dual isolated/root proposal MVP.
- **Revisit when:** Colonies need path policy to limit proposal scope.

#### Untracked files in proposal delta

- **Kind:** follow-up
- **Source:** [008-code-proposal-workspaces](../specs/008-code-proposal-workspaces.md)
- **Summary:** Include or define behavior for untracked files in proposal deltas.
- **Why deferred:** Deferred from 008 ship to keep delta semantics simple.
- **Revisit when:** Real proposals lose important untracked files, or operators ask for explicit rules.

#### Alias removal for bare `code.proposal`

- **Kind:** follow-up
- **Source:** [008-code-proposal-workspaces](../specs/008-code-proposal-workspaces.md)
- **Summary:** Timeline and migration to remove the bare `code.proposal` alias (today → isolated).
- **Why deferred:** Alias keeps older colonies working while `.isolated` / `.root` settle.
- **Revisit when:** Docs and colonies have moved to explicit kinds and the alias is a liability.

### Runtime check gate

Follow-ups from [046-bee-check-gate](../specs/046-bee-check-gate.md), shipped as the runtime stage plus the reference-colony rollout.

#### The gate script's identity is not recorded in the gate result

- **Kind:** follow-up
- **Source:** [046-bee-check-gate](../specs/046-bee-check-gate.md) rollout
- **Summary:** The gate runs `$COLONY_ROOT/scripts/checks.sh`, so the configured command is pinned by the resolved colony snapshot while the script *body* is not: editing the script on the colony root changes gate behaviour mid-trail, and nothing in the trail record says which version ran. The script prints its own `sha256sum "$0"` and the colony `HEAD` into `checks.log`, which makes the evidence available but unparsed.
- **Why deferred:** A digest on `GateResult` (path + content hash + colony revision) is a record-shape change, and the log line already answers the question for a human reading the run.
- **Revisit when:** A gate verdict has to be explained after the script changed, or a trail is audited for "what exactly judged this proposal".

#### Console attribution and `doctor` findings for the gate

- **Kind:** follow-up
- **Source:** [046-bee-check-gate](../specs/046-bee-check-gate.md) — the two gaps named in its status note
- **Summary:** The `GateResult` is written on the run record and the proposal archive record, but the Queen Console run page does not show it, so explaining a rejected proposal means opening `status.json`. `paseka doctor` also reports neither a gate command that cannot resolve on the host nor a gate declared by a Bee that publishes no code.
- **Why deferred:** The runtime stage and the colony rollout came first; both remaining pieces are read-only surfaces.
- **Revisit when:** An operator has to read `runs/<agentId>/status.json` to say why a proposal never appeared, or a colony ships a gate command that is missing on the runtime host.

### NATS / hive substrate

#### Dispatches still run inside the bus callback

- **Kind:** follow-up
- **Source:** trail audit `trace-01a1a867357be42e` (closing round re-dispatched and left `running`)
- **Summary:** The reactor now dispatches a seeded closing round through `runDispatch` (async on the NATS path) and keeps its own echoes suppressed for redelivery, so a slow closing run no longer blocks the durable consumer. Two callback paths still run an agent inline and can hold the consumer past `AckWait` (30s): `handleHumanFeedback` (`review_gate.go`) and `unblockEnergyBlockedTasks` (`energy.go`, ordered before the pending drain on `energy.add`). A rework or energy-unblock run therefore still lets JetStream redeliver the events published during it; the echo memory absorbs the runtime's own events, but any non-idempotent reducer reached by another producer's redelivery is unprotected. Move the rework dispatch onto `runDispatch` and give the energy drain the same treatment, or raise `AckWait` and ack in a way that survives a long handler.
- **Why deferred:** The reported closing-phase failure is fixed by the echo and seed changes; the remaining paths are rarer (human rejection, energy top-up) and the energy drain's ordering needs its own test before it can move off the callback.
- **Revisit when:** A rework or energy-unblock run is observed blocking the reactor, or another task is found stuck `running`/`waiting_review` with no owner after a long run.

## Assumptions and gotchas

### Reading trail data from the filesystem

Trail audit tooling has exactly one reliable join and three traps. Verified against `trace-01a10d602db8d6f6`; see [Ideas — Colony roles](backlog-ideas.md#colony-roles) for the collector that uses it.

- **A run's `taskId` lives only in `request.json`** — it has no `taskId` key in `meta.json`, `status.json`, or `result.json`. Do not grep `prompt.txt` for it: rendered templates embed literal JSON examples, and the first `"taskId":"…"` match in a scout's prompt is a sample task from a partial, not the trail's.
- **`tasks/<taskId>/runs.ndjson` is not a complete task→run join** — on that trail it held only the first builder run. Rework dispatches (`dispatch: direct`, i.e. `verification.failed` fix-ups), guard inspections, and the receiver's commit run are absent from it. Join on `request.json.taskId`, not on this file.
- **`createdAt` in task frontmatter is absent on trails planned before it shipped** — the ledger now stamps it once per task, so wait time is `updatedAt - createdAt` read offline, but a trail written earlier carries only `updatedAt`, which moves on every status write. Read a missing `createdAt` as unknown, never as zero. See [task ledger](../reference/task-ledger.md) § Filesystem task projection.
- **A run's own event log proves only what that bee published** — `<agentId>/events.ndjson` is a per-attempt audit of that run, and nothing the runtime did on its behalf lands there. Runtime-synthesized events (a `MUTATION` proposal above all) are in the trail record, `<traceId>/trail-events.ndjson`. Join the two, or read the trail through `runs.ReadTraceEvents`, which does it for you.
- **Metrics count retained evidence, not broker history** — `paseka inspect metrics` (and `metrics.json`) reads the trail record, run directories, task projections, retained origin, seed evidence, and completion snapshots. A dispatch whose run directory was reused or pruned is not counted, an absent usage report is `null` rather than zero, and gaps appear as `coverage`/`warnings`. Treat a partial total as "what is retained", never as the trail's full cost. See [CLI](../guide/cli.md#paseka-inspect-metrics).

### NDJSON line size

- **The 8 MiB line bound is a read limit, not validation** — every NDJSON reader in the runs package (`trail-events.ndjson`, `events.ndjson`, `pending.ndjson`, `transcript.ndjson`, task `runs.ndjson`) shares one scanner bounded at 8 MiB, and one unreadable log is skipped with a warning rather than failing a whole trail merge — which now holds for the trail record as well as for a run log.
- **A line that will not parse is skipped, not fatal** — both event logs are append-only and written a line at a time, so a crash mid-append leaves a torn last line; the reader drops that line, logs the file and line number, and returns everything before it. Read the missing line as never written, never as a truncated event. Nothing caps what a producer may *write*, so a payload past that bound still makes its own run log unreadable; it now costs one trail instead of every surface in the colony. Read a missing line as absent, never as a truncated event.
- **The Cursor provider session log still uses the `bufio` default** — `internal/adapters/cursor/sessionlog.go` reads Cursor's own JSONL and was left at the 64 KiB default when the runs package got an explicit bound. A provider line past that size is skipped as unreadable rather than parsed.

### Trail export and replay

- **An export is the trail's history; `paseka replay` is the bus's** — an export renders the same rows the timeline shows (trail record merged with the per-run logs, one event once), with full envelopes and payloads; `replay` lists what reached JetStream with kinds and agent ids but no payloads. Comparing the two is what `paseka verify <traceId>` is for.
- **A plan's recommendation can be lost when it becomes a task** — the adapter-probe plan asked for a cache "until forced refresh, with a long backstop TTL" and the task body said "no TTL"; both reviews accepted it. When a plan carries a safety margin, the task body has to carry it too.

### Reading trail events from the bus

- **The colony-wide timeline reads the trail record only** — a trail-scoped read (a trail page, an export, prompt memory) merges `trail-events.ndjson` with every run's `events.ndjson`, but the colony-wide feed deliberately costs one file per trail instead of every run log in every scanned trail. A trail written before the record existed therefore shows **no colony-wide rows** while remaining complete trail-scoped. There is no backfill.
- **The trail record is not proof of broadcast** — the write happens before the bus attempt on purpose, so an event it holds may never have reached JetStream, and nothing on the record says so. Ask `paseka verify <traceId>`, which reports what is only local, only on the bus, or in both.

### Config profiles

- **Concurrent `--profile` processes share worktrees and `state.json`** — two Queen Shell processes with different overlays are allowed, but they still use `.paseka/worktrees/<traceId>/` and the slug’s home registry. Compare adapters on separate traces. See [027](../specs/027-config-profiles.md).

### Eval colony

#### Standing trail live case

- **Kind:** follow-up
- **Source:** [028-standing-trails](../specs/028-standing-trails.md)
- **Summary:** Add a scripted eval-colony case that ticks a standing cue twice on a fixed `traceId` (checkpoint file reuse + stipend replace + overlap refuse) against live NATS.
- **Why deferred:** Platform tests already cover those behaviors in-process; the sibling eval runner is a separate repo and live LLM/script wiring is owned by [003](../specs/003-hive-evals.md).
- **Revisit when:** Extending `paseka-eval-colony` cases past `01`–`14`.

Wiring the side eval colony (`paseka-eval-colony`) and `runner/run-case.sh` against real NATS + `paseka run`. See [003-hive-evals](../specs/003-hive-evals.md).

Tier B in that repo already covers cases `01`–`14` (scripted loop, energy block, first-pass, inject-mutation, kill, human reject, cue hotfix, deferred emit, kill no-redispatch, signal direct, ready-before-plan, artifact scan-flush, deferred-artifact skip, artifact handoff). `runner/reset.sh` purges with `--reseed-energy` (skipped for cue ingress so the cue’s `energy_budget` seeds honey). `check_replay_event_chain` scores `case.yaml` `expect_event_chain` against `paseka replay`. Remaining eval work is Tier C (live LLM) and optional platform helpers (`paseka eval`, seeded `agentId`) — owned by spec 003, not this backlog.

- **Always pass `-C` to `paseka` from runner scripts** — resolving from cwd alone can target the wrong git repo (e.g. parent `paseka` platform) and `purge` the wrong colony. Use `paseka … -C "${EVAL_ROOT}"`.
- **Worktrees are created from `HEAD`, not the working tree** — seed code (`go.mod`, `pkg/`, …) must be **committed** before a trace worktree is created. Do not gitignore materialized seed files at the colony root. Also relevant outside eval; see [008](../specs/008-code-proposal-workspaces.md).
- **Script bees run from the worktree checkout** — `scripts/*.sh` and bee YAML come from git `HEAD`. Uncommitted script changes are invisible inside `.paseka/worktrees/<traceId>/`.
- **`paseka event emit` from script bees needs `-C "$PASEKA_COLONY_ROOT"`** — when cwd is the worktree, emit without `-C` fails colony/home resolution. Guard/receiver scripts must pass colony root explicitly.
- **`paseka event emit` can fail after a successful bus publish** — if audit log append to `.paseka/runs/<traceId>/<agentId>/events.ndjson` fails, emit exits non-zero and the adapter run is marked failed. Normal adapter runs have a run dir; ad-hoc manual emits need a matching run dir.
- **Fixed `trace` + JetStream state accumulates** — reusing case traces leaves ledger KV, depleted honey, and replay history. Stop `paseka run` first, then `paseka purge --bus --trace <case-trace> --reseed-energy` (eval `reset.sh` does this; cue-ingress cases omit `--reseed-energy`). See [CLI](../guide/cli.md) § `paseka purge`.
- **Only one `paseka run` consumer per colony subject prefix** — a second reactor logs `consumer is already bound to a subscription`. Stop the previous runtime before `run-case.sh` starts another.
- **Builder rework is async** — `verification.failed` → builder fix-up via direct dispatch can continue while the task is `waiting_review` or after `completed` (e.g. honey exhausted). Allow time for the guard→builder loop. A direct dispatch deferred by exhausted honey is recorded as pending work and resumes on `energy.add`; a deferred task dispatch now does the same since [spec 044b](../specs/044b-standing-supersession-and-energy-observations.md) shipped, keeping its lifecycle status rather than becoming `blocked`.
- **Oracle scope** — `go test ./...` in the worktree also picks up packages under `cases/…/expect/`. Prefer a narrow path (e.g. `go test ./pkg/...`) in `case.yaml` `oracle.command` and in script-guard bees.
