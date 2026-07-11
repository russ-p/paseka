---
name: documentation
description: Maintain Paseka project documentation (README, docs/, AGENTS.md) following layout, language, and indexing rules. Use when writing, updating, restructuring, or reviewing docs; when adding a new doc under docs/; or when the user asks about documentation principles, README, AGENTS.md, or docs indexes.
---

# Documentation

## Principles

1. **Audience first** — write for the reader of that file (humans vs agents vs deep dive).
2. **One source of truth** — put detail once in `docs/`; other files link, do not duplicate.
3. **Short entry points** — root README and `AGENTS.md` stay brief; prefer links over restating.
4. **Keep indexes current** — every new/renamed/removed top-level doc updates `docs/README.md` and `docs/README-RU.md`.
5. **Default language: English** — Russian only for `*-RU.md` mirrors or historic/special docs (e.g. product brief).

## File roles

| Path | Audience | Role |
|------|----------|------|
| `README.md` | Humans | Short product blurb + quick start (clone, build, run). Link to `docs/` for depth. |
| `README-RU.md` | Humans (RU) | Russian mirror of root README; same scope, same brevity. |
| `docs/README.md` | Humans | Index of main docs under `docs/` (table: link + one-line description). |
| `docs/README-RU.md` | Humans (RU) | Russian mirror of the docs index. |
| `docs/NNN-*.md` | Humans + agents | Canonical design/dev docs. Numbered, English by default. |
| `docs/specs/` | Humans + agents | Feature specs; **not** listed in `docs/README*` indexes. |
| `AGENTS.md` | Coding agents | Minimal: what the project is, how to build/run/test, hard restrictions, links to arch and key docs. |

## Language

- **English** is the default for new and maintained docs.
- **Russian** (`README-RU.md`, `docs/README-RU.md`, or dedicated RU docs) for localization or historic content (e.g. brief).
- When both EN and RU exist for the same entry point, update both or note the gap.
- Do not invent RU copies of every `docs/NNN-*.md` unless asked; indexes may describe English docs in Russian.

## Root README (`README.md` / `README-RU.md`)

Keep for people who land on the repo:

- What the project is (2–4 sentences)
- What it can do (short bullet list)
- How to build and run (minimal commands)
- Optional: high-level tech list

Do **not** put architecture deep-dives, glossary, or full CLI reference here — link to `docs/`.

## Docs index (`docs/README.md` / `docs/README-RU.md`)

- Table of main numbered docs only.
- Exclude `docs/specs/` from the index (mention that specs live there if useful).
- Cross-link EN ↔ RU indexes.
- One-line description per row; match the doc’s actual purpose.

## Numbered docs (`docs/NNN-name.md`)

- Use `NNN-kebab-name.md` (e.g. `003-architecture.md`).
- Reserve high numbers for catch-alls (e.g. `999-backlog.md`).
- Prefer updating an existing doc over adding overlapping ones.
- After add/rename/remove: update both docs indexes.

## AGENTS.md

Write for agents. Keep **as short as possible**.

Include only:

1. One-paragraph project summary
2. How to build / run / test (commands)
3. Main restrictions or invariants (bullets, terse)
4. Links to architecture and other must-read docs — **prefer links over inline detail**

Do **not** paste long architecture, glossary, or CLI reference into `AGENTS.md`.

## Workflow checklist

When changing documentation:

```
- [ ] Right file for the audience (README vs docs vs AGENTS.md)
- [ ] English default; RU only where appropriate
- [ ] No duplicated deep content across entry points
- [ ] docs/README.md (+ RU) updated if docs set changed
- [ ] AGENTS.md still short; new detail lives in docs/ with a link
```

## Anti-patterns

- Growing `AGENTS.md` into a second architecture doc
- Putting full CLI/API reference in root README
- Adding a doc under `docs/` without updating indexes
- Listing every `specs/` file in `docs/README*`
- Translating all numbered docs to Russian by default
