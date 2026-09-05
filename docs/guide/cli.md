# Queen Shell CLI reference

`paseka` is the Queen Shell — a single binary for colony setup, one-shot bee runs, interactive sessions, NATS runtime, and housekeeping.

Build from the repo root:

```bash
go build -o paseka ./cmd/paseka
```

---

## Conventions

### Resolving the colony

Most commands resolve the git repository and colony config from the current working directory. Use `--path` / `-C` to start resolution from another directory inside the repo:

```bash
paseka bee run scout --body "survey" --path /path/to/repo
```

Resolution requires:

- A git repository with `.paseka/colony.yaml` (run `paseka init` first)
- Machine-local config at `~/.config/paseka/<slug>/` (created by `paseka init`)

### Identifiers

| Flag / field | Name in docs | Description |
| ------------ | ------------ | ----------- |
| `--trace` | `traceId` | Flight trail — groups runs, worktrees, and bus events for one feature chain |
| `--body` (bee run / chat) | task body | Free-text nectar passed into the prompt template (`{{.Task}}`) |
| `--task` (task / proposal) | `taskId` | Structured subtask id in the task ledger (e.g. `task-1`) |
| agent id | `agentId` | Unique id per adapter invocation (auto-generated for `bee run`) |

### NATS dependency

| Command | NATS required? |
| ------- | -------------- |
| `paseka init`, `bee run --no-bus`, `bee chat`, `session`, `status`, `colony topology`, `nuc`, `console`, `purge` (filesystem only), `export`, `inspect usage` | No |
| `paseka purge --bus` | Yes — requires `nats.url` and `--trace` |
| `paseka bee run` (default) | Optional — publishes domain events when `nats.url` is configured |
| `paseka run`, `doctor`, `replay`, `signal`, `cue`, `proposal`, `energy`, `task create`, `task start`, `task retry`, `gate telegram` | Yes |
| `paseka task list`, `paseka task show`, `export` | Optional — prefers JetStream KV, falls back to filesystem projection |

Default NATS URL after `paseka init`: `nats://127.0.0.1:4222` (see `docker-compose.yml`).

Non-empty **`PASEKA_NATS_URL`** overrides `nats.url` in home `config.yaml` (useful for containers and shared NATS hosts). Homelab / server container setup: [Homelab deployment](homelab-deployment.md).

---

## Command tree

```
paseka
├── init
├── bee
│   ├── run <role>
│   └── chat <role> [prompt]
├── session
│   ├── list
│   ├── attach <sessionId>
│   ├── stop <sessionId>
│   └── run <role>          (hidden — Ghostty launcher)
├── run
├── task
│   ├── create
│   ├── list
│   ├── show
│   └── start
├── status
├── doctor
├── event
│   ├── emit
│   ├── validate
│   ├── pending
│   └── flush
├── inspect
│   └── usage
├── replay <traceId>
├── cue
│   ├── list
│   └── run <id> <text>
├── signal
├── proposal
│   ├── approve
│   └── reject
├── energy
│   ├── show
│   └── add
├── colony
│   └── topology
├── nuc
│   ├── export
│   └── import <file|url|->
├── console
├── gate
│   └── telegram
├── export
└── purge
```

---

## `paseka init`

Bootstrap `.paseka/` in the current git repository and machine-local home config under `~/.config/paseka/<slug>/`.

| Flag | Short | Default | Description |
| ---- | ----- | ------- | ----------- |
| `--path` | `-C` | cwd | Directory inside the git repository |
| `--adapter` | | `cursor` | Scaffold bees and home config for this adapter (`cursor`, `pi`, or `opencode`; unknown values use `cursor`) |

**Creates (if missing):**

- `.paseka/colony.yaml`, `bees/`, `prompts/`, scout and builder bee definitions (using the selected adapter)
- `~/.config/paseka/<slug>/config.yaml` (includes `nats.url`; non-empty `PASEKA_NATS_URL` overrides it)
- `~/.config/paseka/<slug>/adapters/cursor.yaml` (default adapter)
- `~/.config/paseka/<slug>/adapters/pi.yaml` (when `--adapter pi`)
- `~/.config/paseka/<slug>/adapters/claude.yaml` (optional Claude adapter config)
- `~/.config/paseka/<slug>/adapters/opencode.yaml` (OpenCode binary; always written)

Idempotent — existing files are skipped.

```bash
paseka init
paseka init --adapter pi
paseka init --adapter opencode
paseka init -C /path/to/repo
```

---

## `paseka bee`

Run colony bees via adapters (Cursor Agent CLI by default; Pi CLI when `adapter: pi`; OpenCode CLI when `adapter: opencode`; shell/python when `adapter: script`).

### `paseka bee run <role>`

Dispatch one bee — a single non-interactive adapter invocation (AFK run).

**Arguments:** `role` — bee name matching `.paseka/bees/<role>.yaml` (e.g. `scout`, `builder`).

| Flag | Short | Required | Description |
| ---- | ----- | -------- | ----------- |
| `--body` | `-b` | * | Task body rendered into the prompt template |
| `--prompt` | | * | Inline prompt override (skips template file) |
| `--trace` | | | Flight trail id; generated if omitted |
| `--intent` | | | Task intent for the bee role (vocabulary from bee `intents` or `<role>-intent-*` partials; default from bee config or discovered vocabulary) |
| `--path` | `-C` | | Colony resolution start directory |
| `--no-bus` | | | Skip NATS publish (file-only run) |

