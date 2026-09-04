# Spec 021: Provider session logs and export enrichment

## Status

**(Draft)**
AFK Cursor and Pi persist `providerSessionId`. HITL Cursor uses `create-chat` / `--resume`; HITL Pi records the pinned `--session-id`. Export `--include agent-logs` renders a per-run Agent log via `SessionLogResolver`. Cursor reads `~/.cursor/projects/.../agent-transcripts/<uuid>/<uuid>.jsonl` in place (omit `store not found` / `parse error`). Pi remains stubbed (`not implemented`). Remaining: Pi reader, Cursor `store.db`, `events.ndjson` bus-only cleanup. Do not mix provider CLI stream into `events.ndjson`.

## Problem Statement

After a Flight Trail finishes, Beekeepers want a detailed local report for analysis: which tools ran, what the agent did, and (later) richer session detail. Today:

- Interactive and AFK runs store Paseka-owned artifacts (`summary.md`, `result.json`, PTY `transcript.ndjson`), but those are weak or ANSI-noisy compared to the provider’s own session log.
- Cursor AFK already emits structured `stream-json` with a stable `session_id` and tool calls, but that id is not persisted on the run, and the stream is partly dumped into `events.ndjson` as `TOOL_CALL` / `ASSISTANT_TEXT` — the same file that audits the domain bus.
- Domain bus audit (`SIGNAL` / `INSIGHT` / `MUTATION` / `VERIFICATION` via `paseka event emit`) must stay separate from vendor session chatter.
- Copying provider session stores into `.paseka/runs/` is unnecessary and brittle when the goal is **local** analysis on the machine that ran the agent: association + read-in-place is enough. Missing provider data must not fail the trail or the export.

## Solution

1. **Capture and associate** each adapter invocation with a **provider session id** (Cursor chat UUID, Pi session id, etc.) on the agent run record.
2. **Do not copy** provider log bodies into the colony runs tree. Runtime and export **resolve** logs through the adapter when enrichment is requested.
3. **`paseka export`** optionally (or best-effort) adds a per-run **Agent log** section (MVP: tool-call summary) sourced from provider logs when the id resolves; omit quietly when unavailable.
4. **Stop writing** provider stream diagnostics (`TOOL_CALL`, `ASSISTANT_TEXT`) into `events.ndjson`. That file remains the domain-bus audit log only. Token usage stays on `result.json` as today.
5. **Phased delivery:**
   - **Slice A — AFK Cursor:** parse `session_id` from `stream-json` (earliest on `system` / `init`); persist on the run; export enrichment from Cursor store / stream-derived reader.
   - **Slice B — HITL Cursor:** `agent create-chat` → persist UUID on `session.json` → launch interactive TUI with `--resume=<uuid>`; same export path by id.
   - **Slice C — other adapters:** Pi (already run-local session dir) and Claude when their native id / log surfaces are clear.

PTY remains the attach UI for HITL; it is not the source of truth for structured export when a provider session id exists.

## User Stories

