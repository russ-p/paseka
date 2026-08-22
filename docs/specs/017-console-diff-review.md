# Spec 017: Queen Console Diff Review

## Status

**(Implemented)**
Queen Console merge-diff viewer (A), annotated comb comments (B), and final-gate Request changes rework (C). Complements the Reviews baseline in [002-queen-console-mvp](002-queen-console-mvp.md).

## Problem Statement

Before a final merge, the Beekeeper must judge an accumulated worktree diff in Queen Console. Today the Reviews panel dumps a whole-patch HTML view into a nested pane. Widen only hides the queue and switches layout; it does not make a multi-file change scannable (file list vs body, stable scroll, jump-to-file). The Beekeeper still cannot point at a hunk and send that pointer back to a Bee.

Reject already exists as a single freeform note (`INSIGHT/human.feedback`). Narrative prompt memory truncates that note, so a long, structured review never reaches the next AFK pass intact. For `review: required` the runtime can return the task to `ready`. For the **final merge gate** (`review: final` / `_review`) reject does not reopen isolated work on the same worktree — there is no “request changes and keep the merge gate open” path.

The Beekeeper wants a review surface that is good enough to read a diff like a pull request, mark a few lines, and send one packet of feedback for rework — not a full hosted code-review product.

## Solution

Ship Queen Console Reviews in **three slices**. Each slice is useful alone; later slices reuse earlier UI.

**Slice A — Readable merge preview.** Keep the existing three-dot merge-diff API. Replace the dump-the-whole-patch experience with a file-oriented viewer: sticky file list, per-file bodies, jump-to-file, layout that does not destroy scroll on queue poll, truncation called out clearly. No comments.

**Slice B — Annotate and send.** On the same viewer, the Beekeeper clicks a line (or a small contiguous hunk), writes a comment, and keeps drafts in the browser until Submit. Submit writes the full annotated review to the **trail comb** (not into truncated `{{.Insights}}`), announces it with `SIGNAL/artifact.written` when [014](014-artifacts-protocol.md) is available, and publishes a **short** `INSIGHT/human.feedback` for timeline and routing. For `review: required`, existing reject semantics still return the task to `ready`. Bees read the comb file via `ArtifactsDir` (path in the prompt, body not inlined by runtime).

**Slice C — Final-gate rework.** On `review: final`, Submit comments (request changes) does **not** merge. It keeps the merge gate in `waiting_review`, plans and starts a **new** rework task for the last isolated-proposal Bee (same Flight Trail worktree), consuming honey as usual. When that task completes, the Beekeeper is still on the merge gate and reloads an updated merge-diff. Approve remains the only merge path.

## User Stories

### Slice A — viewer

1. As a Beekeeper on a final merge gate, I want a file list of the accumulated worktree diff, so that I can see scope before reading any hunk.
2. As a Beekeeper, I want clicking a file to scroll or show that file’s diff, so that I can review one path at a time.
3. As a Beekeeper, I want the file list to stay visible while I scroll the body, so that I do not lose my place in a large change.
4. As a Beekeeper, I want added, removed, and context lines visually distinct, so that I can scan intent quickly.
5. As a Beekeeper, I want unified (line-by-line) and side-by-side layouts as explicit choices, so that Widen is not the only way to change format.
6. As a Beekeeper, I want Widen to expand the viewer chrome without forcing a format change I did not ask for, so that layout and diff format stay independent.
7. As a Beekeeper, I want Restore to bring back the review queue without resetting file selection or scroll, so that I can glance at metadata and continue.
8. As a Beekeeper, I want queue polling not to re-fetch or re-render an unchanged merge-diff, so that my scroll and file selection survive the 5-second refresh.
9. As a Beekeeper, I want `--stat` (or equivalent file-level summary) above the viewer, so that I know insert/delete scale before opening files.
10. As a Beekeeper, I want `defaultBranch...traceBranch` and tip SHAs visible, so that I know what I am comparing.
11. As a Beekeeper when the patch is truncated, I want a clear truncated warning and still a usable file list for what was returned, so that I know the preview is incomplete.
12. As a Beekeeper when the worktree branch is missing, I want the existing empty/missing states, so that I am not shown a blank fake diff.
13. As a Beekeeper when the merge-diff is empty, I want the existing empty state, so that I do not hunt for files that are not there.
14. As a Beekeeper, I want binary or unrenderable files listed without crashing the viewer, so that I still see they changed.
15. As a Beekeeper with many files, I want a filter or search on the file list, so that I can jump to a path by name.
16. As a Beekeeper, I want keyboard focus to stay in the viewer while I move between files, so that review is not mouse-only.
17. As a Beekeeper, I want the Reviews queue and approve/reject chrome to remain reachable, so that a better viewer does not hide the merge decision.
18. As a Beekeeper on a `review: required` task without a merge-diff, I want Slice A not to pretend there is a final-gate preview, so that mid-trace gates stay honest until a later per-run preview ships.
19. As a Beekeeper, I want the viewer to stay inside the embedded Queen Console SPA, so that I do not run a second review app.
20. As a Beekeeper with a dark Console theme, I want the viewer to match existing contrast, so that the diff is readable next to the queue.