\* Provide `--body` or `--prompt` for LLM bees. Script bees (`adapter: script`) do not require a prompt; they run the bee's `command:` and receive runtime context via `PASEKA_*` env vars (see [architecture overview](../architecture/overview.md) § Script adapter).

**Behavior:**

- Renders prompt from `.paseka/prompts/` (see [prompt templates](prompt-templates.md))
- Writes run artifacts to `.paseka/runs/<traceId>/<agentId>/`
- Builder bees with `worktree: true` run in `.paseka/worktrees/<traceId>/`
- Publishes domain events (`SIGNAL`, `INSIGHT`, `MUTATION`, `VERIFICATION`) to NATS when configured
- Prints status, trace, agent id, workspace, run dir, output, and diff summary

```bash
paseka bee run scout --body "Plan auth for the API"
paseka bee run builder --body "Implement login" --trace trace-abc123 --intent feature
paseka bee run scout --prompt "Quick spike: {{.Task}}" --body "rate limiting"
paseka bee run builder --body "hotfix" --no-bus
```

### `paseka bee chat <role> [prompt]`

Start an interactive human-in-the-loop session in a long-lived agent process.

**Arguments:**

- `role` — bee name
- `prompt` (optional) — initial prompt text; equivalent to `--prompt`

| Flag | Short | Required | Description |
| ---- | ----- | -------- | ----------- |
| `--body` | `-b` | * | Task body for the prompt template |
| `--prompt` | | * | Inline prompt override |
| `--trace` | | | Flight trail id; generated if omitted |
| `--intent` | | | Task intent for the bee role (vocabulary from bee `intents` or `<role>-intent-*` partials) |
| `--path` | `-C` | | Colony resolution start directory |
| `--terminal` | | | `default` or `ghostty` (overrides `~/.config/paseka/<slug>/terminal.yaml`) |

\* Provide a positional prompt, `--body`, or `--prompt`.

See [interactive sessions](interactive-sessions.md) for session architecture and Ghostty attach.

```bash
paseka bee chat scout "Discuss the auth design"
paseka bee chat builder --body "Walk through the login flow"
paseka bee chat scout "review PR" --terminal ghostty
```

---

## `paseka session`

Manage interactive agent sessions registered in `~/.config/paseka/<slug>/state.json`.

### `paseka session list`

List active sessions for the colony.

| Flag | Short | Description |
| ---- | ----- | ----------- |
| `--path` | `-C` | Colony resolution start directory |

### `paseka session attach <sessionId>`

Attach to a session running in the **current** `paseka` process (in-process PTY relay). No flags.

### `paseka session stop <sessionId>`

Stop a session by id. Tries the in-process manager first, then signals a remote PID from `state.json`.

| Flag | Short | Description |
| ---- | ----- | ----------- |
| `--path` | `-C` | Colony resolution start directory (for remote stop) |

### `paseka session run <role>` (hidden)

Internal entry point used when `bee chat --terminal ghostty` launches a new terminal window. Same flags as `bee chat` (`--path`, `--body`, `--trace`, `--prompt`).

---

## `paseka run`

Start the long-running **Hive Runtime** — a NATS reactor that subscribes to colony events, updates the task ledger, and dispatches ready tasks to bees.

| Flag | Short | Description |
| ---- | ----- | ----------- |
| `--path` | `-C` | Colony resolution start directory |

**Requires:** NATS with JetStream (`nats.url` in home config, or `PASEKA_NATS_URL`).

Runs until interrupted (`Ctrl+C`). On `SIGNAL` / `task.ready` events, dispatches the configured bee and publishes resulting domain events. See [task ledger](../reference/task-ledger.md).

```bash
# Terminal 1
docker compose up -d
paseka run

# Terminal 2 — inject a ready task
paseka signal --type SIGNAL --trace trace-1 \
  --payload '{"kind":"task.ready","taskId":"task-1","title":"Add endpoint","bee":"builder"}'
```

---

## `paseka task`

Inspect and enqueue tasks for a trace. Task execution still requires the long-running `paseka run` reactor.

### `paseka task create`

Create a new task by publishing `task.plan`. Generates `traceId` and `taskId` when omitted.

| Flag | Short | Required | Description |
| ---- | ----- | -------- | ----------- |
| `--title` | | * | Task title |
| `--body` | | * | Inline task body |
| `--stdin` | | * | Read task body from stdin |
| `--file` | | * | Read task body from file |
| `--trace` | | | Flight trail id (generated when omitted) |
| `--task` | | | Task id (generated when omitted) |
| `--bee` | | | Bee role (default: `defaults.default_bee` from `colony.yaml`, fallback `builder`) |
| `--intent` | | | Task intent for the bee role (vocabulary from bee `intents` or `<role>-intent-*` partials) |
| `--depends-on` | | | Task dependencies (repeatable or comma-separated) |
| `--review` | | | Review policy: `none`, `required`, `final` |
| `--autorun` | | | Publish `task.ready` immediately after `task.plan` |
| `--path` | `-C` | | Colony resolution start directory |

\* Provide `--title` and/or one body source (`--body`, `--stdin`, or `--file`).

```bash
paseka task create --title "Add health check" --body "Implement /healthz endpoint" --bee builder --intent feature
cat task.md | paseka task create --title "Implement auth" --stdin --autorun
paseka task create --file task.md --bee builder --autorun
```

On success prints `trace`, `task`, suggested `paseka task start` / `paseka task show` commands, and whether `task.ready` was published.

### `paseka task list`

List tasks for one flight trail.

