# Changelog

Shipped features worth calling out. Design records live under `docs/specs/` in the repo (not published on the docs site) — see [Specs index](specs-index.md).

## 2026-07 — Telegram Human Gateway

Async phone triage via `paseka gate telegram`: long-poll Bot API, allowlisted chats, bus notify + reconcile dedup, `/status` `/energy` `/task` `/invites` `/help`, invite HITL (local PTY on accept), and proposal reject / soft-mid approve (final-merge Console/CLI only).

- Spec: [010-telegram-human-gateway](../specs/010-telegram-human-gateway.md)

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
