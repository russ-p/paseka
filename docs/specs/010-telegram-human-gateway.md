# Spec 010: Telegram Human Gateway

## Status

**Draft.** Design record for a Telegram surface beside CLI (Queen Shell) and Queen Console. Not implemented.

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
- Keep the feature inside the **single `paseka` binary** as an optional subcommand / gate process.
- Explicitly park full interactive agent chat (`bee chat` / PTY / xterm) out of Telegram.

## Non-Goals

- Do not stream or emulate PTY / SessionAdapter over Telegram messages.
- Do not build a multi-user or remote multi-host control plane (same non-goal as [002](./002-queen-console-mvp.md)).
- Do not replace Queen Console for diffs, topology, timeline browsing, or browser attach.
- Do not require Telegram for local CLI/Console workflows.
- Do not implement chat-native coding-agent channels (Claude Code Channels / OpenClaw-style agent respond loops) inside Paseka — that belongs to agent plugins / a separate bot if desired.
- Do not ship federation context-switching (`/use <slug>`) in MVP.

## Current System Context

| Surface | Role today |
| ------- | ---------- |
| CLI | Full operator surface: `run`, `task`, `energy`, `invite`, `proposal`, `bee run|chat`, … |
| Queen Console | Local HTTP (`127.0.0.1:8787`) SPA + API; same mutations; PTY over WebSocket |
| Human Gateway invites | [006](./006-human-gateway-invites.md) — `session.invite` parking lot; accept starts **local** session |
| Energy | Per-trace honey reserve; `paseka energy show|add`; exhausted → task `blocked` |
| Binary | Single `paseka` (~22MB today); Console static embed ~1.8MB; adapters compiled in |

Console and CLI already publish/consume the contracts Telegram needs. The gap is a **remote, push-capable, turn-based** UI with allowlisted chats — not new ledger semantics.

## Decisions

### 1. Telegram is a Human Gateway surface, not a Messenger Bee role

- **Inbound gate** (commands, callback buttons) → same handlers / services Console and CLI use.
- **Outbound notify** (push on selected bus events) → platform gate subscription, optionally later exposable as colony “Messenger” policy.
- Do not require a colony bee YAML named `messenger` to make the gate work; colony may still use `post_exec` hooks independently.

### 2. Async triage only — no PTY in Telegram

| In Telegram | Not in Telegram |
| ----------- | --------------- |
| Status, energy, task inject | `bee chat` byte-stream |
| Invite accept/reject/defer | Ghostty / Console xterm attach |
| Proposal approve/reject (summary + buttons) | Full merge-diff review |
| Deep-link to Console when useful | Remote multi-beekeeper ACL |

On invite **accept**, start the interactive session **locally** (same as Console accept): detached session on the machine running the gate/reactor. Telegram replies with “session started” + optional Console deep-link / attach hint — not a chat transcript of the agent TUI.

### 3. One bot token = one colony (MVP)

| Model | MVP | Later (optional) |
| ----- | --- | ---------------- |
| Binding | Bot token + allowlisted `chat_id`(s) under `~/.config/paseka/<slug>/` | Federation: one bot, `/ctx <slug>` or forum topics |
| Isolation | Subject prefix + apiary home of that slug | Explicit confirmation on destructive cross-colony actions |
| Secrets | Machine-local only (never committed in `.paseka/`) | Shared allowlist still per machine |

Rationale: solo Beekeepers typically run few colonies; wrong-hive mutations are worse than managing a second BotFather token.

### 4. Stay in one binary

- Add `paseka gate telegram` (name bikeshed-ok; alternatives: `paseka telegram`, `paseka run --gate=telegram`).
- Depend on a small Go Telegram Bot API client (expect roughly **+1–3MB** binary growth — negligible vs Console embed).
- Gate process long-polls (default) or serves webhook; does not embed a second SPA.
- Optional later split binary only if attack-surface isolation is required; not MVP.

### 5. Same mutation path as Console

Mutations must not invent parallel state:

| Action | Mechanism |
| ------ | --------- |
| Create + optionally start task | `task.plan` → optional `task.ready` (agentId `telegram`) |
| Top up honey | `SIGNAL/energy.add` |
| Invite decision | `beekeeper.ready` with `accept` \| `reject` \| `defer` ([006](./006-human-gateway-invites.md)) |
| Proposal decision | Same flow as `paseka proposal approve|reject` / Console Reviews |
| Status | Read runtime registry + ledger projections (same sources as Console dashboard) |

### 6. Auth and safety

- Require Bot API token in machine-local config.
- Allowlist Telegram user and/or chat ids; reject everyone else silently or with a fixed denial.
- Destructive actions (reject invite, reject proposal, optional future kill) use **inline confirmation** or two-step callbacks.
- Do not expose arbitrary shell / `paseka` argv from chat text.

