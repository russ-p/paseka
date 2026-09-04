# Spec 025: Resume Cursor HITL Sessions

## Status

**(Draft)**
First slice: Queen Console (and the same session-manager path) reopens a finished Cursor HITL chat via stored `providerSessionId`. New Paseka session; fail closed; no Pi/Claude/`command:` / AFK→HITL.

## Problem Statement

A Beekeeper finishes a Cursor HITL session in Queen Console and later wants to keep talking in **that** agent conversation — same tool-call history, same Cursor chat — not a fresh `create-chat`.

Today the id is already on session meta (`providerSessionId` from [021](./021-provider-session-logs-export.md)), and Console shows it. Starting another session still allocates a new Paseka `agentId` **and** a new Cursor chat. Completed sessions only offer a coarse PTY transcript. Live attach does not help once the process has exited, and attach never works for a session owned by another `paseka` process.

The Beekeeper should not have to copy a UUID into a raw `command:` override or leave Console for Ghostty just to continue work on the same Flight Trail.

## Solution

On an eligible finished Cursor HITL session, Queen Console offers **Resume**. That starts a **new** detached Paseka session (new `sessionId` / `agentId`, new run directory) that launches the Cursor Agent TUI with `--resume <providerSessionId>` and **does not** call `create-chat`.

Same bee role and Flight Trail (and therefore the same worktree ensure path) as the source session. Optional one-line continue text becomes the only new positional prompt; with no text, the TUI opens on the existing chat with no kickoff replay.

After start, the Beekeeper lands on the new session’s in-browser PTY as they would after a normal Console launch. The source run artifacts stay immutable. Honey is not charged. If the session is ineligible or Cursor cannot continue that chat, nothing new is created as a blank conversation.

## User Stories

