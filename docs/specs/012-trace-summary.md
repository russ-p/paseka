# Spec 012: Flight trail summary (`trace.summary`)

## Status

**Approved**

Operational `INSIGHT` kind for a human-readable Flight Trail description in Queen Console and as the merge-commit body. Follow-up to the narrative-summary non-goal in [011-trace-title](011-trace-title.md).

## Problem Statement

Beekeepers see a short Flight Trail title (`trace.title`) but no stable description of what the trail accomplished. Queen Console has no trail-level subtitle, so understanding an outcome requires reading the timeline. Isolated final-gate merge commits default to `paseka: merge trace <traceId>` (or a one-line HITL `--merge-message` / `mergeMessage`) with no body. Conventional commit subjects (HITL) and a narrative “what we did” must not be mixed: they serve different roles.

## Solution

Introduce `INSIGHT/trace.summary` — an operational, last-write-wins Flight Trail summary.

Beekeepers see it as a subtitle/description next to the title in Console (list + detail) and as a read-only merge-body preview on final-gate approve. On merge approve, the review layer uses the resolved summary as the git commit body while the subject stays conventional (`mergeMessage` / `--merge-message` / default). The last AFK work task before final review is told (via prompt context) to emit the summary.

Bee-language prompts say **Flight trail summary**; the wire kind stays **`trace.summary`**.

## User Stories

1. As a beekeeper, I want a short description on the Flight Trail in Console list and detail, so that I understand the outcome without reading the full timeline.
2. As a beekeeper, I want that description muted under the trail title, so that title remains the primary scan label.
3. As a beekeeper, I want dashboard recent-trace rows that share the same projection to show the same subtitle when present, so that surfaces stay consistent.
4. As a beekeeper, I want an empty summary to simply omit the subtitle, so that trails without an emit do not show placeholder noise.
5. As a beekeeper approving an isolated final gate in Console, I want a read-only preview of the merge commit body when a trail summary exists, so that I know what will land in git before I approve.
6. As a beekeeper approving when no trail summary exists, I want the body preview hidden, so that the form stays as simple as today’s subject-only UI.
7. As a beekeeper, I want to keep typing a conventional subject in `mergeMessage` / `--merge-message`, so that HITL commit style stays under my control.
8. As a beekeeper, I want the merge commit body filled from `trace.summary` when I did not already supply a body, so that git history carries the narrative without extra typing.
9. As a beekeeper who already pasted a multiline subject+body into `mergeMessage`, I want that body respected and not appended with `trace.summary`, so that HITL override wins.
10. As a beekeeper approving via CLI without `--merge-message`, I want the default subject plus summary body (when present), so that CLI merges are as informative as Console.
11. As a beekeeper approving via Telegram (no merge message), I want the same subject/body composition rules, so that all merge-on-approve paths behave alike.
12. As a beekeeper with no trail summary, I want merge to behave as today (subject / default only, no body), so that empty trails do not break approve.
13. As a bee on the last AFK work task (sole incomplete non-final task), I want prompt guidance that I must emit `INSIGHT/trace.summary`, so that UI and merge receive one trail description.
14. As a bee on an earlier work task, I want no mid-trail must/optional summary instruction, so that emit duty stays on the trail-closing task.
15. As a bee in interactive chat or ad-hoc `bee run` without that ledger condition, I want `IsLastWorkTask` false and no must-block, so that false obligations are not injected.
16. As a bee or beekeeper, I want to overwrite the summary with a newer non-empty emit, so that a better description can replace a weak one (last-write-wins).
17. As a bee, I want empty/whitespace summary emits rejected, so that a trail summary cannot be cleared by accident in v1.
18. As a bee, I want summaries longer than 800 characters rejected, so that the field stays a short description.
19. As a downstream bee, I do not want `trace.summary` injected into `{{.Insights}}`, so that operational trail metadata does not crowd prompt memory.
20. As a beekeeper scanning dashboard Recent insights, I do not want `trace.summary` as a narrative highlight, so that the subtitle on the trail row is the single UI surface for it.
21. As a beekeeper, I want approve `--summary` to remain the `VERIFICATION/task.completed` note, so that completion ack, trail summary, and merge subject stay distinct.
22. As an implementer, I want a single compose step in the review approve layer, so that Console, CLI, and Telegram cannot diverge on body rules.
23. As an implementer, I want `prompts.Context.IsLastWorkTask` derived from the task ledger at AFK dispatch, so that templates can gate emit guidance without hardcoding bee roles.
24. As a beekeeper, I want prose guidance (1–3 sentences, no conventional prefix) for bees without runtime prefix policing, so that validation stays simple and language-agnostic.
25. As a colony author, I want no runtime auto-synthesis and no completion-contract `required` for `trace.summary` in v1, so that missing summary is soft (UI/merge degrade gracefully).

## Implementation Decisions

### Kind and payload

- New operational `INSIGHT` kind: `trace.summary`.
- Payload shape: `{ "kind": "trace.summary", "summary": "<prose>" }` only — no `taskId` or other fields.
- Envelope authorship (`agentId`, `createdAt`, `seq`) is enough for audit.
- Last-write-wins per trace: latest event by `createdAt`, then `seq`.
- Not projected into `{{.Insights}}` (same class as `trace.title` / `task.plan`).
- Does not drive routing or completion contracts.
- Runtime does **not** auto-emit `trace.summary` in v1.

### Validation