1. As a Beekeeper, I want each Cursor AFK run to record the provider `session_id`, so that I can later open or export that conversation without guessing.
2. As a Beekeeper, I want `paseka export` to include a short tool-call log per run when provider data is present, so that I can analyze what the agent did locally.
3. As a Beekeeper exporting on the same machine, I want enrichment to read Cursor’s existing session store by id, so that Paseka does not duplicate large logs under `.paseka/runs/`.
4. As a Beekeeper exporting on another machine or after Cursor purged chats, I want export to succeed without the Agent log section, so that missing provider data is never a hard failure.
5. As a Beekeeper, I want domain Timeline / `events.ndjson` to show only bus events (`SIGNAL` / `INSIGHT` / `MUTATION` / `VERIFICATION`), so that tool chatter does not pollute choreography audit.
6. As a Beekeeper inspecting Runs in Queen Console, I want run events to remain bus-oriented, so that Console and export share one meaning of `events.ndjson`.
7. As a colony author, I want bees to keep publishing domain events only via `paseka event emit`, so that provider stream parsing never invents bus events.
8. As a Beekeeper running AFK Cursor with `stream-json`, I want the first `system`/`init` line’s `session_id` to be captured, so that association works even if later lines are truncated.
9. As a Beekeeper, I want the same `session_id` accepted from later stream lines if init was missed, so that capture is resilient.
10. As a Beekeeper, I want usage tokens to remain on `result.json`, so that usage include / inspect keep working without stream dump into `events.ndjson`.
11. As a Beekeeper running HITL `bee chat` with Cursor, I want the session to start from a pre-created chat UUID, so that interactive TUI shares the same association model as AFK.
12. As a Beekeeper, I want that UUID stored on `session.json` before the agent process starts, so that a crash mid-session still leaves a pointer.
13. As a Beekeeper attaching via Ghostty or Queen Console PTY, I want attach behavior unchanged, so that HITL UX does not depend on provider log readers.
14. As a Beekeeper, I want PTY `transcript.ndjson` to remain an optional coarse fallback for Console, so that live/completed observation still works when provider enrichment is absent.
15. As a Beekeeper, I want export Agent log to prefer provider data over PTY transcript when both exist, so that structured tool calls beat ANSI dumps.
16. As a Beekeeper using Pi, I want the same association + export enrichment pattern against Pi’s session dir / session id, so that multi-adapter colonies get comparable reports.
17. As a Beekeeper using Claude, I want enrichment when a stable native session id / log API exists, so that Claude is not forced into a Cursor-shaped store.
18. As a Beekeeper, I want adapters that cannot resolve logs to omit Agent log quietly, so that export stays best-effort.
19. As a Beekeeper, I want an explicit `--include` token for provider/agent logs (or clear default best-effort documented), so that export payload size stays under control.
20. As a Beekeeper, I want MVP Agent log to list tool calls (name, summary args/path, call id when present), so that first value is high-signal and cheap.
21. As a Beekeeper, I want richer fields later (system prompt, reasoning) only when the provider exposes them stably, so that MVP is not blocked on incomplete vendor surfaces.
22. As a Beekeeper, I want secrets in tool args redacted or truncated in export when feasible, so that self-contained HTML/MD reports are safer to share off-machine.
23. As a Beekeeper sharing an HTML export, I want omitted Agent log to say why when useful (`no providerSessionId`, `store not found`), so that absence is diagnosable without failing the command.
24. As a runtime implementer, I want `providerSessionId` (and adapter name) on the run/session meta, so that export does not re-parse stdout to find the id.
25. As a runtime implementer, I want an optional adapter capability to resolve session logs by id, so that Cursor/Pi/Claude keep vendor details behind the adapter boundary.
26. As a Beekeeper with `command:` override on a Cursor bee, I want association to still work when stream-json is present on stdout, so that custom argv does not silently drop the id.
27. As a Beekeeper with non-stream AFK output, I want export without Agent log rather than inventing ids, so that text/json modes stay honest.
28. As a Beekeeper, I want `completion_contract` and advisory `publishes` to ignore provider log types forever, so that contracts stay domain-only.
29. As a Beekeeper reading architecture docs later, I want a clear three-way distinction: bus audit, trail comb artifacts, provider session logs, so that “artifact” / “event” / “log” do not collapse.
30. As a Beekeeper purging runs, I want provider stores outside `.paseka/runs/` left alone by `paseka purge --runs`, so that Cursor/Pi global stores are not deleted unless a separate policy says so.
31. As a Beekeeper, I want removing stream `TOOL_CALL`/`ASSISTANT_TEXT` from `events.ndjson` not to break usage aggregation, so that Slice A cleanup is safe.
32. As a Beekeeper opening Queen Console Timeline, I want fewer non-domain lines after cleanup, so that Timeline stays choreography-focused.
33. As an eval / hive-eval author, I want domain event assertions unchanged by this feature, so that evals do not depend on stream dumps in `events.ndjson`.
34. As a Beekeeper, I want interactive session stop/finish flush (deferred emit, artifacts) unchanged, so that log association is orthogonal to bus flush policy.
35. As a Beekeeper, I want `paseka inspect` (or future inspect surface) to show `providerSessionId` when present, so that I can correlate a run with Cursor UI without export.
36. As a Beekeeper, I want documentation that HITL association uses `create-chat` + `--resume`, so that operators understand why chat UUID appears in `session.json`.
37. As a Beekeeper with multiple AFK runs in one trail, I want each run’s Agent log keyed by that run’s provider id, so that tool calls do not merge across agents.
38. As a Beekeeper, I want export to remain usable offline for trail tasks/runs/bus timeline even when every Agent log section is omitted, so that provider enrichment is additive.
39. As a platform contributor, I want Slice A shippable without HITL resume wiring, so that AFK export value lands first.
40. As a platform contributor, I want Slice B to reuse the same meta field and export reader as Slice A, so that HITL does not invent a second association scheme.