1. As a Beekeeper on a finished Cursor HITL session in Queen Console, I want a Resume control, so that I can continue the same provider chat without starting a new one.
2. As a Beekeeper, I want Resume to open an interactive terminal in Console, so that I work in the same HITL surface as a fresh launch.
3. As a Beekeeper, I want the new session to use the same bee role as the source, so that I do not pick a different bee by mistake.
4. As a Beekeeper, I want the new session on the same Flight Trail as the source, so that the worktree and trail context stay aligned.
5. As a Beekeeper, I want a new Paseka session id for the continuation, so that the original run’s transcript, status, and meta stay an honest historical record.
6. As a Beekeeper, I want the new session to keep the same `providerSessionId`, so that Cursor loads the old conversation.
7. As a Beekeeper, I want the new session meta to record which Paseka session it resumed from, so that I can correlate the chain in Console and inspect.
8. As a Beekeeper who types nothing extra, I want the TUI to open without replaying the original task or system prompt as a new user message, so that the agent does not redo the kickoff.
9. As a Beekeeper, I want an optional one-line continue message, so that I can nudge the agent as the first new turn without filling the full launch form.
10. As a Beekeeper, I do not want that continue line rendered through the bee task template, so that Resume is not a second “new task” launch.
11. As a Beekeeper on an `active` session, I do not want Resume, so that I attach or wait instead of starting a second TUI on a live process.
12. As a Beekeeper, I want Resume when the source is `completed`, `failed`, or `cancelled`, so that a crashed or stopped chat is still continuable when Cursor still has it.
13. As a Beekeeper whose session has no `providerSessionId`, I want Resume hidden or disabled with a clear reason, so that I am not offered a dead control.
14. As a Beekeeper on a Pi HITL session, I do not want this Resume, so that a Cursor-shaped `--resume` is not applied to Pi’s run-local session dir.
15. As a Beekeeper on a Claude HITL session, I do not want this Resume, so that missing native ids are not pretended.
16. As a Beekeeper whose bee uses a `command:` override, I do not want Resume, so that custom argv is not rewritten or double-resumed.
17. As a Beekeeper, I want Resume to skip `create-chat`, so that a failed continuation never silently becomes a new empty Cursor chat.
18. As a Beekeeper when Cursor cannot open that chat (purged, wrong machine, CLI error), I want a visible failure and no useful new blank session, so that I know the pointer is stale.
19. As a Beekeeper, I want Resume to fail if the source bee YAML is gone or no longer `adapter: cursor`, so that a renamed colony does not launch the wrong tool.
20. As a Beekeeper, I want current bee params (model alias, force, plan) applied at resume time, so that I am not frozen to the original launch’s model forever.
21. As a Beekeeper, I want worktree ensure to follow the source `traceId` as a normal session launch would, so that a still-registered trail worktree is reused.
22. As a Beekeeper whose worktree was removed, I want ensure to recreate it as today rather than resurrect a deleted path from old meta, so that Resume does not invent a third workspace rule.
23. As a Beekeeper, I want Resume not to consume honey, so that continuing an ad-hoc or already-accepted HITL chat is not a second `session.start` tax.
24. As a Beekeeper who originally accepted an invite, I want Resume still honey-free, so that continuation is not billed like a new accept.
25. As a Beekeeper, I want the new session owned by the current `paseka console` process, so that browser PTY attach works even when the source session was started in Ghostty or another shell.
26. As a Beekeeper, I want to be taken to the new session after a successful Resume, so that I do not hunt for it in the list.
27. As a Beekeeper, I want the source session to remain listed as inactive history, so that I can still read its transcript.
28. As a Beekeeper, I want the new session’s Console transcript to start empty (aside from live PTY capture), so that I am not shown a fake reconstruction of Cursor history.
29. As a Beekeeper, I understand the old conversation appears inside the Cursor TUI, not as Console transcript lines, so that I do not expect spec 021 Agent log in this slice.
30. As a Beekeeper looking at session detail, I want `providerSessionId` still shown, so that Resume is consistent with the id I already see.
31. As a Beekeeper looking at the new session, I want to see it was resumed from another session id, so that the chain is visible without opening files.
32. As a Beekeeper who already resumed once, I want to Resume the continuation as well, so that a chain of Paseka sessions can share one Cursor chat.
33. As a Beekeeper, I want Resume refused if **any** active local session already uses that `providerSessionId`, so that two Console PTYs do not fight over one Cursor chat.
34. As a Beekeeper launching a **new** session from the usual form, I want create-chat behavior unchanged, so that Resume does not alter the empty-chat path.
35. As a Beekeeper using Queen Shell, I want `paseka session resume <sessionId>` to use the same eligibility and launch rules, so that I can continue without the browser.
36. As a Beekeeper on the CLI, I want an optional continue body flag, so that parity with Console’s optional line holds.
37. As a Beekeeper on the CLI, I want a non-zero exit and a readable error when Resume is ineligible, so that scripts do not start a normal chat instead.
38. As a Beekeeper on Telegram, I do not need Resume in this spec, so that Human Gateway scope stays Console/CLI.
39. As a Beekeeper on the Runs tab, I do not want Resume of an AFK Cursor run in this spec, so that stream-json `session_id` vs HITL `--resume` is not assumed.
40. As a Beekeeper, I want `POST /api/sessions` unchanged in meaning (always a new chat), so that existing launch clients cannot accidentally resume.
41. As a Beekeeper calling Resume with a missing session id, I want 404, so that typos are not treated as launch.
42. As a Beekeeper calling Resume on an ineligible session, I want 4xx with a reason (`not cursor`, `no providerSessionId`, `command override`, `still active`, `adapter changed`), so that the SPA can show it.
43. As a Beekeeper, I want Widen/Restore on the new PTY to keep working, so that Resume sessions are first-class Console terminals.
44. As a Beekeeper, I want Stop on the new session to work as today, so that I can end the continuation without touching the source artifacts.
45. As a Beekeeper, I want Live bees to show the new adapter PID, so that a resumed TUI is as visible as any other HITL session.
46. As a Beekeeper, I do not want domain bus events invented from Resume, so that choreography stays explicit (`event emit` / invite accept remain separate).
47. As a Beekeeper who purged Cursor chats but kept `.paseka/runs`, I want Resume to fail rather than create-chat, so that the stored id is not “repaired” into a different conversation.
48. As a Beekeeper who purged runs but still has the Cursor chat, I want Resume unavailable (no source session), so that this slice does not invent resume-by-raw-UUID.
49. As a Beekeeper on another machine than the one that created the chat, I want failure, so that Console does not pretend Cursor’s store is colony-portable.
50. As a Beekeeper, I want `command:` bees to keep today’s override semantics on normal launch, so that Resume gating does not rewrite custom argv elsewhere.
51. As a platform contributor, I want resume to be an explicit session-manager operation, so that adapters do not guess resume from a reused `agentId`.
52. As a platform contributor, I want Paseka `sessionId` to stay equal to `agentId` on the new run, so that [021](./021-provider-session-logs-export.md) identity rules hold.
53. As a Beekeeper reading docs after ship, I want the interactive-sessions guide to distinguish create-chat (new HITL) from Resume (existing `providerSessionId`), so that `--resume` is not described as only an association trick.

