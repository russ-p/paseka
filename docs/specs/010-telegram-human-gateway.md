# Spec 010: Telegram Human Gateway

## Status

**Implemented (MVP).** Shipped 2026-07-18 via `paseka gate telegram` (`4410f01` bootstrap; `4c620a0` invites; `e058e54` energy/task/status notify; `ca01a24` proposal triage; merged in `13f6786`). Webhook transport remains V2 (config accepts `mode: webhook`; runtime requires long-poll).

## Purpose

Add **Telegram** as a third **Human Gateway** surface for the Beekeeper:

- push notifications for HITL and failure states
- minimal status and honey (energy) controls
- inject tasks / signals into the same bus + ledger path as Console
- accept / reject / defer `session.invite` from the phone

Telegram is an **async triage and control gate**, not a remote IDE and not a PTY relay.

This is platform surface design. Colony choreography stays in bee YAML + prompts. Vocabulary hooks: Human Gateway and Messenger Bee in [glossary](../idea/glossary.md).

## Goals

- Give the Beekeeper phone-reachable observability and a short set of mutations without opening Console.
- Reuse existing domain mutations (`task.plan` / `task.ready`, `energy.add`, invite accept/reject, proposal approve/reject) with `agentId: telegram`.
- Bind one bot instance to **one colony slug** (apiary) for MVP auth and subject isolation.
- Keep the feature inside the **single `paseka` binary** as `paseka gate telegram`.
- Explicitly park full interactive agent chat (`bee chat` / PTY / xterm) out of Telegram.

## Non-Goals

- Do not stream or emulate PTY / SessionAdapter over Telegram messages.
- Do not build a multi-user or remote multi-host control plane (same non-goal as [002](./002-queen-console-mvp.md)).
- Do not replace Queen Console for diffs, topology, timeline browsing, or browser attach.
- Do not require Telegram for local CLI/Console workflows.
- Do not implement chat-native coding-agent channels (Claude Code Channels / OpenClaw-style agent respond loops) inside Paseka — that belongs to agent plugins / a separate bot if desired.
- Do not ship federation context-switching (`/use <slug>`) in MVP.
- Do not approve **final-merge** proposals from Telegram (diff-blind merge risk).

## Current System Context

| Surface | Role today |
| ------- | ---------- |
| CLI | Full operator surface: `run`, `task`, `energy`, `invite`, `proposal`, `bee run|chat`, … |
| Queen Console | Local HTTP (`127.0.0.1:8787`) SPA + API; same mutations; PTY over WebSocket |
| Human Gateway invites | [006](./006-human-gateway-invites.md) — `session.invite` parking lot; accept starts **local** session |
| Energy | Per-trace honey reserve; `paseka energy show|add`; exhausted → task `blocked` |
| Binary | Single `paseka` (~22MB today); Console static embed ~1.8MB; adapters compiled in |
| Bus | `SubscribeEvents` durable JetStream consumer (`DeliverNew`) — reused by the gate |

Console and CLI already publish/consume the contracts Telegram needs. The gap is a **remote, push-capable, turn-based** UI with allowlisted chats — not new ledger semantics.

## Decisions

### 1. Telegram is a Human Gateway surface, not a Messenger Bee role

- **Inbound gate** (commands, callback buttons) → same handlers / services Console and CLI use.
- **Outbound notify** (push on selected bus events) → platform gate subscription.
- Do not require a colony bee YAML named `messenger` to make the gate work; colony may still use `post_exec` hooks independently.
- **Later (non-MVP):** optional colony notify policy / thin Messenger Bee may subscribe to the same events; must not fork the mutation path or duplicate Bot API credentials.

### 2. Async triage only — no PTY in Telegram

| In Telegram | Not in Telegram |
| ----------- | --------------- |
| Status, energy, task inject | `bee chat` byte-stream |
| Invite accept/reject/defer | Ghostty / Console xterm attach |
| Proposal reject; mid/soft approve | Full merge-diff review; **final-merge approve** |
| Optional Console deep-link when configured | Remote multi-beekeeper ACL |

On invite **accept**, start the interactive session **locally** (same path as Console accept): detached session on the machine running the gate. Telegram replies with “session started”, host hint that the PTY is on that machine (not in chat), and optional Console deep-link when `console_base_url` is set.

