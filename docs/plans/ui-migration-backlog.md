# UI Migration Backlog

Deferred UI decisions for the Queen Console redesign ([spec 035](../specs/035-queen-console-redesign.md)) — the dead ends, unbuilt transitions, and placeheld sections to return to while `/next/` is still a preview. Product and platform ideas belong in [Backlog](backlog.md); shipped work in [Changelog](changelog.md).

Each item carries a **Kind**, a **Source**, what is pending, why it was set aside, and when to pick it up. Strike an item when the decision lands, not when it is discussed — a stale entry is worse than none. An item whose decision has landed but whose work has not goes to **Decided, not yet built**, which trades *what is pending* for the decision and what it was chosen over.

## Decided, not yet built

Entries below were taken one at a time and settled; the work has not landed, so they are recorded here rather than struck. Each states the decision, so the next pass does not re-open it, and what the decision was chosen over. When one ships it moves to [Changelog](changelog.md) like any other work, and the amendment it makes to the [design-system contract](../architecture/queen-console-design-system.md) lands with it.

The section is empty: every entry taken in this pass has shipped and moved to
[Changelog](changelog.md). What is left of the redesign's unfinished work is in
[Transitions](#transitions) and [Polish on landed routes](#polish-on-landed-routes)
below, and anything still only an idea is in [Backlog](backlog.md).

## Dead ends

A link or control that resolves to a page which does not exist yet. Each one is
a promise the operator can see and cannot keep.

**No live promise is left.** The one entry below is closed and kept only for the
rule it settles. The `PagePlaceholder` entry that used to sit here is gone with
the reason: no route renders a placeholder any more, so no link can dead-end on
one. That component is still in the inventory and referenced by nothing, which is
a fact for whoever curates the inventory rather than a promise an operator sees.

#### Task and run rows on the trail detail carry no link

- **Kind:** blocker
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md)
- **Summary:** Both row blocks are live. The run rows link to `/next/runs/:traceId/:agentId` and the task rows to `/next/tasks/:traceId/:taskId`, so the trail detail is the one place with no inert rows left, and it was a real target waiting rather than a nicety: a task row on a trail is how an operator walks from a trail to the work it produced.
- **Why deferred:** A row that promises a destination and dead-ends is worse than a row that only shows state. The Dashboard set this precedent, the trail detail followed it, and Runs and Tasks have now landed so neither half of the block is mute.
- **Revisit when:** Never. Tasks landed and the task rows link, which closes this; the run rows linked a route earlier. The precedent stands for any future list: a row that promises a destination and dead-ends is worse than one that only shows state, so a new column should link from its first commit rather than after a backlog item.

## Transitions

Movement between routes that is not designed yet. *Where an operator lands* is
mostly settled now — a list keeps its filter and page in the URL, and `Escape`
walks a detail back to the list that owns it — so what is left here is the part
that needs a decision about what "back" and "last" mean rather than a mechanism.

