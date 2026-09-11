# Spec 030: Resume OpenCode HITL Sessions

## Status

**(Implemented)**
Adapter pre-create (`opencode serve` + `POST /session`), `--session` launch, resume eligibility, Console Resume, and `paseka session resume` are in the tree.

## Problem Statement

Spec [025](./025-cursor-session-resume.md) made a finished **Cursor** HITL chat continuable by copying its stored `providerSessionId` into a new Paseka session and launching the Cursor TUI with `--resume <uuid>`. Spec [026](./026-opencode-adapter.md) brought OpenCode in as a first-class adapter but deliberately left OpenCode HITL `providerSessionId` empty: there was no Cursor-style `create-chat`, so a fresh OpenCode TUI run had no native id to store, and Resume was impossible.

That gap is now closable. OpenCode exposes a headless server (`opencode serve`) whose `POST /session` endpoint allocates a session **without running a turn** and persists it to OpenCode's store. That is the missing `create-chat` analog. Without using it, an OpenCode Beekeeper who finishes a HITL chat cannot continue that same conversation in Paseka.

## Solution

For a **new** OpenCode HITL session, Paseka pre-creates the provider session before starting the TUI:

1. Start `opencode serve` on a local ephemeral port with a generated `OPENCODE_SERVER_PASSWORD`.
2. `POST /session` with the workspace directory.
3. Read `id` (`ses_*`) from the response.
4. Stop the server (the session is already durable in the shared OpenCode store).
5. Launch the TUI with `--session <ses_*>`; store `ses_*` as `providerSessionId`.

For **Resume**, Paseka skips pre-create and launches the TUI with `--session <stored providerSessionId>` on a new Paseka session (`resumedFrom` recorded), mirroring Cursor. OpenCode resume joins Cursor as an eligible source in the session manager; Pi and Claude stay ineligible.

If pre-create fails (no `serve` support, HTTP error, timeout), the TUI still launches without `--session` and `providerSessionId` stays empty — the same fail-soft policy Cursor uses when `create-chat` fails. Domain events, honey, and the bus are untouched.

## User Stories

