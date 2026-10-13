# Spec 037: OpenCode V2 Adapter and Version-Proxy Dispatch

## Status

**(Approved)**
Three adapters behind one name: today's OpenCode adapter becomes `opencodev1`, a new `opencodev2` adapter covers OpenCode 2, and `adapter: opencode` becomes a thin proxy that dispatches to one of them. Settled with the Beekeeper: run meta keeps the configured adapter name, cross-major Resume is refused as ineligible, cross-major session continuity is neither verified nor promised, and the console shows a single `opencode` adapter row.

## Problem Statement

Every colony configured with `adapter: opencode` is silently broken the moment OpenCode 2 lands on the machine.

The breakage is not uniform, which is what makes it dangerous:

- **AFK runs partially fail.** OpenCode 2 dropped `opencode run --dir`, so a Bee that relied on `--dir` to scope work to its isolated worktree now runs in whatever directory the server considers default. Workspace isolation quietly stops being isolation.
- **AFK runs lose reasoning and cost accounting.** The `--variant` flag is gone (the variant now rides on the model reference as `provider/model#variant`), and a single-step run no longer necessarily emits a token-bearing `step_finish` event, so `usage` can go missing with no error.
- **HITL sessions cannot start at all.** OpenCode 2's TUI rejects `--model`, `--agent`, `--variant`, `--port`, and `--hostname`; it exits immediately with `Unrecognized flag`. Every `paseka bee chat opencode …` fails.
- **The HITL pre-create path deadlocks for 20 seconds.** The old pre-create talks to a V1 endpoint that answers `405`. The client dutifully retries until its own timeout expires, so opening a session hangs before it degrades — and the degraded launch has no provider session id, so Resume is lost.
- **Usage accounting fails silently forever after.** The old read path requests a URL that OpenCode 2 now answers with the web app's HTML and a cheerful `200`. Decoding fails, the code treats "no answer" as "no usage", and every interactive session reports zero tokens for the rest of its life. Nothing ever tells the Beekeeper.

Paseka cannot simply be upgraded, because OpenCode 1 and OpenCode 2 cannot be installed side by side under the default `opencode` command — the very thing Paseka shells out to. A Beekeeper may have machines on either major version, and a colony may be developed on one and deployed on another.

Meanwhile, OpenCode 2 changed the substrate Paseka leans on, not just the flags: it keeps sessions in a shared per-user store served by a background service, it will create a session with a **caller-supplied id**, it honors `--prompt` together with `--session`, and it exposes a full HTTP API (sessions, messages, per-turn diffs, usage, cost, outcome, interrupt, live event stream). Those are exactly the primitives Paseka has been hand-rolling with throwaway `serve` processes and PTY control-server hacks — or failing to get at all.

So the Beekeeper needs one adapter name that keeps working across both OpenCode majors, plus a v2 path that uses the new API instead of the removed hacks.

## Solution

Split the OpenCode adapter into three named adapters behind one stable name.

- `opencodev1` — today's implementation, unchanged in behavior: AFK `opencode run --dir … --format json`, HITL pre-create through a throwaway `opencode serve` + `POST /session`, kickoff delivery through the TUI's own loopback control server, usage from `GET /session/<id>`. It becomes the frozen V1 line.
- `opencodev2` — a new implementation written against OpenCode 2: AFK `opencode run` scoped by process cwd, and HITL driven by a Paseka-managed `opencode serve` that Paseka itself talks to.
- `opencode` — a thin **proxy** that detects the installed major version and delegates to `opencodev1` or `opencodev2`. It reports itself as `opencode`, so bee YAML, profiles, the Queen Console, session Resume, trails, and every existing colony keep working untouched.

Detection is one cheap `opencode --version` call, cached per binary, overridable by configuration (`version: 1|2`, or separate `v1_binary` / `v2_binary` paths for the case where both majors are installed under different names). Detection is fail-closed: an unparsable version string is an actionable error, never a silent fallback to V1 flags.

