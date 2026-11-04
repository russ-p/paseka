# Spec 048: Native session transcript

## Status

**(Approved)**
Agreed with the Beekeeper after measuring real sessions: reasoning is its own include, `transcript` joins the `--include` family, first page is 100 turns, caps are 8 k text / 16 k reasoning / 2 k tool input / 16 k tool output, Console panel ships as a separate spec, redaction is Paseka's own.

Replaces the tool-call-only provider log seam of [021](021-provider-session-logs-export.md) with one adapter capability that returns a normalized native conversation, and moves filtering to the consumers. Adds `paseka export --include transcript` and a separate `--include reasoning`, fills the Pi reader, and shapes the capability so a paged Console view can be built on it — the Console panel itself is a separate spec. Claude stays out of scope; OpenCode v2 must not read SQLite.

Related: [021](021-provider-session-logs-export.md) owns the association field and the shipped Cursor Agent log; [026](026-opencode-adapter.md) and [037](037-opencode-v2-proxy.md) own the OpenCode lines and list transcript hydration as a follow-up; [030](030-opencode-session-resume.md) and [025](025-cursor-session-resume.md) own resume; [032](032-queen-console-term-engine.md) owns the terminal engine; [035](035-queen-console-redesign.md) owns Console UI.

## Problem Statement

Paseka can already point at a provider conversation: Cursor AFK and HITL store `providerSessionId`, OpenCode and Pi do the same, and `paseka export --include agent-logs` reads Cursor's transcript in place to build a tool-call table. That association is correct and worth keeping. What sits on top of it is too narrow, and the one place where the operator actually reads a conversation is still fed by the wrong source.

**The adapter seam returns only tool calls.** Its result type is a list of `{name, callID, args}` rows. Everything else the provider recorded — user turns, assistant text, reasoning — is thrown away by the reader before any consumer sees it, so no consumer can ask for a conversation even when the data was on disk one line away. Adding a second use case means either a second interface over the same files or a breaking change to the first.

**A HITL transcript is a rendering, not a conversation.** For an interactive session the only per-turn record Paseka keeps is the PTY capture. That file is a stream of terminal writes: escape sequences, cursor moves, partial redraws, spinners, and line rewrites. Reading it back produces something between "unreadable" and "wrong order", and for a TUI-heavy provider it is closer to unreadable. The provider, meanwhile, has already written a clean, ordered, structured conversation to its own store. The operator's view is built from the noisier of the two sources, permanently.

**Filters live in the wrong place.** The tool-call summary, the export limits (100 rows, 200 runes per argument) and the "omit quietly" policy are all baked into the export path. A different consumer that wants a dialogue — the Console conversation, an `inspect` view, a future eval report — cannot reuse any of it and would end up with its own copy of the limits and its own idea of what "no data" means.

**Pi has no reader at all.** Its sessions are an append-only JSONL inside the run's own `pi-sessions` directory — the easiest of all the providers to read, fully local, no glob, no lock — and it is still stubbed as `not implemented` because the seam it had to implement was the narrow one.

**OpenCode looks harder than it is.** The current-generation store is one large SQLite database under a background service, which makes "read the provider's files" sound like adding a database driver and a schema dependency to a runtime that has neither. But OpenCode also ships a JSON export of a whole session and a full HTTP API, and Paseka already talks to that API for usage. The capability needs to say which route is acceptable, so the next contributor does not reach for the database.

## Solution

One adapter capability answers "give me this provider conversation, in this much detail". Consumers ask for what they render.

1. **The capability returns a normalized transcript.** A provider-native session is resolved into ordered turns with typed parts: text, reasoning, tool. Tools carry identity and status always, and input/output only when asked for.

2. **Detail is a request, not a reader variant.** The caller passes a payload mask — text, reasoning, tool input, tool output — plus per-part caps. A tool-call view asks for tool inputs only and never materializes tool outputs; a dialogue view asks for text only and gets tool names without arguments or results. Adapters skip work they were not asked to do, so the cheapest view is actually cheap.