| Flag | Short | Required | Description |
| ---- | ----- | -------- | ----------- |
| `--trace` | | yes | Flight trail id |
| `--path` | `-C` | | Colony resolution start directory |

Reads JetStream KV when NATS is configured; otherwise falls back to `.paseka/runs/<traceId>/tasks/*/task.md`.

```bash
paseka task list --trace trace-1
```

### `paseka task show`

Show one task, its markdown description, and linked agent runs.

| Flag | Short | Required | Description |
| ---- | ----- | -------- | ----------- |
| `--trace` | | yes | Flight trail id |
| `--task` | | yes | Task id |
| `--path` | `-C` | | Colony resolution start directory |

```bash
paseka task show --trace trace-1 --task task-1
```

### `paseka task start`

Publish `task.ready` for eligible planned tasks. Requires NATS.

| Flag | Short | Required | Description |
| ---- | ----- | -------- | ----------- |
| `--trace` | | yes | Flight trail id |
| `--task` | | | Task id; when omitted, starts all eligible planned tasks |
| `--path` | `-C` | | Colony resolution start directory |

```bash
# Terminal 1
paseka run

# Terminal 2 — enqueue one task or all eligible tasks
paseka task start --trace trace-1 --task task-1
paseka task start --trace trace-1
```

### `paseka task retry`

Re-publish `task.ready` for a `failed` or stuck `running` task using the same bee, intent, and body from the task ledger. Requires NATS.

| Flag | Short | Required | Description |
| ---- | ----- | -------- | ----------- |
| `--trace` | | yes | Flight trail id |
| `--task` | | yes | Task id |
| `--path` | `-C` | | Colony resolution start directory |

```bash
# Terminal 1
paseka run

# Terminal 2 — retry after a failed adapter run
paseka task retry --trace trace-1 --task task-1
```

---

## `paseka status`

Read-only colony snapshot: hive runtime, live bees (AFK + interactive), task counts, honey for recent Flight Trails, attention items, and a short recent-trace list. Observe-only — does not start the runtime, dispatch work, or mutate colony state.

| Flag | Short | Description |
| ---- | ----- | ----------- |
| `--path` | `-C` | Colony resolution start directory |
| `--json` | | Emit `schemaVersion` 1 JSON on stdout (machine contract for interface bees) |
| `--check` | | Exit non-zero when the hive **substrate** cannot choreograph (runtime not alive, or NATS configured but unreachable). Pending reviews, invites, failed tasks, and empty honey do **not** fail `--check`. |

Default exit code is **0** whenever the snapshot was produced (including when the reactor is stopped). Snapshot is printed before `--check` evaluates so probes can log JSON/text on failure.

**JSON blocks:** `runtime`, `nats` (light connectivity), `agents`, `activeWorktrees`, `taskCounts`, `energy` (recent-traces window), `attention`, `recentTraces`.

**Follow-up commands (index, not new verbs):**

| Snapshot signal | Command |
| --------------- | ------- |
| Runtime not alive | Beekeeper starts `paseka run` (status never starts it) |
| Live `session` | `paseka session list` / `attach` / `stop` |
| Live `afk` | `paseka task show`, `paseka kill` |
| `waitingReview` | `paseka task show`, `paseka proposal approve\|reject` |
| `pendingInvites` | `paseka invite list` / `accept` / `reject` |
| `failedTasks` | `paseka task retry` |
| `lowEnergyTraces` | `paseka energy add --trace` |
| `recentTraces` | `paseka task list --trace`, `paseka energy show --trace`, `paseka replay` |
| `nats.connected == false` | `paseka doctor` |

```bash
paseka status
paseka status --json
paseka status --check          # health probe for systemd/cron
```

---

## `paseka doctor`

Check NATS connectivity, JetStream resources, and colony bee wiring for the colony.

| Flag | Short | Description |
| ---- | ----- | ----------- |
| `--path` | `-C` | Colony resolution start directory |

**Reports:** connection, JetStream, event stream, task-ledger KV bucket, object store bucket; **code proposal wiring** (worktree ↔ kind mismatches as errors, bare `code.proposal` alias as warnings, missing subscribers / verification publishes as advisories).

Exits with an error if any check fails.

```bash
paseka doctor
```

---

## `paseka replay <traceId>`

List domain events for a flight trail replayed from JetStream.

**Arguments:** `traceId` — flight trail to replay.

| Flag | Short | Description |
| ---- | ----- | ----------- |
| `--path` | `-C` | Colony resolution start directory |

Prints event type, `payload.kind`, and agent id per event. Does not re-execute bees.

```bash
paseka replay trace-abc123
```

---

## `paseka cue`

Run colony **Forage Cue** shortcuts from `.paseka/cues/<id>.yaml`. Publishes `signal` or `task` ingress immediately (no confirm). Full authoring, Console, Telegram, honey, and **external timer/webhook** rules: [Forage Cues](cues.md). Cron and GitHub hooks invoke this command on the hive host; Paseka does not listen for HTTP or run a scheduler.

Requires NATS (same as `paseka signal` / `task create`).

### `paseka cue list`

| Flag | Short | Description |
| ---- | ----- | ----------- |
| `--path` | `-C` | Colony resolution start directory |

Lists cues sorted by id (id, optional standing trail, optional description).

### `paseka cue run`

| Flag | Short | Required | Description |
| ---- | ----- | -------- | ----------- |
| `<id>` | | yes | Cue id (filename without `.yaml`) |
| `<text>` | | yes | Operator text (`Text` / `Title` / `Body` in templates) |
| `--trace` | | | Flight trail id (new trail when omitted; standing cues use `standing.trace` and reject a mismatch) |
| `--set` | | | Template override `key=val` (repeatable; unused keys ignored) |
| `--path` | `-C` | | Colony resolution start directory |