### 3. One bot token = one colony (MVP)

| Model | MVP | Later (optional) |
| ----- | --- | ---------------- |
| Binding | Bot token + allowlisted user/chat ids under `~/.config/paseka/<slug>/` | Federation: one bot, `/ctx <slug>` or forum topics |
| Isolation | Subject prefix + apiary home of that slug | Explicit confirmation on destructive cross-colony actions |
| Secrets | Machine-local only (never committed in `.paseka/`) | Shared allowlist still per machine |

Rationale: solo Beekeepers typically run few colonies; wrong-hive mutations are worse than managing a second BotFather token.

### 4. CLI and process model

- Command: **`paseka gate telegram`** (not a top-level `telegram`, not `run --gate`).
- Separate process from `paseka run` so Telegram network failures do not take down the hive consumer.
- Depend on a small Go Telegram Bot API client (expect roughly **+1–3MB** binary growth — negligible vs Console embed).
- Transport: **long-poll default**; webhook opt-in via config (`mode: webhook` + listen/url) for VPS with public TLS.
- Gate requires **NATS** + valid `telegram.yaml`; does **not** require `paseka run`. AFK progress after `task.ready` still needs a live reactor; `/status` reports hive/reactor alive?.

```text
paseka run                  # reactor (existing)
paseka gate telegram        # long-poll/webhook + notify + commands (new)
# optional: paseka console  # still local UI for heavy HITL
```

Gate responsibilities:

1. Resolve colony context (`-C` / cwd) → slug + NATS prefix + local secrets.
2. Subscribe to notify-worthy events (durable bus consumer) and reconcile projections on startup.
3. Dispatch inbound commands to shared internal packages (tasks, energy, invites, review) — prefer calling packages directly over HTTP loopback to Console.
4. Never block on PTY start beyond the same out-of-band accept path Console uses.

### 5. Same mutation path as Console

Mutations must not invent parallel state:

| Action | Mechanism |
| ------ | --------- |
| Create + optionally start task | `task.plan` → optional `task.ready` (agentId `telegram`) |
| Top up honey | `SIGNAL/energy.add` |
| Invite decision | `beekeeper.ready` with `accept` \| `reject` \| `defer` ([006](./006-human-gateway-invites.md)) |
| Proposal decision | Same flow as `paseka proposal approve|reject` / Console Reviews (with Telegram policy below) |
| Status | Read runtime registry + ledger projections (same sources as Console dashboard) |

### 6. Auth and safety

- Require Bot API token in machine-local config (or `PASEKA_TELEGRAM_BOT_TOKEN`).
- **`allow_from`**: Telegram user ids allowed to issue commands (required, non-empty).
- **`chat_ids`**: destination chats for outbound push (required, non-empty). Inbound commands from a chat not in `chat_ids` are also rejected.
- Non-allowlisted traffic: **silent ignore** (no “access denied” — do not advertise the bot).
- Do not expose arbitrary shell / `paseka` argv from chat text.
- **Two-step confirm** (inline Confirm/Cancel) for: invite Accept, invite Reject, proposal Approve (where allowed), proposal Reject. **Defer** needs no confirm.
- **`/task`**: always preview card (bee, review, truncated text) + Confirm/Cancel before `task.plan` / `task.ready`.
- **Proposal policy:** Reject always allowed (with confirm). Approve allowed only when the gate is **not** a final-merge approve (isolated `review: final` / `_review` merge). Final-merge waiting_review notifications offer Reject + “approve in Console/CLI only”.

### 7. Honey display on invites

Accept still costs **1 honey** (same as CLI/Console, [006](./006-human-gateway-invites.md)). Invite push includes one line `honey: N/M`. Accept is not disabled when honey is low; on insufficient-honey failure, reply with energy top-up buttons.

### 8. Notify pipeline (live + reconcile, no spam)

- Durable JetStream consumer name: `paseka-gate-telegram-<slug>` (via existing `SubscribeEvents`).
- On startup: **reconcile** pending invites and tasks in `waiting_review` / `blocked` / `failed`.
- Dedup via machine-local notify state (e.g. `telegram-notify-state.json` or a section in `state.json`): key by `invite:<id>` or `task:<traceId>:<taskId>:<status>`; push only on first sight or status change. Live bus handler and reconcile share the same dedup logic.

