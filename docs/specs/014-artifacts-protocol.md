# Spec 014: Trace artifacts protocol

## Status

**(Draft)**
Design from extended discussion: trace-scoped comb, `SIGNAL/artifact.written`, runtime flush on AFK/session exit with **per-run content-hash baseline** (announce only delta). Companion exploration of a general deferred-emit buffer: [015-deferred-event-emit](015-deferred-event-emit.md).

## Problem Statement

Bees need a way to hand structured working documents to later bees and to the Beekeeper without committing them to the colony git tree. Today the closest pattern is colony `SIGNAL/spec.ready` with a repo path under `docs/specs/` — that is a **milestone** for durable design records, not a general scratch/handoff surface.

Per-agent run IPC (`summary.md`, `status.json`) is too narrow and dies with one invocation’s mental model. Narrative `INSIGHT` lines are too short and must not drive routing. Beekeepers cannot browse mid-trail research notes, draft briefs, or checklists in Queen Console as first-class trail material. Mid-run bus publishes would wake subscribers on half-finished combs and waste honey.

## Solution

Introduce a **trace artifacts protocol**: a gitignored comb directory under the flight trail’s runs tree, prompt injection of its absolute path, and a platform `SIGNAL` kind `artifact.written` that announces one or more files **after** the producing bee finishes successfully.

Bees write files into the comb during the run. Before each AFK or interactive run starts, runtime snapshots a **per-run baseline** of content hashes for every file already in the trail comb. On successful exit it rescans the comb, compares hashes to that baseline, and publishes a single batched `artifact.written` only for **new or content-changed** files (empty delta ⇒ no event). Colony choreography may filter on `artifactKind`; workflow milestones such as `spec.ready` stay colony-owned and may promote a comb draft into a committed spec.

Queen Console lists and previews trail artifacts. Purge of runs removes the comb with the trace. This MVP does **not** use a general deferred-emit buffer; flush is artifacts-specific (see [015](015-deferred-event-emit.md) for the alternate generalization).

## User Stories

1. As a bee, I want a stable absolute path to the trail comb in my prompt, so that I can write handoff files without inventing directories.
2. As a bee, I want to create Markdown (and other) files under the comb during AFK or chat work, so that later bees can read structured output.
3. As a bee, I want not to publish bus events on every file save, so that half-finished notes do not trigger the swarm.
4. As a beekeeper, I want the swarm to learn about new comb files only after the producing bee completes successfully, so that choreography sees a finished handoff.
5. As a downstream bee, I want a single `SIGNAL/artifact.written` that may list several files, so that one flush equals one choreography tick.
6. As a colony author, I want `artifactKind` to be colony-meaningful (e.g. `draft-spec`, `research-brief`), so that `subscribes` / `auto_invites` can filter without platform hardcoding product kinds.
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
17. As a reactor subscriber, I want to match `SIGNAL` + `artifact.written` and optionally require a given `artifactKind` present in the batch, so that colony flows can start only when the right handoff exists.
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

### 3. Platform event: `SIGNAL/artifact.written`

- Platform-known kind prefix `artifact.*`; MVP ships `artifact.written` only (upsert / announce writes). Optional `artifact.removed` is out of MVP.
- **Not** named `artifact.ready` — readiness/milestones remain colony kinds (e.g. `spec.ready`).
- Event type is `SIGNAL` so `subscribes` / `auto_invites` / `done_when` may match; narrative `INSIGHT` is not used for comb announcement.
- Payload carries a list of items (canonical field `artifacts`). Each item includes:
  - `ref` — path to the file (repo-relative from colony root, or absolute normalized to repo-relative on publish); must resolve under the trail comb.
  - `artifactKind` — short colony token (see heuristics).
  - `title` — optional human label (see heuristics).
  - optional producer metadata (`agentId` of the flushing run) for Console.
- Single-item sugar: top-level `ref` / `artifactKind` / `title` allowed on emit helpers or tests; runtime normalizes to a one-element `artifacts` array before publish.
- One successful flush ⇒ **one** bus event listing only the **delta** items for that run (batch). Do not publish N events for N files in MVP. Unchanged files (hash matches baseline) are omitted from the payload.

### 4. Heuristics for `artifactKind` and `title`

When building flush items from files on disk (no explicit manifest override):

