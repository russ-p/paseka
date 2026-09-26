# UI Migration Backlog

Deferred UI decisions for the Queen Console redesign ([spec 035](../specs/035-queen-console-redesign.md)) — the dead ends, unbuilt transitions, and placeheld sections to return to while `/next/` is still a preview. Product and platform ideas belong in [Backlog](backlog.md); shipped work in [Changelog](changelog.md).

Each item carries a **Kind**, a **Source**, what is pending, why it was set aside, and when to pick it up. Strike an item when the decision lands, not when it is discussed — a stale entry is worse than none.

## Dead ends

A link or control that resolves to a page which does not exist yet. Each one is a promise the operator can see and cannot keep.

#### Trail detail links into a placeholder Timeline

- **Kind:** blocker
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Traces migration)
- **Summary:** "Open timeline" on `/next/traces/:id` points at `/next/timeline?trace=<id>`, which renders `PagePlaceholder`. The route exists and the query is right; nothing reads the param.
- **Why deferred:** Timeline is not migrated, and a filter that scopes a placeholder is invisible. The link was kept because it names the intended contract and costs nothing to honour later.
- **Revisit when:** Timeline is migrated. It must read `trace` on mount, and the filter bar should open with the field filled rather than reading it from the URL behind a "Filters" button.

#### Task and run rows on the trail detail carry no link

- **Kind:** blocker
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md)
- **Summary:** Task rows and run rows render as plain text because `/next/tasks` and `/next/runs` are `PagePlaceholder`. `DetailRow` already takes an `href` and nothing passes it.
- **Why deferred:** A row that promises a destination and dead-ends is worse than a row that only shows state. The Dashboard set this precedent and the trail detail followed it.
- **Revisit when:** Tasks and Runs land. Each row gets an `href` and nothing else changes — no markup work is left.

#### `PagePlaceholder` sends the operator to the legacy root

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md)
- **Summary:** The only action on a placeholder is "Use legacy console" → `/`, which lands on the legacy Dashboard. An operator who clicked through from a trail loses their place.
- **Why deferred:** The legacy console has no deep links for most routes, so there is nothing better to point at yet.
- **Revisit when:** Any legacy route gains a deep-linkable view, or the placeholder can at least name the section it stands in for.

## Transitions

Movement between routes that is not designed yet. The shell routes client-side, but nothing about *where an operator lands* is settled.

#### A published cue does not open the trail it started

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Dashboard)
- **Summary:** `CueRunModal` returns the new `traceId`; the Dashboard toasts `Cue published — trace <id>` and stays put. The trail exists, has no runs yet, and is exactly what the operator wants to watch.
- **Why deferred:** Navigating away mid-poll loses the Dashboard context, and auto-navigating on a background cue would yank the operator off a page they did not act from. Needs a decision, not a patch.
- **Revisit when:** The trail detail can show a live "waiting for a run" state. Then either navigate on an explicit publish, or add an "Open" action to the toast — a toast that carries navigation is the smaller change and should be tried first.

#### Cross-route deep links between a trail and its work

- **Kind:** idea
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md)
- **Summary:** Trail → Task, Trail → Run, Task → Run, Review → Trail. Every list and row that could carry a target already accepts one; none of the targets exists.
- **Why deferred:** Each link is only worth adding once both ends are real, and half of them would be links to placeholders.
- **Revisit when:** The second of each pair is migrated. The trail detail is the hub, so its rows go first.

#### Back behaviour on the trail detail is a hard link

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md)
- **Summary:** `← Traces` is a plain link to `/next/traces`, not `history.back()`. Correct when the trail was deep-linked from a chat or a `paseka replay` hint; wrong when the operator arrived from page 3 of the list and expects to be there.
- **Why deferred:** A hard link is never broken, and history is only better in one of the two cases. Breadcrumbs would serve both but add a component and a rule for a route family that has one page so far.
- **Revisit when:** A second detail route exists, so there is a family to design a shared back affordance for.

#### List state lives in the component, not the URL

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (user story #3)
- **Summary:** `DataTable` keeps its filter and page in local state, so `/next/traces` → a trail → back loses the filter and the page. User story #3 promises switching contexts "without losing scroll position"; neither scroll restoration nor list restoration is implemented or tested.
- **Why deferred:** Moving filter and page into the URL (`?q=&page=`) is a real API decision for every table, and premature for the one list that exists.
- **Revisit when:** A second paginated route lands, or an operator reports losing their place. Server-side trail paging already exists, so the URL can carry a cursor if the decision goes that way.

#### Keyboard chords stop at the route root

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (user story #10)
- **Summary:** `g t` reaches `/next/traces`. Nothing reaches a trail detail, and there is no `Escape`-to-list from a detail, no chord for "open timeline", and no `/` to focus the filter box.
- **Why deferred:** The chord map is `navigation.ts` and is cheap to extend, but chords that only make sense on one route are better designed once the route family exists.
- **Revisit when:** The Sessions route lands and needs its own chords, or the detail-route back affordance is settled — both force a pass over the map.

#### The last route is never a trail detail

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md)
- **Summary:** `rememberRoute` stores only known menu routes, so a deep-linked or last-viewed trail is never remembered and the landing redirect always goes to a menu root.
- **Why deferred:** Landing on a stale trail is worse than landing on the Dashboard, and the trail detail is the first route where "restore where I was" and "land somewhere safe" disagree.
- **Revisit when:** Standing trails exist in a colony — a beekeeper's real "home" is a standing trail, and that is the case this decision exists for.

## Sections still placeheld

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Current Section Design Audit)
- **Summary:** `PagePlaceholder` on Timeline, Tasks, Reviews, Sessions, Bees, Worktrees, Runs, Topology, and System. Settings is partial — theme selection only; the rest of its surface migrates later. Git is migrated, and it deliberately left the read-only worktree list for the `/next/worktrees` route to take.
- **Why deferred:** Deliberate phase order. The shell and the two highest-traffic surfaces (Dashboard, Traces) went first so the design system is proven against real colony data before the rest depend on it. Git went next because its page was the one whose actions were hardest to place well, and the review settled the button and confirmation questions every later mutating route will face.
- **Revisit when:** The next route is picked up; nothing blocks it technically. **Timeline is next** — the trail detail already links into it, and it is the other half of the event story.

