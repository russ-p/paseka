# Spec 014: Trace artifacts protocol

## Status

**(Implemented)**
Trace comb, `SIGNAL/artifact.written`, bee scan-flush, Console list/preview, export `--include artifacts`, human producer helper.

## Problem Statement

Bees need a way to hand structured working documents to later bees and to the Beekeeper without committing them to the colony git tree. Today the closest pattern is colony `SIGNAL/spec.ready` with a repo path under `docs/specs/` — that is a **milestone** for durable design records, not a general scratch/handoff surface.

Per-agent run IPC (`summary.md`, `status.json`) is too narrow and dies with one invocation’s mental model. Narrative `INSIGHT` lines are too short and must not drive routing. Beekeepers cannot browse mid-trail research notes, draft briefs, or checklists in Queen Console as first-class trail material. Mid-run bus publishes would wake subscribers on half-finished combs and waste honey.

## Solution

Introduce a **trace artifacts protocol**: a gitignored comb directory under the flight trail’s runs tree, prompt injection of its absolute path, and a platform `SIGNAL` kind `artifact.written` that announces one or more files.

Bees write files into the comb during the run. Before each AFK or interactive run starts, runtime snapshots a **per-run baseline** of SHA-256 content hashes. On successful exit it rescans the comb and publishes a single batched `artifact.written` only for **new or content-changed** files (empty delta ⇒ no event). Humans (Queen Console, optional CLI copy) may write a comb file and publish `artifact.written` immediately — not via bee-run flush — with the same path-safety rules.

Colony `subscribes` / `auto_invites` / `done_when` match `SIGNAL` + kind `artifact.written`. Payload field `artifactKind` is colony-meaningful metadata for bees and prompts; MVP does **not** extend reactor YAML matchers to nested payload fields. Workflow milestones such as `spec.ready` stay colony-owned and may promote a comb draft into a committed spec.

Queen Console lists comb files from the filesystem and labels them staged vs announced using bus events. Purge of runs removes the comb with the trace. Bee-run announcement is artifacts-specific scan flush; it does not require the [015](015-deferred-event-emit.md) pending buffer. When both exist, follow 015’s complement+skip rule (see Implementation Decisions).

## User Stories