#### Back from a detail is a hard link, and it names the bare list

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (user story #3)
- **Summary:** All five detail routes render their back link as a plain link to the bare list path — `TraceDetail.svelte:130`, `RunDetail.svelte:47`, `TaskDetail.svelte:123`, `SessionDetail.svelte:154`, `ReviewDetail.svelte:118` — and none of them carries the list's query. `Escape` lands on the same bare path, because `owningListPath` resolves a list *root* and the list's `?q=&page=` is not part of one. So the failure this entry always described is now four routes wide and exactly reproducible: arrive from `/next/runs?page=3`, open a run, press `←`, and land on page one of an unfiltered list. **Traces no longer belongs to this item** — it is unpaged, so there is no page to lose; its `?q=` is the whole of what a back link would have to carry.
- **Why deferred:** The list's state being in the URL is what made this cheap — reproducing the operator's page is now a string, so the fix needs neither breadcrumbs nor a shared "last list query" module. The decision is still unmade, though, because `history.back()` is better in one case and worse in the other and this entry has always declined to pick. The narrow answer that serves both is to go back only when the previous history entry *is* the owning list and hard-link otherwise, which keeps a trail deep-linked from a chat landing on the list rather than out of the console.
- **Revisit when:** An operator loses a page to a back link, or the root cutover lands — a back affordance that drops somebody on page one of a list they had narrowed is a first-impression bug, and `/` is still the legacy console.

#### Keyboard chords stop at the route root

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (user story #10)
- **Summary:** The thirteen root chords are complete and discoverable — `SideMenu` renders `g <letter>` as a `kbd` in every entry. What is missing is a chord for a *destination that is a function of the page*: a trail's runs or its timeline, a task's runs, a session's transcript. `consoleRoutes` holds roots, so none of that can be a row in the map.
- **Why deferred:** The two affordances that were actually stuck are not chords and have shipped — `Escape` walks a detail back to its list and `/` reaches the filter — which removes the argument that the value was thin and leaves only the cost. Five detail families now each have page-dependent destinations, so a route-scoped layer would have real work to do; that is also why it is no longer a rounding error. It is still a new thing to own: one rule per family is five rules, and each wants a discoverable hint that a chord map has nowhere to put.
- **Revisit when:** An operator asks for one, or a detail route gains a destination with no on-screen control. That has not happened once, and the count is the number this entry is really tracking.

#### Scroll position does not survive a route change

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (user story #3)
- **Summary:** User story #3 promises switching contexts "without losing scroll position". The half a list can keep by itself has shipped: `?q=&page=` is in the URL, so a table's page *is* its position. The other half has no home — SvelteKit scrolls to the top on a client-side navigation and nothing puts a reader back, and the pages the story is really about (the topology graph, a run's event log, a session transcript, a merge diff) are not paginated and have no URL to restore from.
- **Why deferred:** Restoration is per surface, because each long page knows what "the same place" means for it: an offset for a transcript, a zoom and centre for the graph, a selected file for a diff. A blanket `history.scrollRestoration = 'manual'` plus a stored offset gets the scroll right and the open blocks wrong, and there is no page anybody has complained about yet.
- **Revisit when:** The root cutover lands, and not before. `/` still serves the legacy console while `/next` redirects to the last menu root, so "I lost my place" becomes a first-impression bug the moment the preview *is* the console.

#### The last route is never a trail detail

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md)
- **Summary:** `rememberRoute` stores a path only when it is in `consoleRoutes` (`navigation.ts:143`), and `/next` redirects to it (`+page.ts:7`), so a trail, run, task, proposal, or session detail is never the landing page. A standing trail is the case this exists for, and it has become a path an operator *walks*: the cue toast's **Open trail** and the trail detail's own waiting notice make a cue a deliberate arrival rather than a link somebody pasted.
- **Why deferred:** Landing on a stale trail is worse than landing on the Dashboard, and a trail that finished while the console was closed is exactly that. This colony has no standing trail to decide with — `feature.yaml` and `hotfix.yaml` declare no `standingTrace`, so `cues.StandingTraceIDs` is empty and every trail's `Standing` flag is false. Deciding the landing rule for a case no operator here has would be a guess.
- **Revisit when:** A cue in this colony declares `standingTrace`. The badge, the field, and the API all exist; only the decision is missing.

## Sections still placeheld

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Current Section Design Audit)
- **Summary:** None. Every route under `/next/` is migrated, so no page resolves to `PagePlaceholder` any more — the section is kept because the audit it tracks is the place a new placeheld surface would be recorded, and because two of the migrated routes arrived as a decision rather than a port and are worth finding again. Worktrees came as a split: the Git page's read-only table and its orphan prune moved to `/next/worktrees` together, the data stayed on `GET /api/git`, and `WorktreeCard` was dropped from the inventory. Bees and Settings were both designed against a user story rather than against a tab, because the legacy console has neither.
- **Why deferred:** Nothing further is deferred here. The order was deliberate — the shell and the two highest-traffic surfaces (Dashboard, Traces) went first so the design system was proven against real colony data before the rest depended on it, Git next because its page was the one whose actions were hardest to place well, System because it is the other read-mostly page whose format decisions every later list inherits, Timeline because it settled the feed-row contract and the folded-filter panel, Topology because it brought the first third-party imperative component and the one styling exception, Runs because it opened the Colony group and the first detail route that steps between siblings, Tasks because it closed the audit's other full-column form and was the first user of `Drawer`, and Reviews because it landed the audit's two named components.
- **Revisit when:** A new surface needs a scope decision, or the root cutover is called.

## Components the inventory promises

The [design-system contract](../architecture/queen-console-design-system.md) lists these. They do not exist, so a route that reaches for one will not compile.

#### `Drawer`

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (user story #17)
- **Summary:** Promised as the wide-form variant of `Modal` — slide from right, same focus trap, ESC, and focus restoration. Needed by the Sessions launch form and the Tasks create form, the two surfaces spec'd as wide or multi-step.
- **Why deferred:** Every form that has landed fits `Modal`. `Drawer` is the first component whose only justification is a route that does not exist.
- **Revisit when:** Sessions is migrated. `Drawer` now exists as `Modal` with `placement="right"`, which is what this asked for; the session launch form should use it as-is rather than reaching for a second focus implementation.

#### `BeeCard`

- **Kind:** dropped
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Current Section Design Audit)
- **Summary:** Proposed in the component inventory for `/next/bees`; never written, and now answered: the Bees route shipped as a `DataTable` and `BeeCard` is out of the inventory.
- **Why dropped:** A bee row is one line of facts — a role, an adapter, a vocabulary, a workspace, a last run — which is the shape `DataTable` was built for, and a card per bee would give every row the same seven fields at three times the height. The same call `WorktreeCard` got.

## Polish on landed routes

#### A truncated cell cannot be read in full

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Git and System migrations)
- **Summary:** A `grow` cell truncates with `…` and offers no way to see the rest. The Branches commit subject is complete on a desktop and clipped at the 768px design target; the System process command line is clipped at *every* width, because the server already truncates it at 200 runes and a real adapter command is longer than that.
- **Why deferred:** There is no reveal mechanism, and both obvious ones are wrong. Letting the cell wrap is silently defeated by the `grow` column's own truncation — it would have been a trap in the component contract — and it fights the "cells never wrap" rule that keeps row heights uniform. A native `title` would be a second, inconsistent way to show full text next to the `Hint` component that already owns that job.
- **Revisit when:** A third table needs a prose column, or an operator cannot tell two processes apart on the System page. The fix is one `hint` intent on a `grow` cell feeding the existing `Hint` popover, plus the `lg:` visibility tier the desktop-only columns will want anyway — not a wrap and not a `title`.