The v2 adapter drops every hack the V1 line needed, because V2 removed the reason for them:

- Paseka picks the provider session id itself (`POST /api/session` accepts an `id`), so there is no pre-create race and no provider id guessing.
- The TUI's own `--prompt` now works together with `--session`, so kickoff delivery needs no control server, no port reservation, and no prompt-injection loop.
- Model, agent, and variant are set on the session object at creation time, because the V2 TUI no longer accepts them as flags.

The v2 adapter reads usage, transcripts, and diffs from the OpenCode API instead of from process stdout, so accounting stops depending on event shapes the CLI may or may not emit.

Adapters `opencodev1` and `opencodev2` are also valid `adapter:` values in bee YAML and profiles, for Beekeepers who want to pin a major per Bee instead of relying on detection.

## User Stories

1. As a Beekeeper with an existing `adapter: opencode` colony, I want to install OpenCode 2 and keep my bees working, so that I do not have to rewrite my colony before upgrading.
2. As a Beekeeper still on OpenCode 1, I want my bees to keep behaving exactly as they do today, so that the upgrade is a no-op for me.
3. As a Beekeeper, I want the adapter to detect which OpenCode major is installed, so that I never configure the version by hand.
4. As a Beekeeper, I want version detection cached per binary, so that dispatching a swarm does not spawn a `--version` process per run.
5. As a Beekeeper, I want to pin the major version in machine-local config, so that a broken detection never silently sends V1 flags to a V2 binary.
6. As a Beekeeper with both majors installed under different names, I want separate V1 and V2 binary paths, so that one colony can drive both.
7. As a Beekeeper who pins a version, I want a mismatch between the pin and the actual binary reported clearly, so that a wrong pin is a one-line fix instead of a flag-parsing mystery.
8. As a Beekeeper whose version string cannot be parsed, I want an actionable error naming the binary and the fix, so that I am not left guessing.
9. As a Beekeeper with no `opencode` binary, I want an error that tells me which package installs the major I need, so that I do not install the wrong one.
10. As a Beekeeper running AFK bees on OpenCode 2, I want each run scoped to its own worktree, so that parallel bees cannot see or touch each other's files.
11. As a Beekeeper running AFK bees on OpenCode 2, I want my model, provider, and reasoning variant honored, so that `params.model` and `params.thinking` still mean something.
12. As a Beekeeper, I want an OpenCode 2 model written as `provider/model` or `provider/model#variant` to work, so that the new reference syntax is first-class.
13. As a Beekeeper, I want plan-mode bees to still run under the V2 `plan` agent, so that `params.plan` keeps its meaning.
14. As a Beekeeper, I want auto-approval of tool permissions on non-interactive runs, so that AFK bees never stall on a prompt with nobody watching.
15. As a Beekeeper, I want AFK runs to report token usage even when the CLI emits no token event, so that honey economics stay honest on V2.
16. As a Beekeeper, I want reasoning tokens reported separately, so that I can see what thinking actually cost.
17. As a Beekeeper, I want a reported dollar cost per run when OpenCode provides one, so that I can budget the swarm.
18. As a Beekeeper, I want the run summary to still be the agent's final text, so that honey and Console summaries read the same on both majors.
19. As a Beekeeper, I want the provider session id recorded for V2 AFK runs, so that a run can be traced back to OpenCode.
20. As a Beekeeper, I want the diff of what an OpenCode 2 bee changed captured as a run artifact, so that review works the same as before.
21. As a Beekeeper opening a HITL session on OpenCode 2, I want it to start, so that `paseka bee chat` is usable again.
22. As a Beekeeper opening a HITL session on OpenCode 2, I want no 20-second stall before the TUI appears, so that the session feels immediate.
23. As a Beekeeper opening a HITL session on OpenCode 2, I want my kickoff prompt to land in the chat, so that the bee actually receives the task.
24. As a Beekeeper opening a HITL session on OpenCode 2, I want my kickoff to land in the intended session and not a fresh one, so that the stored provider id stays meaningful.
25. As a Beekeeper opening a HITL session on OpenCode 2, I want no model turn spent before I type, so that starting a session still costs nothing.
26. As a Beekeeper opening a HITL session on OpenCode 2, I want my chosen model, agent, and reasoning variant applied, so that plan-mode chats really plan.
27. As a Beekeeper, I want the V1 out-of-band prompt-delivery machinery gone from the V2 path, so that there is one way a kickoff can be delivered instead of two.
28. As a Beekeeper resuming a finished OpenCode session, I want Resume to remain available after upgrading the binary, so that upgrading does not orphan my chat history on name grounds.
29. As a Beekeeper resuming a session recorded on OpenCode 1 while a V2 binary is installed, I want Resume refused as ineligible with a clear reason, so that I never get a silent empty chat from an id the new major may not understand.
30. As a Beekeeper resuming a session recorded on OpenCode 2 while a V1 binary is installed, I want Resume refused as ineligible for the same reason, so that V2-only ids are not fed to V1 flags.
31. As a Beekeeper resuming a session recorded before the split, I want Resume still offered, so that the upgrade does not strand conversations that predate generation tracking.
32. As a Beekeeper, I want the V1 resume path to keep working unchanged, so that the split is not a regression for existing users.
33. As a Beekeeper with a finished HITL session on either major, I want its token usage reported, so that interactive sessions count toward the trail totals.
34. As a Beekeeper with an OpenCode 2 HITL session that spent reasoning tokens, I want those included in the reported usage, so that the number matches my provider bill.
35. As a Beekeeper whose OpenCode session ended up failing or interrupted, I want that outcome visible, so that a zero-token run is not mistaken for a healthy one.
36. As a Beekeeper reading the trail, I want the run to record which OpenCode generation actually executed, so that I can explain a behavior difference after an upgrade.
37. As a Beekeeper grouping by adapter in Console and `paseka status`, I want OpenCode runs to still appear under one `opencode` name, so that existing dashboards do not fragment.
38. As a Beekeeper, I want `paseka init --adapter opencode` to keep scaffolding `adapter: opencode` bees, so that new colonies are version-agnostic from day one.
39. As a Beekeeper who wants to pin one bee to one major, I want `adapter: opencodev1` or `adapter: opencodev2` accepted, so that I can run both majors in one swarm.
40. As a Beekeeper pinning a bee to a major, I want that bee to skip detection entirely, so that pinning is deterministic.
41. As a Beekeeper, I want `--profile` to accept the pinned names too, so that a profile can target an OpenCode generation.
42. As a Beekeeper, I want the Settings adapter list to keep one `opencode` row, so that I do not see three near-identical adapters.
43. As a Beekeeper with a `command:` override on an OpenCode bee, I want my argv passed through untouched, so that advanced setups are not rewritten.
44. As a Beekeeper whose OpenCode binary is missing, I want the error to name the adapter and the expected package, so that the first run tells me what to install.
45. As a Beekeeper upgrading from OpenCode 1 to 2, I want existing trails and exports readable, so that history is not invalidated by the split.
46. As a Beekeeper, I want old usage source labels to keep their meaning, so that historical exports stay interpretable.
47. As a platform contributor, I want the V1 implementation frozen behind its own name, so that V2 work cannot regress the V1 path.
48. As a platform contributor, I want version detection behind a test seam, so that no test needs a real OpenCode install.
49. As a platform contributor, I want the V2 HTTP client tested against a local stub server, so that CI never needs network or credentials.
50. As a platform contributor, I want the proxy's delegation tested with stub implementations, so that dispatch is verified without spawning agents.
51. As a platform contributor, I want one place that maps adapter names to implementations, so that adding a third OpenCode line is a single registration.
52. As a platform contributor, I want adapter-specific settings to travel as generic options rather than new shared fields per adapter, so that shared types do not grow OpenCode-shaped fields.
53. As a Beekeeper reading docs, I want the adapter reference to describe both majors, their differences, and their install commands, so that I can pick and upgrade deliberately.