1. As a bee, I want a stable absolute path to the trail comb in my prompt, so that I can write handoff files without inventing directories.
2. As a bee, I want to create Markdown (and other) files under the comb during AFK or chat work, so that later bees can read structured output.
3. As a bee, I want not to publish bus events on every file save, so that half-finished notes do not trigger the swarm.
4. As a beekeeper, I want the swarm to learn about new comb files from a bee only after that bee completes successfully, so that choreography sees a finished handoff.
5. As a downstream bee, I want a single `SIGNAL/artifact.written` that may list several files, so that one flush equals one choreography tick.
6. As a colony author, I want `artifactKind` on each item to be colony-meaningful (e.g. `draft-spec`, `research-brief`), so that bees and prompts can distinguish handoffs without the platform enumerating product kinds.
7. As a bee that only drops `research.md`, I want runtime to derive `artifactKind` as `research` when I did not declare one, so that casual writes still get a stable kind.
8. As a beekeeper viewing an artifact in Console, I want an optional human `title` inferred from the first Markdown heading line when missing, so that the UI is readable without extra emit fields.
9. As a bee promoting understanding into a durable spec, I want to keep using colony `spec.ready` after copying or writing under `docs/specs/`, so that milestones stay distinct from comb writes.
10. As a grilling Drone, I want to iterate draft notes in the comb across saves without closing the invite, so that only `spec.ready` (or the invite’s `done_when`) completes the session contract.
11. As a Scout or survey bee, I want to leave research and risk notes in the comb for Builder/Drone, so that `{{.Insights}}` is not overloaded with long prose.
12. As a beekeeper in Queen Console, I want a trail Artifacts panel listing kind, title, path, and producing agent when known, so that I can review handoffs without git or NATS spelunking.
13. As a beekeeper, I want Markdown preview for comb `.md` files, so that I can read drafts in the Console.
14. As a beekeeper, I want missing or empty combs to show an empty state, so that trails without artifacts stay quiet.
15. As a beekeeper, I want filesystem projection of comb files even before flush when a bee is still running, labeled as not yet announced on the bus when applicable, so that HITL inspection is possible mid-session without waking subscribers.
16. As a bee author of prompts, I want `{{.ArtifactsDir}}` documented like `{{.ResultFile}}`, so that templates can instruct write/read locations consistently.
17. As a reactor subscriber, I want to match `SIGNAL` + `artifact.written` without a new YAML matcher on nested `artifactKind`, so that MVP choreography stays on existing subscribe/invite kind matching.
18. As an invite rule author, I want optional `done_when` on `artifact.written` plus file-at-`ref` under the comb, so that some HITL flows can complete on comb handoff without a committed spec — without making that the default grill path.
19. As a beekeeper running `paseka purge --runs`, I want comb files removed with the trace runs tree, so that ephemeral handoffs do not linger.
20. As a beekeeper, I want failed or cancelled runs not to flush `artifact.written` by default, so that broken work does not dispatch the swarm; files may still remain on disk for audit.
21. As a beekeeper inspecting a failed run, I want comb files still visible on disk, so that I can recover useful drafts manually.
22. As a platform implementer, I want `artifact.*` recognized as a platform kind prefix (like ledger kinds), so that validation and projections are shared while colony semantics stay in `artifactKind`.
23. As a bee, I want a single-file sugar (`ref` / `artifactKind` / `title` on the payload) normalized to a one-element `artifacts` array, so that prompts stay simple when only one file matters.
24. As a guard or builder, I do not want git diffs treated as comb artifacts in this protocol, so that code review stays on workspace disk + `MUTATION/code.proposal` (diff materialization may be a separate run-IPC concern).
25. As a beekeeper, I do not want comb paths committed to git, so that working memory stays machine-local and gitignored with runs.
26. As a downstream bee, I want prompt guidance to pass paths (and short titles), not full file bodies inlined by runtime, so that token use stays under bee control.
27. As a colony using Object Store later, I want MVP to require only local comb files, so that NATS blob offload is optional follow-up.
28. As a beekeeper on an interactive session, I want flush of `artifact.written` when the session ends successfully (same success policy as AFK), so that HITL and AFK share one announcement rule.
29. As a bee writing non-Markdown files (e.g. `.json`, `.yaml`), I want them included in the flush with `artifactKind` from the basename without extension, and no Markdown title heuristic, so that structured IPC works.
30. As a platform, I want path traversal rejected (`ref` must stay under the trail comb), so that events cannot point outside the audit boundary.
31. As a beekeeper, I want duplicate basenames or nested paths under the comb supported as distinct `ref`s, so that `research/api.md` and `research.md` do not collide in the batch.
32. As an implementer, I want flush ordered before auto `code.proposal` publish when both apply, so that notes are visible on the bus not later than the mutation scent (exact ordering fixed in implementation decisions).
33. As a bee that rewrites the same comb file across runs, I want a successful run whose content hash changed for that `ref` to flush `artifact.written` including that item (upsert scent), so that subscribers can react to updates without an `artifact.ready` milestone word.
34. As a beekeeper, I want Console to show last-write metadata per `ref` when available, so that I know which run last touched a handoff.
35. As a documentation reader, I want this protocol distinguished from adapter `Artifact` blobs on `result.json` and from Object Store diff offload, so that three different “artifact” words do not collapse.
36. As a beekeeper, I want a successful run that did not create or change any comb file to publish no `artifact.written`, so that no-op bees do not re-wake subscribers on stale comb contents.
37. As a platform, I want the change set computed against a baseline captured at **this run’s start** (not against the last bus event), so that attribution matches “what this bee changed,” analogous to attributable git diff.
38. As an implementer, I want the baseline stored under the producing run directory as an inspectable hash map, so that tests and Beekeepers can see pre-run comb state.
39. As a bee that only renames or deletes a comb file in MVP, I accept that delete/rename may not emit a dedicated signal yet, so that MVP stays write-focused (`artifact.removed` remains follow-up).
40. As a beekeeper submitting review comments in Queen Console, I want the platform to write the comb file and publish `artifact.written` with producer `console` immediately, so that Slice B of [017](017-console-diff-review.md) shares one announcement with bee-produced combs.
41. As a beekeeper using CLI, I want an optional later copy-into-comb helper to publish with the same human producer rules, so that unstructured reject stays available and line-picking stays Console-only.
42. As a bee author, I want prompt partials to say “write the file; runtime announces on success,” so that I do not emit `artifact.written` on every save; live emit stays allowed for debugging and is not hard-denied.
43. As a downstream bee, I want to read `artifactKind` from the event payload (or from the filename under `ArtifactsDir`), so that I can ignore irrelevant handoffs without a new subscribe matcher.
44. As a Beekeeper exporting a trail report, I want `paseka export --include artifacts` to inline comb file bodies in the HTML/Markdown dump, so that I can share handoffs without NATS or Console access (distinct from auto-inlining comb into `{{.Insights}}`).

