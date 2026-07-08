# Spec 002: Queen Console MVP

## Purpose

Introduce a first Web UI for Paseka: **Queen Console**.

The MVP should make the colony observable and operable from a browser without replacing the existing CLI-first runtime model.

This spec is now a living MVP baseline. It reflects the implementation state after the recent Queen Console work: `paseka console`, the embedded SPA, JSON polling APIs, runtime controls, dashboard, timeline, task board, sessions, and runs.

## Goals

- Provide a browser dashboard that answers "what is happening right now?"
- Show current and recent colony activity across traces, tasks, runs, and sessions.
- Expose bee summaries and domain event history in a readable timeline.
- Provide basic task management from the UI.
- Show hive runtime state and provide local start/stop controls for the runtime process.
- Allow launching detached interactive bee sessions from the UI and observing their transcripts.
- Show recent headless adapter runs, summaries, and run events.
- Reuse the current runtime data model and artifacts instead of introducing a separate persistence layer.
- Preserve the CLI as the source of truth for runtime behavior and recovery.

## Non-Goals

- Do not replace `paseka run`, `paseka bee run`, `paseka bee chat`, or `paseka task *`.
- Do not introduce a new database for the MVP.
- Do not build a full multi-user or remote-host control plane.
- Do not implement a full browser terminal or PTY attach in the first release.
- Do not implement proposal approval/rejection from the browser in the first shipped console baseline.
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

## Current Implementation Snapshot

Queen Console currently ships as an embedded Go HTTP server plus static SPA:

- CLI entrypoint: `paseka console`
- Default listen address: `127.0.0.1:8787`
- Static assets embedded from `internal/console/static`
- API package: `internal/console`
- Filesystem projection helpers: `internal/runs`

Implemented UI surfaces:

- **Dashboard**: runtime status, NATS diagnostics, active sessions, active worktrees, task counts, recent traces, failed runs, and recent narrative insights.
- **Timeline**: filterable event feed with trace, task, bee, event type, payload kind, severity, cursor pagination, and optional raw JSON display.
- **Tasks**: grouped task board, task creation, optional autorun, task detail, linked runs, timeline navigation, and start controls for eligible tasks.
- **Sessions**: launch detached sessions for interactive-capable bees, list active and recent sessions, inspect metadata, poll transcript updates, and stop active sessions.
- **Runs**: list recent headless adapter invocations, inspect run metadata and summaries, and poll `events.ndjson` for a selected run.
- **Runtime panel**: start and stop the registered local hive runtime and poll runtime status.

Implemented backend behavior:

- Uses polling JSON endpoints, not WebSocket or SSE.
- Reads run, session, trace, task, transcript, and event state from `.paseka/runs`.
- Reads active sessions, active worktrees, and runtime registration from machine-local colony state.
- Uses NATS diagnostics for dashboard connectivity status; it does not yet consume a live JetStream event stream for console updates.
- Starts detached console sessions through the session manager and adapter session APIs; current detached console sessions use headless agent CLI execution rather than browser PTY attach.

Not implemented in the current baseline:

- Review queue UI and approve/reject actions.
- Dedicated worktrees page or `/api/worktrees` endpoint.
- Browser-native terminal input or PTY relay.
- WebSocket/SSE live transport.

## Primary User Outcomes

The MVP should let a solo developer:

- See whether the hive runtime is alive and whether the colony is active.
- Understand which traces and tasks are currently moving.
- Read what bees produced as summaries and review notes.
- Inspect a trace without manually opening run directories.
- Create tasks, start eligible tasks, and review task status from the browser.
- Start and stop the local hive runtime from the browser.
- Launch a detached bee session from the browser and observe its transcript, even if full in-browser terminal control is deferred.
- Inspect recent adapter runs and their emitted events without manually opening run directories.

## Decisions

### Product Scope

Queen Console MVP is an **observability-first control surface**.

The current baseline prioritizes:

- dashboard visibility
- timeline and event-feed inspection
- task board visibility and basic task actions
- run inspection
- session launch and session observation
- local runtime start/stop control

