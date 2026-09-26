# UI Migration Backlog

Deferred UI decisions for the Queen Console redesign ([spec 035](../specs/035-queen-console-redesign.md)) — the dead ends, unbuilt transitions, and placeheld sections to return to while `/next/` is still a preview. Product and platform ideas belong in [Backlog](backlog.md); shipped work in [Changelog](changelog.md).

Each item carries a **Kind**, a **Source**, what is pending, why it was set aside, and when to pick it up. Strike an item when the decision lands, not when it is discussed — a stale entry is worse than none.

## Dead ends

A link or control that resolves to a page which does not exist yet. Each one is a promise the operator can see and cannot keep.

#### Task and run rows on the trail detail carry no link

- **Kind:** blocker
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md)
- **Summary:** Both row blocks are live. The run rows link to `/next/runs/:traceId/:agentId` and the task rows to `/next/tasks/:traceId/:taskId`, so the trail detail is the one place with no inert rows left, and it was a real target waiting rather than a nicety: a task row on a trail is how an operator walks from a trail to the work it produced.
- **Why deferred:** A row that promises a destination and dead-ends is worse than a row that only shows state. The Dashboard set this precedent, the trail detail followed it, and Runs and Tasks have now landed so neither half of the block is mute.
- **Revisit when:** Never. Tasks landed and the task rows link, which closes this; the run rows linked a route earlier. The precedent stands for any future list: a row that promises a destination and dead-ends is worse than one that only shows state, so a new column should link from its first commit rather than after a backlog item.

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
- **Summary:** `PagePlaceholder` on Sessions, Bees, and Worktrees. Settings is partial — theme selection only; the rest of its surface migrates later. Git is migrated, and it deliberately left the read-only worktree list for the `/next/worktrees` route to take.
- **Why deferred:** Deliberate phase order. The shell and the two highest-traffic surfaces (Dashboard, Traces) went first so the design system is proven against real colony data before the rest depend on it. Git went next because its page was the one whose actions were hardest to place well, and the review settled the button and confirmation questions every later mutating route will face. System followed because it is the other read-mostly page whose format decisions — a metric that may be absent, a column on a different scale from the tiles above it — every later list will inherit. Timeline closed the Work group and settled the feed-row contract (`SignalCard`) and the folded-filter-panel pattern that Tasks, Reviews, Runs, and Sessions will all reuse. Topology closed Diagnostics beside System and is the first route to carry a third-party imperative component, so it is also where the design system's one styling exception is written down. Runs opened the Colony group and, with it, the first detail route that steps between siblings of one parent; Tasks closed the audit's other full-column form and was the first user of `Drawer`; Reviews landed the audit's `DiffViewer` and `CommentThreads` and is the second user of the shared `ReviewActions`.
- **Revisit when:** The next route is picked up; nothing blocks it technically. **Sessions is next** — the last legacy surface left. Bees, Worktrees, and Settings are the remaining three, and none of them is a port: the legacy console has no such tab, so each needs a scope decision rather than a migration, and parity cannot be their acceptance criterion.

## Components the inventory promises

The [design-system contract](../architecture/queen-console-design-system.md) lists these. They do not exist, so a route that reaches for one will not compile.

#### `Drawer`

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (user story #17)
- **Summary:** Promised as the wide-form variant of `Modal` — slide from right, same focus trap, ESC, and focus restoration. Needed by the Sessions launch form and the Tasks create form, the two surfaces spec'd as wide or multi-step.
- **Why deferred:** Every form that has landed fits `Modal`. `Drawer` is the first component whose only justification is a route that does not exist.
- **Revisit when:** Sessions is migrated. `Drawer` now exists as `Modal` with `placement="right"`, which is what this asked for; the session launch form should use it as-is rather than reaching for a second focus implementation.

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

#### A truncated cell cannot be read in full

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Git and System migrations)
- **Summary:** A `grow` cell truncates with `…` and offers no way to see the rest. The Branches commit subject is complete on a desktop and clipped at the 768px design target; the System process command line is clipped at *every* width, because the server already truncates it at 200 runes and a real adapter command is longer than that.
- **Why deferred:** There is no reveal mechanism, and both obvious ones are wrong. Letting the cell wrap is silently defeated by the `grow` column's own truncation — it would have been a trap in the component contract — and it fights the "cells never wrap" rule that keeps row heights uniform. A native `title` would be a second, inconsistent way to show full text next to the `Hint` component that already owns that job.
- **Revisit when:** A third table needs a prose column, or an operator cannot tell two processes apart on the System page. The fix is one `hint` intent on a `grow` cell feeding the existing `Hint` popover, plus the `lg:` visibility tier the desktop-only columns will want anyway — not a wrap and not a `title`.

#### The event feed does not update itself

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Timeline migration)
- **Summary:** `/next/timeline` reads on mount, on apply, on load-more, and on an explicit Refresh. Nothing arrives on its own, so watching a trail during an AFK run means pressing Refresh. The audit's "chunked or streamed loading" was deferred with it.
- **Why deferred:** The feed is cursor-paginated history, so a live update has to answer three questions the current design has no opinion on: whether to prepend or replace, whether to do it while the operator has scrolled away from the top, and how to dedupe a boundary event that arrives on two pages. Guessing wrong is worse than a button — prepending yanks rows out from under a reader, and replacing silently discards their scroll position. The legacy console made the same choice, so nothing regressed.
- **Revisit when:** An operator actually leaves the page open during a run and misses events, or the chrome stream grows a domain event feed to subscribe to. The design needs a scroll-position rule and an id-based dedupe before it needs an endpoint.