## MVP surface

### Outbound notifications (push)

Notify allowlisted chats when:

| Event / condition | Why |
| ----------------- | --- |
| `SIGNAL/session.invite` (`pending`) | Core Human Gateway parking lot |
| Task → `waiting_review` | Proposal HITL |
| Task → `blocked` (incl. honey exhausted) | Energy triage |
| Task → `failed` | Failure triage |

Message shape: short summary (trace, bee/task id, one-line task text) + inline buttons where applicable. Truncate long text; point to Console for detail.

### Inbound commands

| Command / UI | Behavior |
| ------------ | -------- |
| `/status` | Hive runtime alive?, slug, subject prefix, live bee count, pending invite count, optional open blocked/failed counts |
| `/energy` / `/energy <traceId>` | Show remaining/budget; buttons or `/energy add <traceId> <n>` |
| `/task <text>` | `task.plan` + autorun `task.ready` (configurable default bee / review policy) |
| Invite buttons | Accept / Reject / Defer → same as CLI/Console |
| Proposal buttons | Approve / Reject on waiting_review notifications |

Optional MVP niceties (include if cheap): `/invites`, `/help`, Refresh callback on status message.

### Explicitly deferred from MVP

- Daily digest / quiet hours
- `/kill` / `system.kill` (needs its own protocol design — see backlog energy follow-ups)
- Diff rendering or PR links beyond a one-line summary + Console URL
- Forum topics / multi-colony context switch
- Streaming tool-call noise from AFK runs
- Agent-plugin chat bridge (Cursor/Claude channel bots)

## Configuration (machine-local)

Suggested layout (exact keys TBD at implement time):

```text
~/.config/paseka/<slug>/
  telegram.yaml          # or gate.telegram section in existing local config
```

Illustrative fields:

```yaml
enabled: true
bot_token: "…"           # or env PASEKA_TELEGRAM_BOT_TOKEN
allow_from:
  - 123456789            # Telegram user id
chat_ids:
  - -1001234567890       # destination chats for push
notify:
  invites: true
  waiting_review: true
  blocked: true
  failed: true
commands:
  task_autorun: true
  default_review: none   # align with task create defaults
console_base_url: "http://127.0.0.1:8787"  # deep-links; may be unreachable off-LAN
```

Colony shareable config (`.paseka/`) should not hold bot tokens. Optional later: colony-level notify policy flags only (no secrets).

## Process model

Recommended MVP:

```text
paseka run                  # reactor (existing)
paseka gate telegram        # long-poll + notify + commands (new)
# optional: paseka console  # still local UI for heavy HITL
```

Alternative (implementer choice, document in CLI guide when shipping): gate co-started with reactor via flag. Prefer a **separate process** so Telegram network failures do not take down the hive consumer.

Gate responsibilities:

1. Resolve colony context (`-C` / cwd) → slug + NATS prefix + local secrets.
2. Subscribe to (or poll projections of) notify-worthy events.
3. Dispatch inbound commands to shared internal services (tasks, energy, invites, review) — prefer calling packages directly over HTTP loopback to Console.
4. Never block on PTY start beyond the same out-of-band accept path Console uses.

## Fit matrix (product)

| Capability | Telegram fit | Notes |
| ---------- | ------------ | ----- |
| Runtime / live status | High | Single message |
| Energy show/add | High | Buttons on blocked alerts |
| Task inject | High | `/task` text |
| Invite HITL | Highest | Push + two buttons |
| Proposal approve/reject | High | Summary only |
| Merge-diff / topology | Low | Console |
| Interactive `bee chat` | Near zero | Local PTY / agent channels elsewhere |

## Phasing

| Phase | Scope |
| ----- | ----- |
| **MVP** | Config + `paseka gate telegram`; notify invites/blocked/failed/waiting_review; `/status`, energy, `/task`; invite + proposal buttons; allowlist |
| **V2** | Digests, quiet hours, richer `/traces`, Console deep-link polish, optional webhook mode hardening |
| **Later** | Federation `/ctx`, forum topics, `system.kill`, optional agent-channel companion bot (out of this binary’s PTY model) |

## Open questions

1. Exact CLI name: `gate telegram` vs `telegram` vs reactor flag?
2. Should invite accept from Telegram require honey availability messaging before accept (Console/CLI already charge 1 honey)?
3. Default `/task` bee role and review policy — colony default vs gate config?
4. Long-poll vs webhook default for solo machines behind NAT?
5. Whether outbound notify should also be expressible as a thin colony Messenger Bee later without duplicating the gate.

## Related docs

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
# Manual: BotFather token + allowlisted chat → /status, invite push, /task, energy add
```
