# Spec 026: OpenCode Adapter

## Status

**(Draft)**
Adapter package, registration, init scaffolding, and docs are in the tree. Status stays Draft until the feature ships (then Implemented).

## Problem Statement

Beekeepers who already run [OpenCode](https://opencode.ai/docs/cli/) (`opencode`) cannot bind colony bees to that CLI. Today bees must use Cursor, Pi, Claude Code, or a script. Mixing OpenCode into a colony means wrapping it in `command:` or abandoning Paseka dispatch, worktrees, run IPC, and HITL attach.

## Solution

Treat OpenCode as a first-class adapter, same surfaces as Pi and Claude: AFK `paseka bee run` invokes `opencode run`; interactive `paseka bee chat` launches the OpenCode TUI under a Paseka-owned PTY. Bees opt in with `adapter: opencode`. `paseka init --adapter opencode` scaffolds starter bees and home config. Default init stays Cursor. Domain events remain explicit via `paseka event emit`.

## User Stories

1. As a Beekeeper, I want `adapter: opencode` in bee YAML, so that a role launches OpenCode instead of Cursor.
2. As a Beekeeper, I want `paseka bee run` to run `opencode run` non-interactively, so that AFK dispatch works without a TUI.
3. As a Beekeeper, I want `paseka bee chat` to open the OpenCode TUI in a PTY, so that I can steer the bee interactively.
4. As a Beekeeper, I want existing Cursor/Pi/Claude bees unchanged, so that adding OpenCode is opt-in.
5. As a Beekeeper, I want `paseka init --adapter opencode` to scaffold scout/builder/hivewright with `adapter: opencode`, so that a new colony can start on OpenCode.
6. As a Beekeeper, I want default `paseka init` to keep Cursor bees, so that OpenCode is not forced on existing workflows.
7. As a Beekeeper, I want unknown `--adapter` values (including `claude`) to keep falling back to Cursor, so that init stays conservative.
8. As a Beekeeper, I want AFK `--dir` and process cwd to be the colony root or trace worktree (HITL cwd only), so that edits land in the same workspace as other adapters.
9. As a Beekeeper, I want `system_template` concatenated with the task prompt, so that role instructions reach OpenCode despite no `--append-system-prompt` flag.
10. As a Beekeeper, I want `prompt.txt` and `system.txt` written under the run dir even when the CLI sees one joined message, so that audit replay still splits role vs task.
11. As a Beekeeper, I want `params.model` passed as `--model`, so that I can pin `provider/model` after alias resolution.
12. As a Beekeeper, I want `params.provider` joined with a model that has no `/`, so that Pi-style split params still produce OpenCode’s `provider/model` id.
13. As a Beekeeper, I want AFK runs to pass `--auto` when `trust` or `force` is true (bee default), so that headless runs do not block on permission prompts.
14. As a Beekeeper in HITL, I want `--auto` omitted, so that permission prompts stay in the TUI.
15. As a Beekeeper, I want `params.plan: true` to pass `--agent plan`, so that OpenCode’s built-in read-only Plan primary is used.
16. As a Beekeeper, I want Paseka `agentId` never passed as OpenCode `--agent`, so that bee identity is not confused with OpenCode’s agent personas.
17. As a Beekeeper, I want AFK `output_format: json` (default) to use `--format json`, so that session id and usage can be parsed.
18. As a Beekeeper, I want `output_format: text` to use `--format default`, so that I can keep human-formatted stdout when I do not need JSON.
19. As a Beekeeper, I want HITL to omit `--format`, so that the TUI is not forced into JSON event mode.
20. As a Beekeeper, I want `params.thinking` mapped to `--variant`, so that provider reasoning effort is configurable without using OpenCode’s UI `--thinking` flag.
21. As a Beekeeper, I want `params.binary` and home `adapters/opencode.yaml` `binary` to override the CLI name, so that a custom install path works.
22. As a Beekeeper, I want auth to stay in OpenCode (`opencode auth login`, env, project `.env`), so that Paseka does not invent a fake `--api-key`.
23. As a Beekeeper, I want a missing `opencode` binary to fail the run with a clear PATH error, so that I know to install the CLI.
24. As a Beekeeper, I want stdout/stderr captured as artifacts and a workspace git diff after AFK exit, so that inspect/export match other adapters.
25. As a Beekeeper, I want JSON stdout never parsed into `SIGNAL`/`INSIGHT`/`MUTATION`/`VERIFICATION`, so that choreography stays `paseka event emit` only.
26. As a Beekeeper, I want AFK `providerSessionId` taken from the first JSONL `sessionID`, so that I can later correlate `ses_…` with OpenCode’s store.
27. As a Beekeeper, I want HITL `providerSessionId` left empty, so that we do not invent an id OpenCode has not created yet.
28. As a Beekeeper, I want AFK `--title <agentId>`, so that I can find the session in OpenCode’s list when JSON parse fails.
29. As a Beekeeper, I want token usage from the last JSON `step_finish` with `part.tokens` on `result.json`, so that inspect usage works when JSON format is on.
30. As a Beekeeper, I want `command:` to replace flag mapping, so that I can pass a full OpenCode argv when needed.
31. As a Beekeeper with `command:`, I want association/usage to still work if JSON is on stdout, so that custom argv does not drop `sessionID`.
32. As a Beekeeper, I want Queen Console PTY attach and Ghostty unchanged, so that OpenCode HITL uses the existing session manager.
33. As a colony author, I want unknown adapter names to still fail bee load, so that typos do not silently become Cursor.
34. As a Beekeeper, I want `paseka export --include agent-logs` to omit OpenCode Agent log quietly in this slice, so that missing SessionLogResolver is not a hard failure.
35. As a Beekeeper, I want worktree bees to run OpenCode inside `.paseka/worktrees/<traceId>/`, so that isolated proposals stay isolated.
36. As a Beekeeper, I want `post_exec`, completion contracts, and `run_summary` to apply the same as other LLM adapters, so that OpenCode bees participate in choreography.
37. As a Beekeeper, I want `paseka doctor` and status views to treat OpenCode runs like any other adapter process, so that live bees and kill still work.
38. As a platform contributor, I want tests that lock argv mapping without calling a real OpenCode binary, so that CI stays hermetic.
39. As a Beekeeper reading docs, I want architecture, bee-config, interactive-sessions, and CLI init to list OpenCode next to Pi/Claude, so that I can configure bees without reading the spec.
40. As a Beekeeper, I want Honey Reserve charging unchanged (one dispatch), so that OpenCode does not get a special energy rule.

## Implementation Decisions

- Adapter name is `opencode`; default binary is `opencode`. Package lives with other adapters and registers in the AFK dispatcher and the interactive session manager (LLM adapters only).
- Shared `RunParams` are reused. No new bee YAML keys for OpenCode agent personas, permissions, serve/attach, or MCP.
- AFK invocation is `opencode run` with `--dir` set to the workspace and process cwd the same path. Default `--format json`. `--auto` when `trust` or `force`. `--title` is the Paseka `agentId`. Joined system+task prompt is a positional message.
- HITL invocation is `opencode` (no `run`) with process cwd = workspace and `--prompt` for the joined kickoff. No `--dir` (that flag belongs to `run`/`attach`, not the TUI). No `--auto`, `--format`, or `--title`. `plan` still maps to `--agent plan`.
- AFK positional prompt is passed after `--` so a task that looks like a flag cannot be parsed as argv.
- Model flag: use `params.model` as-is when it contains `/`; otherwise prefix `params.provider` when set.
- Home config `~/.config/paseka/<slug>/adapters/opencode.yaml` holds `binary` only. Init always writes this file (like Claude). Missing file defaults to `binary: opencode`.
- `paseka init --adapter opencode` is first-class (like Pi, unlike Claude). Starter bees use `output_format: json`. Default init remains Cursor.
- AFK JSONL parse is tolerant: first `sessionID` → `providerSessionId`; all completed `text` parts concatenated for summary; `step_finish` `part.tokens` summed into `protocol.Usage` with source `opencode.run-json`. Text format uses trimmed stdout as summary and does not invent a session id.
- HITL does not pre-create an OpenCode session (no Cursor-style `create-chat`; OpenCode generates `ses_*` itself).
- Session log export resolver is out of this spec (see spec 021). Export omits Agent log as unsupported.
- Domain bus audit stays `events.ndjson` from `paseka event emit` only.

## Testing Decisions

- Prefer external behavior: argv built for AFK and HITL, parse of representative JSONL, run-dir files after a fake binary, init scaffolding, and adapter name resolution.
- Test the OpenCode adapter package, colony params/home load, colony init, and a narrow runtime `BeeRun` smoke with a fake `opencode` binary (same style as the Pi dispatch test).
- Do not require a real OpenCode install or network. Do not assert internal helper names beyond exported adapter behavior and locked CLI flags.

## Out of Scope

- Provider session log reader for export (`opencode export`, `~/.local/share/opencode/storage/`)
- HITL resume by `providerSessionId` (spec 025 is Cursor-only)
- `opencode serve` / `--attach`, GitHub agent, ACP, plugin install
- Mapping bee role names to OpenCode `--agent` except `plan`
- Changing the default `paseka init` adapter away from Cursor
- Rewriting this repository’s committed `.paseka/bees/*.yaml` from Cursor to OpenCode
- Forwarding a Paseka-managed API key flag (OpenCode has none)

## Further Notes

OpenCode `--agent` is an OpenCode persona (Build, Plan, custom markdown agents), not Paseka `agentId`. Plan mode uses the built-in Plan primary. Auth is OpenCode’s own store and environment; Paseka only launches the process with inherited env.