3. **Filters are projections, and they are shared.** `agent-logs` becomes a projection over the transcript, producing exactly today's table. The new dialogue projection produces user/assistant turns with tool calls collapsed to one line. Export and any Console panel call the same two projections, so limits and omit reasons cannot drift.

4. **Every provider takes its best native route, with dependency rules per provider.** Cursor: the existing transcript JSONL, in place. OpenCode 1: the storage JSON tree, read with the standard library. OpenCode 2: the provider's own JSON export subcommand, or the provider's HTTP API when the adapter already owns a server for that session — **never** the SQLite store, and no new third-party dependency anywhere. Pi: the run-local session JSONL. Claude: unsupported for now.

5. **The Console reads the transcript through the same capability, in pages.** The terminal pane still shows the live TUI; a conversation panel shows ordered turns — the thing that PTY capture cannot produce. The panel and its UI are a separate spec; this capability only has to answer "give me the next page of turns" cheaply. A session with no resolvable provider transcript keeps the PTY capture as a clearly labelled fallback.

6. **Nothing is copied.** Provider stores are read at view time, as in [021](021-provider-session-logs-export.md). Export still renders a self-contained file; the runtime still does not archive vendor stores into the runs tree.

## User Stories

1. As a Beekeeper, I want `paseka export --include agent-logs` to keep producing the same tool-call table, so that existing reports do not change shape.
2. As a Beekeeper, I want `--include transcript` to add a per-run conversation to an export, so that I can read what was actually said without the Cursor or OpenCode UI.
3. As a Beekeeper, I want the conversation to show user and assistant turns in order, so that it reads as a dialogue rather than a log.
4. As a Beekeeper, I want tool calls in the conversation to appear as a single line with the tool name and status, so that long file dumps do not bury the dialogue.
5. As a Beekeeper, I want tool arguments and results available in the Agent log but not duplicated in the conversation, so that one export does not carry the same payload twice.
6. As a Beekeeper, I want reasoning to be its own include, so that a dialogue export never carries thinking blocks unless I asked for them.
7. As a Beekeeper, I want `--include reasoning` to be enough on its own, so that one flag gives me a conversation with thinking rather than a flag that silently does nothing.
8. As a Beekeeper opening a finished Cursor HITL session, I want its conversation readable from Cursor's own transcript, so that I stop reading terminal escape noise.
9. As a Beekeeper opening a finished OpenCode HITL session, I want the same conversation available, so that the two interactive providers behave alike.
10. As a Beekeeper opening a finished Pi HITL session, I want the same conversation available, so that all three supported providers are readable.
11. As a Beekeeper looking at a session whose transcript is gone, I want the PTY capture to stay available as raw data, so that the session is not lost just because the provider store is.
12. As a Beekeeper, I want the terminal pane to keep working as a terminal, so that transcript reading does not cost me attach.
13. As a Beekeeper with an AFK Cursor run, I want its conversation in the export, so that a headless trail is analysable the same way as an interactive one.
14. As a Beekeeper with an AFK OpenCode run, I want its conversation in the export, so that the JSON event stream is not my only record.
15. As a Beekeeper with an AFK Pi run, I want its conversation from the run-local session file, so that no global store is needed at all.
16. As a Beekeeper whose Pi run used several session files in one directory, I want the one matching `providerSessionId` chosen, so that a resumed or branched session does not mix turns.
17. As a Beekeeper whose OpenCode session ran on the 1.x line, I want the transcript read from the storage tree, so that upgrading Paseka does not orphan old history.
18. As a Beekeeper whose OpenCode session ran on the 2.x line, I want the transcript read through OpenCode's own export or API, so that I do not depend on an internal database schema.
19. As a platform contributor, I want the OpenCode generation split hidden behind the `opencode` adapter name, so that export, Console, and trails keep one adapter identity.
20. As a platform contributor, I want no SQLite driver, no provider SDK, and no new third-party module for this feature, so that the runtime's dependency surface stays where it is.
21. As a platform contributor, I want a session store that cannot be read to produce a stable omit reason, so that everything above it treats absence uniformly.
22. As a platform contributor, I want one shared set of omit reasons and one shared truncation helper, so that "missing", "unreadable", and "too big" read the same everywhere.
23. As a Beekeeper, I want a transcript with no data to be omitted with a reason rather than failing the export, so that a stale pointer never breaks a report.
24. As a Beekeeper, I want a giant tool output truncated to a capped length, so that one `cat` of a lockfile cannot make an export unopenable.
25. As a Beekeeper, I want per-part caps requested by the consumer, so that the dialogue view and the tool-call view can be capped differently.
26. As a Beekeeper, I want truncation to be visible in the output, so that I know I am reading a prefix.
27. As a Beekeeper sharing an HTML export, I want obvious secrets in transcript text and tool arguments redacted or truncated, so that a report can leave the machine.
28. As a Beekeeper, I want the conversation view to be read-only, so that nothing I do in an export or Console can mutate a provider session.
29. As a Beekeeper, I want a transcript view for a failed or cancelled run too, so that a run that died still has its dialogue.
30. As a Beekeeper resuming a chain of sessions on one provider id, I want each Paseka run to show the conversation as it stood at that run, so that a resumed chain does not collapse into one entry.
31. As a Beekeeper with multiple agents in one trail, I want one conversation per run, so that turns never merge across agents.
32. As a Beekeeper on another machine, I want a transcript-less export to succeed, so that a report remains portable even when the provider store is not.
33. As a Beekeeper pruning runs, I want provider stores left untouched, so that `prune` does not delete conversations Paseka does not own.
34. As a Beekeeper, I want the exported conversation ordered by provider time, so that it matches the provider's own history view.
35. As a Beekeeper, I want the system prompt excluded from the dialogue by default, so that a 10 KB preamble does not dominate the first screen.
36. As a Beekeeper, I want the system prompt available in the full view, so that a misbehaving run can still be diagnosed.
37. As an adapter author, I want to implement one method that returns a transcript, so that adding a provider does not mean implementing a second reader for summaries.
38. As an adapter author, I want the request to tell me which payloads are wanted, so that I can skip expensive reads without guessing the consumer.
39. As an adapter author for a provider with no readable store, I want to return `unsupported` and stop, so that the seam stays honest instead of faking data.
40. As a Console implementer, I want to ask for a page of turns with a cursor for the next page, so that a long session opens fast instead of being read end to end.
41. As a Console implementer, I want a reasonably large first page, so that most sessions never need a second request.
42. As a Console implementer, I want a provider that cannot window to still answer — a full result plus a done cursor — so that pagination degrades rather than fails.
43. As a Console implementer, I want an explicit has-more signal, so that the view can stop offering "load more" honestly.
44. As a Console implementer, I want the live session to answer for completed turns, so that I can read back a long turn without scrolling the TUI.
45. As a runtime implementer, I want transcript resolution bounded by a context deadline, so that a wedged provider command cannot stall an export or a request.
46. As a runtime implementer, I want resolution to be read-only and side-effect free, so that it is safe to call from an HTTP handler.
47. As an eval author, I want no domain events produced from transcript data, so that choreography assertions stay unaffected.
48. As a Beekeeper, I want the behaviour documented once — association, Agent log, transcript, reasoning, PTY role — so that "log", "transcript", and "event" stop meaning the same thing in conversation.

