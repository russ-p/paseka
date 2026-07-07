# Spec 002: Queen Console MVP

## Purpose

Introduce a first Web UI for Paseka: **Queen Console**.

The MVP should make the colony observable and operable from a browser without replacing the existing CLI-first runtime model.

This spec captures the shared design only. Implementation must not start until explicitly confirmed.

## Goals

- Provide a browser dashboard that answers "what is happening right now?"
- Show live and recent colony activity across traces, tasks, runs, and sessions.
- Expose bee summaries and domain event history in a readable timeline.
- Provide basic task management from the UI.
- Allow starting interactive bee chats from the UI.
- Reuse the current runtime data model and artifacts instead of introducing a separate persistence layer.
- Preserve the CLI as the source of truth for runtime behavior and recovery.

## Non-Goals

- Do not replace `paseka run`, `paseka bee run`, `paseka bee chat`, or `paseka task *`.
- Do not introduce a new database for the MVP.
- Do not build a full multi-user or remote-host control plane.
- Do not implement a full browser terminal or PTY attach in the first release.
- Do not redesign event contracts, task lifecycle, or run-directory layout for the sake of the UI.
- Do not require all interactions to be live-only; filesystem projections must remain usable as fallback state.

## Current System Context

Paseka already exposes most of the state a Web UI needs:

- `.paseka/runs/<traceId>/<agentId>/`
  - `meta.json`
  - `status.json`
  - `request.json`
  - `result.txt`
  - `events.ndjson`
  - `session.json` for interactive runs
  - `transcript.ndjson` for interactive runs
- `.paseka/runs/<traceId>/tasks/<taskId>/`
  - `task.md`
  - `runs.ndjson`
- `~/.config/paseka/<slug>/state.json`
  - active sessions
  - active worktrees
- NATS / JetStream
  - live domain events
  - task ledger KV

The existing architecture already treats `INSIGHT` as narrative context and dashboard timeline data, while `SIGNAL`, `MUTATION`, and `VERIFICATION` continue to drive workflow and routing.

The existing interactive session design also explicitly leaves room for a later WebSocket relay for Queen Console instead of requiring it in the MVP.

## Primary User Outcomes

The MVP should let a solo developer:

- See whether the hive runtime is alive and whether the colony is active.
- Understand which traces and tasks are currently moving.
- Read what bees produced as summaries and review notes.
- Inspect a trace without manually opening run directories.
- Start tasks and review their status from the browser.
- Launch a chat session from the browser and observe its transcript, even if full in-browser terminal control is deferred.

## Decisions

### Product Scope

Queen Console MVP is an **observability-first control surface**.

The first release prioritizes:

- dashboard visibility
- trace/task inspection
- review queue visibility
- task actions
- session launch and session observation

It does not prioritize full browser-native terminal control.

### Backend Shape

The MVP should run from the existing Go application rather than a separate service.

Add a new HTTP server mode under the Paseka binary, for example:

```text
paseka console
```

This server should expose:

- JSON HTTP endpoints for snapshots and detail views
- WebSocket endpoints for live updates where valuable
- static assets for the SPA frontend

### Source of Truth

The UI must not create a second authority for runtime state.

Use the following precedence:

1. Live NATS / JetStream data where available
2. Filesystem projections under `.paseka/runs/`
3. Machine-local state in `~/.config/paseka/<slug>/state.json`

This allows the UI to remain useful even when:

- the runtime is stopped
- NATS is temporarily unavailable
- the user wants to inspect historical traces from local artifacts

### MVP Screens

The MVP should include five primary surfaces.

#### 1. Dashboard

Show a colony-wide snapshot:

- runtime status
- NATS connectivity status
- number of active sessions
- number of active worktrees
- task counts by status
- recent traces with activity
- recent failed runs
- recent narrative insights (`run.summary`, `review.note`, `human.feedback`, `context.note`)

This page answers:

- Is anything running?
- Is anything stuck?
- Which trace needs attention?

#### 2. Timeline / Event Feed

Show a filterable event stream across the colony or within one trace:

- event timestamp
- `traceId`
- `agentId`
- top-level type (`SIGNAL`, `INSIGHT`, `MUTATION`, `VERIFICATION`)
- `payload.kind`
- human-readable summary when available
- link to related run / task / session

Filters should include:

- trace
- task
- bee
- event type
- `payload.kind`
- severity where applicable

The UI should support both:

- a readable timeline for humans
- a raw JSON inspection mode for debugging

#### 3. Trace View

A trace page should aggregate:

- trace-level timeline
- tasks in that trace
- related runs
- worktree state
- pending review items

This is the main operator view for one piece of work.

The UI should make it easy to answer:

- Which task is currently ready or running?
- What happened before this failure?
- Which run produced this proposal?

#### 4. Task Board

Expose task control and task state using the existing task ledger semantics:

- `planned`
- `ready`
- `running`
- `waiting_review`
- `completed`
- `failed`
- `blocked`

Required actions:

- create task
- start eligible task
- inspect task details
- open linked runs

Nice-to-have in the same MVP if simple:

- approve proposal
- reject proposal with human feedback

The board may start as a grouped list instead of a drag-and-drop Kanban.

#### 5. Sessions

The sessions page should support:

- list active sessions
- start a new session
- inspect session metadata
- follow transcript updates
- stop a session

For the MVP, session interaction may remain split:

- launch from Web UI
- execution in existing CLI/session model
- transcript and status visible in browser

Full browser-native terminal control is explicitly deferred.

### Chat UX Phases

Interactive chat should be introduced in two phases.

#### Phase A: Launch + Observe