#### Only the `?trace=` scope is shareable

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Timeline migration)
- **Summary:** `/next/timeline?trace=<id>` is a real deep link, but the other five filters live in the store and reset on reload, so `?type=VERIFICATION&bee=scout` cannot be shared or bookmarked.
- **Why deferred:** The trace filter is the one that identifies *which trail* an operator is looking at, so it is a navigation target worth a URL. The rest are ad-hoc refinements, and mirroring them means the URL and the store are two sources of truth that must round-trip — including through every Back press.
- **Revisit when:** Someone asks for a link to a filtered feed, or a run report wants to cite one. The fix is one `replaceState` per Apply plus reading the query back on entry, which is a small change once someone has asked for it.

#### A nullable list is read in one place

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Sessions migration)
- **Summary:** `Bee.intents` is `null`, not `[]` — `console.BeeView.Intents` has no `omitempty`, so a bee with no prompt templates sends a nil slice as null, and four of this colony's seven bees do. Both launch forms read it inline, which put a live page-killing crash in each. `beeIntents` is now the only reader.
- **Why deferred:** Nothing is deferred; the fix shipped with Sessions. It is recorded because the shape is not visible from the endpoint's name and the next nullable list will be read inline again by whoever meets it first.
- **Revisit when:** A new nullable list appears — check whether the Go field has `omitempty` before typing it as `[]`, and read it through one helper rather than at each use site.

#### The merge diff has no per-word intra-line highlighting

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Reviews migration)
- **Summary:** `lib/diff.ts` reads a patch line by line, so it knows which lines changed but not which words inside them did. diff2html did, by word-diffing each changed line — the reason the legacy vendored it at all, once the DOM-scraping for line numbers is set aside. The side-by-side layout came back with the parser rather than with the library, so this is the last of diff2html's reasons to exist here.
- **Why deferred:** Intra-line highlighting is a second pass over the two versions of a changed line, and a reviewer reads a diff for *what changed*, which the line tint and the `+`/`−` marker already answer. It is also the feature most likely to be wrong in a way nobody notices: a misaligned word range is worse than no highlight.
- **Revisit when:** A reviewer asks for it on a long line — a minified lockfile or a generated file, where a changed line is a wall of text. `DiffViewer` renders per row, so it is a second pass in the parser and a span in the cell rather than a rewrite.

#### A run's events have no paging, because the endpoint caps nothing

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Runs migration)
- **Summary:** `runs.Dir.readEventsFrom` returns `all[skip:]` with no limit, so `GET /api/runs/:traceId/:agentId/events` returns a run's entire event log in one response and `nextCursor` is an index rather than a promise of more. The run page therefore shows no paging control — a button could only ever confirm there was nothing left.
- **Why deferred:** Adding a cap is a server change with a real cost: a run that emits thousands of events would need a bounded response and a client that pages it. That is worth doing only if a run is ever observed emitting that many; the busiest real run in this colony records three.
- **Revisit when:** a run's event log is big enough to be slow, or the endpoint grows a `limit`. The client already knows the cursor shape — `listRunEvents(traceId, agentId, after)` takes an index — so restoring the control is a page change, not a rewrite.

