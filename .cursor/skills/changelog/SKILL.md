---
name: changelog
description: Maintain the shipped-features changelog in docs/plans/changelog.md. Use when recording a shipped feature, updating changelog after implementation, marking a spec Implemented, cutting release notes from shipped items, or when the user mentions changelog or docs/plans/changelog.md.
---

# Changelog

Maintain [docs/plans/changelog.md](../../../docs/plans/changelog.md) — short records of **shipped** features worth calling out. Design: [specs](../specs/SKILL.md). Deferred: [backlog](../backlog/SKILL.md). Durable docs: [documentation](../documentation/SKILL.md). Version tags / GitHub Release prose: [release](../release/SKILL.md).

## What belongs here

| Include | Exclude |
| ------- | ------- |
| User-facing or operator-facing shipped capability | Pure refactors with no behavior change |
| Breaking renames / contract changes operators must notice | Tiny typo/docs-only fixes (unless they are the whole change) |
| Spec implementations reaching **Implemented** | Open Draft/Approved design (specs-index + spec body) |
| Notable MVP baselines (historical table OK) | Backlog ideas and gotchas |

One changelog entry per coherent ship (often one spec or one named change). Prefer fewer, clearer entries over commit-by-commit noise.

## File

| Item | Rule |
| ---- | ---- |
| Path | `docs/plans/changelog.md` only |
| Language | English |
| Order | Newest entry **first** (below the intro) |
| Date in heading | `YYYY-MM` (month of ship), not day — group related ships in the same month under separate headings |

## Entry template

```markdown
## YYYY-MM — <Short title>

<One short paragraph: what shipped and why it matters.>

Optional bullets for multi-surface changes (CLI, Console, protocol):

- …
- …

- Spec: [<NNN>-<slug>](../specs/<NNN>-<slug>.md)
- Canonical: [Guide or reference](../guide/….md), …
```

### Field rules

1. **Title** — product language (`Telegram Human Gateway`), not a commit subject.
2. **Body** — what an operator/Beekeeper gained or must change; skip internal file paths and package tours.
3. **Spec** — required when the ship came from a `docs/specs/` design; omit only for small cross-cutting renames with no dedicated spec.
4. **Canonical** — links to the durable guide/reference/architecture docs that now describe the behavior (create/update those in the same change per documentation skill). Prefer links over restating the full design.
5. **Deferred leftovers** — one line pointing at [Backlog](backlog.md), not a second backlog dump:

   ```markdown
   Deferred from that work: … — see [Backlog](backlog.md).
   ```

6. **Breaking changes** — call out removals/renames in the body or bullets so release notes can lift them later.

### Earlier MVP baselines

Keep the trailing `## Earlier MVP baselines` table for pre-changelog or bulk MVP ships. **Do not** add new rows there for current work — add a dated `## YYYY-MM — …` entry instead. Only extend the table when backfilling historic baselines the user asks to record.

## Workflows

### On ship (feature lands)

1. Add a new `## YYYY-MM — …` entry at the **top** (after the intro paragraph).
2. Set the related spec to **Implemented** and refresh [specs-index](../../../docs/plans/specs-index.md) ([specs](../specs/SKILL.md)).
3. Move durable behavior into guide/reference/architecture; link those as **Canonical**.
4. Move leftover ideas/bugs/assumptions to backlog; remove resolved backlog items ([backlog](../backlog/SKILL.md)).
5. Do **not** duplicate the full spec body into the changelog.

### On release

[release](../release/SKILL.md) may skim this file for user-facing bullets. Changelog entries stay month-titled and durable; GitHub Release notes are version-scoped and may be shorter. If release asks for a changelog update, add/adjust an entry only when something shipped that is not yet recorded.

### Amend an entry

- Same month, same feature, small correction → edit the existing entry.
- New capability after the fact → new entry (or clearly extend the body if it is the same ship still landing).

## Checklist

- [ ] Entry is newest-first under `docs/plans/changelog.md`
- [ ] Heading is `## YYYY-MM — Title`
- [ ] Body is operator-facing; no file-path archaeology
- [ ] Spec + Canonical links when applicable
- [ ] Specs-index / spec status updated if this closes a spec
- [ ] Deferred crumbs → backlog, not left only in the changelog
- [ ] Durable docs updated (documentation skill)

## Anti-patterns

- Commit log paste or PR title dumps
- Parking unshipped design in changelog
- Using changelog as backlog or as a full API reference
- Adding current ships only to “Earlier MVP baselines”
- Listing every touched file instead of linking Canonical docs
