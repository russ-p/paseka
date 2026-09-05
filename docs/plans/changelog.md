# Changelog

Shipped features worth calling out. Design records live under `docs/specs/` in the repo (not published on the docs site) — see [Specs index](specs-index.md).

## 2026-09 — Standing cue identity

A Forage Cue may declare `standing.trace` and `standing.stipend`. Omitting `--trace` / API `traceId` / Telegram `cue:` then publishes on that stable Flight Trail instead of minting a disposable bloom id; a mismatched explicit id fails closed. First tick seeds honey from stipend; later ticks do not yet refill remaining. Standing task cues require `review: none` and `worktree: false`. Nuc export/import carries the YAML as today.

- Spec: [028-standing-trails](../specs/028-standing-trails.md) (Draft; identity slice)
- Canonical: [Forage Cues](../guide/cues.md), [CLI](../guide/cli.md) (`paseka cue`), [Glossary](../idea/glossary.md) (Standing Trail)

Deferred from that work: per-tick stipend replace, overlap refuse, kill fail-closed, Console/status badge — see [Spec 028](../specs/028-standing-trails.md).

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
- Canonical: [Insight kinds](reference/insight-kinds.md), [Architecture overview](architecture/overview.md), [Prompt templates](guide/prompt-templates.md), [Interactive sessions](guide/interactive-sessions.md)

## 2026-08 — Final-gate Request changes (Slice C)

On `review: final` / `_review`, annotated Submit on the merge preview is **Request changes**: it writes the comb packet as before, keeps the merge gate in `waiting_review`, and plans+readies a new AFK rework task for the last isolated-proposal Bee on the same Flight Trail (honey consumed on dispatch). Plain reject remains abandon-only (feedback, no rework). Duplicate Request changes is rejected while non-final work is still in flight. Approve stays the only merge path. CLI `paseka proposal reject --comments-file` on a final gate uses the same rework path.

- Spec: [017-console-diff-review](../specs/017-console-diff-review.md)
- Canonical: [CLI](guide/cli.md) (`paseka console`, `paseka proposal reject`), [Task ledger](reference/task-ledger.md), [Queen Console MVP](../specs/002-queen-console-mvp.md) (Reviews)

## 2026-08 — Session deferred flush uses home NATS config

Interactive session (and other ColonyRoot-only) deferred flush loads `~/.config/paseka/<slug>/config.yaml` before connecting. Previously a partial in-memory context treated NATS as unset, flushed pending events to the run audit log with a no-op publisher, and never reached JetStream or the task ledger.

- Canonical: [Interactive sessions](guide/interactive-sessions.md), [CLI](guide/cli.md) (`paseka event emit --defer`)

## 2026-08 — Queen Console annotated review comments (Slice B)

Queen Console merge preview supports line-anchored comment drafts (click added/context lines; Shift-click for a range). Submit writes `review-comments.md` to the trail comb, publishes `SIGNAL/artifact.written` (producer `console`), then a short `INSIGHT/human.feedback` with optional `ref`. Fail closed if the comb write fails. `review: required` still returns the task to `ready`. CLI: `paseka proposal reject --comments-file` copies an existing Markdown packet into the same comb ref. Final-gate rework shipped separately as Slice C.

- Spec: [017-console-diff-review](../specs/017-console-diff-review.md) (Slice B)
- Canonical: [Queen Console MVP](../specs/002-queen-console-mvp.md) (Reviews), [CLI](guide/cli.md) (`paseka proposal reject`), [Insight kinds](reference/insight-kinds.md), [Prompt templates](guide/prompt-templates.md)

## 2026-08 — Model aliases (`params.model`)

Colony-owned `model_aliases` in `.paseka/colony.yaml` map stable names to vendor model ids; home `config.yaml` overlays the same keys per machine. Bees keep `params.model` as alias or raw id; runtime resolves once before `--model` is passed to the adapter.

- Spec: [019-model-aliases](../specs/019-model-aliases.md)
- Canonical: [Colony layout](guide/colony-layout.md), [Bee config](guide/bee-config.md), [Architecture overview](architecture/overview.md)

## 2026-08 — Trail artifacts protocol (comb + `artifact.written`)

Trace-scoped comb under `.paseka/runs/<traceId>/artifacts/` with `{{.ArtifactsDir}}` prompt injection. Runtime captures a per-run SHA-256 baseline and publishes one batched `SIGNAL/artifact.written` on successful AFK or interactive exit (added/changed files only). Coexistence with deferred `artifact.written` skips duplicate scan flush. Queen Console lists comb files (staged vs announced) with Markdown preview. `paseka export --include artifacts` inlines comb bodies in reports. Human `artifacts.WriteAndAnnounce` helper publishes with producer `console` (for 017 Slice B).