#### A wide table scrolls on a phone rather than reflowing

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Runs migration)
- **Summary:** `DataTable`'s wrapper is `overflow-x-auto`, so a table too wide for the viewport scrolls inside its bordered region. Hard rule 7 says that is the intended answer and gives the reason — clipping the last column puts a link where nothing can reveal it — so this is a floor, not a bug.
- **Why deferred:** The card layout is declined rather than pending, and the counts are why. Ten routes use `DataTable`, and Runs — the table this entry blamed — is the best of the bad: six columns, two of them `secondary`. Bees is seven with none, Reviews six with none, Worktrees five with none. The mechanism that answers this already existed and was simply not applied, which is decided above. A card is cheap in the component but puts the table and the cards both in the DOM, and that is a test-churn cost across ten route suites for pages a solo beekeeper may never open on a phone.
- **Revisit when:** An operator reads these tables on a phone and four columns are still not enough. The fix is a card layout in `DataTable` below 768px, with `secondary` shown rather than hidden there — the card has the room, and a column of `secondary` only removes information.

#### The colony graph is a picture with no text equivalent beside it

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Topology migration)
- **Summary:** The Mermaid block is the graph as text, and it is the same data — but Mermaid is a diagram *description*, not a table, so "which events has nobody subscribed to" is still an eyeball job across a canvas and a wall of Mermaid source.
- **Why deferred:** A wiring table was considered and dropped when the graph was chosen. The canvas carries a label with its size and points at the Mermaid, which is the accessibility floor for an image, so nothing is unreachable — but a sortable table of `Event | Bee | Relation | Dispatch` would answer the gap question directly, and that is a different page rather than a fix.
- **Revisit when:** An operator hunts for an unsubscribed event and cannot find it, or the colony grows past what fits one screen. The projection already carries every field such a table needs, and `DataTable` would compose it without new work.

#### The graph scales down before an operator can read it

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Topology migration)
- **Summary:** Bees are one column and event kinds wrap, so a seven-bee colony fits legibly. Past roughly twenty event kinds the `fit` that frames the graph shrinks the labels again, and the operator has to zoom before they can read anything. The legacy console had the same ceiling and answered it with pan and zoom.
- **Why deferred:** Solving it properly means either measuring label widths (which needs a laid-out canvas, and so cannot be unit-tested — the reason the layout maths is pure) or dropping labels in favour of hover, both of which are worse at the sizes colonies actually run at. Zoom and drag already work.
- **Revisit when:** a real colony crosses roughly twenty event kinds. The cheapest fix is a taller container plus a higher `minZoom` floor, so the graph opens cropped and pannable rather than illegible.

#### The event preview is a guess at eight

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Traces migration)
- **Summary:** The trail detail shows the newest 8 of the projection's 20 events and writes "N older on the timeline". Eight was picked because the section is a preview, not because anything measured it.
- **Why deferred:** The cap is the *projection*, which embeds at most 20 events, so no number of rendered rows reaches the full feed. The feed itself pages back without limit.
- **Revisit when:** A trail with more than 20 events makes the preview look broken. The fix is for the trail detail to read a page of `/api/events?traceId=<id>` — the Timeline route already does — instead of the projection's embedded list; it needs no new cursor, because the feed is newest-first and `after` walks backwards.

## Verification still owed

#### No visual regression or end-to-end suite

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Testing Decisions)
- **Summary:** The spec commits to Playwright E2E (preview root → dashboard → traces filter → detail → comb modal → theme → reload) and to screenshot comparison on Dashboard, Traces list, and Settings. Playwright is not a dependency and neither suite exists. Coverage today is Vitest component and store tests plus Go route tests.
- **Why deferred:** Vitest covers behaviour; it cannot catch a layout regression. The gap was accepted while the routes were still moving, and every layout decision since — the shared row, the `grow` column, the collapse defaults — was checked by hand in a browser rather than by a test.
- **Revisit when:** The route set stops changing, or before the root cutover — a cutover with no visual guard is how a theme silently loses contrast.

#### No generated types from the Go contracts

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (user story #14)
- **Summary:** The spec asked for TypeScript types generated from the Go event contracts (SIGNAL, INSIGHT, MUTATION, VERIFICATION) and for a contract test that decodes sample payloads through them. Neither exists: `web/src/lib/api/types.ts` is hand-written, so a field renamed in Go and forgotten in TS is caught by nothing — only by a page that renders `undefined`.
- **Why deferred:** The routes landed and every view type-checks against the hand-written shapes, so the cost never surfaced; the drift cost is paid the first time an endpoint grows a field the console should show.
- **Revisit when:** A second endpoint is added from the same projection layer, or an API field is renamed and the console breaks.