## Implementation Decisions

### 1. One capability, richer result

The provider-log seam becomes a transcript reader. It keeps the existing association request fields (provider session id, adapter name, workspace, colony root, run directory, agent id) and adds three:

- **include mask** — which payloads the caller wants: text, reasoning, tool input, tool output. Tool identity (name, call id, status) is always present because it is tiny and every view uses it.
- **limits** — per-part rune caps for text, reasoning, tool input, and tool output, applied by the adapter through one shared helper so truncation is uniform.
- **window** — a page request: turn offset, turn limit, and an opaque cursor returned by the previous call. The default first page is **100 turns**, which covers 92 % of Cursor, 91 % of OpenCode, and 82 % of Pi sessions by measurement; 150 turns raises that to 99 / 95 / 95 %.

Defaults are measured, not guessed (this machine, 2026-11: 2066 Cursor transcripts, 327 Pi sessions, 629 OpenCode sessions):

| Measure | Cursor | Pi | OpenCode |
| --- | --- | --- | --- |
| turns/session p50 / p90 / p95 / max | 12 / 45 / 64 / 440 | 18 / 83 / 148 / 393 | 22 / 90 / 144 / 1421 |
| sessions within 100 turns | 98 % | 92 % | 91 % |
| dialogue runes per turn (median) | 449 | 281 | 219 |
| text block runes p90 / p99 / max | 926 / 13.9k / 261k | 3.9k / 28.4k / 268k | 1.7k / 17.8k / 113k |
| reasoning block runes p90 / p99 / max | none recorded | 1.3k / 10.8k / 180k | 2.6k / 11.8k / 52k |
| tool calls per session p50 / p90 | 25 / 94 | 20 / 82 | 29 / 115 |
| tool output runes p90 / p99 / max | 752 / 5.8k / 60k | 4.3k / 20.6k / 268k | 6.9k / 30k / 806k |