## Implementation Decisions

### 1. Product slice

- First slice is **Cursor HITL → Cursor HITL** continuation.
- Primary UX: Queen Console Sessions detail.
- Same launch primitive on Queen Shell (`paseka session resume`) so Console is not a one-off HTTP-only path.
- Do not extend [021](./021-provider-session-logs-export.md) (association and Agent log export stay there).

### 2. Identity

- Always allocate a new Paseka `sessionId` / `agentId` and a new run directory.
- Copy `providerSessionId` from the source; never overwrite Paseka ids with the Cursor UUID.
- New session meta includes `resumedFrom` (source Paseka `sessionId`).
- Do not rewrite the source `session.json` / transcript / status (no `resumedBy` back-pointer in this slice).
- Resume of a session that itself has `resumedFrom` is allowed; eligibility uses **that** session’s adapter, bee, trace, and `providerSessionId`.

### 3. Eligibility (all must hold)

- Source session exists on this colony (active registry, in-process manager, or run-tree session meta).
- Source is **not** `active` (no live PTY / registry row treated as running).
- No other **active** session on this machine/colony shares the same `providerSessionId`.
- Source adapter is Cursor (from session/run meta).
- `providerSessionId` is non-empty.
- Source bee still loads, resolves to the Cursor session adapter, and has **no** `command:` override.
- Source `traceId` and bee role are reused; the client does not pick another bee or trace.

### 4. Launch behavior vs normal HITL

Normal `POST /api/sessions` / `bee chat` is unchanged: Cursor still `create-chat` then `--resume` that **new** UUID.

Resume path:

- Do not call `create-chat`.
- Do not treat missing prompts as a hard error (unlike today’s Cursor session adapter).
- Build interactive argv with workspace, existing Cursor flags from current bee params (force / plan / model / api key), and `--resume <providerSessionId>`.
- If the Beekeeper supplied a continue line, append it as the sole positional prompt; otherwise omit the positional prompt entirely.
- Do not join `system_template` + original task into argv.
- Do not fall back to create-chat or to a no-`--resume` TUI on any error.

If the Cursor CLI fails to start or exits before a usable TUI, the new Paseka session is `failed` (or not registered as a successful active session — pick one policy in implementation and test it). Never leave the Beekeeper in a new empty chat that dropped `--resume`.

Preflight of Cursor’s on-disk chat store is **not** required; fail from CLI behavior. Do not copy or parse `store.db` here.

### 5. Workspace and prompts

- `traceId` is the source session’s trace.
- Worktree: same ensure rules as a session launch that already has that `traceId` ([020](./020-worktree-branch.md), interactive-sessions guide).
- Rendered kickoff templates are not replayed. Optional continue text is stored as the new run’s prompt artifact when present; otherwise prompt/system files may be omitted or empty.
- Model aliases resolve at resume time like any other session start ([019](./019-model-aliases.md)).

### 6. Honey and bus

- Resume does not `energy.consume` / `session.start`.
- Resume does not accept or recreate invites ([006](./006-human-gateway-invites.md)).
- No new NATS session-lifecycle events in this slice.

### 7. Queen Console API and UI

- `POST /api/sessions/{sessionId}/resume`
- Optional JSON body: continue text only, field `body`. Empty / omitted = no positional prompt.
- Success: **201** with the same session view shape as create, representing the **new** session (`active: true`).
- Failures: **404** unknown source; **409** source or sibling `providerSessionId` still active; **400** other eligibility failures with a stable `error` / message string the SPA can show.
- Do not add resume fields to `POST /api/sessions`.

SPA:

- Resume on session detail when eligibility can be decided from the projection (adapter, id, inactive, not `command:` when that flag is on the view or bees list). If `command:` is not on the session view today, add a boolean or omit the button unless adapter+id+inactive hold and let the API reject `command:` bees.
- Optional continue input next to Resume; not the full launch form (no bee/trace/intent/raw-template).
- On 201, select the new `sessionId` and attach PTY as after create.
- Stop / Widen / transcript fallback unchanged.