### Slice B — comments and delivery

21. As a Beekeeper, I want to click an added or context line and open a comment box, so that feedback is tied to a place in the diff.
22. As a Beekeeper, I want to comment on a short contiguous range of lines, so that I can mark a hunk without one note per line.
23. As a Beekeeper, I want drafts to stay in the browser until I Submit, so that I can collect several notes into one packet.
24. As a Beekeeper, I want to edit or delete a draft before Submit, so that I can fix a mistyped note.
25. As a Beekeeper, I want a count of pending drafts, so that I know what I will send.
26. As a Beekeeper, I want Submit to be disabled with no drafts and no overall note, so that I cannot send an empty review.
27. As a Beekeeper, I want an optional overall summary in addition to line comments, so that I can state the theme of the request.
28. As a Beekeeper, I want each stored comment to include path, line (or range), side, a short code snippet, and my text, so that a Bee can find the place after the HTML view is gone.
29. As a Beekeeper, I want comments anchored to the merge-diff `headSha` (and path), so that later diffs are not silently treated as the same lines.
30. As a Beekeeper after a Bee reworks the branch, I do not want old comments auto-rebased onto the new patch, so that stale line numbers cannot lie.
31. As a Beekeeper, I want the full review packet written to the trail comb as Markdown, so that Bees can read it as a file instead of a truncated insight line.
32. As a Beekeeper, I want a short `human.feedback` message on the timeline, so that the Flight Path still shows that I requested changes.
33. As a Beekeeper, I do not want the full comment bodies stuffed into `{{.Insights}}`, so that prompt memory limits cannot silently drop the review.
34. As a Bee on the next AFK pass, I want `ArtifactsDir` (and a stable review-comments ref) in my prompt, so that I know where to open the Beekeeper’s notes.
35. As a Bee, I want the comb path, not the file body, injected by runtime, so that token use stays under my control when I read the file.
36. As a Beekeeper on `review: required`, I want Submit comments to use the same reject routing as today (task returns to `ready` and may dispatch), so that mid-task HITL still loops.
37. As a Beekeeper on `review: required`, I still want a plain reject textarea for a single note with no line comments, so that small gates stay fast.
38. As a Beekeeper when NATS is down, I want Submit to fail like today’s reject, so that I do not think the swarm heard me.
39. As a Beekeeper when the comb cannot be written, I want Submit to fail closed (no fake success, no routing-only insight), so that Bees are not dispatched without the file.
40. As a Beekeeper submitting twice, I want the comb file replaced (upsert) with the latest packet, so that Bees read one current review, not a pile of conflicting drafts.
41. As a Beekeeper, I want `artifactKind` for that file to be stable and obvious (`review-comments` from the basename), so that colony `subscribes` can match it later without a new platform kind.
42. As a Beekeeper, I want Queen Console itself to be the producer (`agentId` console), so that announcement does not wait for a Bee run flush.
43. As a Beekeeper using Queen Shell, I want `paseka proposal reject` to keep accepting a freeform `--feedback` string, so that CLI reject does not require the Console viewer.
44. As a Beekeeper using Queen Shell, I want an optional way to attach an existing Markdown file as the comb review packet (same ref), so that I can request changes without the SPA when I already wrote notes.
45. As a Beekeeper on Telegram, I do not need line-comment authoring in this spec, so that the phone gate stays summary-only as in [010](010-telegram-human-gateway.md).
46. As a Guard Bee, I want existing `review.note` unchanged, so that automated review notes stay distinct from Beekeeper annotations.
47. As a Beekeeper, I want comments only on the previewed patch (not arbitrary repo files outside the diff), so that notes always point at proposed change.
48. As a Beekeeper, I want deleted-line comments only when the unified/side view actually shows that line number, so that I cannot annotate a phantom row.
49. As a Beekeeper, I want Submit to clear drafts on success, so that I do not double-send the same packet.
50. As a Beekeeper, I want a failed Submit to keep drafts, so that a bus error does not wipe my review.

### Slice C — final merge rework

