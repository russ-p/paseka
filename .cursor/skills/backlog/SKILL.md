---
name: backlog
description: Maintain the project backlog in docs/plans/backlog.md. Use when capturing deferred ideas from feature planning, logging bugs or implementation assumptions to revisit later, adding/updating/removing backlog items, or when the user mentions backlog, deferred follow-ups, or docs/plans/backlog.md.
---

# Backlog

Maintain [docs/plans/backlog.md](../../../docs/plans/backlog.md) — deferred work and durable caveats that are useful soon but not in the active feature. Specs: [specs](../specs/SKILL.md). Shipped notes and indexes: [documentation](../documentation/SKILL.md).

## What belongs here

Capture during planning, design talk, or implementation when the item is valuable but **not** part of the current change:

| Kind | Put in backlog | Examples |
| ---- | -------------- | -------- |
| **idea** | Useful near-term product/tech idea, deferred or orthogonal to the active feature | `system.kill`, Windows release builds |
| **follow-up** | Explicit leftover from a shipped or in-progress spec | `proposal_paths` allowlist from 008 |
| **bug** | Known defect deferred (not fixed in the current PR) | emit exits non-zero after bus publish when audit log missing |
| **assumption** | Implementation caveat / gotcha operators and agents must remember | worktrees created from `HEAD`, not the dirty working tree |

**Do not** put here:

- Active design for the feature being built → [docs/specs/](../../../docs/specs/) ([specs](../specs/SKILL.md))
- Already shipped behavior → [changelog](../../../docs/plans/changelog.md) + guide/reference
- Vague someday dreams with no near-term usefulness → omit or ask the user
- Full designs (user stories, API contracts) — backlog points at a future spec; it does not replace one

## File

| Item | Rule |
| ---- | ---- |
| Path | `docs/plans/backlog.md` only (single file) |
| Language | English |
| Published | Yes (linked from docs indexes / MkDocs via plans) |

## Proposed document structure

```markdown
# Backlog

Deferred ideas, follow-ups, bugs, and implementation assumptions outside the active change.
Shipped work: [Changelog](changelog.md). Design drafts: [Specs index](specs-index.md).

## Deferred work

Themable groups of actionable items (ideas, follow-ups, bugs).

### <Theme>

Short context (why this cluster exists). Optional link to a parent spec.

#### <Item title>

- **Kind:** idea | follow-up | bug
- **Source:** link to spec, PR, or "planning (<feature>)"
- **Summary:** one short paragraph — the ask
- **Why deferred:** why not now
- **Revisit when:** concrete exit criteria / signals to pick it up

## Assumptions and gotchas

Non-task operational knowledge. Keep until the platform behavior changes; then delete or rewrite.

### <Theme>

- **<Gotcha title>** — one or two sentences. Link related guide/spec if any.
```

### Section rules

1. **`## Deferred work`** — only items someone could implement later. Prefer **Kind** `idea`, `follow-up`, or `bug`.
2. **`## Assumptions and gotchas`** — facts about current behavior that bite people; not a todo list. No mandatory **Revisit when** (optional if a future change would invalidate the note).
3. **Themes** (`###`) group related items (e.g. Energy / honey, Eval colony, Code proposals). Create a theme when adding the second related item; otherwise a single `### General` or place under the closest existing theme.
4. **Item title** (`####`) — short, searchable, stable enough to link as `backlog.md` + heading anchor from specs/changelog.

### Field rules (Deferred work items)

- **Kind** — exactly one of `idea` | `follow-up` | `bug`.
- **Source** — always set; prefer `../specs/NNN-….md` for spec leftovers.
- **Summary** — the work itself, not the essay.
- **Why deferred** — risk, scope, dependency, or “orthogonal to current feature”.
- **Revisit when** — observable criteria (operator pain, prerequisite shipped, eval needs X). Avoid vague “later”.

Omit fields only when they would be empty noise (rare). Do not invent fake exit criteria.

## Workflows

### Add an item (during planning or coding)

1. Decide **kind** and whether it is actionable (**Deferred work**) or a durable caveat (**Assumptions and gotchas**).
2. Find or create a **theme** section.
3. Insert the item using the template above.
4. If it fell out of a spec’s Out of Scope / Further Notes, link both ways (spec → backlog heading; backlog **Source** → spec).

### Promote to a spec

When an idea grows enough for user stories and decisions:

1. Create/update a spec ([specs](../specs/SKILL.md)).
2. Replace the backlog item with a one-liner pointing at the new spec, or remove it and rely on specs-index — prefer removal if the spec fully owns the topic.
3. Do not duplicate design into backlog.

### Resolve / ship

When the work is done:

1. Remove the backlog item (or move a one-line “done” note only if the user wants a paper trail — default is **delete**).
2. Record the ship in [changelog](../../../docs/plans/changelog.md); update durable docs per [documentation](../documentation/SKILL.md).
3. If an **assumption** is no longer true, delete or rewrite it in the same change.

### Touching legacy sections

Current `backlog.md` may still use older free-form blocks (`Context` / `Backlog item` / `Why deferred` / `Exit criteria`, nested bullets under “Deferred Ideas”). When editing a section:

- Migrate **that section** toward the structure above.
- Do not rewrite the entire file unless the user asks.

## Checklist

- [ ] Item is deferred / orthogonal — not the active feature’s core design
- [ ] Correct top-level section (Deferred work vs Assumptions and gotchas)
- [ ] Kind + Source + Summary (+ Why deferred / Revisit when for actionable items)
- [ ] No shipped changelog content parked here
- [ ] Spec cross-links updated if the item came from Out of Scope
- [ ] On ship: item removed; changelog/docs updated

## Anti-patterns

- Using backlog as a second changelog
- Writing mini-specs (long user stories, API contracts) in backlog items
- Bare bullets with no kind/source/deferral reason
- Mixing gotchas into Deferred work without a clear task
- Leaving resolved items forever “for history”
- Inventing backlog entries for problems already fully owned by an open Draft/Approved spec (link the spec instead)