Page cost follows from the last two rows: a 100-turn dialogue page is roughly 20–45 k runes of text, while the same session's full payload including tool output has a p90 near 0.5 M runes. That gap is why the dialogue view never requests tool output.

The result is a normalized transcript: ordered turns (`user`, `assistant`, `system`), each with a timestamp and typed parts (`text`, `reasoning`, `tool`), plus an omit reason, a truncated flag, and a has-more signal (with the next cursor) for windowing. A tool result is attached to the call it answers, by call id, whichever way the provider stores it — nested in the call part, or as a separate result message — so consumers never see that difference. The shipped tool-call type and its omit constants stay; the tool-call list becomes a projection over the transcript rather than the resolver's own return type.

Consumers use named presets rather than assembling masks by hand: a tool-call preset (tool input only), a dialogue preset (text only), a dialogue-with-reasoning preset (text and reasoning), and a full preset (everything). A new consumer picks a preset; it does not invent limits.

Cap defaults, from the same measurement: dialogue text 8 000 runes, reasoning 16 000 (above p99 for both providers that record it), tool input 2 000 (≈ p95), tool output 16 000 (≈ p97, full view only). The Agent log keeps its shipped 200-rune argument summary and 100-row cap — the row cap sits at ≈ p90 of tool calls per session, so it trims the tail rather than the common case.

Windowing is best effort and never a correctness requirement. An adapter whose format allows a cheap slice (line-oriented JSONL, an API with paging) may use the cursor; one that cannot (a whole-session JSON export) returns everything and, if the returned turns were cut to the requested page, reports that honestly through the cursor signal. A provider must never answer a window with an omission — paging is not a read failure.

### 2. Filtering lives above the adapter

Two pure projections, shared by export and any future Console work:

- **Tool calls** — today's Agent log: name, short arguments (path/url/command preference), call id, capped rows.
- **Dialogue** — user and assistant turns with text, tool calls collapsed to name and status, reasoning only when the consumer asked for it, system turns excluded by default.

Projections take the result type and return render-ready rows; they perform no IO and hold no provider knowledge. Export formats and a Console renderer consume rows, never parts. A tool-call view therefore never materializes tool outputs, and a dialogue view never carries file dumps — the savings come from the request mask, and the projection only shapes what was asked for.

### 3. Provider routes (durable rules)

- **Cursor** — read the existing per-project agent transcript JSONL in place. The same file already contains user and assistant text parts, so one reader serves both projections; no new source is introduced. Measured across 2066 local transcripts it records **no reasoning parts at all**, so the reasoning include is effectively Pi- and OpenCode-only and Cursor must answer it with an empty reasoning set rather than an omission. Workspace-slug-first-then-glob lookup and the UUID guard stay.
- **OpenCode 1** — read the storage JSON tree in place with the standard library: session record, then messages keyed by session id, then parts keyed by message id. Message and part directories are keyed by provider ids, so no project-hash search is needed below the session lookup.
- **OpenCode 2** — use the provider's own JSON export subcommand for the session, tolerantly decoding the `{info, messages}` shape; when the adapter already owns a server for that session, prefer the provider's HTTP API. The SQLite store is **not** opened: no database driver, no schema coupling, no lock handling.
- **Pi** — read the run-local `pi-sessions` JSONL. Entries are typed (`session` header, `message`, plus non-conversation records that are ignored); message roles are `system`, `user`, `assistant`, `toolResult`; content parts are text, thinking, and tool call. A call carries its `arguments` and the result arrives in a later `toolResult` message linked by `toolCallId` with an error flag — the reader folds it into the call so the normalized shape does not leak that difference. The file whose header id matches the provider session id wins; otherwise the newest file in the directory.
- **Claude** — out of scope, returns the existing `unsupported` reason.

