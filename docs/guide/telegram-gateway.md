# Telegram Human Gateway

Telegram is an **async triage and control gate** for the Beekeeper: push notifications plus a short set of mutations from the phone. It is not a remote IDE and does not relay PTY / `bee chat` into chat.

Mutations reuse the same packages as CLI and Queen Console (`agentId: telegram`). Accepting an invite starts an interactive session **on the machine running the gate**.

Design record: [specs/010-telegram-human-gateway.md](../specs/010-telegram-human-gateway.md). Vocabulary: [glossary](../idea/glossary.md) (Human Gateway).

---

## Prerequisites

- Colony already initialized (`paseka init`) with a resolved slug
- NATS URL in `~/.config/paseka/<slug>/config.yaml` (gate will not start without it)
- A Telegram bot token from [@BotFather](https://t.me/BotFather)
- Your Telegram **user id** and the **chat id** where you want pushes (private chat with the bot, or a group/supergroup)

`paseka run` is optional for the gate itself. AFK progress after `/task` still needs a live reactor; `/status` shows whether the reactor is alive.

---

## 1. Create the bot and find ids

1. Message BotFather → `/newbot` → copy the token.
2. Start a chat with your bot (or add it to a group and send a message).
3. Resolve ids (pick one approach):

```bash
# After messaging the bot, inspect getUpdates (replace <TOKEN>)
curl -s "https://api.telegram.org/bot<TOKEN>/getUpdates" | jq .
# message.from.id  → allow_from
# message.chat.id  → chat_ids
```

Or use a helper bot such as `@userinfobot` for your user id. Group/supergroup chat ids are typically negative (e.g. `-100…`).

---

## 2. Machine-local config

Create **`~/.config/paseka/<slug>/telegram.yaml`**. It is **not** created by `paseka init` and must never live under committed `.paseka/`.

```yaml
enabled: true
bot_token: "123456:ABC…"          # or omit and set PASEKA_TELEGRAM_BOT_TOKEN
mode: longpoll                    # webhook is not implemented yet
allow_from:
  - 123456789                     # Telegram user id(s) allowed to run commands
chat_ids:
  - 123456789                     # push destinations (private chat id often equals user id)
  # - -1001234567890              # and/or a group
notify:
  invites: true
  waiting_review: true
  blocked: true
  failed: true
commands:
  task_autorun: true              # Confirm on /task also publishes task.ready
  default_bee: builder
  default_intent: general         # task intent for default_bee (see bee intents)
  default_review: none
  custom:
    feature:
      description: "Intake idea/bug via Scout"
      emit: signal
      type: SIGNAL
      kind: feature.requested
      static:
        priority: medium          # optional extra payload fields
console_base_url: ""              # optional; e.g. Tailscale URL to Queen Console
```

| Field | Required | Notes |
| ----- | -------- | ----- |
| `enabled` | yes | Must be `true` or the gate exits |
| `bot_token` | yes\* | \*Or env `PASEKA_TELEGRAM_BOT_TOKEN` (env wins) |
| `allow_from` | yes | Non-empty; inbound commands from others are **silently ignored** |
| `chat_ids` | yes | Non-empty; pushes go here; commands from chats outside this list are also ignored |
| `mode` | no | Default `longpoll`. `webhook` is rejected at runtime until V2 |
| `commands.*` | no | Defaults: bee `builder`, intent `general`, review `none`, autorun `true` |
| `commands.custom.<name>` | no | Custom slash commands that publish bus `SIGNAL` events (`emit: signal` only). See below. |
| `console_base_url` | no | When set, cards may include a Console deep-link |

### Custom `emit: signal` commands

Use `commands.custom` for colony choreography entry points (e.g. Scout intake on `feature.requested`). Each command:

- Maps to `/name <text>` in Telegram (preview + Confirm, like `/task`)
- Publishes one `SIGNAL` on a **new** `traceId` with `agentId: telegram`
- Does **not** run bees itself — AFK dispatch needs `paseka run` (see [bee routing](../reference/bee-routing.md) §4)

| Field | Required | Notes |
| ----- | -------- | ----- |
| `description` | yes | Shown in `/help` and preview |
| `emit` | yes | Must be `signal` |
| `type` | yes | Must be `SIGNAL` |
| `kind` | yes | `payload.kind` (e.g. `feature.requested`) |
| `static` | no | Extra string fields merged into payload |

Reserved names: `start`, `status`, `help`, `invites`, `energy`, `task`.

Runtime notify dedup state: `~/.config/paseka/<slug>/telegram-notify-state.json` (created automatically).

---

## 3. Run the gate

From inside the colony repo (or pass `-C`):

```bash
# Typical: reactor + gate as separate processes
paseka run                  # AFK hive consumer (optional for notify/commands alone)
paseka gate telegram        # long-poll + notify + commands
# optional: paseka console  # heavy HITL (diffs, topology, browser PTY)
```

```bash
paseka gate telegram -C /path/to/repo
```

Stop with Ctrl-C (SIGINT / SIGTERM). Telegram network failures do not take down `paseka run` because the gate is a separate process.

**One bot token = one colony slug** for MVP. Wrong-hive mutations are worse than managing a second BotFather token.

---

## 4. Commands and buttons

Message the bot from an allowlisted user **and** chat:

| Command | Behavior |
| ------- | -------- |
| `/status` | Reactor alive?, slug, subject prefix, live bees, pending invites; **Refresh** button |
| `/energy <traceId>` | Honey remaining/budget |
| `/energy add <traceId> <n>` | Top up honey (`SIGNAL/energy.add`) |
| `/task <text>` | Preview card → Confirm/Cancel → `task.plan` (+ `task.ready` if autorun) |
| `/feature <text>` (example custom) | Preview → Confirm → `SIGNAL/feature.requested` on new trace (when configured) |
| `/invites` | Pending invites with Accept / Reject / Defer |
| `/help` | Command list |

**Invite Accept / Reject** and **proposal Approve / Reject** use a two-step Confirm. **Defer** is immediate.

On blocked / insufficient-honey replies: **`+1` / `+5` / `+12`** energy buttons (no confirm).

**Proposal policy:** Reject always allowed. Approve allowed only for soft/mid review gates — **not** final-merge (`review: final` / `_review`). Final-merge cards offer Reject + “approve in Console/CLI only”.

On invite **Accept**, the gate starts a detached local session and replies that the PTY is on the gate host (not in Telegram). Attach with `paseka session attach` or Queen Console — see [interactive sessions](interactive-sessions.md).

---

## 5. What gets pushed

Allowlisted `chat_ids` receive short cards (with buttons where applicable) when:

| Condition | Typical buttons |
| --------- | --------------- |
| Pending `session.invite` | Accept / Reject / Defer |
| Task → `waiting_review` | Reject; Approve if not final-merge |
| Task → `blocked` (incl. honey exhausted) | Energy `+1` / `+5` / `+12` when energy-blocked |
| Task → `failed` | Summary only |

On startup the gate **reconciles** pending invites and `waiting_review` / `blocked` / `failed` tasks, using machine-local dedup so restart does not spam.

---

## 6. Troubleshooting

| Symptom | Check |
| ------- | ----- |
| `missing …/telegram.yaml` | Create the file under the colony’s slug home |
| `disabled in telegram.yaml` | Set `enabled: true` |
| `bot_token is required` | Set `bot_token` or `PASEKA_TELEGRAM_BOT_TOKEN` |
| `nats url not configured` | Set `nats.url` in home `config.yaml` |
| `webhook mode is not implemented` | Use `mode: longpoll` |
| Bot ignores messages | User must be in `allow_from` **and** chat in `chat_ids` (silent ignore otherwise) |
| `/task` confirms but AFK never runs | Start `paseka run` on the same colony |
| Invite accept “PTY on this machine” | Expected — attach locally; optional `console_base_url` for a Console link |

---

## Related documentation

| Doc | Topic |
| --- | ----- |
| [CLI](cli.md) | `paseka gate telegram` in the command tree |
| [Colony layout](colony-layout.md) | Slug and machine-local secrets |
| [Interactive sessions](interactive-sessions.md) | Local PTY after invite accept |
| [Task ledger](../reference/task-ledger.md) | Energy, `waiting_review`, review policies |
| [specs/010-telegram-human-gateway.md](../specs/010-telegram-human-gateway.md) | Full MVP design |
| [specs/006-human-gateway-invites.md](../specs/006-human-gateway-invites.md) | Invite lifecycle |