## Implementation Decisions

### 1. Comb location and lifecycle

- Comb root for a flight trail: under that trail’s runs tree, sibling to per-agent run directories (conceptually `runs/<traceId>/artifacts/`).
- Entire comb is gitignored with runs; removed by existing runs purge for the trace.
- Per-agent run directories remain for IPC (`prompt.txt`, `summary.md`, …). Comb is **trace-scoped**, not agent-scoped, so multiple bees read/write the same handoff surface.
- Creating the comb directory is lazy on first write or on first template exposure (runtime ensures it exists when injecting `ArtifactsDir`).

### 2. Prompt context

- Add `ArtifactsDir` (absolute path to the trail comb) to prompt template context, set at dispatch for AFK and interactive sessions.
- Document alongside existing result-file variables; prompts instruct bees to write/read under that path.
- Runtime does not auto-inline file contents into prompts.
- Partials: announcement is automatic on successful bee exit **only for files this run added or changed**. Do not teach mid-run `event emit` of `artifact.written` as the normal path.

### 3. Platform event: `SIGNAL/artifact.written`

- Platform-known kind prefix `artifact.*`; MVP ships `artifact.written` only (upsert / announce writes). Optional `artifact.removed` is out of MVP.
- **Not** named `artifact.ready` — readiness/milestones remain colony kinds (e.g. `spec.ready`).
- Event type is `SIGNAL` so `subscribes` / `auto_invites` / `done_when` may match the **event kind** `artifact.written`; narrative `INSIGHT` is not used for comb announcement.
- Payload carries a list of items (canonical field `artifacts`). Each item includes:
  - `ref` — path to the file (repo-relative from colony root, or absolute normalized to repo-relative on publish); must resolve under the trail comb (typically under `.paseka/runs/<traceId>/artifacts/`).
  - `artifactKind` — short colony token (see heuristics).
  - `title` — optional human label (see heuristics).
- Envelope / item **must** carry producer `agentId`:
  - Bee-run flush: the producing run’s `agentId`.
  - Human Console write: `console`.
  - Optional CLI copy-into-comb: a stable human producer id (same family as `console`; exact token at implementation, documented in CLI help).
- Single-item sugar: top-level `ref` / `artifactKind` / `title` allowed on emit helpers or tests; runtime normalizes to a one-element `artifacts` array before publish.
- One successful **bee-run** flush ⇒ **one** bus event listing only the **delta** items for that run (batch). Do not publish N events for N files in MVP. Unchanged files (hash matches baseline) are omitted from the payload.
- Human/Console (and CLI copy) publishes **one** `artifact.written` for the refs just written (typically a one-element batch), immediately after a successful comb write — not via post-adapter flush.

### 4. Two producer paths

Same path-safety and payload shape. Different timing.

| Path | When | Baseline | Publish |
| ---- | ---- | -------- | ------- |
| **Bee run** | AFK adapter success or interactive session success | Per-run SHA-256 map at start | Scan flush: added+changed only; skip if empty delta |
| **Human** (Queen Console; optional CLI copy) | After the platform writes the comb file | None (not a bee run) | Immediate `artifact.written` for those refs; fail closed if write fails (do not publish) |