51. As a Beekeeper on `review: final`, I want Request changes to **not** merge the worktree, so that incomplete work cannot land on the default branch.
52. As a Beekeeper on `review: final`, I want the merge gate to stay `waiting_review` after Request changes, so that I do not lose the HITL merge slot.
53. As a Beekeeper, I want a new rework task planned and made `ready` on the same Flight Trail, so that ledger and honey accounting stay visible.
54. As a Beekeeper, I want that rework task to use the same isolated worktree as the trail, so that the Bee edits the merge candidate in place.
55. As a Beekeeper, I want the rework Bee to be the last completed task that recorded an isolated `code.proposal`, so that the same Builder (or equivalent) continues the comb.
56. As a Beekeeper when no such Bee can be resolved, I want a clear error and no merge, so that Request changes fail closed.
57. As a Beekeeper, I want rework dispatch to consume honey like any AFK task, so that request-changes loops cannot run forever.
58. As a Beekeeper when honey is exhausted, I want the rework task to block like other AFK work, so that the merge gate does not silently retry.
59. As a Beekeeper while rework is running, I want the merge-diff viewer to remain open and then refresh when the branch tip changes, so that I can re-read after the Bee finishes.
60. As a Beekeeper, I want Approve to remain the only action that merges, so that comments never imply LGTM.
61. As a Beekeeper, I still want a hard reject (feedback without a new rework task) for “stop this approach”, so that Request changes and abandon stay distinct.
62. As a Beekeeper, I want Request changes to include Slice B’s comb packet, so that the rework Bee has line-level notes.
63. As a runtime, I want `AllAFKTasksCompleted` to become false while the rework task is incomplete, so that a second synthetic `_review` is not created.
64. As a runtime, I want an already-`waiting_review` final gate to stay put when AFK work completes again, so that the Beekeeper is not bounced to a new task id.
65. As a Beekeeper, I want the rework task body to point at the comb review file and the original intent, so that the Bee is not dispatched with an empty “fix it” prompt.
66. As a Beekeeper, I want Console to show the new rework task in the board/queue context, so that I can see that work is in flight under the still-open merge gate.
67. As a Beekeeper, I want Request changes disabled while a rework task for this trail is already `ready` or `running`, so that I do not stack duplicate Builder runs.
68. As a Beekeeper after several request-changes rounds, I want each round’s comb file to be the latest packet, so that the Bee is not told to satisfy contradictory historical comments unless I paste them myself.
69. As a Beekeeper, I want Telegram to remain unable to approve final merge, and this spec does not add Telegram Request changes, so that diff-blind merge/rework cannot happen from the phone.
70. As a colony author, I do not want a new orchestrator: Request changes publishes ordinary plan/status/feedback/artifact events, so that bees still react through contracts.

### Cross-cutting

71. As a Beekeeper, I want Slice A to ship without waiting for [014](014-artifacts-protocol.md), so that merge preview becomes usable even if comb is not implemented yet.
72. As a Beekeeper, I want Slice B blocked (or feature-flagged off) until comb write + `ArtifactsDir` exist, so that comments are never routed only through truncated insights.
73. As a Beekeeper, I want Slice C to reuse Slice B’s packet format, so that final-gate rework is not a second comment schema.
74. As a documentation reader, I want Reviews guide text to describe viewer, comments, and final rework separately, so that operators know which slice they have.
75. As a Beekeeper, I want existing approve (optional summary / merge message) unchanged in meaning, so that LGTM stays the merge path from [002](002-queen-console-mvp.md) and [009](009-merge-autostash.md).
76. As a Beekeeper reviewing root proposals, I want this spec not to invent a fake worktree merge-diff, so that root HITL stays on disk / later per-run preview rather than a lying three-dot range.
77. As a Beekeeper, I want large diffs to remain capped at the existing merge-diff size limit, so that Console cannot hang the browser; comments may only target lines present in the returned patch.
78. As a Bee, I want prompt partials to tell me to open the review-comments comb file when present, so that I do not wait for the notes to appear in Insights.
79. As a Beekeeper, I want purge of runs to remove the comb review file with the trail, so that machine-local notes do not linger (same lifecycle as [014](014-artifacts-protocol.md)).
80. As an implementer, I want no new frontend framework required for MVP, so that the embedded static SPA remains the delivery model.

## Implementation Decisions

### 1. Three slices and dependencies

- **A** — Console-only UX on `GET /api/traces/:traceId/merge-diff`. No protocol change. No [014](014-artifacts-protocol.md) dependency.
- **B** — Comment drafts + Submit. **Requires** trail comb directory, `ArtifactsDir` in prompt context, and `SIGNAL/artifact.written` as specified in 014. Do not ship B by stuffing the packet into `human.feedback.message` alone.
- **C** — Final-gate request-changes choreography. Requires B’s packet write. Extends review-domain reject/request-changes; does not introduce a central scheduler.