1. As a Beekeeper, I want a new OpenCode HITL session to store a real `providerSessionId`, so that the chat can be found and resumed later.
2. As a Beekeeper, I want pre-create to run **no** model turn, so that opening a session costs nothing before I type.
3. As a Beekeeper, I want the TUI launched with `--session <ses_*>`, so that my first turn lands in the pre-created chat.
4. As a Beekeeper on a finished OpenCode HITL session, I want Resume in Queen Console, so that I continue the same conversation.
5. As a Beekeeper, I want `paseka session resume <sessionId>` to work for OpenCode, so that I have CLI parity with Cursor.
6. As a Beekeeper, I want Resume to skip pre-create, so that a continuation never silently becomes a new empty chat.
7. As a Beekeeper, I want an optional one-line continue message passed as `--prompt`, so that I can nudge the agent as the first new turn.
8. As a Beekeeper who types nothing extra, I want the TUI to open on the existing chat with no kickoff replay, so that the agent does not redo the original task.
9. As a Beekeeper, I want that continue line to bypass the bee task template, so that Resume is not a second new-task launch.
10. As a Beekeeper, I want a new Paseka `sessionId` / run directory for the continuation, so that the source run stays an honest historical record.
11. As a Beekeeper, I want the new session meta to record `resumedFrom`, so that the chain is visible in Console and inspect.
12. As a Beekeeper, I want the source `session.json` untouched, so that history is immutable.
13. As a Beekeeper, I want Resume on `completed`, `failed`, or `cancelled` sources, so that a crashed or stopped chat is still continuable.
14. As a Beekeeper on an `active` source, I want Resume refused, so that I attach or wait instead of racing a live TUI.
15. As a Beekeeper, I want Resume refused when another active session already uses the same `providerSessionId`, so that two PTYs do not fight over one chat.
16. As a Beekeeper whose source has no `providerSessionId`, I want Resume disabled with a reason, so that I am not offered a dead control.
17. As a Beekeeper whose bee is gone or no longer `adapter: opencode`, I want Resume refused, so that a renamed colony does not launch the wrong tool.
18. As a Beekeeper whose bee uses a `command:` override, I want Resume refused, so that custom argv is not rewritten.
19. As a Beekeeper on a Pi, Claude, or script HITL session, I do not want Resume, so that an OpenCode-shaped `--session` is not applied to another provider.
20. As a Beekeeper, I want current bee params (model alias, plan, variant) applied at resume time, so that I am not frozen to the original launch.
21. As a Beekeeper, I want Resume on the source `traceId`, so that the worktree and Flight Trail context stay aligned.
22. As a Beekeeper, I want Resume to skip honey, so that continuing a chat is not a second `session.start` tax.
23. As a Beekeeper, I want Resume sessions owned by the current `paseka console` process, so that browser PTY attach works even when the source ran in Ghostty or another shell.
24. As a Beekeeper, I want to land on the new session after Resume, so that I do not hunt for it.
25. As a Beekeeper, I want the new Console transcript to start empty, so that I am not shown a fake reconstruction of OpenCode history.
26. As a Beekeeper, I want pre-create failure to degrade to a normal TUI launch, so that a missing `serve` feature never blocks me.
27. As a Beekeeper, I want a custom `binary` override used for both `serve` and the TUI, so that a custom install path works end to end.
28. As a Beekeeper with a `command:` override, I want an explicit `--session` / `-s` in the argv stored as `providerSessionId`, so that association still works.
29. As a Beekeeper, I do not want AFK `opencode run` `providerSessionId` treated as a HITL source, so that stream-json vs TUI resume is not assumed.
30. As a Beekeeper, I want `paseka doctor`, status, and kill to treat OpenCode resumes like any other session.
31. As a platform contributor, I want pre-create to be a best-effort capability, so that it can be faked in tests without a real OpenCode install or network.
32. As a platform contributor, I want tests that lock argv, eligibility, and identity without calling a real `opencode` binary, so that CI stays hermetic.
33. As a Beekeeper reading docs, I want the interactive-sessions guide and CLI reference to describe the pre-create and Resume paths, so that I can reason about `providerSessionId`.
34. As a Beekeeper, I want Honey charging unchanged (one dispatch), so that OpenCode resume does not get a special energy rule.
35. As a Beekeeper, I do not want domain bus events invented from Resume, so that choreography stays explicit (`paseka event emit`).

## Implementation Decisions

### 1. Pre-create is the OpenCode `create-chat`

- `opencode serve --hostname 127.0.0.1 --port <ephemeral>` with `OPENCODE_SERVER_PASSWORD=<random>` on the process env.
- Wait for the server to accept `POST /session` (retry until a short timeout).
- Body `{"directory": "<workspace>"}`; basic auth username `opencode`.
- Response `id` (`ses_*`) is returned; the server process is killed. Sessions are durable in the shared OpenCode store, verified to survive server exit.
- Uses the resolved adapter binary (home `adapters/opencode.yaml` `binary`, `params.binary`, or default `opencode`).
- Best-effort: any error logs a warning and yields an empty id; the TUI still launches.

### 2. Interactive argv

- New HITL: `--session <ses_*>` when pre-create succeeded; otherwise no `--session`.
- Resume: `--session <stored>` always; never pre-create.
- `--prompt` carries the joined system+task kickoff on new sessions; on resume it carries only the optional continue line (never the system prompt).
- `--agent plan`, `--model`, `--variant` unchanged. `--auto`, `--format`, `--title`, `--dir` remain AFK `run`-only.

### 3. `command:` override

- When `req.Command` is set, Paseka never calls `serve`/`POST /session`.
- If the argv contains `--session <id>` or `-s <id>`, that value is stored as `providerSessionId`; otherwise it stays empty.

### 4. Identity

- New session: provider `ses_*` is stored as `providerSessionId` on `session.json` and `meta.json` before the PTY starts.
- Resume: new Paseka `sessionId` / `agentId` / run dir; copy `providerSessionId`; record `resumedFrom` (source Paseka session id). Never overwrite Paseka ids with the OpenCode id, and never rewrite the source.