From the UI, the user can:

- choose bee
- enter task or prompt
- optionally set trace and intent
- start the session

After launch, the UI shows:

- session state
- session id / trace id / agent id
- workspace
- transcript stream
- related run directory metadata

This phase is enough to make sessions visible and manageable in the browser while still relying on the current PTY-owned session model.

#### Phase B: Browser Terminal

Later, Queen Console may add:

- PTY relay over WebSocket
- browser terminal attach
- send input from browser to session
- better session resume / reconnect behavior

This phase depends on a dedicated session relay API and should not block the MVP.

### Review Queue

Because human approval is central to the product model, Queen Console MVP should expose a lightweight review queue for code proposals and review outcomes.

At minimum, show:

- proposals awaiting attention
- source trace and task
- summary
- current run status
- links to diff artifacts when present

If approve/reject actions are included in MVP scope, they should reuse the same domain flows as existing CLI commands.

### Worktree Visibility

The MVP should surface active colony-managed worktrees:

- trace id
- path
- base SHA
- branch
- created at

This is important for understanding isolated mutation state and for debugging stuck or abandoned traces.

## Backend API Shape

The exact routing can evolve, but the MVP should reserve a clean boundary between web transport and existing runtime packages.

Suggested HTTP endpoints:

- `GET /api/dashboard`
- `GET /api/traces`
- `GET /api/traces/:traceId`
- `GET /api/traces/:traceId/events`
- `GET /api/traces/:traceId/tasks`
- `GET /api/traces/:traceId/tasks/:taskId`
- `POST /api/traces/:traceId/tasks`
- `POST /api/traces/:traceId/tasks/:taskId/start`
- `GET /api/runs/:traceId/:agentId`
- `GET /api/runs/:traceId/:agentId/events`
- `GET /api/runs/:traceId/:agentId/transcript`
- `GET /api/sessions`
- `POST /api/sessions`
- `POST /api/sessions/:sessionId/stop`
- `GET /api/worktrees`
- `GET /api/review-queue`

Suggested live endpoints:

- `GET /api/events/stream` via WebSocket or Server-Sent Events
- `GET /api/sessions/:sessionId/stream` via WebSocket or Server-Sent Events

The MVP may use polling instead of streaming for some pages if it materially reduces implementation complexity.

## Data Projection Rules

To keep UI behavior predictable, the backend should normalize data from current sources into a small set of view models.

### Dashboard Projection

Aggregate from:

- active sessions from `state.json`
- active worktrees from `state.json`
- task status counts from task ledger KV or filesystem task projections
- recent run outcomes from `status.json`
- recent summaries from `events.ndjson` and `result.txt`

### Trace Projection

Aggregate from:

- task ledger snapshot for the trace
- all run directories under `.paseka/runs/<traceId>/`
- all events from trace run directories
- linked task projections under `.paseka/runs/<traceId>/tasks/`

### Run Projection

Combine:

- `meta.json`
- `request.json`
- `status.json`
- `result.txt`
- `result.json`, when present
- `events.ndjson`
- `session.json` and `transcript.ndjson`, when present

### Session Projection

Combine:

- session registry entry from `state.json`
- run metadata from the linked run directory
- transcript entries from `transcript.ndjson`

## Frontend Expectations

The frontend should be a lightweight SPA, optimized for operator workflows rather than marketing presentation.

MVP expectations:

- fast navigation between dashboard, trace, task, run, and session views
- readable timelines and summaries
- no dependence on a heavy graph visualization library for the first release
- mobile support is optional; desktop-first is acceptable

## Suggested Implementation Phases

### 1. Read-Only Console

Add:

- HTTP server bootstrap
- dashboard snapshot
- trace list
- trace detail
- run detail
- session list

This phase already provides high-value observability.

### 2. Task Operations

Add:

- create task
- start task
- task detail view
- review queue page

### 3. Session Launch + Transcript Follow

Add:

- start session from UI
- stop session from UI
- transcript follow view
- live status refresh

### 4. Live Streaming Improvements

Add:

- global event stream
- per-session live updates
- better review queue freshness

### 5. Browser Terminal (Deferred)

Only after MVP proves useful:

- PTY relay
- browser terminal component
- reconnect / attach semantics

## Risks and Constraints

### Session Control Is the Hardest Part

The current session model is PTY-owned and optimized for terminal attach, not browser interaction.

This is why browser-native chat input is deferred from the MVP.

### Event History Is Distributed Across Run Directories

The current event audit model is correct for traceability, but the UI backend will need an efficient way to scan and aggregate per-trace events.

Implementation should avoid making every page perform repeated deep filesystem walks.

### Runtime and UI Must Agree on Vocabulary

The UI must preserve existing domain names and lifecycle semantics:

- trace
- task
- bee
- proposal
- review
- worktree
- `SIGNAL`, `INSIGHT`, `MUTATION`, `VERIFICATION`

The UI may present friendly labels, but backend contracts should stay aligned with current protocol terminology.

## Open Questions

- Should `paseka console` also start an embedded runtime, or remain UI-only and connect to an already running `paseka run`?
- Should live updates use WebSocket, SSE, or polling for the first release?
- Should review approval/rejection be included in the MVP, or follow immediately after read-only visibility and task start actions?
- Should the frontend live inside the Go repo as static assets, or in a separate `web/` workspace that is bundled into the binary?

## Verification

After implementation begins and Go code changes are made:

```bash
gofmt -w .
go build -o paseka ./cmd/paseka
```

If the MVP adds frontend build tooling, document the exact build and packaging commands in the implementation PR rather than expanding this spec prematurely.
