# Spec 018: Queen Shell colony status

## Status

**Implemented.** Read-only `paseka status` snapshot as a Queen Console alternative for Beekeepers and interface bees. Honey (energy) is in MVP. Non-zero process exit only with `--check`.

## Problem Statement

Beekeepers and interface bees (agents that drive Queen Shell and report hive state) cannot answer “is the hive runtime up, and what is live right now?” from the CLI. Queen Console already projects runtime, live bees, tasks, invites, and traces over HTTP. Telegram `/status` is a thin human line. Queen Shell splits the same questions across `session list`, `task list --trace`, `energy show --trace`, and `doctor` — none of which reports hive-runtime liveness, and several require identifiers the caller does not yet have.

Without a single observe-only snapshot, an interface bee must start `paseka console`, scrape HTTP, or stitch five commands. That is a poor substitute for the Dashboard plus Live bees header, especially over SSH or in a chat-driven operator loop.

## Solution

Add `paseka status`: a **read-only colony snapshot** printed as human text (default) or stable JSON (`--json`). The command is an **index**, not a second Console: identity, hive runtime, optional NATS ping, live bees (AFK + interactive), task counts, honey for recent Flight Trails, attention items (review, invites, failed tasks, exhausted honey), and a short recent-trace list. Stable ids in the payload let the caller invoke existing mutate/inspect commands (`task show`, `proposal approve`, `invite accept`, `energy add`, `kill`, `session stop`).

Default exit status is **0** whenever the snapshot was produced. `--check` is the only way to get a non-zero exit, and it means **hive substrate is not ready to choreograph** (runtime not alive, or NATS configured but down) — not “there is work waiting.”

Status does not start or stop the runtime, does not require Queen Console, and does not become an orchestrator: the hive reactor still choreographs; the interface bee only observes and, with Beekeeper intent, runs other CLI verbs.

## User Stories