The OpenCode generation split stays behind the `opencode` proxy: callers ask for adapter `opencode`, and the proxy dispatches to the frozen 1.x reader or the 2.x reader by the same detection it already uses for launches. Adapters that cannot resolve return an omit reason and no error.

### 4. Dependency rule (hard)

No new third-party dependency for this feature. Resolution uses the standard library, the provider's files, the provider's CLI subcommand, or the provider's HTTP API. A session store that would require a database driver or a vendor SDK is out of bounds; the correct move there is to find the provider's export surface, and if none exists, return `unsupported`.

### 5. Export surface

Two new opt-in kinds join the existing `--include` family, next to `agent-logs`:

- **`transcript`** — a per-run Conversation section: turns headed by role, tool calls as one line, truncation noted, omission as a one-line reason.
- **`reasoning`** — the same section with thinking blocks, read through the same capability with the reasoning payload requested. Because a bare `reasoning` that changes nothing would be a trap, `reasoning` implies `transcript`; spelling out both is allowed and equivalent.

`--include agent-logs` keeps its name, section, and content. The includes are independent in the sense that each can be requested alone; a run may show the tool-call table, the conversation, both, or neither, and no section can fail the export.

### 6. Console readiness (the panel is a separate spec)

The Conversation panel, its layout, the PTY fallback labelling, and the live polling are **not** in this spec; they get their own. What this spec owes that panel is the ability to open a long session without reading all of it: a default first page large enough that ordinary sessions fit in one request, a cursor for the next page, and a has-more signal that does not lie. Resolution stays a read-only snapshot, so the panel can poll it for completed turns while the terminal pane keeps its PTY relay untouched.

### 7. Read-only, bounded, side-effect free

Resolution runs under a caller-supplied context deadline, opens provider artefacts read-only, never writes to the provider store, never starts a session, never sends a prompt, and returns nothing but data. A failed read is an omit reason, not an error. Repeated resolution is expected and must be cheap enough for an HTTP handler and an export loop.

### 8. Redaction and size

Truncation is Paseka's, applied by one shared helper; tool outputs are only read when the mask asks for them, are capped hard, and default to a small cap. The conversation view never inlines tool output; the full view does, within its cap.

Secrets are also Paseka's problem, not the provider's: the resolver applies the same redaction to text, reasoning, and tool arguments that the rest of the runtime applies to adapter logs, so all providers behave alike and switching providers does not silently change what is safe to share. A provider's own sanitize option is not the primary mechanism — it is at best a redundant second pass, and a transcript read must not depend on it. The broader mechanism this should converge on is tracked as a backlog item, since exports, proposals, and logs all need it.

### 9. What PTY remains

The PTY capture is not cleaned, post-processed, or re-rendered into a dialogue — that was the dead end this spec avoids. It stays a raw terminal record for attach, replay, and fallback. Documentation must keep the three-way distinction explicit: bus audit events, trail comb artifacts, provider transcript.

### 10. Phasing

| Slice | Deliverable |
| --- | --- |
| A | Capability, request/result types, include mask and limits, shared truncation and omit reasons, tool-call projection; Cursor reader extended to text parts; `--include agent-logs` re-pointed at the projection with identical output |
| B | `--include transcript` and `--include reasoning` in HTML and Markdown, dialogue projection, caps |
| C | OpenCode readers: 1.x storage tree, 2.x export/API; dispatch behind the `opencode` proxy |
| D | Pi reader over the run-local JSONL |
| E | Windowed reads (turn offset, cursor, has-more) shaped for the Console panel, including the "provider cannot window" degradation |
| F | Docs: export include table, interactive sessions, architecture adapter notes; spec status |