- `summary` required after trim; empty/whitespace rejected (no clear/sentinel in v1).
- Maximum length: **800** characters.
- No runtime reject for conventional-commit prefixes; prompts/docs guide bees toward plain prose (1–3 sentences, no `feat:` / `fix:`-style subject lines).

### Resolve order

1. Latest `INSIGHT/trace.summary`
2. Else empty

No soft fallback from narrative `run.summary` (UI or merge). Empty → no Console subtitle; merge has no body from this feature.

### Prompt context: `IsLastWorkTask`

- Add boolean `IsLastWorkTask` to `prompts.Context`.
- Computed at **AFK ledger task dispatch** only.
- Definition: among tasks that are **not** final-review (`review: final`, including synthetic `_review`), count those with `status != completed`. The flag is true iff that count is exactly **1** and that task’s id equals the current `TaskID`.
- Aligns with `AllAFKTasksCompleted` / final-gate readiness (failed/blocked siblings keep the count above zero and suppress the flag — consistent with final gate not opening).
- Interactive `bee chat` and ad-hoc `bee run` that are not this AFK dispatch path → `false`.
- Shared emit partial: when `IsLastWorkTask`, instruct the bee that it **must** emit one `INSIGHT/trace.summary` (prompt-level must; not a runtime completion failure).
- No mid-trail optional guidance in v1; role-specific receiver/builder hardcoding is not used.
- Manual `paseka event emit` of `trace.summary` remains valid anytime.
- **Out of scope for this flag’s consumers in v1:** projecting the summary text as `{{.TraceSummary}}` or session-header display of the resolved summary.

### Bee language and naming

- Prose: **Flight trail summary**; wire: `trace.summary`.
- Anti-confusion (document in reference/CLI notes when implementing):
  - `paseka proposal approve --summary` → `VERIFICATION/task.completed` completion note
  - `mergeMessage` / `--merge-message` → git **subject** (and optional HITL body)
  - `INSIGHT/trace.summary` → trail description + default merge **body**
- Do not rename CLI flags in v1.

### Queen Console

- Extend trace list/detail projection with JSON field `summary` (parallel to `title`), resolved via the resolve order above.
- UI: muted subtitle under title on list and detail; same projection may appear on dashboard recent traces without a separate dashboard design.
- Final-gate approve form: read-only merge-body preview when `summary` is non-empty; hide the block when empty.
- Editable merge body in the form is out of scope.
- Dashboard Recent insights continues to use prompt-memory insight kinds only — `trace.summary` is excluded.

### Merge commit composition

- Compose in the **review approve** layer so Console, CLI, and Telegram share one behavior before `worktree.Merge`.
- **Subject:** trimmed `MergeMessage` / `--merge-message` if non-empty; else existing default (`paseka: merge trace <traceId>`).
- **Body:** resolved `trace.summary` when the subject message does **not** already contain a non-empty body.
- **Already has body:** after the subject (first line / first paragraph), trimmed remainder is non-empty → do **not** append `trace.summary` (HITL override).
- Applies to every merge-on-approve path (Console, CLI, Telegram), including default subject with no explicit merge message.
- Non-merge approves (root / soft gates) unchanged — no commit body.
- Exact `git merge -m` argv shape (single message with `\n\n` vs multiple `-m`) is an implementation choice; prefer minimal API change. Spec requires subject/body semantics only.

### Protocol and docs touchpoints (implementation)

- Register kind in protocol validation (mirror `trace.title`).
- Document under operational insights in the insight-kinds reference.
- Colony prompt partials/init templates: conditional must-emit via `IsLastWorkTask`.
- Console API/SPA + review approve compose + resolve helper analogous to trace title resolution.

## Testing Decisions

- Prefer external behavior tests over internal wiring assertions.
- Protocol: accept valid `trace.summary`; reject empty/whitespace and over-800 summaries.
- Resolve helper: last-write-wins by `createdAt` then `seq`; ignore other insight kinds.
- `IsLastWorkTask`: true only for the sole incomplete non-final task; false with two incomplete work tasks, when current task is final-review, when TaskID unset, and when siblings are failed/blocked (still incomplete).
- Review approve compose: default subject + body; explicit subject + body; multiline HITL body not double-appended; no body when summary absent.
- Console projection: `summary` present/omitted on list/detail enrichment (prior art: `trace.title` console tests).
- Do not require end-to-end adapter runs for v1 of this kind.

## Out of Scope

- Runtime auto-synthesis of `trace.summary`
- Completion-contract `required` / `run_summary`-style policy for trail summary
- Soft fallback from `run.summary` for UI or merge
- `{{.TraceSummary}}` or session-header injection of resolved summary text
- Mid-trail optional emit guidance in prompts
- Role-hardcoded receiver/builder emit lists (replaced by `IsLastWorkTask`)
- Separate summarizer bee
- Clearing/resetting a once-set summary (empty emit or clear kind)
- Editable merge body field in Console approve form
- Replacing conventional merge subject with the summary
- Renaming `paseka proposal approve --summary`
- Hiding `trace.summary` from raw event feeds

## Further Notes

- Closes the deferred narrative-summary non-goal called out in [011-trace-title](011-trace-title.md).
- Neighbor contracts: [insight-kinds](../reference/insight-kinds.md), [task ledger](../reference/task-ledger.md) final review / `_review`, merge approve CLI/Console, [prompt templates](../guide/prompt-templates.md).
- Possible follow-ups: `{{.TraceSummary}}`, mid-trail optional updates, clear/sentinel, richer “last task” enums for parallel leaves.
