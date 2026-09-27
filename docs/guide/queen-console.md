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

## Preview the redesign

The Svelte-based Queen Console redesign is available at
`http://127.0.0.1:8787/next/`. It is an in-progress preview; `/` continues to
serve the legacy console until the redesign reaches feature parity and an
explicit cutover. Both UIs use the same root-relative `/api/*` endpoints.

The preview currently ships the shell (top status panel, side menu, theme
switcher), the **Dashboard**, **Traces** (both the list and a trail's own detail
page), **Git**, **System**, **Timeline**, **Topology**, **Runs** (list and
detail), **Tasks** (board and task detail), **Reviews** (queue, proposal, and
merge preview), **Sessions** (list and session detail), **Worktrees**,
**Bees**, and **Settings**. Every route under `/next/` is now migrated; a route
that still renders a "migration pending" card is a bug. The legacy console at
`/` remains the default until the redesign reaches an explicit cutover.

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

Every list under `/next/` — traces, runs, tasks, reviews, sessions, bees,
worktrees, branches, adapters, processes — keeps its **filter and its page in
the URL**, as `?q=` and `?page=`. Two things follow. A filtered list is a link
you can paste to somebody, and **Back** from a trail, run, or task detail
returns you to the page of the list you left rather than to page one. The filter
box is where you start; there is nothing to press to share the view. Typing
narrows the rows, so it also takes you back to the first page, and a `?page=`
that a poll has since emptied corrects itself in the address bar.

On Traces this is worth spelling out, because that list pages twice. **Load
older trails** pulls another fifty from the server and keeps them for the visit
— that loaded window is not in the URL, so a shared link describes the view
*inside* the rows you have, not which fifty you pulled.

Two keys work on every page and are not in the `g` chord map, because they are
not destinations: **Escape** goes back to the list a detail belongs to — from a
trail, a run, a task, a proposal, a session, or the merge preview, to its list —
and **`/`** jumps to the list's filter and selects what is in it, so the next
keystroke replaces it. Neither fires while you are typing, and **Escape** leaves
a dialog alone so it closes rather than navigating out from under you.

### Dashboard

Shows runtime health, live bees, recent Flight Trails, failed runs, pending
reviews, honey pressure, and other items that need beekeeper attention.

Header plaques (Hive runtime, Live bees, Host, Git) and Reviews/Sessions tab
badges stay current over one Server-Sent Event stream (`GET /api/chrome/stream`).
The System and Git tabs still poll their full JSON APIs while those tabs are
open — in the preview, a route's store starts its poll on mount and stops it on
unmount, so an idle console holds none. Git status never fetches remotes on a
timer.

The **Host**, **Live bees**, and **Git** labels are links. Host and Git open
their own pages; **Live bees** opens **Runs**, or **Sessions** when only
interactive bees are live — a bee is not addressable, so the plaque leads to
whichever of the two is holding it.

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

On `/next/timeline` the feed starts unscoped and the filter panel stays folded
until something is in it, so the events own the screen. A trail's *Open
timeline* button links to `/next/timeline?trace=<id>`, which arrives with the
panel open and the feed already scoped — bookmarkable, unlike the legacy tab
switch that set the filter in memory. Six filters (trace, task, bee, contract,
payload kind, severity) apply on **Apply** rather than as you type, and
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

The list shows the most recent trails and stops at what the server sends; **Load
older trails** pulls the next page when there is more history. A trail with
something to say — running now, or carrying failures — is badged in the State
column; a settled trail leaves it empty, so the eye lands on the rows that need
you. Opening a trail goes to its own page (`/next/traces/<traceId>`), so the URL
can be shared and the browser back button works. From there: **+1 / +5 / +12**
top up the honey reserve, **View** on a comb file opens it in a dialog, and
**Open timeline** jumps to the event feed for that trail.

### Reviews

Lists tasks in `waiting_review`. Final isolated gates can open a merge preview
with per-file diffs and line-anchored comments.

- **Approve** merges an isolated final-gate worktree when `defaults.delivery` is
  `local_merge` (default).
- When delivery is `pull_request`, the same control is **Open PR** / **Update PR**
  (title, body, draft). The gate stays `waiting_review` until the host reports
  merged. See [pull-request delivery](pull-request-delivery.md).
- **Request changes** writes `review-comments.md` to the trail comb, publishes
  `INSIGHT/human.feedback`, and plans rework on the same trail.
- Plain reject records feedback without merging.

CLI equivalents are `paseka proposal approve` and `paseka proposal reject`.
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
and holds the send until you confirm you have re-read the diff — your notes are
kept rather than dropped, because their line numbers may no longer mean the same
thing but the thinking behind them is still yours. On the proposal page, **Approve** (or **Open PR** / **Update PR**)
asks for an approval summary and, for a final gate, an optional commit message;
plain **Reject** publishes your feedback without starting a rework, which is the
difference the two buttons make.