Slices A and B are independently shippable and fix the export story. Slice E ships the capability the Console panel will consume; the panel itself is a separate spec and may land after it.

## Testing Decisions

Good tests assert external behaviour: which sections an export contains, which parts a view shows, what a page contains, and what happens when a store is missing or unreadable. No test should assert internal helper names or reader structure.

- **Capability contract tests** — fixtures with a large tool output prove that a tool-call request does not populate texts or outputs, and a dialogue request does not populate tool input or output; both still carry tool name, call id, and status. This is the test that keeps the mask honest.
- **Per-provider reader tests** — table-driven fixtures: Cursor JSONL with text and tool parts; OpenCode 1 storage tree; OpenCode 2 export JSON and API stub; Pi JSONL with system, user, assistant, tool-result roles and multiple files in one directory. Prior art: the existing Cursor transcript tests, Pi session-dir usage tests, and the OpenCode 2 HTTP stub.
- **No-database test** — resolving an OpenCode 2 session with only a SQLite store present (no export, no server) yields an omit reason; the reader never opens the file.
- **Projection tests** — Agent log output is byte-identical to the shipped table for the same fixture; dialogue collapses tools to one line, drops reasoning and system turns by default, keeps them when the reasoning preset is used, and preserves ordering.
- **Export tests** — include presence, `transcript` and `reasoning` separately and together, omission wording, truncation markers, and that a resolve error or timeout never fails the command. Prior art: the existing agent-log export tests.
- **Window tests** — a windowed request returns the requested page with a has-more signal; a provider that cannot window returns the whole transcript and does not fake a cursor; asking for a second page past the end is empty, not an omission.
- **Deadline test** — a resolver that blocks is cut off and reported as an omission, not a hang.

Fixtures and stubs only: no real provider binary, no network, no account.

## Out of Scope

- Claude reader (the provider is not installed for this Beekeeper, and its session surface is unverified).
- The Queen Console Conversation panel — its API shape, layout, PTY fallback labelling, and live polling are a separate spec that consumes this capability.
- Cursor `store.db` and any other SQLite or proprietary store read.
- Rich rendering: markdown bodies, syntax highlighting, diffs, images, attachments.
- Live streaming of transcript updates over WebSocket or SSE.
- Editing, annotating, or resuming from a transcript view.
- Publishing transcript content as domain events.
- Cross-machine or remote transcript fetch; provider stores stay machine-local.
- Per-turn provider diffs, cost breakdowns per part, and other analytics beyond usage already on the run.
- Downscaling or re-rendering the PTY capture.
- A general secret-redaction mechanism across exports, proposals, and logs — only the transcript path applies redaction here; the shared mechanism is tracked as a backlog item.
- Changing `providerSessionId` capture, resume eligibility, or the adapter launch contract.

## Further Notes

- Naming: **provider transcript** is the data; **Agent log** is the tool-call projection; **Conversation** is the dialogue projection. Documentation should stop calling any of them "events".
- Settled: reasoning is a separate `--include` that implies `transcript`; `transcript` lives in the existing `--include` family; the Console panel ships separately and paginates — 100 turns per page, then a cursor; redaction is Paseka's own, not the provider's sanitize option; caps are 8 k text, 16 k reasoning, 2 k tool input, 16 k tool output.
- The measurement above is a snapshot of one machine and will drift; it exists to justify the defaults, not to be re-derived in tests. Tests pin the defaults as constants and assert behaviour at the boundary, not the distribution.
- Pi's session JSONL is run-local by adapter design, so its reader has no glob and no cross-run ambiguity — it should be the simplest reader and the reference implementation for the seam.
- Spec [021](021-provider-session-logs-export.md) remains the record of `providerSessionId` capture and the no-copy policy; when this spec ships, 021's Agent log sections become projections and its Pi stub is retired.
