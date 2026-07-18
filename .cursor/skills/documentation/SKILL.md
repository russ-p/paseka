---
name: documentation
description: Maintain Paseka project documentation (README, docs/, AGENTS.md) following layout, language, and indexing rules. Use when writing, updating, restructuring, or reviewing docs; when adding a new doc under docs/; or when the user asks about documentation principles, README, AGENTS.md, or docs indexes.
---

# Documentation

## Principles

1. **Audience first** — write for the reader of that file (humans vs agents vs deep dive).
2. **One source of truth** — put detail once in `docs/`; other files link, do not duplicate.
3. **Short entry points** — root README and `AGENTS.md` stay brief; prefer links over restating.
4. **Keep indexes current** — every new/renamed/removed published doc updates `docs/README.md`, `docs/README-RU.md`, and `mkdocs.yml` nav.
5. **Default language: English** — Russian only for `*-RU.md` mirrors or historic/special docs (e.g. product brief).

## Layout (immersion layers)

```text
docs/
  idea/           # principles, glossary, historic brief
  guide/          # how to use: layout, CLI, bee config, prompts, sessions, nuc
  reference/      # routing, insight kinds, task ledger
  architecture/   # adapters, IPC, worktrees, packages
  plans/          # changelog, specs-index, backlog
  specs/          # feature design drafts — NOT published on the site
```

Order for readers: **idea → guide → reference → architecture → plans**.

## File roles

| Path | Audience | Role |
|------|----------|------|
| `README.md` | Humans | Short product blurb + quick start. Link to `docs/` for depth. |
| `README-RU.md` | Humans (RU) | Russian mirror of root README. |
| `docs/README.md` | Humans | Index by immersion section (tables + one-line descriptions). |
| `docs/README-RU.md` | Humans (RU) | Russian mirror of the docs index. |
| `docs/idea/` | Humans | Principles and vocabulary. |
| `docs/guide/` | Humans + agents | How to configure and operate a colony. |
| `docs/reference/` | Humans + agents | Event/routing/ledger contracts. |
| `docs/architecture/` | Contributors | Runtime internals and package map. |
| `docs/plans/changelog.md` | Humans | Shipped features; link to specs in repo + canonical docs. |
| `docs/plans/specs-index.md` | Humans | Short status table for `docs/specs/` (bodies stay in git only). |
| `docs/plans/backlog.md` | Humans + agents | Deferred ideas only (not shipped notes). |
| `docs/specs/` | Humans + agents | Feature specs; **excluded from MkDocs**; do not list full bodies in indexes. |
| `AGENTS.md` | Coding agents | Minimal: what the project is, build/run, hard restrictions, links to key docs. |

## Language

- **English** is the default for new and maintained docs.
- **Russian** (`README-RU.md`, `docs/README-RU.md`, or dedicated RU docs) for localization or historic content (e.g. brief).
- When both EN and RU exist for the same entry point, update both or note the gap.
- Do not invent RU copies of every guide unless asked; indexes may describe English docs in Russian.

## Root README (`README.md` / `README-RU.md`)

Keep for people who land on the repo:

- What the project is (2–4 sentences)
- What it can do (short bullet list)
- How to build and run (minimal commands)
- Optional: high-level tech list

Do **not** put architecture deep-dives, glossary, or full CLI reference here — link to `docs/`.

## Docs index (`docs/README.md` / `docs/README-RU.md`)

- Sectioned tables matching immersion layers (and `mkdocs.yml` nav).
- Specs: point to `plans/specs-index.md`, not a full listing of every spec body.
- Cross-link EN ↔ RU indexes.
- One-line description per row; match the doc’s actual purpose.

## Adding or moving docs

- Put new content in the correct folder (`idea` / `guide` / `reference` / `architecture` / `plans`).
- Prefer updating an existing doc over adding overlapping ones.
- After add/rename/remove: update both docs indexes **and** `mkdocs.yml` nav.
- Keep `docs/specs/<NNN>-<slug>.md` paths stable — colony/Drone/`spec.ready` refs depend on them.
- When a spec ships: add a [changelog](docs/plans/changelog.md) entry, update status in [specs-index](docs/plans/specs-index.md), move durable behavior into guide/reference/architecture.

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
- [ ] Right folder for the audience (idea / guide / reference / architecture / plans)
- [ ] English default; RU only where appropriate
- [ ] No duplicated deep content across entry points
- [ ] docs/README.md (+ RU) and mkdocs.yml updated if docs set changed
- [ ] Specs stay unpublished; changelog / specs-index updated when status changes
- [ ] AGENTS.md still short; new detail lives in docs/ with a link
- [ ] If agent-config guides changed: run scripts/gen-llms-full.sh
```

## Anti-patterns

- Growing `AGENTS.md` into a second architecture doc
- Putting full CLI/API reference in root README
- Adding a published doc under `docs/` without updating indexes and nav
- Listing every `specs/` file body in `docs/README*`
- Moving `docs/specs/` paths without updating colony prompts and invite refs
- Translating all guides to Russian by default
- Putting shipped notes in backlog instead of changelog