1. As a Beekeeper, I want `paseka status`, so that I can see hive health without opening Queen Console.
2. As a Beekeeper on SSH, I want a compact text snapshot, so that a terminal is enough to know if `paseka run` is alive.
3. As an interface bee, I want `paseka status --json`, so that I can poll a stable machine contract instead of parsing text or HTTP.
4. As an interface bee, I want `schemaVersion` in JSON, so that I can reject unknown major shapes instead of guessing.
5. As an interface bee, I want colony `slug` and `colonyRoot` in every snapshot, so that I can label which hive I am talking about.
6. As an interface bee, I want `generatedAt`, so that I can detect stale polls and order samples.
7. As a Beekeeper, I want hive runtime status (`running` / `stopped` / `stale`), PID, and heartbeat, so that I can tell a live reactor from a dead `state.json` entry.
8. As an interface bee, I want `runtime.alive` as a boolean, so that I do not reimplement heartbeat rules.
9. As a Beekeeper when the reactor is stopped, I still want a successful snapshot, so that “down” is data, not a CLI failure.
10. As a Beekeeper, I want live bees (AFK adapter runs and interactive sessions) with bee role, kind, PID, and ids, so that I can see who is working now.
11. As an interface bee, I want `traceId`, `agentId`, and `sessionId` on live bees, so that my next CLI call is deterministic.
12. As a Beekeeper, I want AFK vs session counts, so that I can tell headless dispatch from HITL chat.
13. As a Beekeeper, I want colony-wide task counts by status, so that I can see running / waiting_review / failed without a trace id.
14. As a Beekeeper, I want pending Human Gateway invites in attention, so that HITL session requests are visible next to live bees.
15. As a Beekeeper, I want `waiting_review` tasks in attention with `traceId` / `taskId` / review policy, so that I can jump to `task show` and `proposal approve|reject`.
16. As a Beekeeper, I want failed tasks in attention, so that I can decide on `task retry` without scanning every trail.
17. As a Beekeeper, I want honey remaining/budget for recent Flight Trails in the snapshot, so that I can see reserves without `energy show` per trace first.
18. As a Beekeeper, I want exhausted honey (`remaining <= 0` on a seeded trail) in attention, so that a stalled dispatch is obvious.
19. As an interface bee, I want `energy.available` when the ledger cannot be read, so that I do not treat missing honey as zero.
20. As a Beekeeper when NATS is down, I still want filesystem-backed runtime, bees, and traces, so that local truth remains useful.
21. As a Beekeeper, I want a light NATS connected/configured flag, so that I know whether the bus can move work, without running full `doctor`.
22. As a Beekeeper, I want a short recent-trace list with ids, so that I have a handle for `task list --trace` and `replay`.
23. As an interface bee, I want bounded lists (live bees, traces, attention rows), so that a poll cannot dump the whole colony history.
24. As a Beekeeper, I want `--path` / `-C` like other Queen Shell commands, so that I can target a repo from another cwd.
25. As a Beekeeper, I want the command to resolve the colony the same way as `session list`, so that I do not need a running Console process.
26. As a Beekeeper, I want observe-only behavior, so that polling status cannot start the runtime, kill bees, or flush events.
27. As a script author, I want exit code 0 on a successful snapshot even if the reactor is stopped, so that `status` in a pipeline is not a false alarm.
28. As a script author, I want `--check` to exit non-zero only when the hive substrate cannot choreograph, so that systemd/cron/healthchecks have a dedicated probe.
29. As a script author using `--check`, I want waiting reviews, invites, and failed tasks **not** to fail the process, so that pending HITL work is not treated as an outage.
30. As a script author using `--check`, I want a stopped or stale runtime to fail, so that “reactor down” is catchable without JSON.
31. As a script author using `--check`, I want configured-but-disconnected NATS to fail, so that a bus outage is catchable without JSON.
32. As a script author using `--check` with `--json`, I still want the snapshot on stdout, so that I can log why the probe failed.
33. As an interface bee, I want documented follow-up CLI verbs for each attention kind, so that I do not invent new APIs to act.
34. As a Beekeeper, I want existing `session list`, `task list`, `energy show`, `doctor`, and `invite list` to keep working unchanged, so that status is an index, not a replacement of detail commands.
35. As a Beekeeper, I do not want status to start `paseka run` for me, so that an interface bee cannot accidentally spawn a second reactor.
36. As a colony, I want status not to dispatch, retry, or approve anything, so that an interface bee cannot become a hidden orchestrator.
37. As a Beekeeper, I want dead PIDs omitted from live bees the same way Queen Console Live bees does, so that CLI and Console agree.
38. As a Beekeeper with no live bees and no attention, I want a quiet empty snapshot, so that idle hives are easy to read.
39. As an interface bee, I want JSON field names aligned with Console projections (`traceId`, `agentId`, `alive`), so that I can reuse mental models from `/api/runtime` and `/api/agents`.
40. As a Beekeeper, I want text output to include the same blocks as JSON (runtime, bees, honey, attention), so that humans and agents see one model.
41. As a Beekeeper, I want `paseka status --help` to mention `--json` and `--check`, so that the contract is discoverable.
42. As an interface bee when colony resolve fails, I want a non-zero exit even without `--check`, so that “not a colony” is a command error, not a fake empty hive.
43. As a Beekeeper, I want active worktree count in the snapshot, so that isolated proposal workspaces are visible at a glance.
44. As an interface bee, I want low-energy attention to include `traceId` plus remaining/budget, so that I can call `energy add --trace` without a second lookup.
45. As a Beekeeper when honey ledger is unreachable, I want the rest of the snapshot intact, so that energy is best-effort, not all-or-nothing.
46. As a documentation reader, I want `paseka status` in the CLI guide after ship, so that the glossary stub becomes a real command.
47. As a Telegram gate maintainer, I want the option to format `/status` from the same snapshot later, so that phone and CLI do not drift — without requiring that in this MVP.
48. As a Queen Console maintainer, I want CLI to call the shared hive projection, not a forked copy of dashboard logic, so that Live bees and runtime stay consistent.
49. As a Beekeeper, I want `--check` to succeed (exit 0) when runtime is alive and NATS is either healthy or not configured, so that filesystem-only colonies are valid.
50. As an interface bee, I want not to receive event feeds, transcripts, merge diffs, or topology in the snapshot, so that I fetch those with dedicated commands when needed.

## Implementation Decisions

### 1. Command surface

- New Queen Shell command: `paseka status`.
- Flags:
  - `--path` / `-C` — colony resolution start directory (same as sibling commands).
  - `--json` — emit the snapshot as JSON on stdout.
  - `--check` — after printing, exit non-zero if hive **substrate** is not ready (decision 8).