| Field | Rule |
| ----- | ---- |
| `artifactKind` | Basename of the file with the final extension removed (e.g. `draft-spec.md` → `draft-spec`, `risks.json` → `risks`). Nested dirs do not prefix the kind unless a later manifest says otherwise — kind is basename-stem only for MVP. |
| `title` | Optional. For `.md` files only: first non-empty line, with a leading ATX heading marker (`#`…`#` plus following space) stripped if present; trim whitespace; if empty after strip, omit `title`. Non-Markdown: omit `title` unless supplied by an optional staging manifest. |

Explicit staging manifest entries (if present for the run) override heuristics for those refs. MVP may ship scan-only without a bee-written manifest; if a manifest is added in the same milestone, document override precedence: manifest field > heuristic > omit.

### 5. Per-run baseline and flush on exit (MVP mechanism)

Bees are **not** required to call `paseka event emit` for comb files in MVP. Announcement is runtime-owned and **delta-based**, in the same spirit as attributable workspace git diff (baseline before work → compare after).

**Baseline (before adapter / session start)**

- After the run directory exists and before the bee process starts, runtime walks the trail comb with the same file-inclusion rules as flush.
- For each regular file, record `ref` → content hash. Recommended algorithm: **SHA-256** of file bytes (CRC32 acceptable only if documented; prefer SHA-256 for collision resistance on drafts).
- Persist the map under the **producing run** directory (inspectable baseline snapshot for that `agentId`). Empty comb ⇒ empty baseline map.
- Do **not** use mtime-only comparison for MVP publish decisions (unreliable across FS/copy); hash is authoritative.
- Failed baseline walk (permissions, I/O): fail closed for artifacts flush later (skip publish + warn) rather than announcing an untrusted full comb; bee run itself may still proceed unless a harder policy is chosen at implementation — default: warn + treat baseline as empty only if comb was unreadable at start is **rejected**; prefer skip flush on baseline error.

**Flush (on successful AFK completion or successful interactive session end)**

1. Walk the trail comb again (same inclusion rules: recursive regular files; skip dotfiles / editor junk as noted in Further Notes).
2. For each file, compute content hash.
3. Classify:
   - **added** — `ref` absent from baseline;
   - **changed** — `ref` in baseline and hash ≠ baseline hash;
   - **unchanged** — hash matches baseline (omit from event);
   - **removed** — in baseline but missing on disk — **MVP: omit from `artifact.written`** (no delete scent; see Out of Scope).
4. Build `artifacts[]` only for added + changed, with heuristics (and optional manifest overrides).
5. If the delta list is empty, **skip publish** (no-op comb ⇒ no Signal).
6. Else publish **one** `SIGNAL/artifact.written` with that batch (optional producer `agentId` = this run).

**Policies**

- **Do not flush** on failed/cancelled runs; leave files on disk; baseline file may remain for audit.
- Idempotency within one run: flush at most once per successful completion.
- Cross-run: each new run takes its **own** baseline at start. A later bee that edits a file sees changed hash vs its baseline and announces that `ref` again (upsert scent). A later bee that only reads the comb publishes nothing.
- Ordering relative to other post-run publishes: flush `artifact.written` **before** auto `MUTATION/code.proposal` and before synthesized `INSIGHT/run.summary`. If proposal is skipped, still flush when delta is non-empty.

### 6. Relationship to `spec.ready` and ideation

- Keep colony `spec.ready` as the durable-spec milestone ([005-feature-ideation-flow](005-feature-ideation-flow.md)).
- Allowed promotion path: comb draft (`artifact.written`) → write/update `docs/specs/…` → `spec.ready`.
- Default grill `done_when` remains `spec.ready` + file-at-repo-`ref`; do not switch defaults to `artifact.written` in this spec.
- Extend file-at-`ref` checks used by invites so refs under the trail comb are accepted when rules explicitly point there.

### 7. Queen Console

- Trail detail: Artifacts list (kind, title, ref, updated/producer when known) + Markdown preview for `.md`.
- Prefer bus-backed list when events exist; may merge with filesystem scan for mid-run / failed-run visibility (label staged vs announced if both exist).
- Do not treat adapter `result.json` artifacts or Object Store diffs as comb entries.

### 8. Validation and safety

