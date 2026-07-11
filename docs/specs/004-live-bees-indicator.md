# Spec 004: Live Bees Indicator

## Purpose

Add a Queen Console header indicator for **currently live agent processes** — both AFK/headless adapter runs spawned by the hive runtime and interactive bee sessions — analogous to the existing **Hive runtime** panel.

This spec captures the shared design agreed in flight trail `trace-019f51fd4b601818`. Implementation must not start until explicitly confirmed.

## Goals

- Answer “what agent processes are alive right now?” from any console tab.
- Show a compact header panel next to Hive runtime: total count, AFK/session breakdown, and a short `bee/pid` list.
- Reuse filesystem projections and machine-local state as source of truth (same rule as Queen Console MVP).
- Persist AFK adapter PIDs so the UI can verify process liveness.
- Expose a dedicated JSON API for live agents, polled with the runtime header refresh.

## Non-Goals (MVP)

- Stop/kill controls on the Live bees panel.
- A separate Dashboard stat-card duplicate of this indicator.
- Rewriting `status.json` or auto-unregistering sessions on read when a PID is dead.
- Per-item clicks on individual `bee/pid` rows in the header.
- WebSocket / SSE push for live agent updates (stay on polling).
- Cross-host or multi-user process supervision.

## Current System Context

Relevant existing pieces:

| Primitive | Location / behavior | Indicator use |
| --------- | ------------------- | ------------- |
| Hive runtime panel | Header + `GET /api/runtime` | UX pattern to mirror (badge, pid meta, 3s poll) |
| Runtime registry | `~/.config/paseka/<slug>/state.json` → `runtime` | Unchanged; reactor process only |
| Session registry | `state.json` → `sessions[]` with `pid` | Live interactive sessions |
| AFK run status | `.paseka/runs/<traceId>/<agentId>/status.json` | Detect `state=running` AFK runs |
| AFK adapters | `internal/adapters/{cursor,pi,claude,script}` | Today write running status **without** PID |
| `StatusSnapshot` | `internal/protocol` | Extend with optional `pid` |
| `ProcessAlive` | `internal/colony` | Liveness check already used by runtime supervisor |
| Runs / Sessions tabs | Queen Console SPA | Drill-down targets after header click |

Gaps today:

- No header signal for child agent processes (only the reactor).
- AFK `status.json` has no PID, so console cannot distinguish a live adapter from a crashed run left as `running`.
- Dashboard shows `activeSessions` count but not AFK process count or PIDs, and only while the Dashboard tab is active.

## Decisions

### 1. Scope: unified live agents

The indicator covers **both**:

1. **AFK / headless** adapter processes (typically spawned by `paseka run`)
2. **Interactive sessions** (`bee chat` / console-launched sessions)

One panel, one total count, with an AFK vs session breakdown.

### 2. Placement: header only

Render a **Live bees** panel in the Queen Console header next to **Hive runtime**.

Do not add a Dashboard-only card in this MVP. Existing Dashboard `Active sessions` remains as-is.

### 3. Header UX

| Element | Behavior |
| ------- | -------- |
| Label | `Live bees` |
| Badge | Total live count; idle / zero state when none |
| Meta line | `N afk · M session` (wording may pluralize) |
| Detail line | Up to **3** entries as `bee/pid` (e.g. `drone/4242`); then `+N more` |
| Controls | None in MVP (observe-only) |
| Click | Smart navigation (decision 8) |

API path stays technical: `GET /api/agents`. UI label is product language.

### 4. AFK PID persistence

When an AFK adapter starts, write the OS process PID into `status.json` via an extended `StatusSnapshot`:

```json
{
  "protocolVersion": "...",
  "state": "running",
  "pid": 4242,
  "startedAt": "..."
}
```

Apply to all AFK adapters that already write a running snapshot (`cursor`, `pi`, `claude`, `script`).

Implementation note: launch with `Start` (or equivalent) so `Process.Pid` is available before wait; keep writing the final status on exit as today.

### 5. Liveness rules

**AFK (live):**

- `status.json` `state == running`
- `pid > 0`
- `colony.ProcessAlive(pid)`

**Session (live):**

