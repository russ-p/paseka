---
name: specs
description: Write and update feature specifications in docs/specs/. Use when creating a new spec, drafting from an idea, revising an existing spec, changing spec status (Draft/Approved/Implemented/Deprecated), or when the user mentions specs, /docs/specs, or feature design docs.
---

# Feature Specs

Write and maintain feature specs under `docs/specs/`. Specs are design records in git only (not published via MkDocs). For site indexes, changelog, and durable guide/reference docs after a ship, also follow the [documentation](../documentation/SKILL.md) skill.

## When to use

- User asks to write, draft, update, or approve a specification
- Turning a short idea or long design conversation into a durable spec
- Marking a shipped or retired feature’s spec status

## File naming and paths

| Item | Rule |
| ---- | ---- |
| Directory | `docs/specs/` only |
| Filename | `<NNN>-<slug>.md` — zero-padded three-digit number, kebab-case slug |
| Number | Next unused `NNN` after scanning `docs/specs/` (e.g. after `010-…` → `011-…`) |
| Slug | Short feature name; stable once chosen (colony/`spec.ready` refs depend on paths) |
| Title (H1) | `# Spec <NNN>: <Human Title>` matching the number and topic |
| Language | English (project default) |

Examples: `002-queen-console-mvp.md`, `006-human-gateway-invites.md`.

**Do not** rename or renumber existing specs without an explicit user request.

## Status values

Use exactly one of these in the Status section:

| Status | When |
| ------ | ---- |
| **Draft** | Agent creates the specification from a short idea |
| **Approved** | Spec created or updated after a long conversation with the user (agent may ask the user to set Approved) |
| **Implemented** | Feature is implemented |
| **Deprecated** | Feature is removed or completely replaced; the spec is no longer useful for new work |

Status line format:

```markdown
## Status

**(Draft|Approved|Implemented|Deprecated)**
Short note for status
```

The short note may clarify scope (e.g. MVP baseline, colony reference) but must not invent statuses outside the four above.

### Status workflow

1. **New from a short idea** → create file as **Draft**, fill the template as completely as possible; mark gaps explicitly rather than inventing product decisions.
2. **After extended design with the user** → update the body; ask whether to set **Approved**.
3. **After implementation lands** → set **Implemented**; update [docs/plans/specs-index.md](../../../docs/plans/specs-index.md); add/adjust [changelog](../../../docs/plans/changelog.md) and durable guide/reference/architecture docs per the documentation skill.
4. **Feature removed or replaced** → set **Deprecated**; note the replacement (spec or docs) in the status note or Further Notes.

## Spec body template

Every new or fully rewritten spec MUST use this structure. Section headings and intent are fixed; fill the content.

```markdown
# Spec <NNN>: <Human Title>

## Status

**(Draft|Approved|Implemented|Deprecated)**
Short note for status

## Problem Statement

The problem that the user is facing, from the user's perspective.

## Solution

The solution to the problem, from the user's perspective.

## User Stories

A LONG, numbered list of user stories. Each user story should be in the format of:

1. As an <actor>, I want a <feature>, so that <benefit>

<!-- example:
1. As a mobile bank customer, I want to see balance on my accounts, so that I can make better informed decisions about my spending
-->

This list of user stories should be extremely extensive and cover all aspects of the feature.

## Implementation Decisions

A list of implementation decisions that were made. This can include:

- The modules that will be built/modified
- The interfaces of those modules that will be modified
- Technical clarifications from the developer
- Architectural decisions
- Schema changes
- API contracts
- Specific interactions

Do NOT include specific file paths or code snippets. They may end up being outdated very quickly.

Exception: if a prototype produced a snippet that encodes a decision more precisely than prose can (state machine, reducer, schema, type shape), inline it within the relevant decision and note briefly that it came from a prototype. Trim to the decision-rich parts — not a working demo, just the important bits.

## Testing Decisions

A list of testing decisions that were made. Include:

- A description of what makes a good test (only test external behavior, not implementation details)
- Which modules will be tested
- Prior art for the tests (i.e. similar types of tests in the codebase)

## Out of Scope

A description of the things that are out of scope for this spec.

## Further Notes

Any further notes about the feature.
```

## Authoring rules

1. **User perspective first** — Problem Statement and Solution describe value for Beekeeper / operators / agents as users, not internal package layout.
2. **Extensive user stories** — cover happy paths, edge cases, failure modes, HITL vs AFK, CLI and Console where relevant; prefer too many over too few.
3. **Actors from Paseka vocabulary** — prefer glossary terms (Beekeeper, Bee, Queen Console, etc.) when they fit; see [docs/idea/glossary.md](../../../docs/idea/glossary.md).
4. **Implementation Decisions stay durable** — modules, interfaces, contracts, architecture; no brittle file paths or large code dumps (prototype exception above only).
5. **Testing Decisions** — external behavior only; name modules and point at prior-art test styles in-repo when known.
6. **Out of Scope is explicit** — list non-goals so later work does not silently expand the spec.
7. **Do not duplicate durable docs** — link to `docs/guide/`, `docs/reference/`, `docs/architecture/` for shipped behavior; the spec remains the design record.
8. **Partial updates** — when editing an existing older-format spec, prefer migrating touched sections toward this template; full rewrite only when the user asks or the structure is mostly obsolete.

## Checklist (create or major update)

- [ ] Path is `docs/specs/<NNN>-<slug>.md` with correct next `NNN` and stable slug
- [ ] H1 matches `# Spec <NNN>: …`
- [ ] Status is one of Draft / Approved / Implemented / Deprecated plus a short note
- [ ] All template sections present
- [ ] User stories are numerous and in `As an …, I want …, so that …` form
- [ ] Implementation Decisions omit file paths/snippets (unless prototype exception)
- [ ] [docs/plans/specs-index.md](../../../docs/plans/specs-index.md) row added or status/summary updated
- [ ] If **Implemented**: changelog + durable docs updated (documentation skill)
- [ ] If **Deprecated**: replacement or retirement called out

## Anti-patterns

- Publishing full spec bodies in MkDocs or duplicating them in `docs/README.md`
- Inventing statuses (e.g. “In progress”) — map to Draft/Approved/Implemented/Deprecated and put nuance in the short note
- Thin user-story lists that only restate the Solution
- Encoding decisions only as code paths that will rot
- Renaming existing `NNN-slug` files casually