- Default stdout is human text. `--json` is the agent contract.
- NATS is **not** required to run the command. Queen Console HTTP is **not** required.

### 2. Shared projection, not a Console clone

- Build one colony snapshot in the hive-view layer already used by Queen Console and Telegram (`GetRuntime`, `GetAgents`, task board counts, invites, recent traces).
- Do **not** import Console HTTP handlers into CLI. Do **not** duplicate Live bees PID rules; reuse the live-agents projection ([004](./004-live-bees-indicator.md)).
- Dashboard and Telegram `/status` may later consume the same snapshot type; MVP only requires CLI. Telegram wording need not change in this spec.

### 3. Snapshot shape (schemaVersion 1)

JSON object (illustrative contract; names are normative):

```json
{
  "schemaVersion": 1,
  "generatedAt": "RFC3339",
  "slug": "string",
  "colonyRoot": "string",
  "runtime": {
    "status": "running|stopped|stale|stopping",
    "alive": false,
    "pid": 0,
    "startedAt": "RFC3339",
    "lastHeartbeatAt": "RFC3339",
    "subjectPrefix": "string"
  },
  "nats": { "configured": false, "connected": false },
  "agents": { "count": 0, "afk": 0, "sessions": 0, "items": [] },
  "activeWorktrees": 0,
  "taskCounts": { "running": 0, "waiting_review": 0, "failed": 0 },
  "energy": {
    "available": false,
    "traces": [{ "traceId": "string", "remaining": 0, "budget": 0 }]
  },
  "attention": {
    "runtimeStale": false,
    "natsDown": false,
    "waitingReview": [],
    "pendingInvites": [],
    "failedTasks": [],
    "lowEnergyTraces": []
  },
  "recentTraces": [{ "traceId": "string", "title": "string", "updatedAt": "RFC3339" }]
}
```

- Additive fields after v1 are allowed; renaming or changing meaning of v1 fields requires a new `schemaVersion`.
- Omit empty optional strings; keep arrays present (possibly empty) so agents need not null-check every list.
- Agent item fields match live-bees: `kind` (`afk` | `session`), `bee`, `pid`, `traceId`, `agentId`, `sessionId`, `startedAt`, `runDir`.
- Attention rows for tasks carry at least `traceId`, `taskId`, and `title` when known; review rows also `review`. Invite rows carry `inviteId`, `traceId`, `bee`. Low-energy rows carry `traceId`, `remaining`, `budget`.

### 4. Text mode

- Same information as JSON, compact (Telegram-like header plus short lists).
- Live bees: a few `bee/pid` (or `bee/session`) lines, then a remainder count — same truncation spirit as Console header, but JSON still contains the capped full `items` array.
- Do not print suggested shell command strings in JSON. Document the follow-up map in the CLI guide when this ships.

### 5. Follow-up map (index, not new verbs)

| Snapshot signal | Existing command |
| --------------- | ---------------- |
| Runtime not alive | Beekeeper starts `paseka run` (status never starts it) |
| Live `session` | `paseka session list` / `attach` / `stop` |
| Live `afk` | `paseka task show`, `paseka kill` |
| `waitingReview` | `paseka task show`, `paseka proposal approve\|reject` |
| `pendingInvites` | `paseka invite list` / `accept` / `reject` |
| `failedTasks` | `paseka task retry` |
| `lowEnergyTraces` | `paseka energy add --trace` |
| `recentTraces` | `paseka task list --trace`, `paseka energy show --trace`, `paseka replay` |
| `nats.connected == false` | `paseka doctor` |

### 6. Energy in MVP

- Honey is part of the snapshot, not a follow-up-only concern.
- Scope: traces in the **recent-traces window** (same set / same limit as `recentTraces`), not every historical trail.
- Source: task-ledger snapshot (`remaining` / `budget`) when NATS is configured and the ledger reads successfully.
- `energy.available` is true only when that read succeeded for the window (partial failures: skip unread traces; if **none** could be read, `available` is false and `traces` is empty).
- Never invent zeros for unread traces.
- `attention.lowEnergyTraces`: traces in that window with a seeded budget (`budget > 0`) and `remaining <= 0` (same gate the reactor uses to refuse dispatch). Do not add a second “warning percent” threshold in MVP.
- `energy show --trace` remains the detail command.

### 7. NATS ping vs doctor

