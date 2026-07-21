# Changelog

Shipped features worth calling out. Design records live under `docs/specs/` in the repo (not published on the docs site) — see [Specs index](specs-index.md).

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