```bash
paseka cue list
paseka cue run feature "OAuth callback returns 500 on refresh"
paseka cue run hotfix "Fix nil deref in token refresh"
paseka cue run feature "Follow-up" --trace trace-abc123
paseka cue run daily-triage "tick $(date -I)"
```

---

## `paseka signal`

Publish a domain event directly to the NATS bus (manual choreography / testing).

| Flag | Short | Required | Default | Description |
| ---- | ----- | -------- | ------- | ----------- |
| `--type` | | yes | | `SIGNAL`, `INSIGHT`, `MUTATION`, or `VERIFICATION` |
| `--payload` | | yes | | JSON object (include `kind` for task lifecycle events) |
| `--trace` | | | auto | Flight trail id |
| `--agent` | | | `cli` | Agent id on the event envelope |
| `--path` | `-C` | | | Colony resolution start directory |

```bash
# Plan tasks (scout output shape)
paseka signal --type INSIGHT --trace trace-1 \
  --payload '{"kind":"task.plan","tasks":[{"taskId":"task-1","title":"Add endpoint","bee":"builder"}]}'

# Mark task ready for the reactor
paseka signal --type SIGNAL --trace trace-1 \
  --payload '{"kind":"task.ready","taskId":"task-1","bee":"builder"}'
```

Event contracts: [task ledger](../reference/task-ledger.md), `.paseka/prompts/_partials/emit-howto.md` and type-scoped emit partials.

---

## `paseka event`

Validate and publish bus events with machine-readable feedback. This is the intended agent publish path.

By default, `emit` publishes to JetStream immediately. With **`--defer`**, the event is validated and appended to a per-run pending queue (`.paseka/runs/<traceId>/<agentId>/pending.ndjson`); runtime flushes FIFO on **successful** AFK or interactive session completion. Failed or cancelled runs leave pending on disk for inspect / manual flush. **`--defer` requires an existing run directory** for the event's `traceId` and `agentId` — missing run dir fails closed. End-of-session flush resolves home NATS settings from disk even when the session handle only stored the colony root (so a configured bus is not silently skipped).

Trail comb artifacts ([014](../specs/014-artifacts-protocol.md)) use runtime scan flush on success and do **not** depend on this buffer. See [015](../specs/015-deferred-event-emit.md) for the general deferred-emit contract.

**Live-only kinds** (CLI rejects `--defer`): `system.kill`, `energy.add`, `energy.consume`, `session.invite`, `beekeeper.ready`, `task.status`.

### `paseka event emit`

