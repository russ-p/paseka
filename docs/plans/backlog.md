# Backlog

Deferred ideas, follow-ups, bugs, and implementation assumptions outside the active change.
Shipped work: [Changelog](changelog.md). Design drafts: [Specs index](specs-index.md).
Console redesign navigation debt lives in [UI migration backlog](ui-migration-backlog.md) — dead links, unbuilt transitions, and placeheld sections.

## Deferred work

### Trail export and replay

Found by auditing a shipped trail (`paseka export trace-01a10d602db8d6f6`, cross-checked against `paseka replay`). The process worked; what a trail leaves behind to be read afterwards did not.

#### Trail surfaces show a third of the trail's events

- **Kind:** bug
- **Source:** trail analysis (`trace-01a10d602db8d6f6`)
- **Summary:** `runs.ReadTraceEvents` merges only `.paseka/runs/<traceId>/<agentId>/events.ndjson`, which `bus.ProcessEventInput` appends for events a bee published through the CLI. Everything the runtime synthesizes goes straight to the bus and lands only in JetStream. That 28-event trail read as 11 events on every surface: missing were `SIGNAL/feature.requested` (the operator's own request), **both `MUTATION/code.proposal.isolated`** — the events guard actually subscribes to — `SIGNAL/task.ready`, all six `SIGNAL/energy.consume`, three `SIGNAL/task.status`, two auto-generated `INSIGHT/trace.summary`, and the runtime and console `task.plan` / `task.completed`. Affects `paseka export` (`internal/export/export.go:67`) and the Console Timeline (`hiveview.ListEventFeed`) alike — they share the reader.
- **Why deferred:** Needs a decision about what a trail is authoritative from: run directories (durable, agent-authored, survives `purge --bus`) or JetStream (complete, but gone with `--bus`). Appending runtime events to the emitting run directory is the smallest fix; reading the bus on export is the honest one and needs an offline path for a stopped broker.
- **Revisit when:** An operator or agent audits a trail and cannot answer "what was asked" or "what was proposed" from the export, or a review contract is replayed and the triggering `MUTATION` is absent.

#### A reviewed diff does not outlive the review

- **Kind:** bug
- **Source:** trail analysis (`trace-01a10d602db8d6f6`)
- **Summary:** `mutationFromRun` inlines the diff in the `MUTATION` payload and stores an artifact only above 64 KiB (`internal/runtime/publish.go:120`), `paseka replay` prints kinds and agent ids without payloads, and the worktree is removed at merge. So once a trail is merged the exact bytes a guard approved cannot be recovered — that trail's export says "No trail artifacts in the comb", and the reviewed diff of an 18-file, +1433/−14 change is gone.
- **Why deferred:** The artifact size policy is a disk-versus-inspectability trade-off, and lowering the 64 KiB threshold stores a full diff per builder run including the throwaway ones from a reject cycle.
- **Revisit when:** A `verification.success` needs re-audit after its worktree is gone, or an operator asks what a specific run actually proposed.

#### `Honey reserve` in the export Overview reads backwards

- **Kind:** bug
- **Source:** trail analysis (`trace-01a10d602db8d6f6`)
- **Summary:** The Overview card renders `taskledger.FormatHoneyPrimary`, which is `remaining / allocated` (`internal/taskledger/energy.go:44`), under the label `Honey reserve`. That trail's `6 / 12` meant six left of twelve and reads as six spent, while `paseka status` prints the same pair as `6/12 remaining` — so the two surfaces disagree on what the numbers mean.
- **Why deferred:** Cosmetic, and no logic consumes the string.
- **Revisit when:** Someone reads the export Overview for spend rather than for remaining balance.

#### Two `trace.summary` events for one trail step

- **Kind:** bug
- **Source:** trail analysis (`trace-01a10d602db8d6f6`)
- **Summary:** JetStream held bus sequence 10 and 11 as consecutive `INSIGHT/trace.summary` with the same synthetic agent id `agent-<traceId>`, between `task.ready` and `task.status` — a summary that looks hand-published twice, since no Go code in the tree mints that agent id. Payloads cannot be compared from the CLI (see above), so this is unconfirmed.
- **Why deferred:** Unverified and harmless on its own: `runs.ResolveTraceSummary` takes the last non-empty summary, so a duplicate changes nothing downstream.
- **Revisit when:** Raw bus payloads become readable, or a trail shows a summary that disagrees with what actually happened.

#### `paseka export` rejects the trace id as a positional argument

- **Kind:** idea
- **Source:** trail analysis (`trace-01a10d602db8d6f6`)
- **Summary:** The natural invocation `paseka export trace-01a1…` fails with `required flag(s) "trace" not set`; only `--trace` works. Accept the id positionally, as `paseka replay <traceId>` already does, and keep `--trace` as the alternative.
- **Why deferred:** The flag works; this is ergonomics, and export is not a hot path.
- **Revisit when:** Export gets a wrapper script or lands in a documented quickstart.

### Adapter catalog

#### The Agent CLI roster is duplicated across the Go/TS boundary

- **Kind:** follow-up
- **Source:** guard review note, deferred from the adapter-probe trail (`trace-01a10d602db8d6f6`)
- **Summary:** `adapterCLINames` (`internal/console/adapters.go:84`) and `agentCLIAdapterNames` (`web/src/lib/api/types.ts:117`) list the same four adapters in the same order with no cross-language guard. Both sides are pinned by tests (`adapters_api_test.go:119` asserts the literal order, `format.test.ts` asserts the literal `not probed, 4 adapters`), so drift fails loudly rather than silently. Serving the roster from `GET /api/system/adapters` without probing would delete the second list.
- **Why deferred:** The block is lazy by contract — arriving on `/next/system` must issue zero requests — so a roster in the first payload either breaks that rule or adds a second endpoint for four strings.
- **Revisit when:** A fifth adapter is added, or the roster stops being a fixed set.

### Task ledger

#### `autorun` flag on `task.plan`

- **Kind:** idea
- **Source:** `task.ready` race fix planning
- **Summary:** Single `INSIGHT/task.plan` payload field (e.g. `autorun: true`) that runtime translates into a follow-up `SIGNAL/task.ready` on the bus after plan apply — instead of bees emitting two deferred events.
- **Why deferred:** CLI/cues/Telegram already use plan+ready as two events; deferred FIFO pair matches `task create --autorun` without new protocol surface. Ledger `pendingReady` covers out-of-order emits.
- **Revisit when:** Product wants one-event autostart from bees without relying on prompt discipline, with explicit bus observability for the synthesized ready.

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

MVP shipped per-trace honey (`defaults.energy_budget`, `energy.add` / `energy.consume`, reactor gating, `paseka energy show|add`). Loop protection is energy depletion → `blocked` (`Honey reserve exhausted`). These items need separate design or evidence before expanding the MVP.

#### `confidence` (Pollen Quality)

- **Kind:** idea
- **Source:** [Brief](../idea/brief.md); planning (energyToken)
- **Summary:** Filter or weight events by confidence level alongside honey.
- **Why deferred:** Needs protocol and UX design (event shapes, CLI/Console) beyond the anti-loop MVP.
- **Revisit when:** Product brief item is specified with event shapes and operator surfaces, or eval scenarios require confidence filtering.

#### New trace from interrupted worktree

- **Kind:** follow-up
- **Source:** planning (`system.kill` / hard kill); [013-system-kill](../specs/013-system-kill.md)
- **Summary:** After a hard kill (or late-stage avalanche), good early work may already live in `.paseka/worktrees/<traceId>/`. Need an operator path to start a **new** `traceId` that reuses that worktree (or grafts its branch/diff) instead of discarding progress and redoing from `HEAD`.
- **Why deferred:** Orthogonal to kill protocol itself (`paseka kill` shipped); needs worktree registry + trace bootstrap design (identity, honey budget, which tasks/events to carry).
- **Revisit when:** Operators hit “early stages were fine, last stage blew up” without a clean continue path. The console surface is settled — `/next/worktrees` is where the interrupted checkout will be listed, and it will carry a trail link on every row — so the open half is the trace bootstrap, not the place to show it.

#### Energy gate on `paseka bee run` / `bee chat`

- **Kind:** follow-up
- **Source:** planning (energyToken)
- **Summary:** One-shot `bee run` and `bee chat` bypass the reactor today; only paths through `paseka run` consume honey. Gate standalone invocations the same way. The console's **Run bee** (`POST /api/bees/:role/run`) is the same dispatch and carries the same gap — it says so in its own copy rather than pretending otherwise.
- **Why deferred:** Requires adapter-layer changes without a running reactor.
- **Revisit when:** Operators need honey accounting for one-shot/interactive launches, or eval/product rules demand it.

#### Per-bee cost multipliers

- **Kind:** idea
- **Source:** planning (energyToken)
- **Summary:** Charge more than flat `1` per adapter dispatch (per-role or per-intent pricing in bee YAML or routing rules).
- **Why deferred:** Extra configuration surface before evidence that flat cost is too coarse.
- **Revisit when:** Operators report false positives/negatives from flat `1`-token cost or traces that block incorrectly.

#### Honey ↔ LLM token billing

- **Kind:** idea
- **Source:** planning (energyToken)
- **Summary:** Optionally relate honey spend to LLM `usage` on AFK `result.json`. Do not price honey from model tokens without a separate design.
- **Why deferred:** Orthogonal to anti-loop honey; mixing billing models needs an explicit decision.
- **Revisit when:** Product wants cost visibility tied to model usage, with a written design.

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

#### No mutation queue while NATS is down

- **Kind:** idea
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (user story #12)
- **Summary:** User story #12 asked the console to queue mutations locally when NATS disconnects. The banner half shipped — the topbar names `reconnecting` and then `unavailable`, and the transport icon reads the same fact — and the queue did not, deliberately: a queued approve or delete that fires minutes later is not the action the operator took, and the console's store contract already answers `busy` rather than queueing a second mutation. The open question is narrower than the story was: a *draft* the operator was typing (a review comment, a task form) is worth keeping across a reconnect, and a *committed* mutation is not.
- **Why deferred:** The console has no offline story to preserve, so the cost is a lost draft rather than a lost action, and the page already says the stream is down.
- **Revisit when:** An operator loses real typing to a reconnect, or a hive restart becomes routine enough to matter.

#### No form to create a bee or a worktree

- **Kind:** idea
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (user story #17)
- **Summary:** User story #17 asked for forms to open in a drawer rather than a column, and named four: new task, new bee, new worktree, settings edits. The task form and the session launch form both shipped and proved the primitive; the new-bee and new-worktree forms never existed. Both write committed colony YAML (`bees/*.yaml`) and the worktree case also creates a git checkout, which is the same class of risk [036-console-config-write](../specs/036-console-config-write.md) refuses for project config — a browser writing a tracked file is a different decision from a browser writing this machine's configuration.
- **Why deferred:** The routes that display both lists shipped read-only, and starting work has two supported paths today: a launch session, and **Run bee** on `/next/bees` for a headless run.
- **Revisit when:** An operator hand-edits `bees/*.yaml` often enough to want a form for it, or the worktrees route needs a create path for work that has no trail yet.

#### Bee local overlay is invisible and unwritable in Console

- **Kind:** idea
- **Source:** [036-console-config-write](../specs/036-console-config-write.md) (Out of Scope — per-bee editing)
- **Summary:** `.paseka/bees/<role>.local.yaml` is the machine-local overlay (`prompt_template` and `system_template` only, prompt-only by design) that wins over the committed `<role>.yaml` at resolve time, and the console neither reads nor writes it. `BeeView` has no overlay field, so a bee whose prompt is overridden on this machine renders the committed value with no hint that an override exists — the same missing-`source` half that `/next/settings` reports for configuration values. Reporting it is a per-bee `{value, source}` pair on `GET /api/bees`; writing it is the 036 writer (merge one key, temp+rename, preserve mode) pointed at a two-key gitignored file, which is a smaller surface than a full bee form.
- **Why deferred:** 036 puts per-bee editing out of scope on the rule that the console writes what is this machine's and nothing the repository shares, and it never names `*.local.yaml` — so the overlay was left undecided rather than refused. It sits in the repository directory, only its `.gitignore` entry makes it machine-local, so making it writable needs that distinction argued rather than assumed. Until then the overlay is invisible in the product: `LoadAllBees` skips it, and export `--include bees` and Nuc omit it.
- **Revisit when:** Per-bee or per-cue editing is reconsidered, or an operator is surprised by a prompt that is not the one `/next/bees` shows.

#### Console redesign `/next` parity sweep

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md)
- **Summary:** Every `/next` route is migrated and no page resolves to `PagePlaceholder`; what is left is the explicit root cutover from the legacy bundle. Worktrees shipped as a split rather than a port: its worktree table and the orphan prune left `/next/git` for `/next/worktrees`, the data stayed on `GET /api/git`, and `WorktreeCard` was dropped. Bees shipped as the first route to need a **server** answer rather than only markup: `GET /api/bees` grew a `scope` because the endpoint the launch forms read is a picker that hides every non-interactive bee, and `BeeCard` was dropped for the seventh `DataTable`. Settings shipped as the second, on a new `GET /api/config`, and is the only route that is deliberately incomplete — its write half is [Spec 036](../specs/036-console-config-write.md).
- **Why deferred:** The route set is complete, so only the cutover decision remains, and that one is explicitly gated on feature parity being called rather than inferred.
- **Revisit when:** The cutover is called.

#### A worktree registry state the console can see

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Worktrees scope)
- **Summary:** The worktree list is built from the home registry, `git worktree list --porcelain`, and the `.paseka/worktrees/` directory, and then every row that is no longer a checkout root is dropped. So the console cannot show registered-but-missing, on-disk-but-unregistered, or a branch that disagrees with the `worktree.branch` insight, and pressing prune is the only way to learn what it would remove. A per-row state (`registered` / `on disk` / `missing` / `orphaned`) plus a preview of what a prune would drop would make `/next/worktrees` the reconciliation surface its scope was argued to be.
- **Why deferred:** The route ships as a live-checkout inventory and the prune result names what it reconciled, so nothing is lost silently. A row for a checkout that does not exist also changes what the list's empty state means, and the disagreements are rare enough that a state machine is not yet earned.
- **Revisit when:** An operator presses prune to find out what is lying around, or a stale registry row surfaces as a confusing trace error instead of a cleanup.

#### Per-run proposal diff in Reviews

- **Kind:** follow-up
- **Source:** [002-queen-console-mvp](../specs/002-queen-console-mvp.md); planning (reviews)
- **Summary:** Side-by-side preview of per-run `MUTATION/code.proposal.isolated` / `code.proposal.root` for `review: required` tasks. Final merge gate preview (`GET /api/traces/:traceId/merge-diff`) already ships.
- **Why deferred:** Final merge gate was enough for MVP; per-run preview is extra UI surface.
- **Revisit when:** Beekeepers need mid-trace proposal diffs without waiting for the merge gate.

#### An index over trail summaries, so a page costs a page

- **Kind:** follow-up
- **Source:** planning (Traces pagination — the removed `cursorPageScanLimit`)
- **Summary:** `runs.ScanRecentTraces` is the whole story of every trail list in the product, and it is a full walk: `os.ReadDir` over `.paseka/runs`, then `loadTraceSummary` per trace — a second `ReadDir` of that directory, a `LoadRunMeta` file read per agent directory, and `ListTraceTaskIDs` when there are tasks — then a sort of everything, and only then a slice to `limit`. **The limit bounds the result, not the work**, so a 15-row page costs exactly what a 200-row page costs, and both cost what the colony's whole history costs. `ScanTracesAfter` shares that walk and filters a cursor over it in memory, so the trail page pays it once per read like everything else. Eight call sites pay it, and several ask for a wide window on purpose: `console/dashboard.go` reads `dashboardTraceLimit*3`, `hiveview.GetTrace` reads `maxEventScanTraces*4` (200) just to find one trail it could have read directly, and the task board and snapshot each read `TaskBoardTraceLimit`. The fix is a small manifest beside `.paseka/runs` — trace id, last activity, run and task counts, bee list, the failure and active flags — written when a run finishes and consulted instead of the walk, with the walk kept as the fallback that rebuilds it when the manifest is absent or stale. That turns a list read into O(page) and makes `GetTrace` a single read.
- **Why deferred:** Nothing is slow yet — this colony keeps 61 trail directories and a page assembles in single-digit milliseconds — and the walk is also what makes the projection honest, since a summary is derived from files a bee may still be writing. It is orthogonal to cursor-versus-rank and was **not** a reason to prefer either: a rank buys the same walk, and the cursor's old `limit * 10` cap bought nothing at all, which is why the cap went and the cursor stayed. Writing a manifest is a new on-disk artifact that a `paseka init`, a worktree move, or a hand-deleted trail directory can desynchronize, and that class of bug is worse than a slow page.
- **Revisit when:** Trail directories reach the low hundreds — the `getTrace` fallback and `defaultTraceScanLimit` are the two places that start guessing, and the daily standing-tick habit in "Compact standing-trail task history" is what will get there — or when a Console read is measurably slow, which is the only signal that distinguishes this from every other scan in the codebase.

### Pull-request delivery

Leftovers from [024-pull-request-delivery](../specs/024-pull-request-delivery.md). v1 is colony-wide `defaults.delivery`, an explicit home `forge.command`, and reconcile via forge `get`.

#### Per-cue delivery

- **Kind:** follow-up
- **Source:** [024-pull-request-delivery](../specs/024-pull-request-delivery.md)
- **Summary:** Let a Forage Cue override `defaults.delivery` so one trail can publish a PR while the rest of the colony stays on `local_merge`.
- **Why deferred:** Colony-wide policy was enough for homelab vs laptop; per-cue mixing needs cue schema and review UX.
- **Revisit when:** Operators want a single colony that both merges locally and opens PRs depending on the cue.

#### Auto-detect forge from origin

- **Kind:** idea
- **Source:** [024-pull-request-delivery](../specs/024-pull-request-delivery.md)
- **Summary:** Infer `tea` vs `gh` (or a bundled wrapper) from the `origin` host instead of requiring explicit `forge.command`.
- **Why deferred:** Fail-closed explicit argv avoids guessing the wrong CLI or a missing login; v1 wants a visible home-config choice.
- **Revisit when:** Beekeepers routinely have `tea`/`gh` on PATH and trip over the commented init example.

#### Dedicated `pr_open` task status

- **Kind:** follow-up
- **Source:** [024-pull-request-delivery](../specs/024-pull-request-delivery.md)
- **Summary:** A ledger status (or Console badge) distinct from `waiting_review` while a published PR is open, so Reviews does not look like an un-reviewed merge gate.
- **Why deferred:** Homestate PR identity plus URL/state on Reviews was enough for v1; a new status is a ledger/protocol change.
- **Revisit when:** Operators confuse published-but-unmerged trails with “please approve this diff” gates.

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
- **Why deferred:** Push does not rewrite the working tree; isolated worktrees do not need a global lock for v1. Extra coupling to the agents API. The `/next/git` store serialises its own mutations so two clicks cannot race, but that is a UI guard against one page, not this lock — it sees nothing about live bees.
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

### Releases

#### Windows release builds

- **Kind:** idea
- **Source:** planning (GoReleaser / cross-compile)
- **Summary:** Make `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./cmd/paseka` work (Unix-only PTY/HITL today: e.g. `SIGWINCH`, review `Setsid`), then add `windows` to GoReleaser `builds.goos` so release assets include `.exe` archives.
- **Why deferred:** Pipeline already ships linux/darwin from Ubuntu with `CGO_ENABLED=0`; Windows needs build tags/stubs before CI/release changes.
- **Revisit when:** Local/CI Windows cross-build succeeds and release should publish `windows/amd64` (optionally `windows/arm64`).

### NATS / hive substrate

Laptop onboarding still requires an external JetStream (`nats.url` in home config, or `PASEKA_NATS_URL`). Homelab already assumes a shared server ([homelab deployment](../guide/homelab-deployment.md)).

#### Optional embedded NATS

- **Kind:** idea
- **Source:** planning (laptop `paseka init` / `paseka run` DX)
- **Summary:** Opt-in in-process NATS+JetStream for a single-machine hive so first `paseka run` does not need Docker or a separately installed broker. Keep `PASEKA_NATS_URL` / home `nats.url` as the override for shared LAN/homelab JetStream. Store embedded server files under machine-local state (not `.paseka/` in the repo). Choreography contracts stay unchanged.
- **Why deferred:** Orthogonal to bee contracts; needs a written split (embedded vs external, bind address, data dir, one-consumer-per-prefix, shutdown) before changing `paseka init` defaults.
- **Revisit when:** Operators bounce on NATS as the first-run blocker, or we want a zero-dependency laptop path without weakening the homelab “bring your own JetStream” story.

## Assumptions and gotchas

### Trail export and replay

- **A trail export is not the trail's event log** — it merges only what bees published through the CLI, so the intake request, the `MUTATION` that triggers a review, honey consumption, and task status transitions are all absent. `paseka replay <traceId>` lists the runtime half (order, kinds, agent ids) but no payloads. See "Trail surfaces show a third of the trail's events".
- **A plan's recommendation can be lost when it becomes a task** — the adapter-probe plan asked for a cache "until forced refresh, with a long backstop TTL" and the task body said "no TTL"; both reviews accepted it. When a plan carries a safety margin, the task body has to carry it too.

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
- **Builder rework is async** — `verification.failed` → builder fix-up via direct dispatch can continue while the task is `waiting_review` or after `completed` (e.g. honey exhausted). Allow time for the guard→builder loop; treat `blocked` as terminal when honey runs out.
- **Oracle scope** — `go test ./...` in the worktree also picks up packages under `cases/…/expect/`. Prefer a narrow path (e.g. `go test ./pkg/...`) in `case.yaml` `oracle.command` and in script-guard bees.