It does not prioritize full browser-native terminal control or review approval.

### Backend Shape

The MVP should run from the existing Go application rather than a separate service.

Queen Console runs under the existing Paseka binary:

```text
paseka console
```

This server exposes:

- JSON HTTP endpoints for snapshots and detail views
- polling endpoints for incremental transcripts and run events
- static assets for the SPA frontend

### Source of Truth

The UI must not create a second authority for runtime state.

Use the following precedence for the current baseline:

1. Filesystem projections under `.paseka/runs/`
2. Machine-local state in `~/.config/paseka/<slug>/state.json`
3. Runtime process registration and heartbeat state
4. NATS diagnostics for connectivity status

This allows the UI to remain useful even when:

- the runtime is stopped
- NATS is temporarily unavailable
- the user wants to inspect historical traces from local artifacts

Future live subscriptions may add JetStream data as the freshest source for event updates, but the current UI is intentionally useful from local projections alone.

Task creation and start actions are the main exception to read-only fallback behavior: they require NATS and the task ledger KV because they publish domain events instead of writing local files directly.

### MVP Screens

The current MVP includes five primary SPA tabs plus a global runtime panel.

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

Current implementation polls `GET /api/dashboard` every 5 seconds while the Dashboard tab is active.

#### 2. Timeline / Event Feed

Show a filterable event stream across the colony or within one trace:

- event timestamp
- `traceId`
- `agentId`
- top-level type (`SIGNAL`, `INSIGHT`, `MUTATION`, `VERIFICATION`)
- `payload.kind`
- human-readable summary when available
- link metadata for related run or task

Filters should include:

- trace
- task
- bee
- event type
- `payload.kind`
- severity where applicable

The UI supports both:

- a readable timeline for humans
- a raw JSON inspection mode for debugging

Current implementation polls on demand through `GET /api/events` and uses cursor pagination via `after`.

#### 3. Task Board

The Tasks tab exposes task control and task state using the existing task ledger semantics:

- `ready`
- `running`
- `waiting_review`
- `planned`
- `blocked`
- `failed`
- `completed`

Current implementation supports:

- grouped task board across recent traces
- task creation with title, body, bee, optional trace id, sector, intent, dependencies, and autorun
- task detail with status, source, body, summary, commit, dependencies, and linked runs
- start action for eligible tasks
- linked run navigation
- task-scoped timeline navigation

Create and start actions require NATS and the task ledger KV. Read-only board/detail views can fall back to filesystem task projections where KV is unavailable or empty.

The board is a grouped list, not drag-and-drop Kanban. It polls `GET /api/tasks` every 5 seconds while the Tasks tab is active.

#### 4. Runs

The Runs tab exposes recent headless adapter invocations:

- trace id and agent id
- bee and adapter
- state, task, intent, workspace, run directory
- summary from `result.json` or `result.txt`
- event stream from `events.ndjson`
- whether the run has an associated session

The selected run view polls incremental events every 1.5 seconds.

This page answers:

- Which run produced this proposal?
- What did the run summarize?
- Which events did the run emit?

#### 5. Sessions

The sessions page supports:

- list active and recent sessions
- start a new detached session
- inspect session metadata
- follow transcript updates by polling `transcript.ndjson`
- stop a session

Session launch supports:

- choosing an interactive-capable bee
- entering task text
- using an advanced raw-prompt override
- optionally setting trace id and intent

For the MVP, session interaction remains split:

- launch from Web UI
- execution in existing session/adapter model
- transcript and status visible in browser

Full browser-native terminal control is explicitly deferred.

#### Deferred: Trace View

A trace detail API exists and aggregates:

- trace summary
- tasks in that trace
- related runs
- active worktree state for that trace
- recent events

The SPA does not yet expose this as a first-class Trace tab. Dashboard trace clicks currently navigate to the Runs tab and select a matching run when one exists.

### Chat UX Phases

Interactive chat should be introduced in two phases.

#### Phase A: Launch + Observe

Implemented. From the UI, the user can:

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