Bee-run post-adapter flush does **not** apply to Console/CLI human writes. [017](017-console-diff-review.md) Slice B uses the human path (`review-comments` basename-stem).

**Live `paseka event emit` of `artifact.written`:** not hard-denied (debug / HITL). Not the intended bee path. Mid-run live emit can wake subscribers on a half-finished comb; that is operator/prompt discipline, not a validator deny-list in this spec. [015](015-deferred-event-emit.md) already allows `--defer` for this kind.

### 5. Choreography matching (MVP)

- Reactor / invite matching stays **type + event kind**: `SIGNAL` + `artifact.written`.
- Matching on nested `artifacts[].artifactKind` is **out of MVP** (no new YAML matcher). Colonies that care about a specific handoff subscribe to `artifact.written` and let the bee read `ArtifactsDir` / payload.
- Invite `done_when` may use `artifact.written` plus file-at-`ref` under the trail comb when a rule **explicitly** points at a comb path. Default ideation grill remains `spec.ready` + committed spec path ([005](005-feature-ideation-flow.md)). File-at-`ref` for gitignored comb paths is a platform extension, not a git-tracked repo file.

### 6. Heuristics for `artifactKind` and `title` (scan-only)

MVP is **scan-only**. No bee-written staging manifest, no override file. Follow-up if a manifest is needed later (then: manifest field > heuristic > omit).

When building items from files on disk:

| Field | Rule |
| ----- | ---- |
| `artifactKind` | Basename of the file with the final extension removed (e.g. `draft-spec.md` → `draft-spec`, `review-comments.md` → `review-comments`, `risks.json` → `risks`). Nested dirs do not prefix the kind — basename-stem only. |
| `title` | Optional. For `.md` files only: first non-empty line, with a leading ATX heading marker (`#`…`#` plus following space) stripped if present; trim whitespace; if empty after strip, omit `title`. Non-Markdown: omit `title`. |

### 7. Scan inclusion rules

Recursive regular files under the trail comb. **Skip** (do not baseline, hash, or announce):

- names starting with `.`
- names ending in `~`, `.swp`, or `.tmp`

Same rules for baseline walk, bee-run flush, and Console filesystem listing.

### 8. Per-run baseline and flush on exit (bee path)

Bees are **not** required to call `paseka event emit` for comb files. Bee-run announcement is runtime-owned and **delta-based**, in the same spirit as attributable workspace git diff (baseline before work → compare after).

**Baseline (before adapter / session start)**

- After the run directory exists and before the bee process starts, runtime walks the trail comb with the scan inclusion rules above.
- For each included regular file, record `ref` → **SHA-256** of file bytes. Do not use CRC32. Do not use mtime-only comparison.
- Persist the map under the **producing run** directory (inspectable baseline snapshot for that `agentId`). Empty comb ⇒ empty baseline map.
- Failed baseline walk (permissions, I/O): **skip later artifacts flush** and warn. The bee run **continues**. Do **not** treat a failed walk as an empty baseline (that would announce the whole comb as added).

**Flush (on successful AFK completion or successful interactive session end)**

1. Walk the trail comb again (same inclusion rules).
2. For each included file, compute SHA-256.
3. Classify:
   - **added** — `ref` absent from baseline;
   - **changed** — `ref` in baseline and hash ≠ baseline hash;
   - **unchanged** — hash matches baseline (omit from event);
   - **removed** — in baseline but missing on disk — **MVP: omit from `artifact.written`** (no delete scent; see Out of Scope).
4. Build `artifacts[]` only for added + changed, with heuristics. Producer `agentId` = this run.
5. If the delta list is empty, **skip publish** (no-op comb ⇒ no Signal).
6. Else publish **one** `SIGNAL/artifact.written` with that batch.

**Policies**