Eligibility in the list can stay coarse; authoritative checks are on POST.

### 8. Queen Shell

- `paseka session resume <sessionId>` with optional continue text flag (mirror Console body).
- Prints the new session id on success; errors on ineligible sources.
- Console always starts detached and attaches via the existing PTY hub.
- Queen Shell attaches in the current terminal, same idea as `paseka bee chat`.

### 9. Modules (indicative)

- Session manager: resume operation (load source meta, eligibility, new ids, launch).
- Cursor session adapter: explicit resume request (id + optional continue; skip create-chat and skip prompt-required).
- Session / run meta: optional `resumedFrom`.
- Queen Console HTTP + SPA Sessions.
- Queen Shell `session resume`.
- Session list/detail projections: surface `resumedFrom` when set.

### 10. Security / locality

- Resume uses this machine’s Cursor CLI and store, same as any HITL chat.
- No fetch of another host’s Cursor data.
- Continue text is operator-authored, not a second raw-prompt escape that bypasses bee identity.

## Testing Decisions

Good tests assert **external behavior**: eligibility; new vs old ids; argv contains `--resume` with the source UUID and does **not** invoke create-chat; no positional prompt when continue is empty; positional continue when set; system/task kickoff not joined; source session files unchanged; new meta has `resumedFrom` and the same `providerSessionId`; HTTP 201/4xx shapes; SPA exposes Resume and the resume path.

Do not require a real Cursor account or `store.db` in CI. Fake CLI binaries (existing Cursor session adapter tests) are enough for argv and create-chat skip. Optional manual note: `agent --resume <HITL uuid>` on a real finished Console session.

Cover at least:

- Happy path: finished Cursor HITL meta → new session, copied id, `resumedFrom`, `--resume` present.
- Empty continue vs non-empty continue argv.
- Ineligible: missing id; Pi/Claude meta; `command:` bee; active source; second active session with same provider id; bee gone; bee adapter no longer cursor.
- Resume must not call create-chat even if the fake CLI would succeed at create-chat.
- Normal HITL launch still calls create-chat (regression).
- Source `session.json` bytes / `providerSessionId` unchanged after resume.
- Console handler tests for POST resume 201 and 4xx.
- SPA static test: resume path and control strings exist.
- Manager/CLI: resume of a resumed session (chain) still eligible.

Prior art: Cursor session adapter tests (`create-chat` / `--resume` / `command:` override), session manager providerSessionId persistence tests, Console session create/list/PTY tests, `internal/console/static_test.go` string presence.

## Out of Scope

- Pi resume (`--session-dir` / `--session-id` of the **source** run).
- Claude resume.
- Bees with `command:` overrides.
- Resume from Queen Console **Runs** (AFK `providerSessionId` / stream-json vs HITL `--resume` compatibility).
- Resume by typing a raw Cursor UUID with no source Paseka session.
- Reusing the source `sessionId` / run directory / PID.
- Rewriting source meta with `resumedBy`.
- Structured chat UI or hydrating Console transcript from Cursor logs ([021](./021-provider-session-logs-export.md) Agent log).
- Cross-process attach to the **original** PTY ([002](./002-queen-console-mvp.md)); Resume creates a new process instead.
- Telegram / other Human Gateway Resume.
- Honey changes; invite re-accept; bus session lifecycle events.
- Preflight / repair of Cursor’s global store; copying chats into `.paseka/runs`.
- Auto-resume of the next bee on the same provider chat without an operator click.
- Changing Paseka `sessionId` away from `agentId`.

## Further Notes

- [021](./021-provider-session-logs-export.md) already uses `--resume` only to **associate** a brand-new HITL chat. This spec is the deliberate multi-run continuation that 021 called out of scope.
- Console transcript will remain a weak view of resumed work until Agent log export exists; product copy should not promise the old turns as NDJSON.
- Related: [002](./002-queen-console-mvp.md) (Sessions PTY), [006](./006-human-gateway-invites.md) (honey on accept only), [019](./019-model-aliases.md), [020](./020-worktree-branch.md), [interactive sessions](../guide/interactive-sessions.md).
- After implementation: changelog; interactive-sessions guide; Console sessions section; `paseka session` CLI help; 021 Further Notes can stay association-focused.
- Ask before promoting this Draft to **Approved** after review.
