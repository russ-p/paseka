# Changelog

Shipped features worth calling out. Design records live under `docs/specs/` in the repo (not published on the docs site) — see [Specs index](specs-index.md).

## 2026-09 — Traces pages once, the cursor could always reach the bottom, and the button finally does something

Three things landed on the trail list at once, and one of them is a course correction: the paging went to a cursor, and it should have been there all along.

**The cursor's ten-page ceiling is gone, and it was never a cost.** A cursor page called `ScanRecentTraces(root, limit*10)` — which truncates to the top *n* — and *then* applied the cursor, so past ten pages the scan held nothing older than the cursor, returned an empty page, and the store read that short page as the end of history. **Load older trails** therefore stopped at 500 trails and told the operator the history ended there. The cap was pure waste: `scanAllTraces` walks every trail on every read whatever the page asks for, so the cursor was being filtered against rows the walk had already read and then thrown away. `ScanTracesAfter` filters over the whole walk instead — one in-memory pass, no ceiling. A Go test now walks thirteen pages of two, which the old cap could not do.

**An offset was tried first and was the wrong answer.** It fixed the ceiling, and it cost more than it bought. A rank is only correct until the colony produces a trail, because a new one pushes everything below it down; so the store had to detect new trails on every poll and move the offset by hand, or the operator would click *Load older* and be served rows already on screen. A cursor names a trail, so a poll that prepends fifty of them changes nothing about where the next page starts. Reverting took the whole correction out of the store: `merge` is back to one line, and the offset bookkeeping is gone rather than merely justified.

**The list had two paging controls for one job.** A 15-row client page sat in the table, *Load older trails* pulled fifty more from the server, and they were stacked: pressing the button appended fifty rows that landed **behind** the pager, so nothing on screen changed and the button read as broken. The `1–15 of 50` label counted the loaded window while reading as the size of the history. The client page and the count are gone, and the server page is the only depth the list has.

- **`pageSize={0}` is the contract, and `DataTable` honours it in full.** Every row is on screen, so there is no pager, no range label, and no `?page=` written — `?q=` is the whole of the route's shareable view. The `pageSize > 0` branches were already half-there for this and wrong in two places: the skeleton row count was `Math.min(pageSize, 3)`, which is **zero** rows for an unpaged loading table, so a cold Traces would have shown an empty body instead of skeletons.
- **A store that pages asks for one row past its page.** `tracesStore` requests `pageSize + 1` and drops the extra, so `hasMore` is answered by the response instead of inferred from a full page. The inference was wrong exactly once per history: a total that was a whole multiple of the page size ended on a full page, so the button promised a page that returned nothing and then vanished. The probe row is never part of the window and never becomes the next cursor, or the boundary would sit one trail past what the operator can see.
- **No count, because there is no honest one.** What is loaded is not the history, and a total would mean the full scan the list exists to avoid. The route's `Showing N trails` caption went with the label: same number, same lie.

Web 1026 tests, up five; one Go test replaced by four. golangci-lint clean, svelte-check clean,
console rebuilt into internal/console/next/dist.

## 2026-09 — A shared `?page=` link no longer breaks on arrival

Opening a list with `?page=2` in the address bar threw `Cannot call replaceState(...) before router is initialized` and stopped responding. It was not a cursor or offset problem, and it had been there since `?q=&page=` shipped — arriving on a page the store had not filled yet was the one case the write-back got wrong, and it is the case every shared list link is.

**The cause is a coincidence, which is why it hid.** `DataTable` publishes its view through an effect that runs on mount, and a cold list route mounts with its store still empty. With no rows there is nothing to page, so `currentPage` derives as 0 and the effect published page one over the page the operator arrived on — reaching `replaceState` before the router could accept it, which throws. That throw is the visible symptom; the quieter half is that an effect which throws is an effect Svelte has to tear down, so the table's own write-back was dead for the rest of the visit. **It only ever fired on a `?page=` link**, because *spends nothing on the default view* — the rule that keeps `?q=&page=` off a clean URL — returned early on exactly the default arrival that would have thrown, and a `?page=2` arrival is the one case whose query differs.

- **A table with no rows has no page to publish**, so the effect waits for the rows and lets the pass that follows them do the writing. The seeded page is honoured the moment it is knowable, and a bookmark the rows cannot fill is still corrected — `?page=9` over three rows still lands on `?page=1`, which a "skip the first pass" guard would have silently stopped doing.
- **`loading` was the tempting signal and the wrong one.** Only two of the ten `DataTable` call sites pass it, so guarding on it would have left eight routes crashing. The row count is the condition because every table has one, and it keeps the `loading` prop to the job its name says: drawing skeletons.
- **The regression test is at the component, not the route.** jsdom's SvelteKit `replaceState` does not throw, so a route test passes against the broken code — the first version of this test did exactly that and had to be thrown away. The invariant that actually holds is "no write before there are rows", and that is checkable without a router.
- **`traces.test.ts` no longer leaks its page into the next test.** One test's `?page=1` was seeding whichever table mounted after it, which is how a URL-state test can pass for the wrong reason.

Web 1021 tests, up two. golangci-lint clean, svelte-check clean, console rebuilt into
internal/console/next/dist.

## 2026-09 — A comb file the preview refuses now says how large it is

A trail's comb refused an oversized body with "file too large for inline preview" and nothing else. That is the whole of what an operator knew, and it decides nothing: a 600 KiB file and a 600 GiB one look identical, so "raise the cap, page it, or look somewhere else" was a guess. The size is now on the trail's artifact list and beside the refusal in the preview, which is what makes the next call makeable on a number rather than on a hunch.

- **The size was already known.** `ItemFromFile` stats every comb file to set its mtime, and `MergeAnnounced` copies items wholesale, so `Bytes` on `Item` and on `hiveview.ArtifactView` costs one field and no extra read. A test pins the merge specifically, because a field added to one side and not the other would be dropped on every announced file — the comb files an operator actually looks at.
- **The two caps are now separate constants**, equal today and not the same symbol. They answer different questions: what may be read on one screen, and what may be shipped in a trace export. An operator who hits the preview ceiling and raises the one constant they can find has raised the size of every export archive, which is the wrong lever for the symptom. The decision the entry deferred is now one number away.
- **No range read, and none needed yet.** The largest comb file this colony has produced is 3381 bytes and the cap is a hundred and fifty-five times that, so a paged modal is a real feature for a case that has not happened. The size is what makes the *next* decision possible; it is not itself the fix.
- **`formatProcessBytes` is now `formatSize`.** It was a byte-count formatter at human scale wearing the name of the one column that happened to use it, and the comb list is the second caller. The GiB formatter the metric tiles use stays separate: machine memory is the one place an operator thinks in GiB, and a process at 1 GiB printing "1.00 GiB" where it printed "1024 MiB" would be a change nobody asked for.

9 tests, 1018 total. golangci-lint clean, svelte-check clean, console rebuilt
into internal/console/next/dist.

## 2026-09 — A DataTable row can carry one action, and a branch the sweep skips gets a Delete

`DataTable` cells are declarative — `href` for a link, `badge` for a state — and a cell had no way to hold a control. The stated reason was that a snippet cannot be built inside `<script>`, where the column objects live, and that blocks arbitrary markup rather than a callback. So `action` is the same shape as its neighbours: `(row) => { label, kind?, onselect } | null`, rendering one button beside the cell's own text and nothing at all on a row it returns `null` for.