- **Do not flush** on failed/cancelled runs; leave files on disk; baseline file may remain for audit.
- Idempotency within one run: flush at most once per successful completion.
- Cross-run: each new run takes its **own** baseline at start. A later bee that edits a file sees changed hash vs its baseline and announces that `ref` again (upsert scent). A later bee that only reads the comb publishes nothing.
- Ordering on successful bee completion (with [015](015-deferred-event-emit.md) present): (1) flush deferred pending FIFO; (2) artifacts scan flush **unless** skipped by coexistence; (3) auto `MUTATION/code.proposal`; (4) synthesized `INSIGHT/run.summary`; (5) `post_exec`. If 015 pending is empty, step 1 is a no-op. If proposal is skipped, still scan-flush when delta is non-empty.

### 9. Relationship to 015 (coexistence)

[015](015-deferred-event-emit.md) is **Implemented**. This spec does **not** depend on the pending buffer for MVP artifacts.

**Complement + skip:** keep scan flush enabled. If a pending or already-flushed deferred `artifact.written` exists for that run when scan would run, **skip** the scan-synthesized event. Prefer one announcement path per bee run. Do not silently merge multiple deferred `artifact.written` lines (batch only via one event with `artifacts[]`). Joint coverage: eval colony cases `12-artifact-scan-flush`, `13-artifact-deferred-skip`, `14-artifact-handoff`.

### 10. Relationship to `spec.ready` and ideation

- Keep colony `spec.ready` as the durable-spec milestone ([005-feature-ideation-flow](005-feature-ideation-flow.md)).
- Allowed promotion path: comb draft (`artifact.written`) → write/update `docs/specs/…` → `spec.ready`.
- Default grill `done_when` remains `spec.ready` + file-at-repo-`ref`; do not switch defaults to `artifact.written` in this spec.
- Extend file-at-`ref` checks used by invites so refs under the trail comb are accepted when rules explicitly point there.

### 11. Queen Console

- Trail detail: Artifacts list (kind, title, ref, updated/producer) + Markdown preview for `.md`.
- **Always** list from the filesystem scan (same inclusion rules). Merge bus events to label **announced** vs **staged** (on disk, no matching `artifact.written` yet — mid-run or failed run). Empty comb ⇒ empty state.
- Human Submit of review comments uses the human producer path (section 4); list/preview of comb files can ship with this spec before 017 Slice B.
- Do not treat adapter `result.json` artifacts or Object Store diffs as comb entries.

### 12. Validation and safety

- Reject any publish (flush or human) whose resolved path escapes the trail comb.
- Empty comb at bee flush, or non-empty comb with empty delta → no event.
- Extremely large files: still hash for baseline/delta; payload references path only (no inline bodies). Optional size warning in logs; no Object Store requirement in MVP.
- Platform validators know `artifact.written` shape; they do **not** enumerate allowed `artifactKind` values.
- Baseline hashes are **not** required on the bus payload in MVP; disk baseline under the run is enough for flush logic and inspect.

### 13. Explicit non-overlap

| Concept | Role |
| ------- | ---- |
| Comb + `artifact.written` | Trace working handoff files + scent (bee: after success delta; human: after write) |
| Colony `spec.ready` | Milestone: durable spec path ready for breakdown |
| Adapter `Artifacts` on run result | Process outputs (stdout, stderr, diff capture) |
| Object Store `ref` on mutations | Large blob offload for proposals |
| [015 deferred emit](015-deferred-event-emit.md) | General queue-then-flush for any kind; coexistence = skip scan if deferred `artifact.written` already pending/flushed for that run |
| [017](017-console-diff-review.md) review-comments file | Human comb write + `artifact.written`; comments body not an insight |

### 14. Modules (indicative)

- Runs / colony path helpers — ensure comb dir; resolve refs; SHA-256 baseline map; scan skip-list.
- Prompt context — `ArtifactsDir`.
- Protocol — payload types for `artifact.written` / batch items; required producer `agentId`.
- Runtime — capture baseline before adapter/session start; on success compute delta, heuristics, flush; skip scan when 015 deferred `artifact.written` already covers the run. Prior art: attributable git baseline before run.
- Human producer — Console (and optional CLI) write + publish with path safety; not the adapter exit hook.
- Invites completion — allow comb refs when configured; match event kind only.
- Console API + UI — FS list + announced/staged + preview.
- Prompt partials / bee docs — write the comb; announcement on success for this run’s delta.
- Purge — already covered if comb lives under runs; verify docs/tests.