## Implementation Decisions

### 1. Association field

- Persist **`providerSessionId`** (string) on the agent run, alongside existing `adapter` identity.
- AFK: canonical store is `result.json`; also rewrite `meta.json` after capture. Observers (`LoadRunMeta`, Queen Console, `paseka inspect usage --agent`) read the projection.
- AFK Pi uses the same run-local `--session-dir <runDir>/pi-sessions --session-id <agentId>` pattern as HITL, then prefers the JSON session header `id` when present.
- HITL: write onto session meta before process start; keep Paseka `sessionId` == `agentId` as today; do not overwrite Paseka ids with Cursor UUIDs.
- Optional companion: `provider` / adapter name already present; do not invent a second id namespace beyond adapter + providerSessionId.

### 2. Capture — AFK Cursor (Slice A)

- Reliable source: Cursor Agent CLI with print + `stream-json` / `json`. Docs: `session_id` is stable for the whole conversation; present on `system`/`init`, user, assistant, tool_call, and final `result`.
- Earliest capture: first non-empty `session_id` (prefer `type=system` + `subtype=init`).
- Extend stream parse to return `SessionID` (and keep usage / summary extraction). Do **not** append `TOOL_CALL` / `ASSISTANT_TEXT` protocol events into `events.ndjson`.
- Tool-call payloads for export come from provider log resolution or from an adapter-local parse used only by the export reader — not from the bus audit file.

### 3. Capture — HITL Cursor (Slice B)

- Before PTY start: run `agent create-chat` (or equivalent) to obtain a UUID.
- Store UUID as `providerSessionId` on session meta.
- Build interactive argv with `--resume=<uuid>` (plus existing workspace / model / force / prompt mapping).
- Do not rely on `--output-format` in TUI mode for association.
- Verify on real CLI that create-chat UUID is the same id used in store / resume before locking the flag shape.

### 4. No copy policy

- Runtime must not copy Cursor `store.db`, project `agent-transcripts` jsonl, or Pi session blobs into `.paseka/runs/` for this feature.
- Association pointer is enough. Export and optional inspect resolve at read time.
- Exception already in product: Pi may keep sessions under run-local `pi-sessions` by adapter design (session-dir); that is provider-owned storage colocated by choice, not a Paseka re-export copy of Cursor’s global store.

### 5. Adapter capability

- Optional adapter seam: `SessionLogResolver` resolves a tool-call summary by `providerSessionId` (+ workspace / runDir hints).
- Adapters without the capability → omit `unsupported`.
- Cursor reads local `agent-transcripts` jsonl by UUID (workspace slug first, then glob). Missing file → `store not found`; bad jsonl → `parse error`. SQLite `store.db` is a later follow-up.
- Pi remains stubbed (`not implemented`) until a session-dir reader lands.
- Claude deferred until native id/log surface is specified in a follow-up decision note.

### 6. Export enrichment

- Add include kind **`agent-logs`** (opt-in). Per-run Agent log subsection in HTML and Markdown (under Runs). MVP content: tool calls (name, path/short args, `call_id` when present, timestamps if cheap).
- Later (out of MVP priority): system prompt, reasoning/thinking blocks when provider exposes them.
- Missing id / missing store / parse error → omit section or show a one-line omitted reason; export exit code stays success for trail-only data.
- Do not merge provider tool calls into the domain Timeline section.

### 7. `events.ndjson` boundary (hard)

- Canonical meaning: audit of **domain bus** publishes for that run (emit + runtime domain synthesis).
- Remove (Slice A) writing `TOOL_CALL` and `ASSISTANT_TEXT` from Cursor/Claude stream parsers into that file.
- `IsDomainEvent` remains the gate for NATS and completion contracts; cleanup aligns the file with that mental model for Console/export readers.
- PTY transcript stays a separate file; not bus audit.

