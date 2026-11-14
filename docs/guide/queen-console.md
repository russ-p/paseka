# Queen Console

Queen Console is Paseka's local web UI for observing a colony and handling
human-in-the-loop work. It is embedded in the `paseka` binary and reads the
same runtime, task-ledger, session, and filesystem projections as Queen Shell.

## Start the Console

```bash
paseka console
# Open http://127.0.0.1:8787
```

`paseka --profile pi console` uses that overlay for the Console process (topology and bee list show effective adapters). There is no in-Console profile switcher.

Use `--path` / `-C` to select a colony and `--addr` to change the listen
address:

```bash
paseka console -C /path/to/colony --addr 127.0.0.1:8787
```

Queen Console does **not** enforce authentication. Keep it on localhost or a
trusted private network. For a persistent container deployment, see
[Homelab deployment](homelab-deployment.md).

## One console at the root

Queen Console is served at `http://127.0.0.1:8787/`, and every route is a root
path: `/dashboard`, `/traces`, `/traces/<traceId>`, `/timeline`, `/reviews`,
`/settings`, and so on. There is no second console and no preview URL — the
title, the shell, and the startup banner all name the product.

`/next` and every path beneath it **permanently redirect** (308) to the same
path at the root, query string included: an old `/next/traces/<traceId>?q=x`
bookmark lands on `/traces/<traceId>?q=x`, and a browser tab still holding the
retired shell resolves its cached `/next/_app/...` asset requests too. The
redirect is a link-stability promise for addresses people already have, not a
second mount point: there is no expiry and no way to switch back.