## Implementation Decisions

### 1. Three adapter names, one user-facing name

- The OpenCode adapter family registers three names: `opencode` (proxy), `opencodev1`, `opencodev2`.
- All three are valid `adapter:` values in bee YAML and in profiles. The proxy is the default-facing name; the other two are explicit pins.
- The proxy's `Name()` returns `opencode`. Adapter name resolution, the LLM-adapter profile allow-list, console adapter projections, session Resume eligibility, and interactive-adapter checks keep matching the configured name, not the implementation.
- The V1 implementation moves to its own adapter package with `adapterName = "opencodev1"`; the V2 implementation is a new package with `adapterName = "opencodev2"`. The existing V1 tests move with it unchanged.

### 2. Proxy resolution and detection

- Resolution order for the binary: `params.binary` from the Bee, then the machine-local `binary`, then the default `opencode`.
- When `version` is pinned, that major's `v1_binary` / `v2_binary` override `binary` when set.
- Auto detection runs `<binary> --version` with a short timeout, parses the leading major (`v2.0.27` → 2, `1.18.35` → 1), and selects the matching implementation.
- Detection results are cached in-process per resolved binary path plus its modification time and size, so a burst of runs detects once and a replaced binary re-detects.
- Detection is **fail-closed**: unparsable, empty, or failed output produces an error naming the binary, the raw output, and the `version` pin that bypasses detection. There is no fallback to V1.
- A missing binary produces an adapter error that names the install route for the selected major.
- When a pin is set, detection is skipped for dispatch, but a cheap verification still runs when the binary answers; a detected major that disagrees with the pin is an explicit mismatch error. If verification cannot run, the pin is trusted.
- The proxy delegates `Run`, `SessionCommand`, and (when the selected implementation supports it) session usage resolution, and passes through the resolved binary so post-run accounting talks to the same executable that ran the agent.