### 8. Modules (indicative)

- Cursor (and Claude) stream parse / AFK adapter — capture id; stop dumping stream diagnostics into events.
- Runs / session meta — persist `providerSessionId`.
- Session manager + Cursor session adapter — Slice B create-chat + resume argv.
- Export include + HTML/Markdown renderers — Agent log subsection under each run (`--include agent-logs`).
- Cursor/Pi `SessionLogResolver`; Cursor jsonl reader; Pi stub.
- Docs: architecture (association), interactive sessions (HITL resume), CLI (`paseka export` include), bee/event boundary reminder.

### 9. Security / size

- Truncate large tool args in MVP export.
- Prefer redacting known secret-shaped values when a shared redaction helper already exists for adapter logs; do not block MVP on perfect redaction.
- Cap number of tool-call rows per run if needed to keep HTML usable.

### 10. Phasing

| Slice | Deliverable |
| ----- | ----------- |
| A (partial) | AFK Cursor + Pi `providerSessionId` on run meta/result (export Agent log and stream→events cleanup not in this slice) |
| A (remaining) | Stop stream→events dump |
| B (partial) | HITL Cursor `create-chat` / `--resume` + Pi HITL persist `providerSessionId` (export Agent log not in this slice) |
| Export (stubs) | `--include agent-logs` + per-run Agent log; Pi stub omit `not implemented` |
| Cursor jsonl | Read-in-place `agent-transcripts/<uuid>/<uuid>.jsonl` for Agent log tool calls |
| C | Pi (and later Claude) log readers; Cursor `store.db` if jsonl is absent |

## Testing Decisions

Good tests assert external behavior: id persisted on run/session meta; export includes or omits Agent log correctly; `events.ndjson` after AFK Cursor run contains domain lines only (no `TOOL_CALL`/`ASSISTANT_TEXT` from stream); usage still on result; HITL argv contains resume id when Slice B lands.

Prior art:

- Cursor stream parse tests (summary, usage, tool_call parsing shapes) — extend for `session_id`, and assert adapters no longer append stream diagnostics as protocol events.
- Export include tests (`usage`, `artifacts`, …) — new include / section presence and omitted reasons.
- Interactive session adapter tests (Pi session-dir / session-id argv) — Cursor resume argv parallel.
- Completion contract / domain filter tests — ensure contracts still ignore non-domain types and that file content expectations for Console event feed stay domain-focused where asserted.

Prefer fixtures with synthetic stream lines and fake log reader seams over live Cursor CLI in unit tests. Optional smoke: real `create-chat` only in manual/dev notes, not required CI.

## Out of Scope

- Publishing provider tool calls or assistant text as NATS domain events.
- Making provider log enrichment required for successful export or successful bee runs.
- Copying Cursor global chat DBs into the colony for archival/replication across machines.
- Replacing Queen Console live PTY with a structured chat UI fed by provider logs (may be a later Console spec).
- Cross-process or remote fetch of another host’s `~/.cursor` store.
- Full reasoning / system-prompt dump in MVP.
- Changing honey / energy accounting based on provider sessions.
- MCP wrappers for session log read.
- Telegram or Human Gateway surfaces for provider logs.
- Automatic resume of a previous trail’s provider session across a new `agentId` (association is per run; deliberate HITL continuation is [025](./025-cursor-session-resume.md)).
- Renaming Paseka `sessionId` away from `agentId`.

## Further Notes

- Related: [001 Pi integration](001-pi-integration.md) (run-local `--session-id` / `--session-dir`), [002 Queen Console](002-queen-console-mvp.md) (transcript vs PTY), [014 artifacts](014-artifacts-protocol.md) (trail comb ≠ provider logs), [025 Cursor HITL resume](025-cursor-session-resume.md) (new Paseka session, same `providerSessionId`). Architecture overview already states domain events are not inferred from assistant stdout — this spec extends that boundary to “do not park CLI stream diagnostics in the bus audit file.”
- Naming: user-facing **Agent log** / **provider session**; wire field **`providerSessionId`**. Avoid calling these “events” in UI copy.
- After implementation: update changelog, CLI export help, interactive-sessions guide (HITL resume), architecture adapter result-collection notes; set status to Implemented.
- Ask before promoting this Draft to **Approved** after review.