**The gap it closes is on the Git page.** `leftover` is the sweep's *name* filter, so a `feature/login` branch that is merged, not the default, and not held by a worktree is skipped by the header sweep — and had no verb at all. The operator filtered down to it and found nothing to press, which is `git branch -d` away. Each branch row now offers **Delete** where `gitBranchDeletable` agrees with the server, and the button is offered per row precisely so the table says which branches are deletable: the predicate mirrors the four guards `gitroot.DeleteBranch` holds, and the server still refuses with the reason, so the button only decides whether to offer.

- **The row action and the sweep share one dialog.** Same verb, same guards, so `confirming` is either the sweep or a named branch, the title says which, and a refusal lands in the same place either way. Closing forgets the name, or the next sweep dialog would open already armed for a row the operator is no longer looking at. `gitStore.deleteBranch(name)` and `run('delete')` share one guard and one pending verb, because a push during a delete would race on the same refs.
- **A cell with no action stays exactly as it was**, which took a real fix. The `{#if}` that renders the button left a whitespace text node in *every* cell, and the `badge: () => null` contract — a cell that renders nothing must render nothing, which two tests assert — started failing. The block now opens against the `{/if}` above it on purpose, and the reason is written where the next person will trip over it.
- **Worktrees deliberately did not get one.** A worktree holds uncommitted work, so the row is the state and the page's one confirmed sweep is the verb. A second destructive row action is also the point where `action` stops being one control and becomes a list — which is what the entry's own revisit condition named.

11 tests, 1013 total. svelte-check clean, console rebuilt into
internal/console/next/dist.

## 2026-09 — Hard rule 7 says a wide table must hide its widest columns

Hard rule 7 said a `DataTable` too wide for 768px scrolls inside its own bordered region, and gave the reason: clipping the last column puts a link somewhere with no way to reveal it. That is a floor, not an answer — two columns are never on screen together, so a sideways pan loses more than a hidden column does, while a hidden column loses only what was hidden, and nothing, because `text` feeds the filter whatever the cell is doing. The rule now says so, and **a table of more than four columns must mark at least one `secondary`**.

**The backlog entry's premise turned out to be stale, and the audit is the useful part of this.** It named Bees, Reviews, Worktrees, and System as the tables that had never used `secondary`. All four already did — each of them had been written after the mechanism landed. The census found the table the entry had not named: **Settings**, whose adapter roster was five columns with no `secondary` at all, so the one thing a phone could not see about an adapter was the variable it reads its key from. `API key env` and `File` are now `secondary`; the adapter, its binary, and whether the key resolves stay.

That is the argument for writing the rule down rather than fixing the table. Nothing in the contract required `secondary`, so a table added without it was a table nobody had checked, and four of the ten being right was luck rather than a rule working. The threshold is stated as a count rather than a per-table list, because a list of ten tables rots the first time one of them grows a column and the rule becomes a thing that was true once.

A card layout stays out of scope, for the reason the entry gave and which the audit confirmed: the columns already carry the `label` and `text` a card needs, so the component work is small, but a card puts the table and the cards both in the DOM, and `bees.test.ts` resolves a cell with `screen.getByText(text).closest('tr')`, which throws on two copies — a test-churn cost across ten route suites for pages a solo beekeeper may never open on a phone.

1 test, 1002 total. svelte-check clean.

## 2026-09 — A cued trail waits to exist instead of reporting a 404

Publishing a cue mints a trace id and puts a SIGNAL on the bus, and **nothing is written to disk** until a bee picks that signal up. So the trail an operator had just created was a 404 for as long as the colony took to answer, and the page had nothing but "not found" to say about an action it had just taken successfully. That is a server fact wearing a transition's clothes: no client-side navigation design could have made the destination real, which is why the entry sat undecided for as long as it did.

- **A 404 on the trail detail now reads as `awaitingTrail`.** The page keeps its ten-second poll and says the trail will appear when a bee takes the cue — which also fixes a deep link opened a moment after the publish. It is **not** skeletons: nothing failed and there is no payload to stand in for, so a `role="status"` notice replaces the body instead.
- **The wait is bounded at 30 seconds, and that bound was the real work.** A wait with no end is how a mistyped id becomes a spinner that never resolves, and how a cue nobody picked up — an actual problem — hides behind the same silence. The two dead ends converge, so the timeout message names both: the id may be wrong, or no bee took the cue. The clock starts at the first 404 and is reset when the operator switches trails, because a spent clock handed to a second trail would report it dead on arrival. A 404 for a trail already on screen is still a failure, because a deleted trail is gone.
- **The comb's 404 is settled by the trail's verdict.** Both endpoints fail for the same reason while a cue is in flight and the two reads race, so a queued artifact error would have been on screen the moment the trail landed.
- **The cue never navigates.** Its toast carries **Open trail** instead. A standing cue continues a trail the operator may already be watching, so the new one is not necessarily what they want to see — and a link somebody presses cannot yank them off a page they never acted from. A toast with something to press is a question rather than a receipt, so it lives 12 seconds against the plain toast's 4, and selecting it dismisses the notice rather than navigating out from under it.

14 tests, 1001 total. svelte-check clean, console rebuilt into
internal/console/next/dist.

## 2026-09 — Escape goes back to the list, and `/` reaches the filter

The `g <key>` chord map covers every root and `SideMenu` renders it, but two things an operator reaches for are not destinations and so could never be rows in it. **Escape** now returns to the list the current path belongs to, and **`/`** focuses the list's filter and selects what is in it — so the next keystroke replaces it rather than appending to a term you did not mean to keep.

**Escape** is found by running `isRouteActive` backwards. That is the same test the side menu uses to light a menu entry, run in reverse, so all five detail families and `/reviews/:traceId/:taskId/preview` are covered with no per-route wiring — and a detail route added later is covered without anybody registering it. A path that *is* a list has nowhere to go, so Escape does nothing there rather than reloading the page under the operator.

**It yields twice, and both yields are the point.** A dialog keeps Escape, because `Modal` already closes itself on it and a second, invisible way to dismiss a dialog is how a form loses what was typed. An editable target keeps it too — the same rule that stops a chord firing in the middle of a word — so a half-written review note is never thrown away by a reflex. That rule also settled the case the backlog listed as an open exception: a terminal focuses an off-screen `<textarea>`, so xterm's Escape never reaches the window to be claimed, and no terminal-specific check turned out to be needed.

**A shell affordance finds its target by asking the document.** `dialogOpen()` looks for `[role="dialog"]`, which `Modal` renders inside `{#if open}` and which `Drawer` is; `listFilter()` looks for `[data-list-filter]`, an attribute rather than an id because the element's identity is "the list's filter" and not one particular list's. Each costs a selector, and neither can drift from what is on screen the way a hand-maintained set of open flags can — which also means the first route with two tables needs no change here.

13 tests, 8 of them a new `shell.test.ts` that renders the real layout: Escape to the owning list, Escape inert on a list, both yields, `/` focusing and selecting, `/` inert where there is no list, and the chord map still working. svelte-check clean, console rebuilt
into internal/console/next/dist.

## 2026-09 — A list's filter and page live in its URL

Every `DataTable` under `/next/` — traces, runs, tasks, reviews, sessions, bees, worktrees, branches, adapters, processes — now keeps its filter and its page in the query as `?q=` and `?page=`. Ten routes gained it from one component, which is the argument for the fix living there rather than in each route: nothing about a route's column config changed, and nothing about the table's behaviour did either.

This is the half of user story #3 a list can keep by itself. A narrowed list is a link an operator can paste to somebody, and **Back** from a trail, run, or task detail now returns to the page of the list they left instead of page one. The other half — the scroll offset of a long page that has no page number, like a run's event log or a session transcript — is per surface and has no URL to live in, so it stays its own entry.