### 3. Machine-local configuration

The machine-local `adapters/opencode.yaml` file gains three optional keys alongside the existing `binary`:

- `version`: `auto` (default), `1`, or `2`.
- `v1_binary`, `v2_binary`: optional explicit paths, for installations where both majors coexist under different names.
- `detect_timeout`: optional timeout override for detection.

`binary` keeps its current meaning and its current default, so existing files stay valid. The adapter config loader keeps returning a default of `opencode` when the file or key is missing.

### 4. Adapter-specific settings travel as generic options

- The shared run-params struct gains an optional string map of adapter options, merged key-by-key when overlays are combined.
- Bee `params.options.<key>` supplies values as strings; machine-local adapter config supplies the defaults; Bee values win.
- The OpenCode proxy reads `version`, `v1_binary`, and `v2_binary` from these options. Other adapters ignore unknown keys.
- Adapter-extra resolution routes each OpenCode name to the right binary and options: `opencodev1` pins major 1 and prefers `v1_binary`; `opencodev2` pins major 2 and prefers `v2_binary`; `opencode` stays unpinned. A pinned adapter must never fall through to another adapter's binary defaults.

### 5. V1 line is frozen

- The V1 implementation keeps its current behavior: AFK `run` with `--dir`, `--format json`, `--variant`, `--auto`, `--title`; HITL pre-create through a throwaway `serve` plus `POST /session`; kickoff through the TUI control server (`--port` / `--hostname`) and the V1 prompt route; usage from `GET /session/<id>`.
- The prompt-delivery description type stays in the shared adapter contract for the V1 path. It is simply not used by V2.
- The V1 usage source labels are unchanged, so historical run files keep their meaning.

### 6. V2 AFK runs