Slice A shipped without waiting on 014. [014](014-artifacts-protocol.md) and Slices B/C are implemented (comb + human/Console producer + Request changes rework).

### 2. Viewer (Slice A)

- Remain in the embedded Queen Console static SPA; merge preview is a dedicated Reviews sub-page (not a new top-level tab). Approve/reject stay on Reviews detail.
- Keep the merge-diff contract from [002](002-queen-console-mvp.md): three-dot `defaultBranch...traceBranch`, stat, unified patch, 1 MiB truncate, missing/empty flags, `baseSha` / `headSha`.
- Replace whole-patch HTML dump with file list + per-file hunk rendering on the preview page. Reviews detail shows summary (`--stat`, branch/SHA) and **Open merge preview**.
- Polling rule already in 002 stands: do not re-render an unchanged patch for the selected final gate (`state.tab` stays `reviews` while preview is open).
- Diff output format (unified vs side-by-side) is a separate control on the preview page; default unified for long files.
- No new HTTP API required for A unless pagination of the patch is later needed; MVP keeps one truncated blob.

### 3. Comment model (Slice B)

- Drafts are **client-only** until Submit (no bus events per keystroke).
- Each comment: `path`, `side` (new/old as shown), `startLine`, optional `endLine`, optional `snippet`, `body`, plus the preview `headSha` copied from the merge-diff view.
- Overall optional summary field.
- Submit is one packet. Do not persist GitHub-style threads, resolve state, or suggestion patches in MVP.
- After rework, previous comments are **not** mapped onto a new diff. The comb file is the audit of the last Submit; the Beekeeper annotates the new preview from scratch if they request changes again.

### 4. Delivery: comb file + short insight (Slice B)

- Write Markdown to a **stable trail-comb ref** whose basename-stem is `review-comments` (014 kind heuristic). One current file per trail (upsert on each Submit).
- Runtime must **not** inline that file into `{{.Insights}}`. Prompt context exposes `ArtifactsDir`; templates instruct Bees to read the review-comments file when it exists.
- Publish `SIGNAL/artifact.written` for that ref with producer `agentId` console. This is a **human Console write**, not a Bee-run flush: 014’s post-adapter flush does not apply. The human/Console (and optional CLI copy) producer path is specified in [014](014-artifacts-protocol.md) (same path-safety rules).
- Also publish existing `INSIGHT/human.feedback` with `taskId` and a **short** `message` (summary or a one-line “review comments written to comb”). Optional additive `ref` on the payload is allowed if it stays optional and does not replace `message`.
- Fail closed if comb write fails: do not publish feedback/routing events that would dispatch a Bee without the file.
- `paseka proposal reject --feedback` remains the unstructured path. Optional CLI flag to copy a local Markdown file into the same comb ref may ship with B; line-picking stays Console-only.

### 5. Routing today vs Request changes (Slices B and C)

- **`review: required`:** Submit comments uses the existing reject path after a successful comb write: `human.feedback` → reactor returns that task to `ready` and may dispatch. No new task id.
- **`review: final` before C:** Slice B may still write the comb + short feedback, but must **not** merge and must **not** pretend rework was dispatched. UI copy must say notes were stored; Request changes (rework) is C.
- **`review: final` with C:** Request changes = comb upsert + `artifact.written` + short `human.feedback` + **new rework task** (plan + ready) on the same `traceId`. Final gate task stays `waiting_review`. Do not complete it. Do not call merge.
- Plain reject on final (no rework task) remains for abandon-style feedback, matching today’s “publish feedback only” behavior.
- Approve unchanged: merge when policy says so ([008](008-code-proposal-workspaces.md), [009](009-merge-autostash.md)), then complete the final gate.

### 6. Rework task shape (Slice C)

- New `taskId` (not reuse of a `completed` work task; do not rewind completed → ready in MVP).
- Same isolated worktree / trail branch as the merge candidate.
- Bee: last completed non-final task with `proposalWorkspace: isolated`; if missing, fail closed.
- `review` policy: `none` for MVP (Guard is not automatically re-inserted; Beekeeper is still on the final gate). Colony may later subscribe to `artifact.written` / `review-comments` — out of this spec’s required path.
- Task body: include original trail intent pointers plus instruction to apply the comb review-comments file; do not paste the full Markdown into the bus event.
- Honey: normal `energy.consume` on dispatch.
- While rework is `ready` or `running`, disable another Request changes for that trace.
- When rework completes, `ActivateFinalReviewGate` must leave an existing `waiting_review` final task in place (already true if the function no-ops on that status). `AllAFKTasksCompleted` is false during rework, which prevents a second synthetic `_review`.
- Refresh merge-diff when `headSha` changes; do not keep stale comments overlaid.