### 9. `/task` defaults live in gate config

Machine-local `telegram.yaml` (not colony shareable config):

| Key | MVP default | Notes |
| --- | ----------- | ----- |
| `commands.default_bee` | `builder` | Aligns with `paseka task create` |
| `commands.default_intent` | `general` | Intent for `default_bee` (see bee intents / `<role>-intent-*` partials) |
| `commands.default_review` | `none` | |
| `commands.task_autorun` | `true` | Phone triage expects plan+ready after Confirm |

No `/task --bee` switch in MVP.

## MVP surface

### Outbound notifications (push)

Notify allowlisted `chat_ids` when:

| Event / condition | Why |
| ----------------- | --- |
| `SIGNAL/session.invite` (`pending`) | Core Human Gateway parking lot |
| Task → `waiting_review` | Proposal HITL |
| Task → `blocked` (incl. honey exhausted) | Energy triage |
| Task → `failed` | Failure triage |

Message shape: short summary (trace, bee/task id, one-line task text, honey N/M when relevant) + inline buttons where applicable. Truncate long text; point to Console only when `console_base_url` is set.

### Inbound commands

| Command / UI | Behavior |
| ------------ | -------- |
| `/status` | Hive/reactor alive?, slug, subject prefix, live bee count, pending invite count, optional open blocked/failed counts; **Refresh** callback |
| `/energy` / `/energy <traceId>` | Show remaining/budget |
| `/energy add <traceId> <n>` | Arbitrary top-up |
| Energy buttons | On blocked / insufficient-honey replies: **`+1` / `+5` / `+12`** (no confirm) |
| `/task <text>` | Preview → Confirm → `task.plan` + autorun `task.ready` per gate defaults |
| `/invites` | Pending invites with Accept / Reject / Defer buttons |
| `/help` | Command list |
| Invite buttons | Accept / Reject / Defer (Accept/Reject two-step; Defer immediate) |
| Proposal buttons | Reject (two-step); Approve only when not final-merge (two-step); else Console-only hint |

### Explicitly deferred from MVP

- Daily digest / quiet hours
- `/kill` / `system.kill` (needs its own protocol design — see backlog energy follow-ups)
- Diff rendering or PR links beyond a one-line summary + optional Console URL
- Forum topics / multi-colony context switch
- Streaming tool-call noise from AFK runs
- Agent-plugin chat bridge (Cursor/Claude channel bots)
- `/traces` (V2)
- Co-starting the gate inside `paseka run`

## Configuration (machine-local)

```text
~/.config/paseka/<slug>/
  telegram.yaml                 # required for gate; not created by paseka init
  telegram-notify-state.json    # or equivalent notify dedup state (runtime)
  config.yaml                   # existing NATS / colony_root
```

Illustrative `telegram.yaml`:

```yaml
enabled: true
bot_token: "…"                 # or env PASEKA_TELEGRAM_BOT_TOKEN
mode: longpoll                 # or webhook
# webhook:
#   listen: "127.0.0.1:8443"
#   url: "https://example.com/paseka/telegram"
allow_from:
  - 123456789                  # Telegram user id (required, non-empty)
chat_ids:
  - -1001234567890             # push destinations (required, non-empty)
notify:
  invites: true
  waiting_review: true
  blocked: true
  failed: true
commands:
  task_autorun: true
  default_bee: builder
  default_intent: general
  default_review: none
console_base_url: ""           # optional; e.g. Tailscale/tunnel URL to Console
```

Colony shareable config (`.paseka/`) must not hold bot tokens. Optional later: colony-level notify policy flags only (no secrets).

If `telegram.yaml` is missing or `enabled: false`, `paseka gate telegram` exits with a clear error.

## Fit matrix (product)

| Capability | Telegram fit | Notes |
| ---------- | ------------ | ----- |
| Runtime / live status | High | Single message + Refresh |
| Energy show/add | High | `+1`/`+5`/`+12` on blocked alerts |
| Task inject | High | `/task` with confirm |
| Invite HITL | Highest | Push + buttons; local session on accept |
| Proposal reject / soft approve | High | Summary only |
| Final-merge approve | None (MVP) | Console / CLI |
| Merge-diff / topology | Low | Console |
| Interactive `bee chat` | Near zero | Local PTY / agent channels elsewhere |