- Invocation is `opencode run` with `--format`, `--auto`, `--title`, optional `--agent`, and a single `--model` built from provider, model, and reasoning variant. The variant rides on the model reference as `provider/model#variant`; there is no separate variant flag.
- `--dir` is gone. Workspace is the adapter process working directory, and it is asserted before dispatch. Location isolation is verified against the created session's location rather than assumed from a flag.
- A custom `command:` override is still passed through untouched.
- JSON event parsing keeps the tolerant line reader: `text` parts build the summary, tool and error events are ignored for summary purposes, and the provider session id is read from the event envelope.
- Because a single-step V2 run may emit no token event, token usage falls back to the API: after the run, the adapter reads the session record for the session id it just created.

### 7. V2 session ownership

- For HITL, Paseka starts its own `opencode serve` on a loopback ephemeral port with a generated server password, and keeps it for the lifetime of the session.
- Paseka generates the provider session id itself (V2 session ids follow a fixed charset and length) and creates the session through the API with that id, the workspace as the location, the title, the agent, and the model with its variant.
- If the API rejects the chosen id, the adapter falls back to letting OpenCode create the session and reads the assigned id back. V2 `--session` also creates a missing session, so a session id can always be resolved before the PTY starts.
- The TUI is launched against Paseka's server with the session id, the kickoff prompt, and auto-approval. No model, agent, variant, port, or hostname flags are passed, because the V2 TUI rejects them.
- A custom `command:` override skips session creation entirely, and an explicit session id in the argv is recorded as the provider session id, exactly as in V1.

### 8. V2 kickoff delivery

- The kickoff is passed as the TUI's own prompt flag together with the session id. Verified against V2, this combination delivers the prompt into the pre-created session, so the V1 out-of-band control-server path is not ported.
- Resume continues an existing session by id; a continue line becomes the prompt flag on that session; an empty continue line launches the TUI on the existing session with no prompt.

### 9. V2 accounting

- Token usage, reasoning tokens, cost, and outcome come from the session record read over the API, not from CLI stdout.
- The shared usage struct gains optional reasoning-token and cost fields, added additively so that projections, exports, and Console tolerate their absence.
- New usage source labels are introduced for the V2 paths (run-stream and session-record) instead of reusing the V1 labels, so a reader can tell where a number came from.
- A session that reports no usage, an unknown session, or an unexpected body means "no usage", never a failure — the same fail-soft rule the V1 path already uses.

### 10. Run identity and metadata

- The adapter name recorded in run meta is the **configured** name, so a bee that says `opencode` keeps reading as `opencode` in trails, Console, status, and exports.
- Run meta and the interactive session record gain an optional field naming the implementation that actually executed (for example `opencodev2`), for debugging after an upgrade and for major-aware Resume.
- The configured name reaches the implementation through the run and session requests, so the implementation writes it without knowing who delegated. The implementation name is written by the same path, so a delegated run and a direct pinned run record both fields the same way.
- The provider session id, workspace, profile, and run directory behavior are unchanged for both lines.

### 11. Resume eligibility

- Resume eligibility is keyed on adapter name and includes the proxy name plus both pinned names, so a colony is not broken by the split.
- Because the recorded adapter name is the configured name, upgrading a colony from V1 to V2 does not make existing sessions ineligible on name grounds.
- Resume eligibility is **major-aware and fail-closed**: the stored session records the OpenCode generation it ran on, and a session whose recorded generation does not match the generation that would run the continuation is refused as ineligible, with a dedicated reason code. A session recorded as V1 is never launched on a V2 binary, and a session recorded as V2 is never launched on a V1 binary — Paseka does not depend on the provider to fail such a launch usefully.
- One deliberate exception: a session recorded **before** this split carries no generation, and is treated as matching whatever major is installed. Old sessions are not stranded by the upgrade; a V1-era session attempted on a V2 binary degrades to the normal launch failure path if the provider cannot continue it.
- A source pinned to one name (`opencodev1` or `opencodev2`) continued through a bee pinned to the other is still refused with the existing adapter-changed reason: a name change is a configuration change, not a generation mismatch.
- All other Resume rules — active source, busy provider id, missing id, `command:` override, missing bee — are unchanged.