- Spec: [014-artifacts-protocol](../specs/014-artifacts-protocol.md)
- Canonical: [Architecture overview](architecture/overview.md) (runs comb), [Prompt templates](guide/prompt-templates.md) (`ArtifactsDir`), [Bee routing](reference/bee-routing.md), [CLI](guide/cli.md) (`paseka export --include artifacts`)

## 2026-08 — Queen Console merge-diff viewer (Slice A)

Queen Console Reviews final merge gates show merge-diff summary on the detail panel; **Open merge preview** opens a dedicated full-page viewer (sticky file list, path filter, jump-to-file, per-file hunks via vendored Diff2Html, unified or side-by-side format). Line-number gutters stay clipped to each file pane instead of overlaying scrolled hunks. Approve/reject stay on Reviews. Queue polling still skips re-fetch when the same gate stays selected.

- Spec: [017-console-diff-review](../specs/017-console-diff-review.md) (Slice A only; B/C follow)
- Canonical: [Queen Console MVP](../specs/002-queen-console-mvp.md) (Reviews tab), [CLI](guide/cli.md) (`paseka console`)

## 2026-08 — Queen Shell colony status

`paseka status` is a read-only colony snapshot for Beekeepers and interface bees: runtime liveness, live bees, task counts, honey for recent Flight Trails, attention items (reviews, invites, failures, exhausted honey), and recent traces. Default text output; `--json` emits `schemaVersion` 1 for agents. `--check` exits non-zero only when the hive substrate cannot choreograph (runtime down or configured NATS unreachable) — pending HITL work is not treated as an outage.

- Spec: [018-cli-colony-status](../specs/018-cli-colony-status.md)
- Canonical: [CLI](guide/cli.md) (`paseka status`)

## 2026-08 — `task.ready` race fix (prompts + ledger)

Scout and Drone breakdown prompts now defer both `task.plan` and post-plan `task.ready` (FIFO flush), with slim ready payloads (`taskId` only). The task ledger parks unmatched `task.ready` kicks on `pendingReady` until the matching `task.plan` registers the task, then promotes on the plan event — so live-ready-before-plan no longer loses autostart. Cleared on `system.kill`.

- Canonical: [Task ledger](reference/task-ledger.md), [Prompt templates](guide/prompt-templates.md)

## 2026-08 — Export `--include` (richer payload)

`paseka export` accepts composable `--include` slices independent of `--format`: `usage` (trace aggregate + per-run tokens), `durations` (wall-clock per run), `bees` (committed `.paseka/bees/*.yaml` for trail roles), `colony` (`.paseka/colony.yaml`), `cues` (all colony cues), and `artifacts` (trail comb file bodies). Default export stays trail-only with no config snapshots.

- Canonical: [CLI](guide/cli.md) (`paseka export`)

## 2026-08 — Export `--format` (HTML | Markdown)

`paseka export` now accepts `--format html` (default) or `--format md`. Both renderers share the same `TraceExportData` (overview, tasks, runs, event timeline); the output filename extension matches the format. Markdown keeps run and event summaries verbatim and fences raw event JSON for agent-friendly trail dumps.

- Canonical: [CLI](guide/cli.md) (`paseka export`)

## 2026-08 — Forage Cues (cue layer)

Named colony ingress shortcuts (`.paseka/cues/<id>.yaml`) publish `signal` or `task` choreography without hand-writing emit JSON. One definition drives Queen Shell (`paseka cue list|run`), Queen Console **Run cue** (`GET/POST /api/cues`), and Telegram `commands.custom` with `cue: <id>`. Optional per-cue `energy_budget` seeds a smaller initial honey reserve on fresh trails; `paseka init` scaffolds `feature` and `hotfix`. Nuc export/import includes cues with `--cues` filter.

- Spec: [016-cue-layer](../specs/016-cue-layer.md)
- Canonical: [Forage Cues](guide/cues.md), [CLI](guide/cli.md) (`paseka cue`), [Telegram gateway](guide/telegram-gateway.md), [Colony layout](guide/colony-layout.md), [Nuc packs](guide/nuc.md), [Task ledger](reference/task-ledger.md)

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
- Canonical: [Queen Console MVP](../specs/002-queen-console-mvp.md) (Sessions / Reviews tabs)

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