The console bundle is **not committed** — it is a build artifact the Go binary
embeds at compile time. A binary built without it answers every console route
with a page saying the bundle was not built and how to build it, and answers a
missing `_app` asset with 404 rather than the app shell, so a source-only
`go install` carries no usable console. See
[Queen Shell CLI reference](cli.md) for the two-step build, or
[Rebuild `paseka` inside the container](homelab-deployment.md#rebuild-paseka-inside-the-container)
for the container.

The console ships the shell (top status panel, side menu, theme switcher), the
**Dashboard**, **Traces** (both the list and a trail's own detail page),
**Git**, **System**, **Timeline**, **Topology**, **Runs** (list and detail),
**Usage**, **Tasks** (board and task detail), **Reviews** (queue, proposal, and
merge preview), **Sessions** (list and session detail), **Worktrees**, **Bees**,
**Skills**, and **Settings**. The JSON API stays root-relative at `/api/*`; it
does not move with the shell.

## What requires the Hive Runtime

The Console process is separate from `paseka run`.

- Filesystem-backed Runs, Sessions, System, Git, and much of trace history are
  available without the reactor.
- Live routing, task dispatch, current JetStream state, honey updates, and
  event-driven review transitions require configured NATS; start
  `paseka run` when the colony should choreograph work.
- Use `paseka status --check` or `paseka doctor` when the UI reports a runtime
  or bus problem.

## Operator tour

Every list — traces, runs, tasks, reviews, sessions, bees, worktrees,
branches, adapters, processes — keeps its **filter in the URL**, as
`?q=`, and most also keep their **page** there as `?page=`. Two things follow. A
filtered list is a link you can paste to somebody, and **Back** from a trail,
run, or task detail returns you to the view of the list you left rather than to
its first page. The filter box is where you start; there is nothing to press to
share the view. Typing narrows the rows, so it also takes you back to the first
page, and a `?page=` that a poll has since emptied corrects itself in the
address bar.

**Traces is the one list with no page of its own.** **Load older trails** pulls
another fifty from the server and adds them to what is already on screen, so the
list grows in one direction and there is nothing to page. Its shareable view is
the filter alone, and no row count is shown: what is loaded is not the history,
and a total would mean counting every trail the colony has kept.

Two keys work on every page and are not in the `g` chord map, because they are
not destinations: **Escape** goes back to the list a detail belongs to — from a
trail, a run, a task, a proposal, a session, or the merge preview, to its list —
and **`/`** jumps to the list's filter and selects what is in it, so the next
keystroke replaces it. Neither fires while you are typing, and **Escape** leaves
a dialog alone so it closes rather than navigating out from under you.

### The menu

On a desktop the menu is a column beside the page, always open, in one of two
states: the full list of routes, or a rail of icons. The button in the menu's
head swaps the two, and the choice is remembered across reloads along with your
theme. Each route has its own icon, and in the rail every entry still carries
its name — hover it to read it — so nothing becomes a guess. The panel is headed
by the app's own mark and the word *Navigate*, because the topbar already says
which console you are looking at.

Under 768px the menu is off the page: the button in the top-left corner opens it
as a panel, choosing a route closes it, and **Escape** closes it and puts the
cursor back on the button. The two states above the breakpoint are the rail and
the full list; the state below it is that panel being open, and the panel always
shows the labels, because a screen of icons is not a menu.

The panel's foot carries the **build stamp**, and it is the link: `0.6.0+67730c4`
in the full list, the bare commit in the rail, where that is all the width holds.
Clicking it opens this build's commit on GitHub — the repository itself when the
build has no commit to show. The href comes from the server rather than the
bundle, so a console built from a fork opens that fork rather than upstream.

Hover it for the unabbreviated sha, the commit date, and whether the build is a
tagged release or a dirty development build; those are the values a bug report
needs. A stamp that has not arrived draws nothing at all — not a dash, and not a
link built from a guess.

`paseka console` prints the same stamp in its startup banner, so a build is named
before the browser ever opens.

### Dashboard

Shows runtime health, live bees, recent Flight Trails, failed runs, pending
reviews, honey pressure, and other items that need beekeeper attention.

Header plaques (Hive runtime, Live bees, Host, Git) and Reviews/Sessions tab
badges stay current over one Server-Sent Event stream (`GET /api/chrome/stream`).
The System and Git tabs still poll their full JSON APIs while those tabs are
open — a route's store starts its poll on mount and stops it on unmount, so an
idle console holds none. Git status never fetches remotes on a timer. The
System tab's **Agent CLIs** block is deliberately outside that poll: it execs
external binaries, so it reads once when you open the folded block and then
only when you press Refresh.

The **Host**, **Live bees**, and **Git** labels are links. Host and Git open
their own pages; **Live bees** opens **Runs**, or **Sessions** when only
interactive bees are live — a bee is not addressable, so the plaque leads to
whichever of the two is holding it.

The **Git** plaque's badge is the colony root against origin — `in sync`, `↑3`,
`↓2`, `no origin`, or `fetch` when there is nothing to compare — so amber means
Fetch, Push or Pull on the Git page would do something. The working tree is not
in it and is not hidden either: a dirty tree reads as the word `dirty` on the
panel's third line, next to how long ago the remote was last read. That is
deliberate, because a dirty root is close to this colony's normal state — a bee
proposing on the root stages its edits and does not commit, and nothing commits
them for it — so a badge that followed the tree would be lit for most of every
session and would look like the reviews and invites badges beside it. Check the
word before starting work; a staged change from a previous run is what a new run
will pick up.

**Run cue** opens the cue dialog. Publishing a cue mints a trail id and puts a
signal on the bus, but **nothing is written to disk until a bee picks that
signal up**, so the trail page waits for up to 30 seconds and says so rather
than reporting a failure. The publish toast carries **Open trail** if you want
to go and watch it; the console never takes you there by itself, because a
standing cue continues a trail you may already be watching. If nothing claims
the cue, the page stops waiting and tells you to check either the id or whether
a bee is running.

### Traces and Timeline

**Traces** groups tasks, runs, insights, usage, and artifacts by `traceId`.
**Timeline** exposes the event stream for diagnosing routing and handoffs.
Use `paseka replay <traceId>` for the CLI equivalent.

On **Timeline** the feed starts unscoped and the filter panel stays folded
until something is in it, so the events own the screen. A trail's *Open
timeline* button links to `/timeline?trace=<id>`, which arrives with the
panel open and the feed already scoped, so a scoped feed is a link you can
bookmark. Six filters (trace, task, bee, contract, payload kind, severity)
apply on **Apply** rather than as you type, and
**Clear N filters** in the header returns to the colony-wide feed. **Load more**
pages strictly older events; if a page fails the button stays, so it is a retry.

The feed does not move on its own — it is recorded history, and a timer that
moved rows under you while you were reading one would be worse than useless. If
you are watching a run, the **Auto-refresh** selector in the header offers
**Every 5s / 10s / 15s / 60s**; a tick re-reads the newest page and replaces the
list rather than prepending, so nothing shifts under you. Polling pauses while
the tab is hidden and re-reads once when you come back. **Refresh** always works
and returns the selector to **Manual**. Every row's *Raw event*
disclosure shows the underlying `protocol.Event`; it costs no request, because
the raw envelope already arrives with the row.

Trails bound to a standing Forage Cue carry a **standing** badge, and their
honey reads `remaining / stipend` — see [Forage Cues](cues.md).

A trail that has stopped admitting dispatch carries a **paused** badge and its
deferred count beside the state, from the same admission projection `paseka
energy show` prints. A paused trail still shows each task's real lifecycle
status; honey is a trail condition and never appears as a task `blocked`.

The list shows the most recent trails and stops at what the server sends; **Load
older trails** pulls the next page when there is more history. A trail with
something to say — running now, carrying open failures, or holding a damaged
outcome record — is badged in the State column: `active` for work in flight,
`1 failure` / `N failures` for failures that no later success of the same task
has superseded, and `record error` when the outcome pair itself is conflicting
or incomplete. A record problem never clears on a later success and never
inflates the count; when both axes exist the count is the badge and `record
error` is a separate detail flag. A settled trail leaves the cell empty, and so
does a trail whose failures were all superseded — its detail flags keep a quiet
`resolved failures` hint, so the history is demoted rather than erased. The
badge is a server-side projection shared by this console and Telegram; the
historical `hasFailures` flag still exists for export and inspect but no
longer decides what the badge says. The closing/merge stage and `paseka
status` attention are separate answers and are unchanged. Opening a trail goes
to its own page (`/traces/<traceId>`), so the URL can be shared and the
browser back button works. From there: **+1 / +5 / +12**
top up the honey reserve, **View** on a comb file opens it in a dialog,
**Open timeline** jumps to the event feed for that trail, and **Run bee** starts
a headless run *on this trail* — the same form as the one on the **Bees** page,
with the trail filled in, so continuing a trail's work needs no id typed by
hand. Clear the field for a fresh trail instead. Each comb file's row
carries its size on disk, and a file the dialog will not render — too large, or
not text — says so *and* says how large it is, so you can tell a file worth
raising a limit for from one that is simply not a page of text.

### Reviews

Lists tasks in `waiting_review`. Final isolated gates can open a merge preview
with per-file diffs and line-anchored comments.

- **Approve** merges an isolated final-gate worktree when `defaults.delivery` is
  `local_merge` (default).
- When delivery is `pull_request`, the same control is **Open PR** / **Update PR**
  (title, body, draft). The gate stays `waiting_review` until the host reports
  merged. See [pull-request delivery](pull-request-delivery.md).
- **Request changes** (final gate, on the merge preview) writes `review-comments.md` to the trail comb, publishes `INSIGHT/human.feedback` with `verdict: negative`, and plans rework on the same trail.
- **Reject** publishes `INSIGHT/human.feedback` with `verdict: negative`. On a mid-trail `review: required` gate the runtime returns that task to `ready` for another pass on the same worktree; on a final gate it records feedback only and the merge gate stays `waiting_review`, so use **Request changes** there to start rework.
- **Discard** closes the task terminally — "not doing this" — with an optional
  reason. It merges nothing, publishes nothing, sends no feedback, and plans no
  rework; the worktree is left in place so the result can be revived later.

CLI equivalents are `paseka proposal approve`, `paseka proposal reject`, and
`paseka proposal discard`.
Review approval does not push the default branch to its remote. The Git tab
never pushes the worktree head; PR publish does.

The **queue** is every task that stopped at a review gate. A final merge gate is
badged apart from a mid-trail review, and each row says what approving will
actually do — merging a local branch, or opening a pull request — because that is
the question a reviewer has not yet asked of it.

Open a proposal for what the bee wrote, what changed, and the decision. **Open
merge preview** goes to a full-screen diff: the changed files on the left with
their per-file counts, the patch itself as numbered lines, and a path filter that
narrows the list without hiding the body — a hidden file would move every line
number a note is attached to. The preview is its own page, so it can be linked to
and Back behaves.

**Unified** shows the patch as one column, the way the server sent it. **Split**
puts the old and new side by side, which is easier for a rewritten block and is
the only way to leave a note on the *old* line of a line that did not change. The
toggle is next to the filter and applies to the whole diff, and it keeps the
filter you typed.

**Click a line to leave a note on it**; click a second line in the same file to
widen the range. Drafts stay in your browser until you send them, and a single
note about one line is submitted without a pointless zero-width range. **Request
changes** sends the notes with your overall summary and starts a rework task; it
is disabled while a rework from an earlier rejection is still in flight.

If the bee pushes while you are reviewing, the panel says so, names both commits,
and holds the send until you have confirmed you have re-read the diff — your notes are
kept rather than dropped, because their line numbers may no longer mean the same
thing but the thinking behind them is still yours. On the proposal page, **Approve** (or **Open PR** / **Update PR**)
asks for an approval summary and, for a final gate, an optional commit message;
plain **Reject** publishes your feedback without starting a rework, which is the
difference the two buttons make — and **Discard** is the third way out, a terminal
close with an optional reason that merges nothing and plans no rework.


### Tasks

The **task board** groups every task the colony's ledger knows into one column
per lifecycle status, in the order the pipeline runs — ready, running, waiting
review, planned, blocked, skipped, failed, completed. A card says what it is, who takes
it, how many runs it has made, what it waits on, and whether the ledger says it
is startable or retryable right now. Each column scrolls on its own, because a
colony is mostly completed history and the work should not be buried under it. A
status the colony has no task in is left out rather than shown empty.

A task deferred by exhausted honey stays in its own column with a quiet
**pending** badge; the paused trail and its queue are shown on the trail, not as
a task status. `skipped` is its own neutral column: terminal work a new standing
allocation superseded, and not a failure, a cancellation, or a completion.

**New task** opens a form from the side. Pick the bee and the intents narrow to
the prompt templates that bee actually declares. Leave the trail id empty for a
new trail, or name a standing one to add the task to it; both ids are generated
server-side otherwise. Each review policy says what it means as you pick it, and
**Start immediately** publishes `task.ready` so a dispatcher picks the task up
without a trip through the board. Creating and starting both need NATS and a
running `paseka run`.

Open a task for its metadata, the body it was handed (folded — reading a task is
about what it did, not what it was given), the bee's own summary, and its linked
runs, each linking on to the run's own page. **Start** and **Retry** appear only
when the server says the task is eligible, and report the ledger's refusal in its
own words when it says no. A task waiting on you carries **Approve**, **Request
changes**, and **Discard** here rather than only on the review queue: the
pull-request fields sit under Approve and appear only for a task delivered as a
pull request, Request changes is one box — your feedback becomes the rework
task's body — and Discard is one question with an optional reason, closing the
task without feedback or rework.
Creating a task from the CLI is `paseka task create`; the other task CLI
equivalents are `paseka task list`, `paseka task show`, `paseka task start`, and
`paseka task retry`.

### Runs

**Runs** shows AFK and HITL run records, summaries, status, usage when the
adapter reports it, and the provider session id when available.

On **Runs** the list is compact: the state is badged, the trail and the run
are both links, and the adapter is searchable rather than a column. Opening a run
gives its metadata, the adapter's own summary, the task body, and the events it
recorded. **Those events are a readable list** — contract, payload kind, and what
was said — with the raw event folded away on each row rather than printed as
`[TYPE #seq] {json}`. An event whose payload carries no summary still says
something, because the row falls back to the payload's own fields and an
`artifact.written` shows the artifact it wrote.

**The chevrons either side of a run's id step through that trail's runs** in start
order — the previous attempt, the next one — which is how you read what a bee did
across retries. They are real links, so a run in a trail can be shared and Back
behaves. A run with no sibling that way leaves its chevron disabled, and a run
older than the recent window says it has no known position rather than pretending
it is alone. The page refreshes every 5 seconds, because a run's state is the one
thing on these pages that moves while you watch. A run's *events* are what it
announced on the bus, not its transcript; the transcript is the task body above
them.

CLI equivalents are `paseka bee chat`, `paseka session resume`, `paseka session ...`, and
`paseka inspect usage`.

### Usage

**Usage** answers "what did the colony spend per day in this range" from the
retained run records. It reads the same projection as **Runs**, so the page and
`paseka inspect usage` cannot disagree about what one execution reported.

The range control offers **7**, **30**, and **90** days and a custom **From** /
**To** pair. Dates are local calendar days in an explicit IANA zone: the page
sends the browser's zone, and the report echoes it. `since` is inclusive and
`until` exclusive, every day in the range is zero-filled, and a run is placed by
its **start** time, so a run that crosses midnight lands entirely on its start
day. The server defaults an omitted range to the last 30 days ending now and
refuses a range longer than 366 days with a plain-text reason.

The headline is **token volume**: input, output, reasoning, cache read, and
cache write, their sum, executions, distinct trails, and coverage. Reasoning is
never folded into output and cache kinds are never folded into input, because
providers differ about what those fields already contain. The daily table below
it is the authoritative reading of the same numbers, including recorded provider
cost where an adapter reported it and the count of executions that did. **Token
counts are labelled as volume, never as money**: the page never multiplies a
token count by a rate, and recorded cost is shown only where the provider itself
recorded one. A day with executions but no reported usage reads **unmeasured**
rather than looking like a quiet day, and a day still holding an in-flight run
reads **incomplete** — named by date under the chart and in the daily table's
measurement column. The coverage area prints what the totals could not see:
executions that reported usage, executions excluded because they have no usable
start time, and run records the reader could not open. A warning names each
unreadable record, a pruned trail is absent rather than a zero-token row, and a
range that begins before the retained record warns that its earlier days are
**unmeasured, not quiet**.

The chart's **Breakdown** selector stacks the same days by adapter, model, Bee,
or execution purpose instead of the token components. All four axes arrive in
the one report, so switching axes redraws the chart from data already on screen
and never issues a second read. The adapter key is the recorded adapter name
with its implementation version beside it when the two differ, so an `opencode`
proxy run and its `opencodev2` implementation stay separate groups; the model
key is the resolved model id with the recorded alias as a secondary label, never
the grouping key. A missing adapter, model, Bee, or purpose is an explicit
**unknown** group, and a legacy record without a purpose is `unknown`, not
`work`. The period breakdown table under the chart ranks every key of the
selected axis by token volume and shows its executions, tokens, share of the
period, and recorded cost with its own reporting count. The chart draws the top
keys that fit the palette and groups the rest as **other**; the table still
lists every key, so nothing is hidden, only the marks are bounded.

A **Top trails** table ranks the period's trails by reported tokens. Each row
names the trail by its recorded title, or by its id when no title exists, and
carries its executions (with the number that reported usage), its component
total, and recorded provider cost with the count that reported it. Only trails
with reported usage are ranked: a trail whose runs reported nothing is a
coverage gap, not a zero-token row. The table carries the top ten and states how
many measured trails sit outside it and what token volume they account for, so
the rows on screen plus that remainder are the period total. The trail name is
the table's only link, and it opens the existing trail detail instead of
repeating that report here.

**Filters** narrow the same records before aggregation. Four pickers offer the adapter, model, Bee, and purpose values present in the current view, and each applied value becomes a **removable chip**: a filter that matches nothing is still a chip with an ×, so no view is a dead end. Values within one axis are alternatives, the axes combine, and the chips read the same key the breakdown does — filtering an adapter by name keeps every implementation of it, and `unknown` is a selectable value rather than a hidden one. A filter never moves the range or the day boundaries: the day table stays zero-filled across the same dates, every section (headline, chart, breakdown, top trails, coverage) counts only matching records, and the report echoes the filter set it applied. When the active filters match nothing, the page says so in its own empty state — distinct from a quiet range — and offers to clear them.

**Drill-down happens in place.** Selecting a day column in the chart narrows the range to that local day, and selecting a row in the period breakdown adds that row's key to the filters; selecting the same day or row again undoes it. Neither leaves the page: the axis, the filters, and the chosen range stay as they were, so the operator follows a suspicion without losing the view they were reading. The narrowed view is a **removable chip** beside the filters (`Day: 2026-09-25`), and removing it restores the range it was narrowed from in one click — no Back press, and the original context is one chip away. The chart's day columns and the table rows are real controls: they are keyboard reachable, their names state the action (`Narrow the range to …`, `Add model filter …`, `Clear model filter …`), and the breakdown row's key button reports `aria-pressed` for a key already applied. The only click that leaves the page is a trail link in **Top trails**.

The range, the drilled day, the selected breakdown axis, and every filter **live in the URL** (`?since=&until=&day=&axis=&adapter=&model=&bee=&purpose=`), written with `replaceState`. `since`/`until` keep the chosen range even while `day` narrows it, so a link carries both the narrowing and the context it was read from. Reload restores the view, a copied link shows the same days and filters, and Back does not walk backwards through the values that were added. Changing a filter, the range, or the day issues one read; changing the breakdown axis reads nothing, because all four axes are already on the report.

This page deliberately has **no poller**: the report walks run directories, so it
reads on mount, on a range or filter change, and when you press **Refresh**. The
server may reuse the report it parsed for a short window (30 seconds), so a
range or filter change does not walk every run directory twice; the page names
the scan behind the numbers as **observed** with the report's `observedAt`, so a
reused answer is explicit rather than hidden. **Refresh** is the one read that
bypasses that window, scans again, and can show a trail that finished since the
last read; the cache is in process only and nothing is written to disk. A failed
refresh keeps the last good report on screen with its error beside it. The page
gives five degraded states distinct copy instead of one generic message: no
executions in the range, executions that reported no usage, a range older than
the retained record, a range longer than the 366-day cap, and a failed read.

The JSON endpoint is `GET /api/usage?since=YYYY-MM-DD&until=YYYY-MM-DD&timezone=IANA/Zone`,
with each filter axis repeated once per value (`&adapter=opencode&adapter=cursor`).
`&refresh=1` is the refresh control's path: it bypasses the short-lived scan
cache and returns a report with a new `observedAt`.
It is read-only: it changes no file, publishes no event, and needs no bus. An
invalid date, an unknown zone, a reversed range, or a range over 366 days is a
`400` with a plain-text reason; a colony with no `.paseka/runs` returns an empty
report rather than an error.

### Sessions

Every interactive session the colony knows about, newest activity first: **Launch
session** opens a form for a bee, an intent drawn from that bee's own prompt
templates, the task, and an optional trail id. **Pending invites** are folded away
when there are none.

Open a session for what it is and what it wrote. A running one gets a **terminal**
you can type into, with **Full screen** for when the surrounding page is in the way
and **Detach** to let go of it without stopping anything. A finished one gets its
**transcript** instead — kept to the most recent lines, following the newest one
only while you are already at the bottom, with a **Latest** button the moment you
reach back into history. **Stop** asks first, then says whether it killed the
process outright or asked it to exit, because those are not the same act.
**Resume** continues the provider's conversation as a *new* session; where it cannot
— an adapter that does not support it, or a provider that never reported a session
to continue — the page says which, rather than showing nothing.

If a session is running in another process (`paseka bee chat` in a terminal), the
console says so and points at `paseka session attach`: it can only relay a terminal
its own process holds. Stop still works, because that is a signal rather than a
socket.

### Topology

Visualizes bee subscriptions, publications, dispatch mode, and automatic
invites from colony YAML. Generate the same graph as Mermaid with
`paseka colony topology`.

On **Topology** the same graph is drawn, with bees in a left column and
event kinds wrapped beside them. Solid edges are subscriptions, dashed are
declared publications, dotted are colony invites, and a faded edge is a
subscription the bee never wrote down — an empty `subscribes` means any
`task.ready` reaches it. Each contract keeps one hue, taken from the console
theme, so the graph follows a theme switch instead of staying dark. Drag a node
to rearrange; the shape is remembered per colony, and **Reset layout** puts it
back. The page re-reads only when you press **Refresh**, because the projection
comes from committed config and changes when a commit lands rather than on a
clock. The Mermaid block below the graph is the same data as text — the graph is
a picture, so this is the form you can paste into a pull request or read aloud.
It starts folded, since the graph is the reason to be on the page, and its copy
button sits in the block's own header.

### System

Shows the OS view of the Console process: host identity, CPU, memory, uptime,
load, colony disk, and a capped process list. In a container this is the
container's PID namespace; no Docker API is queried. Read it, do not act on it:
there are no kill, nice, or signal controls, and a process name is a hint that
work is happening rather than a ledger of what the colony started.

On **System** the metrics lead the page, and the Host plaque in the topbar
links here. Three of them — available memory, the 5 and 15 minute load, and
colony disk — are here precisely because the plaque has no room for them. A
figure the server could not measure leaves its tile out instead of showing a
dash, so a box that is missing memory looks missing rather than broken. The
process table starts folded with the count in its summary line, and a live
adapter's row is badged so you can find it without cross-referencing the Live
bees panel.

Two numbers on this page use different denominators: the CPU tile is the whole
machine and stops at 100%, while a process row is measured against a single
core, so a busy process reads 172%. That is the server's arithmetic, not a
broken table. CPU percent also needs two samples, so it shows a dash with the
reason on the very first poll after a restart. Nothing here needs the Hive
runtime, and the page keeps working when it is stopped.

Below the processes, the **Agent CLIs** block says which `cursor`, `pi`,
`claude`, and `opencode` binaries this box can actually launch. It starts folded
and probes nothing until you open it — the probe runs those four binaries, so
you should only pay for it when you want the answer. The summary line reads
`not probed, 4 adapters` until then, and `2 of 4 found` afterwards. Each row reports the
binary the colony resolved for that adapter, so a `binary:` set in your
machine-local adapter file is what you see, not the loader's default. A CLI that
is not installed reads `not found` with no version and no error, which is a
state rather than a fault: the colony can still declare bees for it. The probe
runs without your adapter credentials, and the server caches the answer until
you press **Refresh** in the block's header — install a CLI, press it, and the
count follows. `script` is not listed, because its binary is whatever each bee's
`command:` names, so there is no single entry to look for.

This page is about the **box**: which OS, which kernel, which pids. Which
**build** sits on it is one question, and the side menu's foot answers it from
every page — `paseka version` answers the same thing from the shell.

### Git

Shows colony root status relative to `origin`, managed worktrees, and leftover
merged branches. Fetch only updates remote-tracking refs; Pull is
fast-forward-only; Push is explicit and never uses `--force`. This tab does
**not** push worktree branches or open pull requests — that is Reviews publish
when `defaults.delivery` is `pull_request`.

On **Git** the same three actions sit as one compact group in the page
header, with **Push** highlighted only while the clone has commits it has not
published. **Delete N leftovers** removes local state, so it asks first and names
every branch involved; a refusal (say, a branch a live worktree still holds) is
reported per branch instead of being rounded up to a failure. Without an
`origin` remote the three actions are disabled and the page says why. A branch
row carries one word — `current`, `leftover`, or `merged` — so a settled branch
does not repeat its flags, and a branch that is merged, is not the default, and
is not held by a worktree also gets a **Delete** in its own row. The sweep only
takes branches named `paseka/*` or a conventional `feature/`, `hotfix/`, or
`fix/`; the per-row button is for the ones it skips, so a single settled branch
does not need a shell. Both open the same confirmation, titled with what they
are about to delete. The worktree table and **Prune orphans** moved to the
**Worktrees** tab; the Git page keeps a count and a link to it. The page
re-reads the clone after every action and otherwise refreshes on a 15-second
timer — slower than a rapid poll because each read shells out to `git` several
times — and it runs one action at a time, so a second click is refused rather
than queued behind a push. Nothing here needs the Hive Runtime.

### Worktrees

Lists the colony's isolated worktrees — one per trail that mutates code, each a
checkout under `.paseka/worktrees/` holding that trail's branch. A row shows the
branch, the trail it belongs to, whether its working tree is `dirty` or `clean`,
the pull request it opened, and the base SHA it was cut from; the trail id links
to the trail, and a worktree with no trail (an unregistered checkout) stays
plain text rather than becoming a dead link.

**Prune orphans** lives here rather than on the Git tab, because it is the
cleanup that reconciles this list: it drops checkouts under `.paseka/worktrees`
that no trail claims any more and unregisters the ones whose directory is
already gone. **Branches are kept** — a prune is not a branch delete — and the
dialog says so before anything is removed. The result reports what was
reconciled by name, so a sweep that found nothing reads as "No orphan
worktrees" rather than as silence.

The list is rebuilt on every poll, and a row disappears on its own once its
checkout is gone: the server reports the checkouts that exist now, so a registry
entry pointing at a deleted directory is not shown as a broken row — press
**Prune orphans** to reconcile it. Nothing here needs the Hive Runtime.

### Bees

Lists **every** bee in the colony, which is the difference from the two dropdowns
the launch forms use: those offer only bees an interactive session can start, so
a `script` bee is absent from them and present here. A row shows the role, the
adapter, the intent vocabulary its prompt templates declare, its `sector`, where
it works — an isolated `worktree` or the `colony root` — how many adapter
processes it holds live, and its most recent run with that run's state badged and
linked to the run's own page.

Two things on this page are worth knowing because they are easy to misread:

- **A blank Last run cell means the bee has never run**, not that the console
  failed to find one. The server works it out by reading the whole run history
  rather than the recent-runs page the Runs tab shows, so a bee that last ran
  weeks ago in a busy colony is still reported correctly. A bee that has run and
  *failed* says so in red — that is the health signal the page is for.
- **A live count is momentary.** It is joined from the same status stream the
  topbar's Live bees panel reads, so the two cannot disagree, and it needs no
  poll of its own. A **script** bee is counted the same way even though it can
  never be launched as a session.

The header carries **Run bee**, next to **Refresh**: one headless run of a bee
you pick, the browser equivalent of
`paseka bee run <role> --body "…"`. The form asks which bee — every one in the
roster, including the **script** rows no session can start — and then offers that
bee's intent, the task, and an optional trail id, plus **Advanced: write the
prompt myself** for a prompt sent verbatim, skipping the template. For a **script**
bee the task is optional, because it runs the command from its own YAML and a
task is only the prompt it is handed.

Three things about it are worth knowing:

- **It does not need `paseka run`.** The console dispatches the adapter itself,
  so a run starts with the Hive Runtime stopped. This is why it is not a cue or a
  task: `Run cue` and `New task` both publish an event for the runtime to pick
  up, and neither does anything while it is down. Honey is not spent on a run
  started this way — the same as `paseka bee run`; only work dispatched through
  the runtime is metered.
- **The toast names the trail, because the answer arrives before the agent
  does.** The console replies as soon as the run is under way, with no agent id
  and no output: the run lasts as long as the agent takes. **Open trail** takes
  you to the trail it joins, where the page waits for it to appear, and the run
  itself is watched under **Runs**. The roster is not refreshed for you — press
  **Refresh** once the run has landed and you want to see it in Last run.
- **A refusal is a refusal, not a run that quietly dies.** Naming a bee that does
  not exist, pressing Run with no task for a bee that reads a prompt, and running
  a bee with nothing configured to render the task through are all answered in
  the form. A failure *after* that — an adapter binary missing from `PATH`, a
  deleted template file — can only reach the terminal running
  `paseka console`.

The rest of the page is committed configuration, so it is read once and does not
refresh on a timer; press **Refresh** after a commit adds or changes a bee, or
after a run lands and you want to see it. Nothing else here needs the Hive
Runtime.

### Skills

Shows the colony skill store and the trails' skill proposals together. The
inventory lists every skill with its state — `ok`, `invalid` with the rule it
broke, or `disabled` with the Beekeeper's reason — the roles whose selection
carries it, its description, and its provenance (`hand-written`, `proposed`, or
`nuc`). The roles are the server's resolved gate, so a role that selects `*` or a
pattern is named against every skill it actually receives.

The proposals pane reads the trails' decision data: each row carries its source
trail, the skill, when it was proposed, and a state of `pending`, `installed`,
`refused` with the reason, `trialled` with the destination trail, or `missing`
when the bundle is gone. Press **View** to read the proposal's `SKILL.md` and the
reason it was proposed. A missing bundle or an unreadable trail is reported on
the page rather than hidden.

A pending or trialled proposal can be decided here, as a deliberate act:

- **Approve** shows the validated install plan first — the skill, its source
  trail, the destination (`store`) and target directory, every file the install
  would write, and the working-tree consequence (new untracked paths, and that
  committing is still yours). Nothing is written until **Install skill** is
  pressed. The action calls the same atomic, hash-verified batch path as
  `paseka skills approve`, so a plan cannot disagree with the shell.
- **Refuse** asks for an optional reason and says in the dialog that the refusal
  is terminal for that exact bundle: it cannot be approved or trialled
  afterwards. Confirming records one `refused` decision beside the proposal.
- **Cancel** returns to the review and changes nothing.
- A failed action shows the server's reason and leaves the store, ledger, and
  decision files untouched. Only one decision runs at a time: a second click
  while one is in flight is refused as busy rather than queued.
- A successful action answers with the refreshed inventory, so the row and the
  store update together without pressing **Refresh**.
- No console route accepts a skill body, and no action writes a commit.
  Trialling a proposal on one trail (`approve --into`), and `disable` and
  `remove`, remain shell verbs ([skills guide](skills.md#16-reviewing-proposals)).

### Settings

Shows what the colony is actually configured with, and **where each value came
from**. That second half is the reason the page exists: several settings are
decided by something other than the file you would edit, and an operator who
changes the file and watches nothing happen needs to be told why.

- **Transport** — the NATS URL and subject prefix in force.
- **Adapters** — one row per adapter with its binary, the environment variable it
  reads its API key from, whether that variable resolves, and whether the value
  came from `adapters/<name>.yaml` or was inferred from a default.
- **Human gateway** — whether the Telegram gate is configured and switched on,
  whether a bot token resolves, and one row per notify category for its push
  mode.
- **Colony** — the slug, the checkout root, the selected profile, and the
  terminal a session attaches with.
- **Appearance** — the theme picker, the one setting the console keeps itself.

Read the three things on this page carefully:

- **A value in brackets was not read from a file.** `(the default)` means the code
  supplied it because nothing wrote it; `$PASEKA_NATS_URL` means an environment
  variable is deciding it. A **warning** appears when an environment variable
  outranks the file, because editing the file then changes nothing until the
  process is started differently.
- **The console never receives a secret.** An adapter row shows the *name* of the
  variable it reads and whether that variable currently resolves, and the gate
  reports `botTokenSet` rather than the token. This is the same rule the adapters
  follow at runtime, so a key never enters the colony config at all.
- **The gate is reported whether or not it works.** A missing `telegram.yaml` and
  `enabled: false` are both configurations, not failures, so the page says which
  one you have instead of showing a read error.

The page is **read-only** apart from the theme, and that is a scope decision
rather than a missing feature: letting the console write configuration means
giving it a merge-not-clobber path onto YAML that nothing outside `paseka init`
currently writes, which is a platform change and not a route. The page says it is
read-only rather than offering a control that would quietly do nothing.

To change a setting today, edit `~/.config/paseka/<slug>/` or `.paseka/` and
restart the console. The page therefore reads once and waits for **Refresh**
rather than polling. Nothing here needs the Hive Runtime.

## Common operator actions

| Goal | Console | Queen Shell |
| ---- | ------- | ----------- |
| Check colony health | Dashboard | `paseka status` |
| Diagnose NATS or wiring | Runtime notices | `paseka doctor` |
| Review a proposal | Reviews | `paseka proposal approve\|reject\|discard` |
| Add honey | Trace detail | `paseka energy add --trace ...` |
| Stop AFK work on a trail | Trace/task context | `paseka kill --trace ...` |
| Work with an interactive bee | Sessions | `paseka bee chat`, `paseka session ...` |
| Inspect routing | Topology | `paseka colony topology` |
| See what a colony can do, and what ran last | Bees | `paseka status` |
| Run one bee headlessly, runtime up or not | Bees (row **Run**) | `paseka bee run <role> --body ...` |
| Publish repository changes | Git | regular `git` commands |
| Clean up isolated checkouts | Worktrees | `paseka prune` |
| See what a setting resolves to, and why | Settings | `paseka doctor` |

## Troubleshooting

- **Dashboard says the runtime is down:** start `paseka run`; the Console does
  not start it automatically.
- **NATS is unreachable:** run `paseka doctor` and verify
  `PASEKA_NATS_URL` or machine-local `nats.url`.
- **A review has no merge diff:** confirm the proposal came from an isolated
  worktree and that the worktree still exists — the Worktrees tab lists the
  checkouts that are still there, and **Prune orphans** reconciles the rest.
- **A session cannot attach:** check `paseka session list`; cross-process PTY
  attachment depends on the active session registry and terminal setup.
- **A bee is missing from a launch dropdown:** expected if its adapter is
  `script` — those cannot be started as an interactive session. The **Bees** tab
  lists every bee including those, says so on the row, and **Run bee** there is
  the only launch a `script` bee has.
- **Run bee says it started and nothing appears:** the console answers before the
  agent does. Watch the **Runs** list, and check the terminal running
  `paseka console` — a failure past the form's own checks (a missing adapter
  binary, a deleted template) is only logged there.
- **Remote Git state looks stale:** use explicit Fetch. Polling `/api/git`
  and the header chrome stream intentionally do not contact the remote.
- **Header plaques freeze behind a reverse proxy:** disable response buffering
  for `/api/chrome/stream` (the Console already sends `X-Accel-Buffering: no`).
- **The console says the bundle was not built:** this binary was compiled
  without the frontend step, which is what `go install` and a bare
  `go build ./cmd/paseka` produce. The page is the whole console in that state:
  every console route answers with it, and requesting an `_app` asset returns
  404 rather than the shell. Run `pnpm --dir web install --frozen-lockfile`,
  `pnpm --dir web build`, then `go build`; release archives and the container
  image already do this, so they need no extra step.
- **A console route 404s a script or stylesheet:** the bundle is missing that
  asset, not hiding it — the server answers a missing file with 404 rather than
  the app shell, so a stale `_app` hash from an older bundle reads as a 404.
  Rebuild the frontend and the binary together.
- **An old `/next/...` bookmark or tab:** it redirects permanently to the same
  root path with its query string intact. A redirect that lands on 404 means
  that route no longer exists, not that the redirect is missing.
- **The frontend build fails in the bundle guard:** the output still carries a
  `/next` base path. `web/scripts/check-console-bundle.sh` rejects it before it
  can ship links under the retired prefix; the SvelteKit base path belongs to
  the root.
- **The version reads `dev` with no commit:** the binary was built outside a git
  checkout, so nothing stamped it. The Docker image is the usual case — its build
  context excludes `.git` — and takes the stamp as build args instead; see
  [homelab deployment](homelab-deployment.md) and `docker/dev/.env.example`.

The implemented API and UI baseline is recorded in
[Spec 002](../specs/002-queen-console-mvp.md). Header chrome streaming is
[Spec 029](../specs/029-console-chrome-stream.md). Durable operator behavior belongs
in this guide; draft Console work remains in the specs index.

## Related docs

- [CLI reference](cli.md)
- [Interactive sessions](interactive-sessions.md)
- [Task ledger](../reference/task-ledger.md)
- [Pull-request delivery](pull-request-delivery.md)
- [Homelab deployment](homelab-deployment.md)