### 7. Modules (indicative)

- Console SPA Reviews viewer (Slice A) and comment drafts (Slice B).
- Console HTTP: existing merge-diff GET; Submit reuses or slightly extends reject (optional structured comments that the server renders to comb Markdown — server is source of written file so path safety stays on the platform).
- Review domain (`Approve` / `Reject` plus Request-changes for final).
- Protocol: optional `ref` on `human.feedback`; no new insight kind required.
- Artifacts/comb helpers from 014; Console producer path.
- Task ledger / reactor: plan+ready for rework; do not skip-dispatch the rework task (`ShouldSkipDispatch` remains final-gate only).
- Prompt partials: when comb review-comments exists, tell the Bee to read it.
- Telegram: no new actions in this spec ([010](010-telegram-human-gateway.md) still forbids final-merge approve from the phone).

### 8. Gaps left explicit

- Exact Markdown template for the comb file (headings per file vs one list) is an implementation choice as long as path, SHA, lines, snippet, and body are present and stable enough for Bees to parse by eye.
- Whether Submit on `review: required` also writes `artifact.written` or only comb + `human.feedback`: **write both** when 014 exists, so Console and Bee-produced combs share one announcement.
- Per-run `MUTATION/code.proposal` preview for `review: required` remains the [002](002-queen-console-mvp.md) / backlog follow-up; this spec’s viewer should be reusable there later, but A ships on final merge-diff first.

## Testing Decisions

Good tests assert operator-visible behavior and bus/ledger outcomes, not CSS class names or hunk-parser internals.

- **Slice A:** merge-diff API contract unchanged (prior art: Console merge-diff handler tests, worktree three-dot tests). Static/UI contract tests may assert file-list + body containers and that Widen does not uniquely own output format. Polling must not require re-render when patch bytes and SHAs are unchanged (prior art: Reviews static contract tests).
- **Slice B:** Submit writes comb file under the trail comb; path escape rejected; failure to write does not publish `human.feedback`. Successful Submit publishes short feedback and `artifact.written` with `review-comments` kind. Insights projection still truncates narrative lines and does **not** contain the full packet. `review: required` still returns the task to `ready` (prior art: reactor `handleHumanFeedback` tests, Console reject handler tests).
- **Slice C:** Request changes does not merge; final task stays `waiting_review`; a new non-final task is planned and readied; honey consumed on dispatch; second Request changes while rework running is rejected; after rework completes, final task still `waiting_review` and no second `_review` is synthesized (prior art: `ActivateFinalReviewGate` tests, `AllAFKTasksCompleted` tests). Approve still merges (prior art: review approve tests).
- CLI unstructured reject remains green without a comb file.
- Telegram still cannot approve final merge.

## Out of Scope

- Hosted pull-request product: persistent threads, resolve/unresolve, suggested commits, approval reviews vs comment-only, CODEOWNERS, CI checks, review batches as first-class bus types.
- Auto-rebasing comments across new `headSha` values.
- Inlining comb bodies into `{{.Insights}}` or raising insight caps as a substitute for 014.
- Embedding or depending on an external annotation app.
- New frontend framework, Monaco, or a separate review HTTP server.
- Per-run proposal diff for `review: required` (002 remaining item); root-proposal fake merge-diff.
- Telegram line comments or Telegram final Request changes.
- Rewinding a `completed` task to `ready`.
- Automatic Guard re-run after Beekeeper request-changes (final gate remains the human merge review).
- Treating the git patch itself as a comb artifact (014 non-overlap stands); this spec stores **comments**, not the diff.
- Multi-user concurrent review on one trail.
- Object Store offload of the review-comments file.

## Further Notes

- Slice A is the cheapest relief for the current Widen pain and should not wait on 014.
- Slice B exists because Beekeeper feedback is currently the wrong width for the job: routing events must stay small; review packets must be files. That is the same split 014 already chose for research notes vs Insights.
- Slice C is the only slice that changes choreography: annotated Request changes on `review: final` plans a new rework task; plain reject stays abandon-style feedback.
- Human/Console `artifact.written` (not only post-run flush) is specified in [014](014-artifacts-protocol.md); Slice B implements that path, it does not invent a third announcement.
- After A, the same file-oriented viewer is the natural home for per-run proposal preview; keep that as a 002 follow-up rather than blocking 017.