| Flag | Short | Required | Default | Description |
| ---- | ----- | -------- | ------- | ----------- |
| `--stdin` | | yes | | Read one JSON event object from stdin |
| `--defer` | | | `false` | Queue for flush on successful run/session completion instead of publishing now |
| `--agent` | | | `agent` | Default agent id when omitted from JSON |
| `--path` | `-C` | | | Colony resolution start directory |

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"trace-1","agentId":"agent-1","type":"VERIFICATION","payload":{"kind":"verification.success","summary":"All requirements met"}}
EOF
```

End-of-run handoff (deferred until successful exit):

```bash
paseka event emit --defer --stdin <<'EOF'
{"traceId":"trace-1","agentId":"agent-1","type":"INSIGHT","payload":{"kind":"task.plan","tasks":[{"taskId":"task-1","title":"Add endpoint","bee":"builder"}]}}
EOF
```

Live success response:

```json
{"ok":true,"traceId":"trace-1","type":"VERIFICATION","kind":"verification.success","subject":"paseka.events.VERIFICATION.verification.success","eventLogPath":"/path/to/.paseka/runs/trace-1/agent-1/events.ndjson"}
```

Deferred success response:

```json
{"ok":true,"deferred":true,"traceId":"trace-1","type":"INSIGHT","kind":"task.plan","pendingPath":"/path/to/.paseka/runs/trace-1/agent-1/pending.ndjson"}
```

Validation failure:

```json
{"ok":false,"error":"schema_validation_failed","details":[{"path":"payload.summary","message":"required"}]}
```

Defer denied (live-only kind):

```json
{"ok":false,"error":"defer_denied","details":[{"path":"payload.kind","message":"kind \"session.invite\" cannot be deferred; use live emit"}]}
```

### `paseka event validate`

Same stdin input as `emit`, but validates only and does not publish.

```bash
paseka event validate --stdin <<'EOF'
{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"task.plan","tasks":[{"taskId":"task-1","title":"Add endpoint"}]}}
EOF
```

### `paseka event pending`

Show deferred event count and kinds for a run (does not read raw `pending.ndjson` by hand).

| Flag | Short | Required | Description |
| ---- | ----- | -------- | ----------- |
| `--trace` | | yes | Flight trail id |
| `--agent` | | yes | Agent run id |
| `--path` | `-C` | | Colony resolution start directory |

```bash
paseka event pending --trace trace-1 --agent agent-1
```

### `paseka event flush`

Publish pending events FIFO, or discard without publishing. Uses the same flush path as runtime on successful completion. Mid-flush publish errors stop at the first failure; already-published events stay on the bus and remaining items stay queued for retry.

| Flag | Short | Required | Description |
| ---- | ----- | -------- | ----------- |
| `--trace` | | yes | Flight trail id |
| `--agent` | | yes | Agent run id |
| `--discard` | | | Clear pending queue without publishing |
| `--path` | `-C` | | Colony resolution start directory |

```bash
paseka event flush --trace trace-1 --agent agent-1
paseka event flush --trace trace-1 --agent agent-1 --discard
```

---

## `paseka inspect`

Filesystem projections for flight trails and runs (no NATS).

### `paseka inspect usage`

Show LLM token usage from `result.json` projections — trace aggregate by default, or one run with `--agent`. Mirrors Queen Console trace/run usage fields (`inputTokens`, `outputTokens`, cache read/write). Only Cursor AFK runs report usage today; others print `usage: (none)`. With `--agent`, if the run stored a native **`providerSessionId`**, it is printed as `providerSessionId:` (Cursor stream-json `session_id`, Cursor HITL `create-chat` UUID, or Pi session id).

| Flag | Short | Required | Description |
| ---- | ----- | -------- | ----------- |
| `--trace` | | yes | Flight trail id |
| `--agent` | | | Agent id (single run; omit for trace aggregate) |
| `--path` | `-C` | | Colony resolution start directory |

```bash
paseka inspect usage --trace trace-1
paseka inspect usage --trace trace-1 --agent agent-abc
```

---

## `paseka energy`

Manage per-trace honey reserve (`energyToken`). `paseka energy show` prints `budget:` (seed) and `remaining:`. After a post-seed top-up it also prints `added:` and remaining / allocated (`energyBudget + energyAdded`). Unseeded traces print remaining without a `/ 0` denominator. Low-energy in Queen Console is remaining vs seed — see [Task ledger](../reference/task-ledger.md) § Honey reserve.

### `paseka energy show`

| Flag | Short | Required | Description |
| ---- | ----- | -------- | ----------- |
| `--trace` | | yes | Flight trail id |
| `--path` | `-C` | | Colony resolution start directory |

### `paseka energy add`

Inject honey into a trace (`SIGNAL` / `energy.add`). When `paseka run` is active, the command publishes only and the reactor projects the ledger update (avoids double-counting). When the reactor is stopped, the CLI applies the event locally after publish so `energy show` reflects the top-up immediately.

Blocked tasks with exhausted reserve are unblocked when the reactor is running.

| Flag | Short | Required | Description |
| ---- | ----- | -------- | ----------- |
| `--trace` | | yes | Flight trail id |
| `--amount` | | yes | Positive number of tokens to add |
| `--path` | `-C` | | Colony resolution start directory |

```bash
paseka energy show --trace trace-1
paseka energy add --trace trace-1 --amount 5
```

---

## `paseka kill`

Hard-kill a trace (`SIGNAL` / `system.kill`). Cancels non-terminal tasks, stops new AFK dispatch, and cancels in-flight adapter processes for that `traceId`. Does not stop interactive sessions — use `paseka session stop` for those.

When `paseka run` is active, the command publishes and waits for the ledger projection. When the reactor is stopped, the CLI applies the event locally after publish.

| Flag | Short | Required | Description |
| ---- | ----- | -------- | ----------- |
| `--trace` | | yes | Flight trail id |
| `--reason` | | | Optional summary recorded on cancelled tasks |
| `--path` | `-C` | | Colony resolution start directory |

```bash
paseka kill --trace trace-1
paseka kill --trace trace-1 --reason "dispute avalanche"
```

---

## `paseka colony`

Colony configuration projections (filesystem only — no NATS).

### `paseka colony topology`

Print the colony EDA topology as a Mermaid `flowchart` derived from `.paseka/bees/*.yaml` and `.paseka/colony.yaml` `auto_invites`. Emits the same Mermaid string as Queen Console `GET /api/colony/topology` for the same colony root. Useful for docs, PRs, and offline inspection when `paseka run` is stopped.

| Flag | Short | Description |
| ---- | ----- | ----------- |
| `--path` | `-C` | Colony resolution start directory |
| `--out` | | Write Mermaid to file instead of stdout |

```bash
paseka colony topology
paseka colony topology --out docs/colony-topology.mmd
paseka colony topology -C /path/to/repo
```

See [specs/007-colony-eda-topology.md](../specs/007-colony-eda-topology.md) for graph semantics (subscribe/publish/invite edges, implicit `task.ready`, intent annotations).

---

## `paseka nuc`

Export and import portable **Nuc** packs (bee YAML + prompts) between Colonies. Full format and conflict policy: [nuc](nuc.md).

### `paseka nuc export`

| Flag | Short | Default | Description |
| ---- | ----- | ------- | ----------- |
| `--output` | `-o` | stdout | Write nuc file; prints path when set |
| `--bees` | | all roles | Comma-separated bee roles to export |
| `--name` | | colony slug | `metadata.name` in the nuc file |
| `--description` | | | Optional `metadata.description` |
| `--path` | `-C` | cwd | Colony resolution start directory |

```bash
paseka nuc export -o minimal.nuc.yaml
paseka nuc export --bees scout,builder -o scout-builder.nuc.yaml
```

### `paseka nuc import <file|url|->`

| Flag | Default | Description |
| ---- | ------- | ----------- |
| `--force` | off | Overwrite existing bee and prompt files |
| `--dry-run` | off | Show import plan without writing |
| `--verbose` / `-v` | off | List created, skipped, and overwritten paths |
| `--path` / `-C` | cwd | Colony resolution start directory |

Default conflict policy is **skip** (same as `paseka init` for existing files). Use `--force` to replace whole files. No field-level merge — use git to review imports.

```bash
paseka nuc import ./minimal.nuc.yaml
paseka nuc import https://example.com/nucs/dev.nuc.yaml --dry-run -v
paseka nuc import ./dev.nuc.yaml --force
```

---

## `paseka gate`

Human Gateway surfaces outside Queen Console. MVP: Telegram only.

### `paseka gate telegram`

Long-poll Telegram Bot API for one colony: push notifications, `/status` `/energy` `/task` `/invites` `/help`, optional `commands.custom` `emit: signal` slash commands, invite and proposal buttons. Separate process from `paseka run`. Requires machine-local `~/.config/paseka/<slug>/telegram.yaml` and NATS.

| Flag | Short | Default | Description |
| ---- | ----- | ------- | ----------- |
| `--path` | `-C` | cwd | Directory inside the git repository |

```bash
paseka gate telegram
paseka gate telegram -C /path/to/repo
```

Setup, allowlist, and command reference: [Telegram gateway](telegram-gateway.md). Design: [specs/010-telegram-human-gateway.md](../specs/010-telegram-human-gateway.md).

---

## `paseka console`

Start the local Queen Console web UI (embedded SPA + JSON API).

| Flag | Short | Required | Default | Description |
| ---- | ----- | -------- | ------- | ----------- |
| `--addr` | | | `127.0.0.1:8787` | Listen address (keep on localhost unless you understand the exposure risk) |
| `--path` | `-C` | | | Colony resolution start directory |

```bash
paseka console
paseka console --addr 127.0.0.1:8787
```

Open the printed URL in a browser. Queen Console does not enforce authentication yet.

The header **Host** plaque shows CPU and memory for the machine (or container PID namespace) where `paseka console` is running. Click it — or open the **System** tab — for identity, load averages, optional colony-root disk, and a top-process table. Live bees stays the adapter liveness signal; System Info does not infer “tests are running”.

The header **Git** plaque shows whether the colony clone is ahead of, behind, or in sync with `origin` (and a dirty flag). Click it — or open the **Git** tab — to Fetch remote-tracking refs, Push the default branch (explicit, never `--force`; git hooks skipped unless you opt in), or Pull fast-forward only as a backup when an inbound webhook sidecar did not update the clone. The same tab lists colony-managed worktrees and leftover merged branches. Review Approve does **not** push; publish is a separate Git-tab action. `GET /api/git` does not fetch on poll. `paseka status` and Telegram `/status` are unchanged. See [homelab deployment](homelab-deployment.md) for inbound sidecar vs Console Push.

Useful surfaces for HITL: Reviews tab lists `waiting_review` tasks; for final merge gates (`review: final` / `_review`) the detail panel shows merge-diff summary and **Open merge preview** (dedicated full-page file list + per-file diff bodies, unified or side-by-side). On the preview page, click diff lines to draft comments and **Request changes** to write `review-comments.md` to the trail comb, publish short `human.feedback`, and plan a rework task on the same worktree (merge gate stays open). Plain reject on the detail panel still publishes unstructured feedback only (no rework). Approve from Reviews after previewing. Full baseline: [specs/002-queen-console-mvp.md](../specs/002-queen-console-mvp.md).

---

## `paseka proposal`

Human-in-the-loop actions for code proposals (`MUTATION` with `kind: code.proposal.isolated`, `code.proposal.root`, or legacy alias `code.proposal`).

### `paseka proposal approve`

Approve a review-gated task and publish `VERIFICATION` / `task.completed`. Behavior branches on proposal workspace:

| Task context | Merge trace worktree? | Auto-commit? |
| ------------ | ---------------------- | ------------ |
| Isolated final gate (`review: final` / `_review`, `proposalWorkspace: isolated`) | Yes, when worktree exists | Yes (merge commit) |
| Root soft gate (`proposalWorkspace: root`, `review: required`) | **No** (R1 ack) | **No** |
| Isolated `review: required` mid-task | No (unless final gate) | No |

For **isolated** final merge gates, preview the accumulated worktree diff in Queen Console Reviews first (`paseka console`). Root approve advances the ledger only — beekeeper commits `.paseka/` / `docs/` changes on colony root manually.

| Flag | Short | Required | Default | Description |
| ---- | ----- | -------- | ------- | ----------- |
| `--trace` | | yes | | Flight trail id |
| `--task` | | yes | | Review task id (e.g. `task-1`, `_review`, or a `review: final` task) |
| `--summary` | | | `approved by human` | `VERIFICATION/task.completed` completion note (not `INSIGHT/trace.summary`) |
| `--merge-message` | | | | Merge commit **subject** for isolated final gate (optional HITL body); distinct from `INSIGHT/trace.summary`, which supplies the default merge **body** |
| `--path` | `-C` | | | Colony resolution start directory |

### `paseka proposal reject`

Reject a review-gated task — publishes human `INSIGHT` / `human.feedback`. Tasks with `review: required` return to `ready` for another AFK pass when `paseka run` is active. On a **final merge gate**, unstructured `--feedback` is abandon-style (no rework). `--comments-file` writes the comb packet and, on `review: final`, also plans a rework task for the last isolated-proposal Bee.

| Flag | Short | Required | Default | Description |
| ---- | ----- | -------- | ------- | ----------- |
| `--trace` | | yes | | Flight trail id |
| `--task` | | yes | | Task id |
| `--feedback` | | | `Please revise the proposal.` | Short summary for the bee (also used as overall summary with `--comments-file`) |
| `--comments-file` | | | | Path to a Markdown file copied into `.paseka/runs/<traceId>/artifacts/review-comments.md` before publishing feedback and `artifact.written`. On `review: final`, also plans rework (same as Console Request changes) |
| `--path` | `-C` | | | Colony resolution start directory |

```bash
# Isolated final gate — may print merge commit SHA
paseka proposal approve --trace trace-1 --task _review --summary "LGTM, merged"
# Root soft ack — no merge commit line
paseka proposal approve --trace trace-1 --task task-cfg-1 --summary "Config changes acknowledged"
paseka proposal reject --trace trace-1 --task task-1 --feedback "Use the existing auth middleware"
# Attach a pre-written review packet (line comments authored offline or exported elsewhere)
paseka proposal reject --trace trace-1 --task task-1 --comments-file ./my-review.md --feedback "See line notes in comb"
```

---

## `paseka export`

Write a self-contained report for one flight trail to the current working directory. The file includes trace overview, tasks, runs (oldest first), and the full event timeline (oldest first). Use `--format html` (default) for a styled HTML page, or `--format md` for Markdown suitable for agent chats. Use `--include` to add optional analysis slices (usage, durations, committed config snapshots, agent logs) without changing the renderer.

| Flag | Short | Required | Description |
| ---- | ----- | -------- | ----------- |
| `--trace` | | yes | Flight trail id |
| `--format` | | | Export renderer: `html` (default) or `md` |
| `--include` | | | Optional payload slices: `usage`, `durations`, `bees`, `colony`, `cues`, `artifacts`, `agent-logs` (repeatable or comma-separated) |
| `--path` | `-C` | | Colony resolution start directory |

**Output file:** `paseka-export-<slug>-<traceId>.html` or `.md` (extension matches `--format`) in the current working directory.

**HTML** styling matches Queen Console (dark theme, JetBrains Mono from CDN with system fallbacks). Run and event summaries are rendered as Markdown inside the page; raw events are expandable JSON.

**Markdown** keeps run and event summaries as-is (no HTML conversion) and puts each raw event in a fenced `json` code block.

**`--include` slices** (independent of `--format`):

| Token | Adds to export |
| ----- | -------------- |
| `usage` | Trace usage aggregate (when present) and per-run token lines from `result.json` |
| `durations` | Wall-clock duration per finished run (`FinishedAt - StartedAt`) |
| `bees` | Committed `.paseka/bees/<role>.yaml` for bees on this trail (no `*.local.yaml`) |
| `colony` | Raw `.paseka/colony.yaml` (or a short note when missing) |
| `cues` | All committed `.paseka/cues/*.yaml` in the colony |
| `artifacts` | Trail comb files under `.paseka/runs/<traceId>/artifacts/` (inline bodies; skips hidden/temp files; omits binary/oversize with a note) |
| `agent-logs` | Per-run **Agent log** (tool-call summary) resolved through the adapter by `providerSessionId`. Cursor reads `~/.cursor/projects/.../agent-transcripts/<uuid>/<uuid>.jsonl` in place. Missing id, unsupported adapter, missing transcript (`store not found`), parse errors, Pi stub (`not implemented`), or resolve errors omit the section with a reason; export still succeeds. |

Default export omits all config snapshots. Home config and machine-local overlays are never included.

Data is read from `.paseka/runs/` (same as Queen Console). Task and energy fields prefer JetStream KV when NATS is configured; otherwise filesystem projections are used.

```bash
paseka export --trace trace-abc123
paseka export --trace trace-abc123 --format md
paseka export --trace trace-abc123 --include usage,durations
paseka export --trace trace-abc123 --include bees --include colony --format md
paseka export --trace trace-abc123 --include artifacts --format md
paseka export --trace trace-abc123 --include agent-logs --format md
paseka export --trace trace-abc123 -C /path/to/repo
```

---

## `paseka purge`

Remove ephemeral colony artifacts. Without `--yes`, shows a plan and asks for confirmation.

| Flag | Short | Description |
| ---- | ----- | ----------- |
| `--runs` | | Remove `.paseka/runs/` trace directories |
| `--worktrees` | | Remove `.paseka/worktrees/` and associated git worktrees |
| `--cache` | | Remove `.paseka/cache/` |
| `--state` | | Reset `~/.config/paseka/<slug>/state.json` worktree registry |
| `--bus` | | Remove JetStream task-ledger KV, stream events, and object-store artifacts for `--trace` (requires NATS) |
| `--trace` | | Flight trail id (required with `--bus`) |
| `--reseed-energy` | | After `--bus` purge, seed honey to colony `defaults.energy_budget` (requires `--bus` and `--trace`) |
| `--all` | | Purge runs, worktrees, cache, and state (does **not** include `--bus`) |
| `--yes` | `-y` | Skip confirmation prompt |
| `--path` | `-C` | Colony resolution start directory |

At least one target flag (`--runs`, `--worktrees`, `--cache`, `--state`, `--all`, or `--bus`) is required.

**`--bus` behavior:** Deletes the task-ledger KV entry for the trace (tasks, energy reserve), domain events for that `traceId` from the JetStream event stream, and trace-scoped object-store artifacts (`<traceId>-*.diff`). Requires a configured `nats.url` and `--trace`. Not included in `--all` — pass `--bus` explicitly when resetting bus state.

**Stop the reactor first:** Stop `paseka run` before `purge --bus` so the reactor is not reading or writing task-ledger KV while keys and stream messages are deleted. Running bus purge against an active reactor can cause races and stale in-memory state.

**`--reseed-energy`:** Operator hygiene for retrying work on a fixed trace — after a successful `--bus` purge, seeds the trace honey reserve (`budget` and `remaining`) to the colony `defaults.energy_budget` (same source as reactor first seed). Works with the reactor stopped; verify with `paseka energy show --trace <id>`. Not an eval harness command.

```bash
paseka purge --runs --yes
paseka purge --all

# Reset bus state and reseed honey for a fixed trace (retry after failed run)
paseka purge --bus --trace my-trace --reseed-energy --yes

# Eval case reset (filesystem + bus for one fixed trace)
paseka purge --runs --worktrees --state --bus --trace eval-01-add-function --yes
```

---

## Typical workflows

### First-time setup

```bash
paseka init                    # Cursor adapter (default)
paseka init --adapter pi       # Pi adapter
paseka init --adapter opencode # OpenCode adapter
agent login                    # or export CURSOR_API_KEY (cursor init)
docker compose up -d           # local NATS + JetStream
paseka doctor
```

### One-shot scout → builder (no reactor)

```bash
TRACE=$(paseka bee run scout --body "Plan user settings page" | awk '/trace:/ {print $2}')
paseka bee run builder --trace "$TRACE" --body "Implement settings form"
```

### Choreographed run with reactor

```bash
paseka run &                   # background reactor
paseka task create --title "Add health check" --body "Implement /healthz" --bee builder --autorun
paseka task list --trace <printed-trace>
paseka replay <printed-trace>
```

### Manual plan + start

```bash
paseka run &                   # background reactor
paseka signal --type INSIGHT --trace trace-1 \
  --payload '{"kind":"task.plan","tasks":[{"taskId":"task-1","title":"Add health check","bee":"builder"}]}'
paseka task start --trace trace-1
paseka task list --trace trace-1
paseka replay trace-1
```

### Interactive design session

```bash
paseka bee chat scout "Let's design the notification system"
paseka session list
paseka session stop <sessionId>
```

### Feature ideation (soft path)

Manual Phase 0 flow for raw ideas → spec → task ledger. See [specs/005-feature-ideation-flow.md](../specs/005-feature-ideation-flow.md).

Ideation `SIGNAL` kinds (`feature.requested`, `feature.classified`, `spec.ready`) are **colony choreography contracts** — field shapes are defined in the ideation spec and bee emit partials, not enforced by `internal/protocol`. Platform HITL kinds (`session.invite`, `beekeeper.ready`) are documented in [specs/006-human-gateway-invites.md](../specs/006-human-gateway-invites.md).

```bash
TRACE=trace-$(date +%s)
paseka signal --type SIGNAL --trace "$TRACE" \
  --payload '{"kind":"feature.requested","title":"…","body":"…"}'
paseka bee run scout --intent intake --trace "$TRACE" \
  --body "Intake the feature.requested on this trail"
# With paseka run up and default auto_invites: invite list shows pending grilling invite
paseka invite accept <inviteId>
# Or manual path before accept:
# paseka bee chat drone --intent grilling --trace "$TRACE" "Grill: …"
# After grilling writes docs/specs/… and emits spec.ready:
paseka bee chat drone --intent breakdown --trace "$TRACE" "Break down docs/specs/…"
```

### Session invites (Human Gateway)

```bash
paseka invite list [--trace <id>] [--status pending|accepted|completed|incomplete]
paseka invite record --trace "$TRACE" --bee drone --intent grilling --body "Grill: …"
paseka invite accept <inviteId>          # detached session; costs 1 honey; attach with session attach
paseka invite accept <inviteId> --attach # attach terminal immediately
paseka invite reject <inviteId>
paseka invite reject <inviteId> --defer  # mark deferred
```

Accept consumes **1 honey** from the trace reserve. If exhausted, invite stays `pending` — run `paseka energy add --trace "$TRACE" --amount 1` (or more) and retry. Ad-hoc `bee chat` does not consume honey.

Invite statuses: `pending`, `accepted`, `completed`, `incomplete`, `cancelled`, `deferred`. Accepted invites become `completed` when a bus event matches the invite's persisted `done_when` contract (optional file-at-`ref` check); `incomplete` when the session ends without a valid artifact.

Seed offline with `invite record`, or configure **`auto_invites`** in `.paseka/colony.yaml` so `paseka run` auto-publishes when a matching bus event arrives. With empty `auto_invites`, no auto-invite runs. Accept publishes `beekeeper.ready` and starts an interactive session on the **same** `traceId`.

See [specs/006-human-gateway-invites.md](../specs/006-human-gateway-invites.md) and [bee routing](../reference/bee-routing.md) §7–8.

---

## Related documentation

| Doc | Topic |
| --- | ----- |
| [Brief](../idea/brief.md) | Product vision, EDA, NATS role |
| [architecture overview](../architecture/overview.md) | Colony layout, adapters, runs IPC |
| [prompt templates](prompt-templates.md) | Prompt template resolution |
| [task ledger](../reference/task-ledger.md) | Task lifecycle events on the bus |
| [interactive sessions](interactive-sessions.md) | `bee chat`, sessions, Ghostty |
| [Telegram gateway](telegram-gateway.md) | Setup and run `paseka gate telegram` |
| [specs/005-feature-ideation-flow.md](../specs/005-feature-ideation-flow.md) | Feature ideation soft path (intake → grill → breakdown) |
| [specs/006-human-gateway-invites.md](../specs/006-human-gateway-invites.md) | Session invites, auto_invites, done_when |
| [specs/010-telegram-human-gateway.md](../specs/010-telegram-human-gateway.md) | Telegram Human Gateway design |