### 15. Export inlining (opt-in)

- `paseka export --include artifacts` walks the trail comb (same skip-list) and inlines text bodies into the self-contained report (HTML: Markdown rendered; Markdown export: nested sections or fenced blocks).
- Default export omits comb bodies. Binary/invalid UTF-8/oversize files get an omitted note, not inline content.
- Distinct from runtime prompt inlining (still out of scope for `{{.Insights}}`).

## Testing Decisions

Good tests assert external behavior: directory injection, flush/skip policies, payload shape/heuristics, path safety, Console/API projections — not internal helper names.

Modules / areas:

- Protocol validation for `artifact.written` (single sugar → array; required fields including producer `agentId`; kind prefix).
- Heuristics unit tests: basename-stem kinds; Markdown title strip of leading `#`; non-md omits title; nested path kind still basename-stem; no manifest required.
- Scan skip-list: `.hidden`, `foo~`, `foo.swp`, `foo.tmp` never appear in baseline, flush, or Console list.
- Baseline + flush integration: SHA-256; pre-existing unchanged comb file after success → no Signal; new file → Signal with that item and this run’s `agentId`; edited file (hash change) → Signal; failed run → no Signal; empty comb → no Signal; second run editing same `ref` → second Signal; path escape rejected; baseline I/O failure → bee still succeeds, **no** full-comb announce.
- 015 coexistence (when both present): pending/flushed deferred `artifact.written` ⇒ no second scan event for that run.
- Human producer: Console (or test double) write + immediate `artifact.written` with producer `console`; write failure ⇒ no event; path escape rejected. Does not require a successful bee run.
- Prompt context: `ArtifactsDir` absolute and under the trail runs tree.
- Invite `done_when` with comb `ref` when configured (prior art: invite completion tests around `spec.ready` + `require_file`). Subscribe/invite fixtures match `artifact.written` only — no nested `artifactKind` matcher in MVP.
- Console/API: FS list always; staged vs announced labels; preview smoke (prior art: trace detail handlers in console package tests).

Prior art: auto-publish proposal tests in runtime; `ProcessEventInput` validation patterns; invite completion tests; runs projection tests; 015 pending flush ordering tests.

## Out of Scope

- General deferred `paseka event emit --defer` for arbitrary kinds (owned by [015](015-deferred-event-emit.md); this spec only cites coexistence).
- Extending `subscribes` / `auto_invites` / `done_when` YAML to match nested `artifactKind`.
- Bee-written staging manifest or CRC32 hashes.
- Hard-deny of live `event emit` for `artifact.written`.
- `artifact.ready` or other milestone synonyms for comb writes.
- Changing default ideation grill completion from `spec.ready` to comb events.
- Treating git diffs / `diff.patch` as comb artifacts; merge-diff APIs; Object Store sync of comb files.
- Auto-inlining comb file bodies into `{{.Insights}}` or prompts.
- Cross-apiary replication of pending comb state.
- `artifact.removed` / announcing deletes or renames detected vs baseline; comb GC independent of runs purge.
- Trace-global “last announced hash” registry separate from per-run baseline (MVP baseline is always per producing run at start).
- Rich manifest schema (versioned index formats, signatures).
- Putting content hashes on the bus payload (optional later).
- MCP tools for artifact write/flush.
- Telegram surfaces for artifact preview.
- 017 Slice B/C UI (line comments, request-changes choreography) — this spec only supplies comb + human producer.

## Further Notes

- Naming collision: English “artifact” already means adapter result blobs and Object Store objects. User-facing copy may say **comb** / **trail artifacts**; wire kind stays `artifact.written`.
- Scan flush remains the bee convenience (“files on disk imply deferred intent”). Explicit `--defer artifact.written` is optional once 015 is in the colony; coexistence prevents double announce.
- CLI copy-into-comb may ship with 017 Slice B rather than the first 014 Console list/preview slice; the producer contract is defined here so B does not invent a third event shape.
