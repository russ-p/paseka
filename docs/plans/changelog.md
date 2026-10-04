# Changelog

Shipped features worth calling out. Design records live under `docs/specs/` in the repo (not published on the docs site) — see [Specs index](specs-index.md).

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
