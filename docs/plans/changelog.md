# Changelog

Shipped features worth calling out. Design records live under `docs/specs/` in the repo (not published on the docs site) — see [Specs index](specs-index.md).

## 2026-07 — Deferred event emit buffer

Bees can stage bus events until a run or session completes successfully. `paseka event emit --defer` validates and appends to per-run `pending.ndjson`; runtime flushes FIFO on success (before `run.summary` synthesis). Operators inspect with `paseka event pending` and recover with `paseka event flush` or `--discard`. Platform control kinds (`system.kill`, `energy.*`, `session.invite`, `beekeeper.ready`, `task.status`) are live-only.

- Spec: [015-deferred-event-emit](../specs/015-deferred-event-emit.md)
- Canonical: [CLI](../guide/cli.md) (`paseka event emit`, `pending`, `flush`), [Prompt templates](../guide/prompt-templates.md), [Interactive sessions](../guide/interactive-sessions.md)

Deferred from that work: 014 scan-flush coexistence with deferred `artifact.written` — see [Backlog](backlog.md).

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