## Components the inventory promises

The [design-system contract](../architecture/queen-console-design-system.md) lists these. They do not exist, so a route that reaches for one will not compile.

#### `Drawer`

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (user story #17)
- **Summary:** Promised as the wide-form variant of `Modal` — slide from right, same focus trap, ESC, and focus restoration. Needed by the Sessions launch form and the Tasks create form, the two surfaces spec'd as wide or multi-step.
- **Why deferred:** Every form that has landed fits `Modal`. `Drawer` is the first component whose only justification is a route that does not exist.
- **Revisit when:** Sessions or Tasks is migrated. It should be `Modal` plus a placement and width, not a second focus implementation.

#### `BeeCard` and `WorktreeCard`

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Current Section Design Audit)
- **Summary:** Both are in the component inventory; neither has been written. The audit proposes them for `/next/bees` and for the read-only worktree list split out of the Git route.
- **Why deferred:** The Traces work showed that `DataTable` covers a list and `DetailRow` covers a short related list. Whether bees and worktrees need a card at all is unproven.
- **Revisit when:** `/next/bees` or `/next/worktrees` is migrated — and the first question then is whether to build the card or drop it from the inventory and use what exists. The Git migration is evidence for dropping it: the worktree list on `/next/git` is a `DataTable`, and nothing about it wanted a card.

## Polish on landed routes

#### A branch cannot be deleted from its own row

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Git migration)
- **Summary:** `/next/git` deletes only the merged leftovers, in one confirmed sweep. A single branch that is merged but not a leftover — or one a live worktree no longer holds — has no delete path in the console; the operator filters to it and then has nothing to press. The legacy console had a per-row Delete (with no confirmation at all, which was worse).
- **Why deferred:** `DataTable` cells are declarative by contract — a Svelte snippet cannot be built inside `<script>`, so a button in a cell needs a new intent that no other table has asked for. The sweep is the operation the section exists for, and deleting one branch is `git branch -d` away.
- **Revisit when:** A second table needs per-row actions (Tasks, Runs, or Reviews are the likely candidates), or an operator reaches for a shell to delete a single branch. The fix is a DataTable action intent, not a Git-page workaround.

#### A branch subject truncates at 768px

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Git migration)
- **Summary:** The Branches table gives the leftover width to the commit subject, so it is complete on a desktop and truncates with `…` at the 768px design target. The full text is one `git log` away. The worktree table has no such column.
- **Why deferred:** The alternative — letting the subject wrap — was tried and is worse: a `wrap` intent is silently defeated by the `grow` column's own truncation, so it would have been a trap in the component contract, and wrapping fights the "cells never wrap" rule that keeps row heights uniform.
- **Revisit when:** A second visibility tier is needed anyway (`lg:` for desktop-only columns), or an operator scans subjects at 768px and cannot tell branches apart. The fix is a breakpoint, not a wrap.

#### An oversized comb file is unreadable

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Traces migration)
- **Summary:** The server refuses to inline a comb body over 512 KiB and the modal shows `file too large for inline preview`. There is no partial body, no paging, and no download link, so a large `checkpoint.json` cannot be read at all.
- **Why deferred:** The cap is the server's and predates the redesign; the legacy console had the same dead end. Raising the cap trades memory for a rare case.
- **Revisit when:** An operator hits it in a real trail, or comb files start growing past a few hundred KiB. A range read is the fix; a bigger cap is not.

#### The event preview is a guess at eight

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Traces migration)
- **Summary:** The trail detail shows the newest 8 of the projection's 20 events and writes "N older on the timeline". Eight was picked because the section is a preview, not because anything measured it.
- **Why deferred:** The projection is capped at 20 server-side, so no number of rendered rows reaches the full feed.
- **Revisit when:** Timeline lands and can take a `before` cursor — then the preview can be a real "latest 8" window of an arbitrarily long feed rather than a slice of a fixed 20.

#### Only the trail id is copyable

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Traces migration)
- **Summary:** `MetaRow.copy` renders an icon-only copy button and the trail id is the only row that sets it. On the same page, the worktree **Path** and **Base SHA** are the obvious next candidates — an operator pastes a worktree path into a shell more often than a base SHA.
- **Why deferred:** The trail id is the one value quoted verbatim in CLI output, bug reports, and `paseka replay`. The others were left alone rather than half-done.
- **Revisit when:** Anyone pastes a worktree path by hand. It is a one-word change per row.

## Verification still owed

#### No visual regression or end-to-end suite

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Testing Decisions)
- **Summary:** The spec commits to Playwright E2E (preview root → dashboard → traces filter → detail → comb modal → theme → reload) and to screenshot comparison on Dashboard, Traces list, and Settings. Playwright is not a dependency and neither suite exists. Coverage today is Vitest component and store tests plus Go route tests.
- **Why deferred:** Vitest covers behaviour; it cannot catch a layout regression. The gap was accepted while the routes were still moving, and every layout decision since — the shared row, the `grow` column, the collapse defaults — was checked by hand in a browser rather than by a test.
- **Revisit when:** The route set stops changing, or before the root cutover — a cutover with no visual guard is how a theme silently loses contrast.