The current implementation uses detached session execution and transcript polling. It does not provide browser input to an already-running PTY.

#### Phase B: Browser Terminal

Later, Queen Console may add:

- PTY relay over WebSocket
- browser terminal attach
- send input from browser to session
- better session resume / reconnect behavior

This phase depends on a dedicated session relay API and should not block the MVP.

### Review Queue

Because human approval is central to the product model, Queen Console should expose a lightweight review queue for code proposals and review outcomes. This is deferred from the current baseline.

At minimum, show:

- proposals awaiting attention
- source trace and task
- summary
- current run status
- links to diff artifacts when present

If approve/reject actions are included in MVP scope, they should reuse the same domain flows as existing CLI commands.

### Worktree Visibility

The dashboard currently surfaces only the count of active colony-managed worktrees. The trace detail API can include the active worktree associated with a trace.

A future dedicated worktree view should surface:

- trace id
- path
- base SHA
- branch
- created at

This is important for understanding isolated mutation state and for debugging stuck or abandoned traces.

## Backend API Shape

The current routing keeps web transport in `internal/console` and runtime/run/session behavior in existing packages.

Implemented HTTP endpoints:

- `GET /api/runtime`
- `POST /api/runtime/start`
- `POST /api/runtime/stop`
- `GET /api/dashboard`
- `GET /api/tasks`
- `POST /api/tasks`
- `GET /api/traces`
- `GET /api/traces/:traceId`
- `GET /api/traces/:traceId/events`
- `GET /api/traces/:traceId/tasks`
- `POST /api/traces/:traceId/tasks/start`
- `GET /api/traces/:traceId/tasks/:taskId`
- `POST /api/traces/:traceId/tasks/:taskId/start`
- `GET /api/events`
- `GET /api/bees`
- `GET /api/sessions`
- `POST /api/sessions`
- `GET /api/sessions/:sessionId`
- `GET /api/sessions/:sessionId/transcript`
- `POST /api/sessions/:sessionId/stop`
- `GET /api/runs`
- `GET /api/runs/:traceId/:agentId`
- `GET /api/runs/:traceId/:agentId/events`

Deferred suggested endpoints:

- `GET /api/worktrees`
- `GET /api/review-queue`

Deferred live endpoints:

- `GET /api/events/stream` via WebSocket or Server-Sent Events
- `GET /api/sessions/:sessionId/stream` via WebSocket or Server-Sent Events

The current MVP uses polling instead of streaming.

## Data Projection Rules

To keep UI behavior predictable, the backend should normalize data from current sources into a small set of view models.

### Dashboard Projection

Aggregate from:

- runtime registration and heartbeat state
- NATS diagnostics
- active sessions from `state.json` and the in-process session manager
- active worktrees from `state.json`
- task status counts from filesystem task projections
- recent run outcomes from `status.json`
- recent traces from `.paseka/runs`
- recent narrative insights from `events.ndjson`

### Trace Projection

Aggregate from:

- task ledger KV for the trace when populated
- filesystem task projections for the trace as fallback
- all run directories under `.paseka/runs/<traceId>/`
- all events from trace run directories
- linked task projections under `.paseka/runs/<traceId>/tasks/`
- active worktree state from machine-local colony state

The trace projection exists in the backend, but a dedicated Trace tab is not part of the current SPA.

### Task Board Projection

Aggregate from recent trace directories and per-trace task snapshots:

- prefer task ledger KV when populated
- fall back to `.paseka/runs/<traceId>/tasks/` projections
- group tasks by status
- expose `canStart` using task-ledger eligibility checks
- include run counts from each task's `runs.ndjson`

Task detail combines the task snapshot with:

- body, intent, summary, commit, dependencies, sector, and bee
- source marker (`jetstream-kv` or `filesystem`)
- linked runs from `.paseka/runs/<traceId>/tasks/<taskId>/runs.ndjson`

Task mutations publish existing domain events instead of writing a separate console-owned state:

- create publishes `task.plan`
- autorun and start publish `task.ready`
- console mutations identify the agent as `console`