### 5. Eligibility (all must hold)

- Source session exists on this colony (in-process manager, home registry, or run-tree `session.json`).
- Source is **not** active (a registry row with a dead PID does not block).
- No other active session on this machine/colony shares the `providerSessionId`.
- Source adapter is in the resumable set (`cursor`, `opencode`).
- `providerSessionId` is non-empty.
- Source bee still loads, resolves to the **same** adapter as the source, supports interactive sessions, and has **no** `command:` override.
- Source `traceId` and bee role are reused; the client does not choose another bee or trace.

The generic ineligible code becomes `not_resumable` (was `not_cursor`); `adapter_changed` keeps its meaning.

### 6. Console and CLI

- `POST /api/sessions/{sessionId}/resume` and `paseka session resume <sessionId>` are unchanged in shape; they now accept OpenCode sources.
- The Console session detail shows Resume when `adapter` is `cursor` or `opencode`; the button remains disabled without a `providerSessionId`.
- `POST /api/sessions` still always starts a **new** chat.

### 7. Honey and bus

- Resume does not consume honey, accept invites, or publish session-lifecycle events.

## Testing Decisions

Good tests assert external behavior and stay hermetic:

- Argv mapping: new HITL includes `--session <id>` after a stubbed pre-create; resume includes `--session <stored>` and no kickoff; pre-create failure omits `--session`; `--auto`/`--format`/`--title`/`--dir` never appear in the TUI argv.
- Pre-create stub: assert it is called with the resolved binary and workspace on new sessions, and **not** called on resume or `command:` override.
- `command:` override with `--session` / `-s` stores the id; without it stays empty.
- Resume manager: OpenCode happy path (copy id, `resumedFrom`, new session id, source unchanged); OpenCode resume with empty vs non-empty continue; ineligible Pi/Claude/script (`not_resumable`); missing id; active source; busy provider id; bee gone; adapter changed; `command:` override; chain resume.
- Console handler: OpenCode resume returns 201; non-resumable returns 400 `not_resumable`.
- Static contract: the SPA Resume control accepts `opencode`.

Do not require a real OpenCode install, `serve`, or network. A package-level pre-create seam is overridden in the adapter tests; manager tests use the existing recording session adapter.

## Out of Scope

- AFK `opencode run` → HITL resume (stream-json `providerSessionId` vs TUI `--session` is not assumed).
- Resume from Queen Console **Runs**.
- Pi (`--session-id` of the source run) and Claude resume.
- Bees with `command:` overrides being rewritten or double-resumed.
- Reusing the source Paseka `sessionId` / run dir / PID, or a `resumedBy` back-pointer.
- Structured chat UI or hydrating the Console transcript from OpenCode's store.
- Cross-process attach to the original PTY; Resume starts a new process.
- `opencode serve` lifecycle beyond pre-create (no long-lived Paseka-managed server), `--attach`, ACP, MCP.
- Telegram / other Human Gateway Resume.
- Honey, invite re-accept, bus session lifecycle events.
- Changing Paseka `sessionId` away from `agentId`.

## Further Notes

- Verified against OpenCode `1.18.30`: `POST /session` returns `cost: 0`, `tokens: 0`, persists to `~/.local/share/opencode/opencode.db`, survives `serve` exit, and is readable via `opencode export` / `opencode session list --format json`.
- `opencode run` refuses to create a session with no message ("You must provide a message or a command"); a throwaway AFK turn is **not** used as `create-chat`.
- `--continue` exists but resumes the most recent session globally, so it is not a substitute for a deterministic resume-by-id.
- Capturing the exit banner (`Continue opencode -s ses_…`) or polling `opencode session list` are possible fallbacks but are non-deterministic (ANSI, hub-only, timing); pre-create is the primary path.
- Related: [026-opencode-adapter](./026-opencode-adapter.md), [025-cursor-session-resume](./025-cursor-session-resume.md), [021-provider-session-logs-export](./021-provider-session-logs-export.md), [interactive sessions](../guide/interactive-sessions.md).