- **`replaceState`, never `pushState`.** A filter is not navigation, and Back must not walk an operator backwards through the letters of a word. It is also why a table never watches the history: nothing it does creates an entry to move between.
- **The query merges, keeps the hash, and spends nothing on the default view.** A param the table does not own — `?trace=`, the timeline's deep link — survives a keystroke; an anchored note stays anchored; and an unfiltered first page is a clean URL rather than `?q=&page=0`.
- **The page published is the page on screen, not the page requested.** A bookmark a poll has invalidated is clamped for display, and leaving the stale number in the address bar would make the URL disagree with the table under it. A `?page=` the URL cannot be trusted for is read strictly, because `parseInt('1.5')` is `1` and a number somebody got wrong is not a page to round in their favour.
- **Traces pages three times, and only the middle layer is a URL.** The server cursor behind **Load older trails** and the filter are not in the query, so a shared link describes the view *inside* the rows you have rather than which fifty you pulled — which is the promise of user story #3, not half of it. `stateKey` namespaces both params (`?runs.q=&runs.page=`) for a route that grows a second table; absent everywhere today, and deliberately not derived from `label`, which is prose written for a human.

Two things fell out of pinning it down. The reset that returns a table to page one on a keystroke moved **out of an effect and into the input handler**, because an effect also runs on mount and would have thrown away the very `?page=` the table was seeded with. And a table in a test renderer needed a `replaceState` that works: `$app/navigation` throws before the router is initialised, so `tests/setup.ts` stands one in against the History API — and resets the query after each test, because a filter one test typed was otherwise seeding the next test's table and emptying it.

18 tests, 9 in the new module, 973 total. svelte-check clean, console rebuilt into
internal/console/next/dist.

## 2026-09 — The event feed can be watched while it is being read

`/next/timeline` had no way to move without being asked, which made watching an AFK run a matter of pressing **Refresh** and hoping. The header now carries an **Auto-refresh** selector — **Manual**, **Every 5s**, **Every 10s**, **Every 15s**, **Every 60s** — and it arrives on **Manual**, because most visits to a recorded history are a read, and a feed that moved on its own would be a second reader deciding when this one looks. The four steps are the console's existing cadences plus a minute, written as a ladder of numbers because picking a cadence is comparing them. A cadence does not survive the visit: a feed that remembered its timer would re-arm it for whoever opened the page next.

- **A tick is the reset read Refresh already performs.** The list is replaced, the cursor and `hasMore` recompute, and nothing is prepended — so choosing a cadence costs no scroll position, no row identity, and no boundary event. That is the whole reason a timer can be offered at all: the three questions a live feed has to answer first, whether to prepend or replace, how to behave under a scrolled reader, and how to dedupe an event arriving on two pages, all stop being questions.
- **The feed's one read-in-flight guard is now a cadence rule too.** A tick landing inside an **Apply** is refused rather than queued, because it is the same overlap that would append two pages against one cursor. No second guard, because there is no second kind of read.
- **Refresh returns the selector to Manual.** A deliberate read is the operator taking the cadence back, and a timer resuming behind the click would leave them unsure whether the button had done anything. Polling pauses on `visibilitychange` the way the chrome stream does — a hidden tab is not somebody waiting on a feed, and each tick reads up to fifty trail directories — and returning re-reads at once rather than waiting out the rest of the interval. The handler is also called once on mount, because a tab opened in the background never fires the event and is exactly the case the pause exists for.
- **The Refresh button keeps its label and spins its icon**, the Sessions button's shape. `Refreshing…` is four characters wider than `Refresh`, which in a header row pushes the whole control group — invisible on a click, a flinch every five seconds once a cadence is armed. The word stays, the `RefreshCw` spins, and `aria-busy` carries the state to a reader who cannot see the motion. An **action** button still names its own verb while it waits (`Fetch` → `Fetching…`), because there the operator started it by hand and a verb is information rather than reflow; the contract now says which case is which.
- **The spin lasts at least a second, counted from when the read started.** A feed read answers in tens of milliseconds, which is one or two frames of motion — long enough to look like a glitch, too short to be feedback. The read itself is never delayed and `disabled` still follows it, so the button is clickable again the moment the feed is current even while the acknowledgement is on screen; a second read landing inside another's second extends the one already there, which is what makes a five-second cadence read as a heartbeat rather than as five flickers. The page's own arrival is exempt, because the skeletons already said so and a spinning Refresh on a feed nobody armed is the icon claiming a cadence that does not exist.
- **A latent store bug surfaced while pinning that down.** `inFlight` was a plain `let` behind a `busy` getter, so the `disabled` binding on three buttons had no reactive value to re-evaluate on and was computed once at mount — it refreshed only when something else in the same subtree happened to change, which it had been doing by accident while the button's label was still reading `loading`. The tell is a control that stays disabled after its work finished; the fix belongs in the store, and `inFlight` is `$state` now.

The build was a Svelte trap worth recording: the mount effect calls `timeline.start()`, which reads the cadence to decide whether to arm its timer, so tracking it made every choice the operator made re-run the effect — whose cleanup stops the store and whose body starts it again. The control that only meant to set a timer spent a full reset read. The effect is now `untrack`ed, and re-arming belongs to the setter, which owns its timer the way every other store owns its own.

- Spec: [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Timeline)
- Canonical: [Queen Console](../guide/queen-console.md), [Queen Console design system](../architecture/queen-console-design-system.md)

## 2026-09 — Worktree path and base SHA are copyable on a trail

A trail's worktree block now puts **Path** and **Base SHA** on the clipboard alongside the trail id. The path is the value an operator pastes into a shell, and it was the one long value on the page with no way to take it whole — expanding the hint and retyping it character by character was the only route to a path the console had already fetched. The rule behind it is now written down: a copy button marks a value you paste somewhere else, so a count or a timestamp never gets one, and a worktree with no base SHA offers a single button rather than one that copies an em dash.

- Spec: [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Traces)
- Canonical: [Queen Console design system](../architecture/queen-console-design-system.md)

## 2026-09 — Live bees plaque links to where the bees are

The topbar's **Live bees** panel is a link, which closes the last plaque that summarized a page it could not reach. It follows the legacy console's rule, because a bee is not addressable and there is nothing to point at but the surface holding it: an AFK bee opens **Runs**, an interactive-only colony opens **Sessions**, and an idle plaque opens **Runs**, which is where the next bee will appear. A colony running both kinds lands on Runs — the order the legacy panel used, and the order the plaque's own `afk · session` line reads.

- The three navigable plaques are **Host** → System, **Live bees** → Runs or Sessions, and **Git** → Git. The label is the link, not the whole panel: a stretched overlay would swallow the hover popover inside the panel, so the panel stays hoverable and the visible label is what you click.
- The destination is a single pure function, `liveBeesPath`, rather than branching in the component, so the topbar cannot drift from the legacy order.