### 12. Console, profiles, and scaffolding

- The console adapter list, the Settings adapter rows, and the client-side adapter name lists keep a single `opencode` entry that reads the one machine-local adapter file. The pinned names are accepted wherever adapter names are validated (bee YAML, profiles, adapter allow-lists) but never get their own console rows.
- `paseka init --adapter opencode` keeps scaffolding bees with `adapter: opencode` and the same adapter file, now including the new optional keys as commented hints.
- `paseka init` also accepts `opencodev1` and `opencodev2` for a Beekeeper who wants a pinned scaffold, and the printed next-steps hints name the correct install command and auth flow for the selected major.

### 13. Errors and observability

- Launch and completion logging records the adapter name, the resolved implementation, the resolved binary, and the detected or pinned major, so a swarm log answers "which OpenCode ran this".
- Adapter errors name the implementation and the fix: missing binary, unreadable version, pin mismatch, and rejected session id are four distinct messages.

## Testing Decisions

Good tests assert external behavior and stay hermetic. No test requires a real OpenCode install, a running server, network access, or provider credentials.

- Name and resolution: the adapter allow-list accepts all three names; adapter-extra resolution gives each pinned name its own binary and never falls through to another adapter's defaults; Bee options overlay machine-local defaults; the LLM-adapter profile check accepts all three names.
- Detection: table-driven over version strings for both majors, padded output, unexpected leading text, empty output, and garbage; detection caching means a second resolve spawns nothing; a replaced binary re-detects; unparsable output fails with an actionable message; a pin skips dispatch detection; a pin that disagrees with a detected major is a mismatch error; a missing binary error names the selected major's install route.
- Proxy delegation: with stub implementations, the proxy returns the delegate's run result, session command, and usage unchanged; it reports its own name as `opencode`; the resolved binary reaches the delegate; usage resolution is skipped when the selected implementation does not support it.
- Metadata: a delegated run records the configured adapter name and the implementation name; a pinned bee records the pinned name.
- V1 regression: the existing V1 argv, parse, create, usage, and session tests move with the package and keep passing, with only the adapter name changed.
- V2 argv: AFK argv contains no `--dir` and no separate variant flag, contains the combined model reference, honors plan and auto-approval, and sets the title; TUI argv contains only server, session, prompt, and auto-approval, and never model, agent, variant, port, or hostname.
- V2 API client: a local stub server asserts the request paths, basic auth, the create-session body (caller-supplied id, location, agent, model with variant), the wrapped response shape, the usage read, and the prompt call; the wrapped response is decoded correctly and a rejected caller id degrades to the fallback path.
- V2 parsing: single-step and multi-step event streams both yield a summary and a provider session id; a stream with no token event yields no usage from stdout and the session-record fallback supplies it instead; reasoning tokens and cost map onto the usage struct.
- Session lifecycle: a HITL launch against a stub server records the provider session id before the PTY starts; resume skips session creation; a `command:` override with an explicit session id records it.
- Resume: proxy-named sessions stay eligible after an upgrade; a V1-era session continued on a V2 binary and a V2 session continued on a V1 binary are both refused with the generation-mismatch reason; a session with no recorded generation is still eligible; pinned sources resumed by a differently pinned bee are refused with adapter-changed; ineligible reasons keep their codes.
- Console and client contracts: the single `opencode` row remains in the console adapter list and in the client-side name lists; resume controls accept `opencode`.
- Scaffolding: `paseka init --adapter opencode` still writes bees with `adapter: opencode` and an adapter file that documents the new keys; `opencodev1` and `opencodev2` scaffolds pin the expected adapter name.

Prior art in-tree: the fake-binary script pattern for adapter argv tests, the package seam for session pre-create, the stub session adapter in session-manager tests, and `httptest` servers for control-server delivery tests.