#### Only the `?trace=` scope is shareable

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Timeline migration)
- **Summary:** `/next/timeline?trace=<id>` is a real deep link, but the other five filters live in the store and reset on reload, so `?type=VERIFICATION&bee=scout` cannot be shared or bookmarked.
- **Why deferred:** The trace filter is the one that identifies *which trail* an operator is looking at, so it is a navigation target worth a URL. The rest are ad-hoc refinements, and mirroring them means the URL and the store are two sources of truth that must round-trip — including through every Back press.
- **Revisit when:** Someone asks for a link to a filtered feed, or a run report wants to cite one. The fix is one `replaceState` per Apply plus reading the query back on entry, which is a small change once someone has asked for it.

#### The merge diff is parsed here, not by diff2html

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Reviews migration)
- **Summary:** The legacy console vendored diff2html v3 and then scraped its generated DOM for the line numbers a review comment anchors to (`td.d2h-ins .line-num2`). `lib/diff.ts` parses a patch into rows carrying their own line numbers instead, so the anchors are a field read and the console depends on no diff library.
- **Why deferred:** A side-by-side layout and per-word intra-line highlighting went with it, and neither is what a reviewer reads first. The cost is a parser to maintain against git's output format, in exchange for no `@html`, no third-party CSS, and comment anchors that survive a dependency bump.
- **Revisit when:** A reviewer asks for side-by-side, or a diff format appears that the parser mishandles. Rendering is per-row and isolated in `DiffViewer`, so adding a second layout is a renderer rather than a rewrite.

#### A moved diff head silently discards review drafts

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Reviews migration)
- **Summary:** Comment drafts live in the browser, pinned to the diff's `headSha`. When the agent pushes and the head moves, the drafts and the overall note are dropped without a word to the reviewer. The line numbers they carry are meaningless against the new commit, so re-aiming them would be worse than losing them — but losing them silently is still the wrong shape.
- **Why deferred:** The right fix is to say so: keep the drafts, show that the head moved, and let the reviewer re-read the diff before sending. That is a UI decision about interrupting a review, not a parser fix, and it is not worth guessing at.
- **Revisit when:** Someone loses a review to it.

#### A run's events have no paging, because the endpoint caps nothing

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Runs migration)
- **Summary:** `runs.Dir.readEventsFrom` returns `all[skip:]` with no limit, so `GET /api/runs/:traceId/:agentId/events` returns a run's entire event log in one response and `nextCursor` is an index rather than a promise of more. The run page therefore shows no paging control — a button could only ever confirm there was nothing left.
- **Why deferred:** Adding a cap is a server change with a real cost: a run that emits thousands of events would need a bounded response and a client that pages it. That is worth doing only if a run is ever observed emitting that many; the busiest real run in this colony records three.
- **Revisit when:** a run's event log is big enough to be slow, or the endpoint grows a `limit`. The client already knows the cursor shape — `listRunEvents(traceId, agentId, after)` takes an index — so restoring the control is a page change, not a rewrite.

#### A wide table scrolls on a phone rather than reflowing

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Runs migration)
- **Summary:** `DataTable`'s wrapper is `overflow-x-auto`, so a table too wide for the viewport scrolls inside its bordered region. Six columns of monospace identifiers do not reflow into anything readable on a 390px phone, and the previous `overflow-x-hidden` clipped the last column outright — putting a link with no way to reveal it.
- **Why deferred:** Scrolling is the honest floor, not a good answer. A phone operator still has to pan sideways to compare two trails.
- **Revisit when:** A route's table is genuinely unusable at phone width, which the Runs list is close to. The fix is a card layout below 768px — the Dashboard's `TraceRow` list is the precedent — rather than another column of `secondary` hiding, which only removes information.

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

#### The topbar plaque is a link, not a click target

- **Kind:** follow-up
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (System migration)
- **Summary:** The legacy console made the whole Host, Live bees, and Git panel a click target, keyboard-operable with Enter and Space. `/next` makes the Host and Git *labels* links instead, and Live bees is not a link at all until Runs and Sessions land.
- **Why deferred:** A stretched overlay over the panel (`after:absolute after:inset-0`) would swallow the `Hint` popover that sits inside it, because the pseudo-element paints above the panel's own content and intercepts its pointer events — so the panel would either stop being hoverable or stop being clickable. The label link keeps both, and it is visible, which the legacy's invisible target was not.
- **Revisit when:** Runs and Sessions exist and Live bees can link somewhere, or an operator reports not noticing the labels. If whole-panel clicking is ever wanted, the fix is to lift the popover out of the hit area rather than to re-add a keydown handler on a `div`.

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
- **Why deferred:** The cap is the *projection*, which embeds at most 20 events, so no number of rendered rows reaches the full feed. The feed itself pages back without limit.
- **Revisit when:** A trail with more than 20 events makes the preview look broken. The fix is for the trail detail to read a page of `/api/events?traceId=<id>` — the Timeline route already does — instead of the projection's embedded list; it needs no new cursor, because the feed is newest-first and `after` walks backwards.

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