- Spec: [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (top panel)
- Canonical: [Queen Console](../guide/queen-console.md), [Queen Console design system](../architecture/queen-console-design-system.md)

## 2026-09 — Settings, and what each setting resolves to

`/next/settings` is the last route of the console redesign, and the first one whose subject the legacy console never had at all. It answers what the colony is configured with **and what decided each value** — which is the part that matters, because several settings are decided by something other than the file you would edit. An operator who changes `config.yaml` and watches nothing happen can now see why: the page names `PASEKA_NATS_URL` as the thing actually in force and warns that the file is not consulted. Values nobody wrote are labelled as defaults, a value the code supplied is distinguished from one the file declares, and a colon-separated prefix nobody declared is reported as the bus default rather than named after a file that never mentioned it.

- **Transport, Adapters, Human gateway, Colony, Appearance.** The adapter table shows each adapter's binary, the environment variable it reads its API key from, whether that variable currently resolves, and whether the value came from `adapters/<name>.yaml` or was inferred from a default — a distinction no loaded config can make on its own, because every adapter loader fills in a default for a key the file omits. The gate's seven push categories are listed with their modes, and a gate that is present but switched off is reported as a configuration rather than as a failed read.
- **The console never receives a secret.** An adapter row carries the *name* of a variable and whether it resolves; the gate reports `botTokenSet`, never the token. That is the model the adapters already follow at runtime, so a key never enters the colony config at all.
- `GET /api/config` is new, read-only, and reports no liveness — whether NATS is connected is already on the status stream the topbar keeps, and asking twice would be the duplicate poll the console's store contract forbids. The page reads once and waits for **Refresh** rather than polling, because configuration changes when a human edits a file.

**This closes the redesign's route set: no page under `/next/` is placeheld any more.** The legacy console remains at `/` until the explicit root cutover.

- Spec: [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Settings)
- Canonical: [Queen Console](../guide/queen-console.md), [Queen Console design system](../architecture/queen-console-design-system.md)
- Out of scope for that work: user story #9 also asks for these settings to be *editable* "without editing files", and the console still cannot write configuration. That is a platform capability rather than a route — nothing outside `paseka init` writes the colony's YAML, and that writer is create-only — so it is tracked separately — see [Backlog](backlog.md).

## 2026-09 — Bees roster in Queen Console

`/next/bees` lists the colony's whole roster: role, adapter, intent vocabulary, sector, whether it works in an isolated worktree or against the colony root, how many adapter processes it holds live, and its most recent run with that run's state badged. The page shows **every** bee, including `script` adapters that run headless and cannot be started as an interactive session — the launch dropdowns in Sessions and Tasks deliberately still offer only the bees a session can start.

- `GET /api/bees` gained `?scope=colony`. **The default is unchanged and still returns the launch picker**, so the two launch forms and the legacy console read exactly what they read before; an unknown scope answers 400. The roster scope adds a per-bee `interactive` flag, the bee's `sector` (documented in bee YAML and previously visible in no console surface), and a **server-derived `lastRun`**. That last one is not cosmetic: the client cannot compute it, because the runs list is capped at the 50 most recent colony-wide and a busy colony would otherwise report a quiet bee as never having run. The picker does not pay for the lookup at all, so opening a launch drawer does not walk every run directory in the colony.
- The route reads once and does not poll — a bee's identity and configuration come from committed bee YAML — and its live column is joined from the status stream the topbar already keeps, so it cannot disagree with the Live bees plaque. Refresh stays because a run landing is the one thing on the page that changes without a commit.

- Spec: [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (Bees)
- Canonical: [Queen Console design system](../architecture/queen-console-design-system.md), [Bee config](../guide/bee-config.md)

## 2026-09 — Age-based prune

`paseka prune` adds an age-aware sibling to `paseka purge`: it removes `.paseka/worktrees/` and `.paseka/runs/` trace directories whose last activity predates a retention period (default **14 days**, `--older-than 14d|2w|336h`) instead of wiping every trace. Filesystem flags mirror purge (`--runs`, `--worktrees`, `--all`, `--yes`, `-C`), and a plan is shown before deleting. Worktrees with uncommitted changes are never auto-removed. With `--bus`, prune also removes task-ledger KV, stream events, and artifacts for correlatable traces, using ledger task activity to protect traces whose files have gone quiet while their tasks were touched recently, and cleaning up stale ledger-only traces.

- Spec: [033-cli-prune](../specs/033-cli-prune.md)
- Canonical: [CLI](../guide/cli.md) (`paseka prune`)

## 2026-09 — Clean shutdown and serialized runtime state

Runtime writes to home `state.json` are serialized so a stopped bee can never flip back to running from a stale write, and `paseka run` treats an OS signal (Ctrl+C) as a clean exit instead of a failed run — operators can shut the hive down without false error states. `paseka console` prints a shutdown notice on Ctrl+C.

- Canonical: [CLI](../guide/cli.md) (`paseka run`, `paseka console`)

## 2026-09 — Resume OpenCode HITL sessions

A new OpenCode HITL session now pre-creates its provider chat before the TUI starts — `opencode serve` + `POST /session`, the OpenCode analog of Cursor's `create-chat`, with **no model turn and no cost** — and launches the TUI with `--session <ses_*>`, storing `ses_*` as `providerSessionId`. Because the OpenCode TUI ignores `--prompt` when `--session` is set, the kickoff (and any Resume continue line) is delivered out-of-band through the TUI's own loopback control server (`--port` + `/session/<id>/prompt_async`), so the first turn actually lands in the pre-created chat. A finished OpenCode HITL session can then **Resume** in Queen Console or via `paseka session resume`: a new Paseka session on the same Flight Trail that copies the id, records `resumedFrom`, skips honey, and fails closed when ineligible. Pre-create failure degrades to a normal TUI launch; Pi/Claude/script sources stay ineligible (the ineligible reason is now `not_resumable`).

- Spec: [030-opencode-session-resume](../specs/030-opencode-session-resume.md)
- Canonical: [Interactive sessions](../guide/interactive-sessions.md), [CLI](../guide/cli.md) (`paseka session resume`), [Queen Console](../guide/queen-console.md), [Architecture overview](../architecture/overview.md)

## 2026-09 — Pull-request delivery

Isolated final-gate approve can **publish a pull request** instead of merging into the colony clone. Colony `defaults.delivery: pull_request` is opt-in; empty still means local merge. The apiary points `forge.command` at a `tea` / `gh` / custom script (JSON stdin/stdout). Approve pushes the worktree head, upserts the PR, and keeps `waiting_review` until the host reports `merged`. Queen Console Reviews grows Open/Update PR; `paseka proposal approve` takes `--pr-title` / `--pr-body` / `--draft`. Telegram cards show the URL. The Git tab still only pushes default.

- Spec: [024-pull-request-delivery](../specs/024-pull-request-delivery.md)
- Canonical: [Pull-request delivery](../guide/pull-request-delivery.md), [Colony layout](../guide/colony-layout.md), [Queen Console](../guide/queen-console.md), [CLI](../guide/cli.md) (`paseka proposal approve`), [INSIGHT kinds](../reference/insight-kinds.md) (`pr.body`)

Deferred from that work: per-cue delivery, auto-detect forge, a dedicated `pr_open` task status — see [Backlog](backlog.md).

## 2026-09 — Config profiles

A Beekeeper can run the same colony through a named overlay (`paseka --profile pi`, `PASEKA_PROFILE`, or sticky `profile:` in home `config.yaml`) without rewriting committed bee YAML. Global `adapter:` replaces LLM bees (script and `command:` roles stay put unless a per-bee exception says otherwise). Unknown names fail closed and list what exists. `--no-profile` ignores a sticky default. Queen Console inherits the process overlay; there is no in-UI switcher. The Pi adapter ships a committed `.paseka/profiles/pi.yaml` (`--profile pi`) and no longer swallows or mispasses its prompt argument.

- Spec: [027-config-profiles](../specs/027-config-profiles.md)
- Canonical: [Colony layout](../guide/colony-layout.md), [Bee config](../guide/bee-config.md), [CLI](../guide/cli.md) (global `--profile`), [Architecture overview](../architecture/overview.md)

Deferred from that work: concurrent processes with different profiles still share worktrees and home `state.json` — see [Backlog](backlog.md).

## 2026-09 — Resume Cursor HITL sessions

A finished Cursor HITL chat can continue in the same Cursor conversation without `create-chat`. Queen Console **Resume** (optional one-line continue) and `paseka session resume` start a **new** Paseka session that copies `providerSessionId`, records `resumedFrom`, skips honey, and fails closed when the source is ineligible or still live. New launches still create a fresh chat.

- Spec: [025-cursor-session-resume](../specs/025-cursor-session-resume.md)
- Canonical: [Interactive sessions](../guide/interactive-sessions.md), [CLI](../guide/cli.md) (`paseka session resume`), [Queen Console](../guide/queen-console.md)

## 2026-09 — Standing trail checkpoints and doctor smells

Standing ticks keep procedure memory in the trail comb (`checkpoint.json`, optional `journal/`) and prompt partials tell bees to read that file first, write it atomically, and spawn bloom work on a **new** `traceId`. `paseka doctor` warns when a standing SIGNAL kind has only isolated worktree subscribers, when a standing tick bee publishes isolated `code.proposal`, or when an isolated proposal already sits on a standing trail. `purge --runs` is documented as wiping that memory.

- Spec: [028-standing-trails](../specs/028-standing-trails.md)
- Canonical: [Forage Cues](../guide/cues.md) § Checkpoints, [Prompt templates](../guide/prompt-templates.md), [CLI](../guide/cli.md) (`paseka doctor`, `paseka purge`)

Deferred from that work: live eval-colony standing case, ledger compact after a year of ticks — see [Backlog](backlog.md).

## 2026-09 — Queen Console chrome stream

Always-visible header plaques (Hive runtime, Live bees, Host, Git) and Reviews/Sessions tab badges now update from one Server-Sent Event stream instead of a bundle of timed GETs. System and Git tabs still poll their full snapshots while open. Git status never fetches remotes from the header clock.

- Spec: [029-console-chrome-stream](../specs/029-console-chrome-stream.md)
- Canonical: [Queen Console](../guide/queen-console.md)

Deferred from that work: domain Timeline SSE (`/api/events/stream`) — see [Spec 002](../specs/002-queen-console-mvp.md).

## 2026-09 — Standing trail badge in Console, status, and Telegram

Flight Trails bound to a standing Forage Cue are now visually distinct from bloom trails on every operator surface, so a months-old triage identity no longer reads as a stuck feature.

- Queen Console: **standing** badge in Dashboard and Traces lists, `standing` flag in trail detail, and honey shown as `remaining / stipend` for those trails.
- `paseka status`: `· standing` marker on honey, low-honey, and recent-trace lines; `--json` adds `standing: true` to `energy.traces`, `attention.lowEnergyTraces`, and `recentTraces`.
- Telegram `/traces`: `standing` plus remaining honey on standing trail lines.

The badge is derived from colony cue YAML (`standing.trace`), not a ledger flag — delete the cue and the trail reads as an ordinary leftover trail.

- Spec: [028-standing-trails](../specs/028-standing-trails.md)
- Canonical: [Queen Console](../guide/queen-console.md), [CLI](../guide/cli.md) (`paseka status`), [Forage Cues](../guide/cues.md), [Telegram gateway](../guide/telegram-gateway.md)

## 2026-09 — Standing trail first title

The first standing `cue run` that seeds honey also publishes `INSIGHT` / `trace.title` when the trail has none yet: cue `description` (trimmed, 120-character cap), or the cue id if description is empty. Later ticks and a human/bee title already on the trail are left alone, so lists show “Daily triage” instead of a raw id without clobbering a refined name.

- Spec: [028-standing-trails](../specs/028-standing-trails.md)
- Canonical: [Forage Cues](../guide/cues.md), [INSIGHT kinds](../reference/insight-kinds.md) (`trace.title`)

## 2026-09 — Console lib assets in `go install` binaries

Queen Console third-party files (xterm, Diff2Html, cytoscape, fonts) live under `/lib/...`. A directory named `vendor` is stripped from Go module zips, so `go install` used to ship a binary that answered those URLs with the SPA `index.html`. Missing `.js`/`.css` requests now 404 instead of falling back to HTML.

- Canonical: [Queen Console](../guide/queen-console.md)

## 2026-09 — Standing trail overlap refuse

A second standing `cue run` fails closed while the trail already has an open tick (`planned`, `ready`, `running`, `waiting_review`) or a live AFK adapter on that `traceId`. The error says the trail **is busy** and names the blocking task status or bee. No stipend and no ingress. `blocked` / finished tasks and interactive sessions do not block, so a drained tick can start the next ration and a HITL inspect is not a tick.

- Spec: [028-standing-trails](../specs/028-standing-trails.md)
- Canonical: [Forage Cues](../guide/cues.md), [CLI](../guide/cli.md) (`paseka cue`)

## 2026-09 — Standing trail stipend

Later ticks of a Standing Trail **replace** remaining honey with the cue stipend (`SIGNAL` / `energy.stipend`) instead of leaving leftover tokens or stacking `energy.add`. The event does not change seed budget or `energyAdded`, and it does not unblock honey-blocked tasks from a previous tick. A killed standing trail refuses `cue run` (error names `system.kill`). First tick still seeds via `SeedEnergy` as in the identity slice.

- Spec: [028-standing-trails](../specs/028-standing-trails.md)
- Canonical: [Forage Cues](../guide/cues.md) § Honey, [Task ledger](../reference/task-ledger.md) § Honey reserve, [Event contracts](../reference/event-contracts.md)

## 2026-09 — Standing cue identity

A Forage Cue may declare `standing.trace` and `standing.stipend`. Omitting `--trace` / API `traceId` / Telegram `cue:` then publishes on that stable Flight Trail instead of minting a disposable bloom id; a mismatched explicit id fails closed. First tick seeds honey from stipend. Standing task cues require `review: none` and `worktree: false`. Nuc export/import carries the YAML as today.

- Spec: [028-standing-trails](../specs/028-standing-trails.md)
- Canonical: [Forage Cues](../guide/cues.md), [CLI](../guide/cli.md) (`paseka cue`), [Glossary](../idea/glossary.md) (Standing Trail)

## 2026-09 — Cursor Agent log from local transcripts

`paseka export --include agent-logs` now fills Cursor **Agent log** tool calls from `~/.cursor/projects/<slug>/agent-transcripts/<uuid>/<uuid>.jsonl` (same UUID as `providerSessionId`). Files are read in place, never copied into `.paseka/runs/`. Missing or unreadable transcripts omit with `store not found` / `parse error`; a conversation with no tools is an empty table. Pi stays stubbed.

- Spec: [021-provider-session-logs-export](../specs/021-provider-session-logs-export.md) (Draft; Cursor jsonl reader)
- Canonical: [CLI](../guide/cli.md) (`paseka export`), [Architecture overview](../architecture/overview.md) (adapter result collection)

## 2026-09 — Export Agent log (adapter stubs)

`paseka export --include agent-logs` adds a per-run **Agent log** subsection (tool-call table, or a one-line omit reason). Enrichment is best-effort: missing `providerSessionId`, adapters without a resolver, stub readers, and resolve errors never fail the command. Cursor and Pi implement the resolve seam as stubs (`not implemented`) until vendor session stores are read in place. Provider logs are not copied into `.paseka/runs/`.

- Spec: [021-provider-session-logs-export](../specs/021-provider-session-logs-export.md) (Draft; export plumbing)
- Canonical: [CLI](../guide/cli.md) (`paseka export`), [Architecture overview](../architecture/overview.md) (adapter result collection)

## 2026-09 — HITL provider session id

Interactive Cursor and Pi sessions record the provider’s native session id as `providerSessionId` on `session.json` and `meta.json` before the PTY starts. Cursor HITL runs `agent create-chat` and launches the TUI with `--resume`; a failed create-chat still starts the session without a pointer. Pi HITL stores the pinned `--session-id` (`agentId`, or the flag already on a `command:` override). Queen Console session detail shows the id when set. Export Agent log and `events.ndjson` cleanup stay out of this slice.

- Spec: [021-provider-session-logs-export](../specs/021-provider-session-logs-export.md) (Draft; HITL association)
- Canonical: [Interactive sessions](../guide/interactive-sessions.md), [Architecture overview](../architecture/overview.md) (interactive sessions)

## 2026-08 — AFK provider session id

Headless Cursor and Pi runs record the provider’s native session id as `providerSessionId` on `result.json` / `meta.json`. Cursor parses `session_id` from stream-json (prefer `system`/`init`). Pi AFK pins `--session-dir` / `--session-id` like interactive sessions, then prefers the JSON session header when present. Queen Console run detail and `paseka inspect usage --agent` show the id when set. Missing id does not fail the run. Export enrichment and HITL Cursor resume are not in this slice.

- Spec: [021-provider-session-logs-export](../specs/021-provider-session-logs-export.md) (Draft; association slice)
- Canonical: [Architecture overview](../architecture/overview.md) (adapter result collection), [CLI](../guide/cli.md) (`paseka inspect usage`)

## 2026-08 — Queen Console Git

Queen Console header adds a **Git** plaque (ahead/behind `origin`, dirty, or synced) next to Host. Click or Enter/Space opens a **Git** tab: colony root vs origin with explicit **Fetch** (remote-tracking only), **Push** of the default branch (never `--force`; hooks skipped by default), and fast-forward-only **Pull** as a backup when a webhook sidecar did not update the clone. The tab lists colony-managed worktrees (with prune of orphans) and leftover merged branches (`paseka/*`, `feature/` / `hotfix/` / `fix/`) with safe `git branch -d`. Reviews warn when origin is ahead of the local default branch; Approve still does not push. Git ops do not depend on NATS. Credentials stay in the system git helper (`HOME`/`PATH` of `paseka console`).

- Spec: [023-console-git](../specs/023-console-git.md)
- Canonical: [CLI](../guide/cli.md) (`paseka console`), [Homelab deployment](../guide/homelab-deployment.md)

Deferred from that work: autostash list, porcelain file list, incoming log vs origin, mutation lock while bees run — see [Backlog](backlog.md#console-git).

## 2026-08 — Queen Console System Info

Queen Console header adds an observe-only **Host** plaque (CPU percent and memory used/total, optional 1-minute load) next to Hive runtime and Live bees. Click or Enter/Space opens a **System** tab with hostname/kernel, OS/arch, CPU count, uptime, console PID, load averages, memory, optional colony-root disk, and a capped top-process table. Live bee PIDs are highlighted client-side. `GET /api/system` reads the OS view of the `paseka console` process (container PID namespace in Docker); it does not call Docker APIs and does not depend on NATS or Hive runtime. Linux uses `/proc`; other OSes keep identity fields and leave load/processes empty. `/proc/meminfo` may still describe the **host** when cgroups do not hide it.

- Spec: [022-console-system-info](../specs/022-console-system-info.md)
- Canonical: [CLI](../guide/cli.md) (`paseka console`), [Homelab deployment](../guide/homelab-deployment.md)

## 2026-08 — Honey remaining / allocated

Honey UIs (CLI, Queen Console, Telegram, hive status, export) show **remaining / allocated**, where allocated is the frozen seed plus post-seed `energy.add` totals (`energyAdded` on the task ledger snapshot). `energyBudget` stays the initial seed so Forage Cue overrides and `budget == 0` seeding still work. When the trail has been topped up, a second line shows `seed N · topped M`. Console **low** still uses remaining vs seed (`budget/4`).

`paseka energy show` still prints parseable `budget:` and `remaining:` integers; `added:` and the remaining/allocated fraction appear only after a post-seed top-up. Unseeded trails print remaining without a `/ 0` denominator. Ledger snapshots from before `energyAdded` can still show remaining above seed until a new top-up.

- Canonical: [Task ledger](../reference/task-ledger.md) (Honey reserve), [Forage Cues](../guide/cues.md) § Honey, [CLI](../guide/cli.md) (`paseka energy`), [Telegram gateway](../guide/telegram-gateway.md)

## 2026-08 — Worktree branch name (`worktree.branch`)

Planner bees can emit `INSIGHT/worktree.branch` so isolated Flight Trail worktrees use readable git refs (`feature/…`, `hotfix/…`) instead of only `paseka/<traceId>`. Runtime applies names on worktree ensure and renames immediately when a later insight lands; collisions fail closed (no detached HEAD fallback). Merge, merge-diff, Queen Console trace detail, and `state.json` follow the live branch name. Path stays `.paseka/worktrees/<traceId>/`.

- Spec: [020-worktree-branch](../specs/020-worktree-branch.md)
- Canonical: [Insight kinds](../reference/insight-kinds.md), [Architecture overview](../architecture/overview.md), [Prompt templates](../guide/prompt-templates.md), [Interactive sessions](../guide/interactive-sessions.md)

## 2026-08 — Final-gate Request changes (Slice C)

On `review: final` / `_review`, annotated Submit on the merge preview is **Request changes**: it writes the comb packet as before, keeps the merge gate in `waiting_review`, and plans+readies a new AFK rework task for the last isolated-proposal Bee on the same Flight Trail (honey consumed on dispatch). Plain reject remains abandon-only (feedback, no rework). Duplicate Request changes is rejected while non-final work is still in flight. Approve stays the only merge path. CLI `paseka proposal reject --comments-file` on a final gate uses the same rework path.

- Spec: [017-console-diff-review](../specs/017-console-diff-review.md)
- Canonical: [Queen Console](../guide/queen-console.md) (Reviews), [CLI](../guide/cli.md) (`paseka proposal reject`), [Task ledger](../reference/task-ledger.md)

## 2026-08 — Session deferred flush uses home NATS config

Interactive session (and other ColonyRoot-only) deferred flush loads `~/.config/paseka/<slug>/config.yaml` before connecting. Previously a partial in-memory context treated NATS as unset, flushed pending events to the run audit log with a no-op publisher, and never reached JetStream or the task ledger.

- Canonical: [Interactive sessions](../guide/interactive-sessions.md), [CLI](../guide/cli.md) (`paseka event emit --defer`)

## 2026-08 — Queen Console annotated review comments (Slice B)

Queen Console merge preview supports line-anchored comment drafts (click added/context lines; Shift-click for a range). Submit writes `review-comments.md` to the trail comb, publishes `SIGNAL/artifact.written` (producer `console`), then a short `INSIGHT/human.feedback` with optional `ref`. Fail closed if the comb write fails. `review: required` still returns the task to `ready`. CLI: `paseka proposal reject --comments-file` copies an existing Markdown packet into the same comb ref. Final-gate rework shipped separately as Slice C.

- Spec: [017-console-diff-review](../specs/017-console-diff-review.md) (Slice B)
- Canonical: [Queen Console](../guide/queen-console.md) (Reviews), [CLI](../guide/cli.md) (`paseka proposal reject`), [Insight kinds](../reference/insight-kinds.md), [Prompt templates](../guide/prompt-templates.md)

## 2026-08 — Model aliases (`params.model`)

Colony-owned `model_aliases` in `.paseka/colony.yaml` map stable names to vendor model ids; home `config.yaml` overlays the same keys per machine. Bees keep `params.model` as alias or raw id; runtime resolves once before `--model` is passed to the adapter.

- Spec: [019-model-aliases](../specs/019-model-aliases.md)
- Canonical: [Colony layout](../guide/colony-layout.md), [Bee config](../guide/bee-config.md), [Architecture overview](../architecture/overview.md)

## 2026-08 — Trail artifacts protocol (comb + `artifact.written`)

Trace-scoped comb under `.paseka/runs/<traceId>/artifacts/` with `{{.ArtifactsDir}}` prompt injection. Runtime captures a per-run SHA-256 baseline and publishes one batched `SIGNAL/artifact.written` on successful AFK or interactive exit (added/changed files only). Coexistence with deferred `artifact.written` skips duplicate scan flush. Queen Console lists comb files (staged vs announced) with Markdown preview. `paseka export --include artifacts` inlines comb bodies in reports. Human `artifacts.WriteAndAnnounce` helper publishes with producer `console` (for 017 Slice B).

- Spec: [014-artifacts-protocol](../specs/014-artifacts-protocol.md)
- Canonical: [Architecture overview](../architecture/overview.md) (runs comb), [Prompt templates](../guide/prompt-templates.md) (`ArtifactsDir`), [Bee routing](../reference/bee-routing.md), [CLI](../guide/cli.md) (`paseka export --include artifacts`)

## 2026-08 — Queen Console merge-diff viewer (Slice A)

Queen Console Reviews final merge gates show merge-diff summary on the detail panel; **Open merge preview** opens a dedicated full-page viewer (sticky file list, path filter, jump-to-file, per-file hunks via vendored Diff2Html, unified or side-by-side format). Line-number gutters stay clipped to each file pane instead of overlaying scrolled hunks. Approve/reject stay on Reviews. Queue polling still skips re-fetch when the same gate stays selected.

- Spec: [017-console-diff-review](../specs/017-console-diff-review.md) (Slice A only; B/C follow)
- Canonical: [Queen Console](../guide/queen-console.md) (Reviews)

## 2026-08 — Queen Shell colony status

`paseka status` is a read-only colony snapshot for Beekeepers and interface bees: runtime liveness, live bees, task counts, honey for recent Flight Trails, attention items (reviews, invites, failures, exhausted honey), and recent traces. Default text output; `--json` emits `schemaVersion` 1 for agents. `--check` exits non-zero only when the hive substrate cannot choreograph (runtime down or configured NATS unreachable) — pending HITL work is not treated as an outage.

- Spec: [018-cli-colony-status](../specs/018-cli-colony-status.md)
- Canonical: [CLI](../guide/cli.md) (`paseka status`)

## 2026-08 — `task.ready` race fix (prompts + ledger)

Scout and Drone breakdown prompts now defer both `task.plan` and post-plan `task.ready` (FIFO flush), with slim ready payloads (`taskId` only). The task ledger parks unmatched `task.ready` kicks on `pendingReady` until the matching `task.plan` registers the task, then promotes on the plan event — so live-ready-before-plan no longer loses autostart. Cleared on `system.kill`.

- Canonical: [Task ledger](../reference/task-ledger.md), [Prompt templates](../guide/prompt-templates.md)

## 2026-08 — Export `--include` (richer payload)

`paseka export` accepts composable `--include` slices independent of `--format`: `usage` (trace aggregate + per-run tokens), `durations` (wall-clock per run), `bees` (committed `.paseka/bees/*.yaml` for trail roles), `colony` (`.paseka/colony.yaml`), `cues` (all colony cues), and `artifacts` (trail comb file bodies). Default export stays trail-only with no config snapshots.

- Canonical: [CLI](../guide/cli.md) (`paseka export`)

## 2026-08 — Export `--format` (HTML | Markdown)

`paseka export` now accepts `--format html` (default) or `--format md`. Both renderers share the same `TraceExportData` (overview, tasks, runs, event timeline); the output filename extension matches the format. Markdown keeps run and event summaries verbatim and fences raw event JSON for agent-friendly trail dumps.

- Canonical: [CLI](../guide/cli.md) (`paseka export`)

## 2026-08 — Forage Cues (cue layer)

Named colony ingress shortcuts (`.paseka/cues/<id>.yaml`) publish `signal` or `task` choreography without hand-writing emit JSON. One definition drives Queen Shell (`paseka cue list|run`), Queen Console **Run cue** (`GET/POST /api/cues`), and Telegram `commands.custom` with `cue: <id>`. Optional per-cue `energy_budget` seeds a smaller initial honey reserve on fresh trails; `paseka init` scaffolds `feature` and `hotfix`. Nuc export/import includes cues with `--cues` filter.

- Spec: [016-cue-layer](../specs/016-cue-layer.md)
- Canonical: [Forage Cues](../guide/cues.md), [CLI](../guide/cli.md) (`paseka cue`), [Telegram gateway](../guide/telegram-gateway.md), [Colony layout](../guide/colony-layout.md), [Nuc packs](../guide/nuc.md), [Task ledger](../reference/task-ledger.md)

## 2026-07 — Deferred event emit buffer

Bees can stage bus events until a run or session completes successfully. `paseka event emit --defer` validates and appends to per-run `pending.ndjson`; runtime flushes FIFO on success (before `run.summary` synthesis). Operators inspect with `paseka event pending` and recover with `paseka event flush` or `--discard`. Platform control kinds (`system.kill`, `energy.*`, `session.invite`, `beekeeper.ready`, `task.status`) are live-only.

- Spec: [015-deferred-event-emit](../specs/015-deferred-event-emit.md)
- Canonical: [CLI](../guide/cli.md) (`paseka event emit`, `pending`, `flush`), [Prompt templates](../guide/prompt-templates.md), [Interactive sessions](../guide/interactive-sessions.md)

## 2026-07 — Hard trace kill (`system.kill`)

Beekeepers can emergency-stop a trace without waiting for honey to drain. `paseka kill --trace <id>` publishes `SIGNAL/system.kill`: marks the trace `killed`, cancels non-terminal tasks, blocks new AFK dispatch, and cancels in-flight adapter processes. `energy.add` after kill does not redispatch.

- Spec: [013-system-kill](../specs/013-system-kill.md)
- Canonical: [Task ledger](../reference/task-ledger.md), [CLI](../guide/cli.md) (`paseka kill`)

## 2026-07 — `paseka inspect usage`

Operators can dump LLM token usage from the terminal without opening Queen Console. `paseka inspect usage --trace <id>` prints a trace aggregate summed from runs that report `usage` on `result.json`; `--agent` scopes to one run.

- Canonical: [CLI](../guide/cli.md) (`paseka inspect usage`)

## 2026-07 — Colony `defaults.default_bee`

Colonies can set the default AFK task role in `.paseka/colony.yaml` (`defaults.default_bee`). Reactor `task.ready` dispatch, `paseka task create` / `task start` / `task retry`, and review helpers resolve empty `task.bee` through this setting (platform fallback `builder`). `paseka init` scaffolds `default_bee: builder`.

- Canonical: [Colony layout](../guide/colony-layout.md), [CLI](../guide/cli.md), [Bee routing](../reference/bee-routing.md)

## 2026-07 — Queen Console tab attention badges

Sessions and Reviews tabs show pending invite and review counts (1–9, then `9+`) with background polling so counts stay fresh while you are on other views.

- Spec: [002-queen-console-mvp](../specs/002-queen-console-mvp.md)
- Canonical: [Queen Console](../guide/queen-console.md) (Sessions / Reviews)

## 2026-07 — Homelab / server container apiary

Operator-facing `docker/dev/` image (Ubuntu 24.04, Go, git, Cursor Agent CLI, prebuilt `paseka`) with compose volumes for colony repo, paseka home, and Cursor config. Default command is Queen Console on `0.0.0.0:8787`; `PASEKA_NATS_URL` reuses a host or LAN JetStream. Guide covers `colony_root` path matching and trusted-network Console exposure.

- Canonical: [Homelab deployment](../guide/homelab-deployment.md), [`docker/dev/`](../../docker/dev/)

## 2026-07 — `PASEKA_NATS_URL` override

Non-empty `PASEKA_NATS_URL` overrides `nats.url` in home `config.yaml` for runtime and CLI — useful for containers, shared LAN JetStream, and multi-environment setups without editing yaml per host.

- Canonical: [CLI](../guide/cli.md) (NATS dependency), [Colony layout](../guide/colony-layout.md), [Homelab deployment](../guide/homelab-deployment.md)

## 2026-07 — Queen Console topology layout persistence

Colony EDA topology node positions persist in browser `localStorage` per colony slug on drag; layout restores on reload. **Reset layout** clears saved positions and re-runs the default layout.

- Spec: [007-colony-eda-topology](../specs/007-colony-eda-topology.md)
- Canonical: [CLI](../guide/cli.md) (`paseka colony topology`), [Colony EDA topology](../specs/007-colony-eda-topology.md) (Console Topology tab)

## 2026-07 — Flight trail summary (`trace.summary`)

Operational `INSIGHT/trace.summary` sets a human Flight Trail description for Queen Console (muted subtitle) and the default merge-commit **body**. Conventional merge **subject** stays HITL (`mergeMessage` / `--merge-message` / default). The sole incomplete non-final AFK work task gets must-emit guidance via `{{.IsLastWorkTask}}`.

- Spec: [012-trace-summary](../specs/012-trace-summary.md)
- Canonical: [INSIGHT kinds](../reference/insight-kinds.md), [Prompt templates](../guide/prompt-templates.md), [CLI](../guide/cli.md) (approve `--summary` vs `--merge-message`)

## 2026-07 — Queen Console honey top-up

Beekeepers can top up a trace honey reserve from Queen Console without switching to CLI or Telegram. The Trace view Energy section exposes `+1` / `+5` / `+12` controls (aligned with Telegram) backed by `POST /api/traces/:traceId/energy/add`.

- Spec: [002-queen-console-mvp](../specs/002-queen-console-mvp.md)
- Canonical: [CLI](../guide/cli.md) (`paseka energy add`), [Telegram gateway](../guide/telegram-gateway.md)

## 2026-07 — Run log artifact rename (`summary.md`)

AFK and interactive runs now persist the human-readable run log as `summary.md` instead of `result.txt`. Success semantics remain on process exit and `INSIGHT/run.summary`; the file is a log only. Template keys (`{{.ResultFile}}`, `$RESULT_FILE`, `PASEKA_RESULT_FILE`) are unchanged — only the basename changes. Runtime still reads legacy `result.txt` when present for adapter summary preference.

- Canonical: [Architecture overview](../architecture/overview.md), [Colony layout](../guide/colony-layout.md)

## 2026-07 — Flight trail title (`trace.title`)

Operational `INSIGHT/trace.title` sets a human Flight Trail name for Queen Console and planner prompts. Runtime resolves `{{.TraceTitle}}` with fallbacks from `feature.requested` and task ledger titles.

- Spec: [011-trace-title](../specs/011-trace-title.md)
- Canonical: [INSIGHT kinds](../reference/insight-kinds.md), [Prompt templates](../guide/prompt-templates.md)

## 2026-07 — Telegram notify modes

`paseka gate telegram` notify policy now supports per-category **`off` / `silent` / `sound`** modes, splits `waiting_review` into `review_required`, `review_final`, and `commit_gate` (AFK defer), and pushes on live **`task.completed`** events (default silent; not reconciled on gate restart).

- Spec: [010-telegram-human-gateway](../specs/010-telegram-human-gateway.md) §8
- Canonical: [Telegram gateway](../guide/telegram-gateway.md)

## 2026-07 — Telegram custom signal commands

`paseka gate telegram` supports `commands.custom` in `telegram.yaml` — configurable slash commands that publish colony `SIGNAL` events (preview + Confirm). Example: `/feature` → `feature.requested` for Scout intake when `paseka run` is active.

- Spec: [010-telegram-human-gateway](../specs/010-telegram-human-gateway.md) §10
- Canonical: [Telegram gateway](../guide/telegram-gateway.md)

## 2026-07 — SIGNAL direct dispatch

Reactor direct dispatch now supports colony `SIGNAL` events (e.g. `feature.requested` → Scout `intake`). Platform SIGNAL kinds (`task.*`, `energy.*`, invite protocol) remain denylisted for direct AFK runs.

- Canonical: [Bee routing](../reference/bee-routing.md) §4 Direct path

## 2026-07 — Prompt text flag `body`

Hard rename of free-text prompt input to avoid collision with ledger `taskId`:

- CLI: `paseka bee run` / `bee chat` / `invite record` use `--body` / `-b` (removed `--task` / `-t` on those commands)
- Queen Console: session launch form label **Task body**; `POST /api/sessions` and run detail JSON use `body` for prompt text
- Unchanged: `--task` on `paseka task *` and `proposal *` (task id); template variable `{{.Task}}`; protocol `session.invite` payload field `task`

- Canonical: [CLI](../guide/cli.md), [Interactive sessions](../guide/interactive-sessions.md), [Prompt templates](../guide/prompt-templates.md)

## 2026-07 — Telegram Human Gateway

Async phone triage via `paseka gate telegram`: long-poll Bot API, allowlisted chats, bus notify + reconcile dedup, `/status` `/energy` `/task` `/invites` `/help`, invite HITL (local PTY on accept), and proposal reject / soft-mid approve (final-merge Console/CLI only).

- Spec: [010-telegram-human-gateway](../specs/010-telegram-human-gateway.md)
- Canonical: [Telegram gateway](../guide/telegram-gateway.md)

## 2026-07 — Merge autostash on approve

Final merge on isolated proposal approve autostashes a dirty colony root (including untracked files) and restores afterward.

- Spec: [009-merge-autostash](../specs/009-merge-autostash.md)

## 2026-07 — Code proposal workspaces

Dual proposal paths: `code.proposal.isolated` (worktree + AFK merge gate) and `code.proposal.root` (shared workspace + soft human ack). Alias `code.proposal` → isolated. `paseka doctor` wiring checks.

- Spec: [008-code-proposal-workspaces](../specs/008-code-proposal-workspaces.md)
- Canonical: [Architecture overview](../architecture/overview.md) §2, [Bee routing](../reference/bee-routing.md), [Bee config](../guide/bee-config.md), [Task ledger](../reference/task-ledger.md), [CLI](../guide/cli.md)

Deferred from that work: `proposal_paths` allowlist, untracked files in proposal delta, alias removal timeline — see [Backlog](backlog.md).

## Earlier MVP baselines

| Area | Spec | Notes |
| ---- | ---- | ----- |
| Queen Console MVP | [002](../specs/002-queen-console-mvp.md) | `paseka console`, SPA, polling APIs, reviews, sessions |
| Live bees indicator | [004](../specs/004-live-bees-indicator.md) | Header live-agents panel |
| Colony EDA topology | [007](../specs/007-colony-eda-topology.md) | Topology tab + `paseka colony topology` |
| Pi adapter | [001](../specs/001-pi-integration.md) | First-class `adapter: pi` |
| Human gateway invites | [006](../specs/006-human-gateway-invites.md) | `session.invite`, `auto_invites`, `done_when` |
| Feature ideation flow | [005](../specs/005-feature-ideation-flow.md) | Colony reference choreography (classify → grill → breakdown) |