- entry present in session registry (and/or in-process manager as today)
- `pid > 0`
- `colony.ProcessAlive(pid)`

Dead PIDs are **excluded** from the live header count. Projections may later expose them as `stale` elsewhere; MVP console **must not** mutate `status.json` or unregister sessions on read.

Legacy AFK runs still marked `running` but lacking `pid` are not counted as live.

### 6. API: `GET /api/agents`

New endpoint, separate from `GET /api/runtime` (runtime remains reactor-only).

Suggested response shape:

```json
{
  "count": 3,
  "afk": 2,
  "sessions": 1,
  "items": [
    {
      "kind": "afk",
      "bee": "drone",
      "pid": 4242,
      "traceId": "trace-…",
      "agentId": "…",
      "startedAt": "…",
      "runDir": "…"
    },
    {
      "kind": "session",
      "bee": "hivewright",
      "pid": 4243,
      "traceId": "trace-…",
      "agentId": "…",
      "sessionId": "…",
      "startedAt": "…",
      "runDir": "…"
    }
  ]
}
```

Rules:

- Return **all live items** up to a soft-cap of **50**.
- UI truncates display to **3** + `+N more`.
- Sort stably (recommend: `startedAt` ascending, then `kind`, then `bee`).

### 7. Polling

Reuse / merge with the existing header runtime poll:

- One shared ~**3s** timer loads `GET /api/runtime` and `GET /api/agents` together.
- Active on every tab (same as runtime today).

### 8. Click navigation

Clicking the Live bees panel (not individual rows):

1. If any live AFK → open **Runs** tab
2. Else if any live sessions → open **Sessions** tab
3. Else → open **Runs** tab

Per-item header drill-down is out of scope; Runs/Sessions tabs already provide detail.

### 9. Source of truth precedence

Follow Queen Console MVP:

1. Filesystem projections under `.paseka/runs/` (AFK `status.json` + meta)
2. Machine-local `state.json` (sessions)
3. `ProcessAlive` for honesty of “live”
4. Do not require NATS for this indicator

## Implementation Outline

When implementation is confirmed:

1. **Protocol / adapters**
   - Add optional `pid` to `protocol.StatusSnapshot`
   - On AFK launch, capture and persist PID in the running snapshot for each adapter
2. **Console backend**
   - Add `AgentsView` projection (scan AFK running+alive; merge sessions+alive)
   - Wire `GET /api/agents` in `internal/console` handlers
   - Unit tests: live / dead PID / missing PID / soft-cap
3. **SPA**
   - Header markup + styles beside runtime panel
   - Render badge, breakdown, truncated `bee/pid` list
   - Shared poll with runtime; smart-nav click handler
4. **Docs**
   - Keep this spec as the feature source of truth
   - Optionally cross-link from [002-queen-console-mvp.md](./002-queen-console-mvp.md) under a short “Related specs” note (no need to duplicate decisions)

## Acceptance Criteria

- Header shows **Live bees** next to Hive runtime on all tabs.
- With no live agents: idle/zero state is clear; click still navigates (to Runs).
- With mixed AFK + sessions: total, breakdown, and up to 3 `bee/pid` entries are correct; overflow shows `+N more`.
- Killing an AFK OS process (without clean status finalization) removes it from the live count on the next poll.
- Killing a session OS process removes it from the live count without requiring registry cleanup.
- `GET /api/agents` returns counts and items consistent with the panel (cap 50).
- New AFK runs write `pid` into `status.json` while running.
- No stop/kill buttons; no Dashboard duplicate card; no rewrite-on-read.

## Open Questions (deferred)

- Whether `RunView` / Runs tab should also surface `pid` for consistency (nice-to-have, not required for this indicator).
- Whether a later pass should offer stop/kill from the header or from Runs/Sessions detail.
- Whether stale (running + dead pid) should appear as an explicit warning elsewhere in the console.

## Related

- [002-queen-console-mvp.md](./002-queen-console-mvp.md) — Queen Console baseline, runtime panel, projection rules
- [006-interactive-sessions.md](../006-interactive-sessions.md) — session registry and PID handling
- [003-architecture.md](../003-architecture.md) — adapter run dirs and colony state layout