### Tasks

The **task board** groups every task the colony's ledger knows into one column
per lifecycle status, in the order the pipeline runs — ready, running, waiting
review, planned, blocked, failed, completed. A card says what it is, who takes
it, how many runs it has made, what it waits on, and whether the ledger says it
is startable or retryable right now. Each column scrolls on its own, because a
colony is mostly completed history and the work should not be buried under it. A
status the colony has no task in is left out rather than shown empty.

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
own words when it says no. A task waiting on you carries **Approve** and
**Request changes** here rather than only on the review queue: the pull-request
fields sit under Approve and appear only for a task delivered as a pull request,
and Request changes is one box — your feedback becomes the rework task's body.
Creating a task from the CLI is `paseka task create`; the other task CLI
equivalents are `paseka task list`, `paseka task show`, `paseka task start`, and
`paseka task retry`.

### Runs

**Runs** shows AFK and HITL run records, summaries, status, usage when the
adapter reports it, and the provider session id when available.

On `/next/runs` the list is compact: the state is badged, the trail and the run
are both links, and the adapter is searchable rather than a column. Opening a run
gives its metadata, the adapter's own summary, the task body, and the events it
recorded. **Those events are a readable list** — contract, payload kind, and what
was said — with the raw event folded away on each row, where the legacy console
printed `[TYPE #seq] {json}`. An event whose payload carries no summary still says
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

On `/next/topology` the same graph is drawn, with bees in a left column and
event kinds wrapped beside them. Solid edges are subscriptions, dashed are
declared publications, dotted are colony invites, and a faded edge is a
subscription the bee never wrote down — an empty `subscribes` means any
`task.ready` reaches it. Each contract keeps one hue, taken from the console
theme, so the graph follows a theme switch instead of staying dark. Drag a node
to rearrange; the shape is remembered per colony, and **Reset layout** puts it
back. The page re-reads only when you press **Refresh**, because the projection
comes from committed config and changes when a commit lands rather than on a
clock. **Copy Mermaid** and the Mermaid block below the graph are the same data
as text — the graph is a picture, so this is the form you can paste into a pull
request or read aloud.

### System

Shows the OS view of the Console process: host identity, CPU, memory, uptime,
load, colony disk, and a capped process list. In a container this is the
container's PID namespace; no Docker API is queried. Read it, do not act on it:
there are no kill, nice, or signal controls, and a process name is a hint that
work is happening rather than a ledger of what the colony started.

On `/next/system` the metrics lead the page, and the Host plaque in the topbar
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

### Git

Shows colony root status relative to `origin`, managed worktrees, and leftover
merged branches. Fetch only updates remote-tracking refs; Pull is
fast-forward-only; Push is explicit and never uses `--force`. This tab does
**not** push worktree branches or open pull requests — that is Reviews publish
when `defaults.delivery` is `pull_request`.

On `/next/git` the same three actions sit as one compact group in the page
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
**Worktrees** tab; the Git page keeps a count and a link to it. The preview
re-reads the clone after every action and otherwise refreshes on a 15-second
timer — slower than the legacy tab because each read shells out to `git` several
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

The rest of the page is committed configuration, so it is read once and does not
refresh on a timer; press **Refresh** after a commit adds or changes a bee, or
after a run lands and you want to see it. Nothing here needs the Hive Runtime.

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
  whether a bot token resolves, and the push mode of each of the seven
  categories.
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
| Review a proposal | Reviews | `paseka proposal approve\|reject` |
| Add honey | Trace detail | `paseka energy add --trace ...` |
| Stop AFK work on a trail | Trace/task context | `paseka kill --trace ...` |
| Work with an interactive bee | Sessions | `paseka bee chat`, `paseka session ...` |
| Inspect routing | Topology | `paseka colony topology` |
| See what a colony can do, and what ran last | Bees | `paseka bee run`, `paseka status` |
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
  lists every bee including those, and says so on the row.
- **Remote Git state looks stale:** use explicit Fetch. Polling `/api/git`
  and the header chrome stream intentionally do not contact the remote.
- **Header plaques freeze behind a reverse proxy:** disable response buffering
  for `/api/chrome/stream` (the Console already sends `X-Accel-Buffering: no`).
- **Topology, Sessions, or merge preview load as HTML / fail as JS after
  `go install`:** third-party Console files are served from `/lib/...`
  (not `/vendor/...`). Module zips omit any `/vendor/` path, so an older
  binary can embed the SPA without those bundles. Rebuild or reinstall a
  version that ships `static/lib`.

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