## Out of Scope

- Changing the domain event set, honey rules, or bus choreography because of the split.
- Removing the V1 adapter, or deprecating V1 support.
- Console live streaming of OpenCode events, transcript hydration from the OpenCode store, and per-turn diff views — valuable follow-ups that reuse the V2 API but are separate specs.
- V2-native worktree management (`worktree_create` and friends), ACP, the plugin system, and Code Mode tools.
- Answering OpenCode permission requests through the API from Queen Console. The adapters keep auto-approval for non-interactive runs.
- Migrating or rewriting existing `opencode` usage source labels in already-written run files.
- Continuing a session across OpenCode majors. V1 and V2 report the same store path and V2 exposes an experimental V1-migration endpoint, but that continuity is neither verified nor promised; cross-major Resume is refused as ineligible instead.
- Pinning an OpenCode version per *provider* or per *model*; the pin is per adapter, overridable per Bee.
- Any change to how Pi, Claude, Cursor, or script adapters behave.

## Further Notes

- Verified empirically against OpenCode `2.0.27` (binary fetched to a scratch directory, isolated config/data directories, and a local mock provider — no real provider credentials were used):
  - `run` keeps `--format default|json`, `--auto`, `--title`, `--agent`, `--model provider/model#variant`, `--thinking`, `--session`, `--fork`, and the `--` message separator; `--dir`, `--variant`, `--port`, `--username`, `--password`, and `--attach` are gone. A directory argument is accepted by the TUI, not by `run`.
  - The workspace of a V2 AFK run is the process working directory: a run started in a scratch directory created its session with that directory as the location.
  - The event stream is shape-compatible: `step_start`, `tool_use`, `step_finish` (with `part.tokens` and `part.cost`), `text`, and `error`, each carrying a top-level session id. A single-step run emitted only the `text` event, which is why usage needs the session-record fallback.
  - The TUI accepts only `--standalone`, `--server`, `--auto`, `--continue`, `--session`, `--prompt`, and a directory. `--model` aborts the launch with `Unrecognized flag`. Launched with `--session <id> --prompt <text>` against a session created by the caller, the prompt was delivered into that session and the turn ran.
  - `POST /session` answers `405`; `POST /api/session` answers `200` but **ignores** the V1 body shape and uses the server's own location; the response is wrapped as `{"data": {...}}`. `GET /session/<id>` and `GET /event` answer `200` with the web app's HTML.
  - Basic auth through `OPENCODE_SERVER_USERNAME` / `OPENCODE_SERVER_PASSWORD` still works against `serve`, and `serve` still accepts `--hostname` / `--port`.
  - Session ids supplied by the caller are accepted; the session record carries tokens (including reasoning), cost, outcome, model, and time; `POST /api/session/<id>/prompt` admits a turn and returns immediately.
  - The event stream emits `session.usage.updated`, `session.text.delta`, `session.step.*`, `session.execution.*`, `session.inbox.*`, and `server.connected`, which is the raw material for a future Console live view.
  - `opencode api <operationId>` exists as a CLI bridge to the API; the adapter uses its own typed HTTP client so calls stay testable and credential handling explicit.
- Settled with the Beekeeper while writing this spec:
  - Run meta keeps the configured adapter name (`opencode`); the implementation that ran is recorded as an extra field.
  - Cross-major Resume is refused as ineligible (fail-closed) rather than attempted.
  - Cross-major session continuity is not worth chasing: it is neither verified nor promised, and no code depends on it.
  - The console shows one `opencode` adapter row; the pinned names stay an escape hatch.
- Related: [026-opencode-adapter](./026-opencode-adapter.md), [030-opencode-session-resume](./030-opencode-session-resume.md), [021-provider-session-logs-export](./021-provider-session-logs-export.md), [027-config-profiles](./027-config-profiles.md), [interactive sessions](../guide/interactive-sessions.md), [bee configuration](../guide/bee-config.md).