## Phasing

| Phase | Scope |
| ----- | ----- |
| **MVP** | `telegram.yaml` + `paseka gate telegram`; long-poll; bus+reconcile notify with dedup; `/status`, `/energy`, `/task`, `/help`, `/invites`; invite + proposal buttons (final-merge approve blocked); allowlist + silent ignore |
| **V2** | Digests, quiet hours, `/traces`, Console deep-link polish, webhook hardening |
| **Later** | Federation `/ctx`, forum topics, `system.kill`, optional Messenger Bee / agent-channel companion (out of this binary’s PTY model) |

## Resolved questions

1. **CLI name** → `paseka gate telegram` (separate process).
2. **Honey before accept** → show `honey: N/M` on invite push; do not disable Accept; fail path offers energy buttons.
3. **`/task` defaults** → gate `telegram.yaml` (`builder` / `general` / `none` / autorun true).
4. **Transport** → long-poll default; webhook opt-in.
5. **Messenger Bee** → not in MVP; later may share notify events without forking mutations.
6. **Config file** → `~/.config/paseka/<slug>/telegram.yaml`.
7. **Allowlist** → `allow_from` + `chat_ids`; silent ignore for others.
8. **`/task` confirm** → preview + Confirm/Cancel.
9. **Invite accept UX** → local session + explicit “PTY on gate host” warning.
10. **`paseka run` dependency** → optional; NATS required; status shows hive alive?.
11. **Notify source** → durable bus subscribe + startup reconcile.
12. **Reconcile spam** → machine-local notify dedup state.
13. **Console links** → optional `console_base_url`; empty → ids only.
14. **Energy buttons** → `+1` / `+5` / `+12`.
15. **Proposal approve** → no final-merge from Telegram.
16. **Two-step** → Accept / Approve / Reject; Defer immediate.
17. **Optional cmds** → `/help`, `/invites`, status Refresh in MVP.

## Related docs

- [Telegram gateway](../guide/telegram-gateway.md) — setup, config, run, commands (canonical operator guide)
- [006-human-gateway-invites.md](./006-human-gateway-invites.md) — invite parking lot; accept starts local session
- [002-queen-console-mvp.md](./002-queen-console-mvp.md) — local Console; non-goal remote control plane
- [interactive sessions](../guide/interactive-sessions.md) — PTY / `bee chat`
- [task ledger](../reference/task-ledger.md) — task statuses, energy
- [colony layout](../guide/colony-layout.md) — slug, apiary secrets
- [glossary](../idea/glossary.md) — Human Gateway, Messenger Bee, Queen surfaces

## Verification (when implementing)

```bash
gofmt -w .
go test ./internal/gate/... ./internal/invites/... ./internal/tasks/...   # packages as introduced
go build -o paseka ./cmd/paseka
# Manual: BotFather token + allowlisted chat → /status, invite push, /task confirm, energy +1/+5/+12
# Manual: final-merge waiting_review offers Reject only; soft/mid Approve with two-step
```

## Implementation

Shipped under `cmd/paseka/gate.go` and `internal/gate/telegram/`:

| Slice | Commit | Coverage |
| ----- | ------ | -------- |
| Bootstrap | `4410f01` | `paseka gate telegram`, `telegram.yaml`, allowlist silent ignore, long-poll, `/status` + Refresh, `/help` |
| Invites | `4c620a0` | Invite push + reconcile dedup, honey line, `/invites`, Accept/Reject two-step, Defer immediate, energy `+1`/`+5`/`+12`, local session + PTY-on-host warning |
| Energy / task / status notify | `e058e54` | `/energy` show/add, `/task` preview+confirm (`agentId: telegram`), blocked/failed/waiting_review push |
| Proposals | `ca01a24` | waiting_review buttons; Reject always; Approve only when not final-merge; Console/CLI hint otherwise |

Durable consumer: `paseka-gate-telegram-<slug>`. Notify dedup: `~/.config/paseka/<slug>/telegram-notify-state.json`.

**Not in MVP (as designed):** webhook runtime (rejected until V2), digests/quiet hours, `/traces`, federation `/ctx`, PTY-in-chat.