### Run Projection

Combine:

- `request.json`
- `status.json`
- `result.txt`
- `result.json`, when present
- `events.ndjson`
- `session.json` and `transcript.ndjson`, when present

`meta.json` remains useful for legacy started-at metadata, but the current run projection is anchored on `request.json`.

### Session Projection

Combine:

- session registry entry from `state.json`
- active in-process session manager entries
- run metadata from the linked run directory
- transcript entries from `transcript.ndjson`

### Timeline Projection

Aggregate from recent trace directories and each run's `events.ndjson`, then normalize events into feed rows with:

- stable cursor id
- timestamp
- trace id and agent id
- bee role when it can be resolved from run metadata
- event type
- payload kind
- task id when present
- severity for narrative insights
- human-readable summary for known task, mutation, verification, and prompt-memory insight kinds
- raw event payload for debugging

## Frontend Expectations

The frontend should be a lightweight SPA, optimized for operator workflows rather than marketing presentation.

Current baseline:

- fast navigation between dashboard, timeline, task, session, and run views
- readable timelines and summaries
- no dependence on a heavy graph visualization library for the first release
- plain static assets served by the Go binary
- polling-based freshness for runtime, dashboard, task board, transcripts, and run events
- mobile support is optional; desktop-first is acceptable

## Suggested Implementation Phases

### 1. Session Console (Implemented)

Includes:

- HTTP server bootstrap
- session list
- bee list
- session launch
- session detail
- transcript polling
- session stop

### 2. Runs Projection (Implemented)

Includes:

- recent run list
- run detail
- run summary
- run event polling

### 3. Runtime Controls (Implemented)

Includes:

- runtime status endpoint
- local runtime start
- local runtime stop
- global runtime panel in the SPA

### 4. Dashboard + Timeline (Implemented)

Includes:

- dashboard snapshot
- trace summaries
- task counts
- failed run highlights
- recent narrative insights
- filterable event feed
- raw event JSON inspection
- cursor pagination

### 5. Task Operations (Implemented)

Includes:

- task board tab
- grouped task list across recent traces
- create task
- optional autorun on create
- start eligible task
- task detail view
- linked run navigation
- task timeline navigation

### 6. Trace View (Partially Implemented)

Backend projection exists for trace detail and trace-scoped events. A first-class Trace tab is still deferred.

### 7. Review Queue (Deferred)

Add:

- review queue page
- proposal detail
- approve proposal
- reject proposal with human feedback

### 8. Live Streaming Improvements (Deferred)

Add:

- global event stream
- per-session live updates
- better review queue freshness

### 9. Browser Terminal (Deferred)

Only after MVP proves useful:

- PTY relay
- browser terminal component
- reconnect / attach semantics

## Risks and Constraints

### Session Control Is the Hardest Part

The current session model is PTY-owned and optimized for terminal attach, not browser interaction.

Detached launch and transcript observation are implemented. Browser-native chat input is still deferred.

### Event History Is Distributed Across Run Directories

The current event audit model is correct for traceability, but the UI backend will need an efficient way to scan and aggregate per-trace events.

Implementation should avoid making every page perform repeated deep filesystem walks.

The current implementation bounds this work by scanning recent traces and capping event-feed results. If console usage grows beyond local solo operation, the next step is a cached projection or live event index rather than increasing scan depth.

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

- Should `paseka console` continue supervising an external `paseka run`, or should a future console mode embed the runtime in-process?
- Should live updates move from polling to WebSocket or SSE once the review queue exists or session/event volume grows?
- Should the next UI expansion prioritize first-class Trace view, Review Queue, or Worktree view?
- Should frontend code stay as embedded static assets, or move to a small bundled frontend workspace if UI complexity grows?

## Verification

For documentation-only updates, no build is required.

After Go code changes are made:

```bash
gofmt -w .
go build -o paseka ./cmd/paseka
```

If the console adds frontend build tooling, document the exact build and packaging commands in the implementation PR rather than expanding this spec prematurely.