- Snapshot `nats.configured` / `nats.connected` is a **light connectivity** check (home `nats.url` / `PASEKA_NATS_URL`, can connect).
- Full JetStream/stream/KV/wiring stays `paseka doctor`.
- If NATS is not configured, `configured` and `connected` are false, `attention.natsDown` is false (not an outage).

### 8. Exit codes

| Situation | Exit |
| --------- | ---- |
| Colony resolve / IO / unexpected error | Non-zero (command failure). Independent of `--check`. |
| Snapshot produced, any live state including stopped runtime | **0** unless `--check`. |
| `--check` and runtime not alive (`stopped` or `stale`, `alive == false`) | Non-zero. |
| `--check` and NATS configured but not connected | Non-zero. |
| `--check` and runtime alive, and NATS either connected or not configured | **0**, even if attention is non-empty (reviews, invites, failed tasks, empty honey). |

Print the snapshot before applying `--check` so probes can capture JSON/text on failure.

`--check` does **not** treat attention, live bees, or honey exhaustion as process failure.

### 9. Limits and liveness

- Cap live agent items and recent traces consistently with existing hive-view limits (do not dump unbounded `.paseka/runs`).
- Live bees: PID liveness via the same `ProcessAlive` rule as [004](./004-live-bees-indicator.md). Do not unregister dead sessions/runs on read.
- Task counts / attention task rows: colony-wide board scan already used by Console (recent-trace window), not “all traces ever.”

### 10. Choreography boundary

- Status is observe-only. No `RegisterSelf`, no supervisor start/stop, no `task.ready`, no approve/retry/kill.
- Interface bees use status to **report and to choose existing CLI verbs** after Beekeeper intent. Auto-retry / auto-approve from status is out of scope and against colony choreography ([principles](../idea/principles.md)).

## Testing Decisions

Good tests assert **external behavior**: stdout shape, exit codes, and which ids appear — not which helper built the struct.

- **CLI**: `paseka status` / `--json` / `--check` / `--path`; command error when not a colony; exit 0 with stopped runtime without `--check`; `--check` non-zero when runtime not alive; `--check` zero when attention is non-empty but substrate is healthy; `--check` non-zero when NATS configured and disconnected.
- **Snapshot assembly**: live AFK + session items; stale vs running runtime; energy traces and `lowEnergyTraces` only when `remaining <= 0`; `energy.available` false when ledger missing; invites and waiting_review in attention; NATS unconfigured does not set `natsDown`.
- **JSON**: `schemaVersion` present; required arrays exist when empty; no command-failure exit for `alive: false` without `--check`.

Prior art: hive-view tests for runtime/agents; Console dashboard/handler tests that assemble projections; CLI command tests that capture stdout (`colony topology`, `task list`, Telegram `FormatStatus`). Prefer fixtures with fake `state.json`, run `status.json` PIDs, and ledger snapshots over hitting a live NATS in unit tests.

## Out of Scope

- `--watch` / streaming / WebSocket.
- Starting or stopping hive runtime from `status`.
- Cloning Queen Console tabs (events, runs detail, merge diff, topology, transcripts, PTY).
- Replacing `doctor`, `session list`, `task list`, `energy show`, `invite list`.
- Suggested command strings inside JSON.
- Colony-wide honey beyond the recent-trace window; percent-based “low honey” warnings.
- Cross-host / multi-machine runtime supervision.
- Changing Telegram `/status` text in this MVP (reuse later is allowed).
- New HTTP endpoints (CLI may share Go types; Console API stays as in [002](./002-queen-console-mvp.md)).
- Interface-bee auto-orchestration (retry/approve/kill without Beekeeper intent).

## Further Notes

- Glossary already lists `paseka status` as a Queen Shell verb; after implementation, [CLI guide](../guide/cli.md) is the canonical operator doc. Do not grow [AGENTS.md](../../AGENTS.md) with the JSON schema.
- Related: [002](./002-queen-console-mvp.md) dashboard + `/api/runtime`, [004](./004-live-bees-indicator.md) live bees, [010](./010-telegram-human-gateway.md) Telegram `/status`, [013](./013-system-kill.md) kill follow-up, task ledger honey in [task ledger](../reference/task-ledger.md).
- After ship: changelog entry; CLI guide command tree; optional later alignment of Telegram `/status` to this snapshot.