- Reject flush items whose resolved path escapes the trail comb.
- Empty comb at flush, or non-empty comb with empty delta → no event.
- Extremely large files: still hash for baseline/delta; payload references path only (no inline bodies). Optional size warning in logs; no Object Store requirement in MVP.
- Platform validators know `artifact.written` shape; they do **not** enumerate allowed `artifactKind` values.
- Baseline hashes are **not** required on the bus payload in MVP (optional later for clients); disk baseline under the run is enough for flush logic and inspect.

### 9. Explicit non-overlap

| Concept | Role |
| ------- | ---- |
| Comb + `artifact.written` | Trace working handoff files + scent after success |
| Colony `spec.ready` | Milestone: durable spec path ready for breakdown |
| Adapter `Artifacts` on run result | Process outputs (stdout, stderr, diff capture) |
| Object Store `ref` on mutations | Large blob offload for proposals |
| [015 deferred emit](015-deferred-event-emit.md) | Possible future general “queue then flush any kind”; **not** required for this MVP |

### 10. Modules (indicative)

- Runs / colony path helpers — ensure comb dir; resolve refs; read/write per-run baseline hash map.
- Prompt context — `ArtifactsDir`.
- Protocol — payload types for `artifact.written` / batch items.
- Runtime — capture baseline before adapter/session start; on success compute delta, heuristics, flush (same post-run seam as proposal auto-publish). Prior art: attributable git baseline before run.
- Invites completion — allow comb refs when configured.
- Console API + UI — list/preview.
- Prompt partials / bee docs — how to write the comb; remind that announcement is automatic on success **only for files this run added or changed**.
- Purge — already covered if comb lives under runs; verify docs/tests.

## Testing Decisions

Good tests assert external behavior: directory injection, flush/skip policies, payload shape/heuristics, path safety, Console/API projections — not internal helper names.

Modules / areas:

- Protocol validation for `artifact.written` (single sugar → array; required fields; kind prefix).
- Heuristics unit tests: basename-stem kinds; Markdown title strip of leading `#`; non-md omits title; nested path kind still basename-stem.
- Baseline + flush integration: pre-existing unchanged comb file after success → no Signal; new file → Signal with that item; edited file (hash change) → Signal; failed run → no Signal; empty comb → no Signal; second run editing same `ref` → second Signal; path escape rejected; baseline I/O failure → no unsafe full-comb announce.
- Prompt context: `ArtifactsDir` absolute and under the trail runs tree.
- Invite `done_when` with comb `ref` when configured (prior art: invite completion tests around `spec.ready` + `require_file`).
- Console/API list+preview smoke (prior art: trace detail handlers in console package tests).

Prior art: auto-publish proposal tests in runtime; `ProcessEventInput` validation patterns; invite completion tests; runs projection tests.

## Out of Scope

- General deferred `paseka event emit --defer` for arbitrary kinds ([015](015-deferred-event-emit.md)).
- `artifact.ready` or other milestone synonyms for comb writes.
- Changing default ideation grill completion from `spec.ready` to comb events.
- Treating git diffs / `diff.patch` as comb artifacts; merge-diff APIs; Object Store sync of comb files.
- Auto-inlining comb file bodies into `{{.Insights}}` or prompts.
- Cross-apiary replication of pending comb state.
- `artifact.removed` / announcing deletes or renames detected vs baseline; comb GC independent of runs purge.
- Trace-global “last announced hash” registry separate from per-run baseline (MVP baseline is always per producing run at start).
- Rich manifest schema beyond optional simple overrides (versioned index formats, signatures).
- Putting content hashes on the bus payload (optional later).
- MCP tools for artifact write/flush.
- Telegram surfaces for artifact preview.

## Further Notes

- Naming collision: English “artifact” already means adapter result blobs and Object Store objects. User-facing copy may say **comb** / **trail artifacts**; wire kind stays `artifact.written`.
- If [015](015-deferred-event-emit.md) later ships, this MVP flush can remain as a convenience (“files on disk imply deferred intent”) or bees can switch to explicit deferred emits that coalesce into one `artifact.written` batch — decide at 015 approval time without blocking 014.
- Untracked whether recursive scan should ignore `*.tmp` / editor swap files — recommend skipping names starting with `.` and ending in `~` or `.swp` in implementation; list in release notes if tightened later.
