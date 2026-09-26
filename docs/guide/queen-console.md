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
page), **Git**, **System**, **Timeline**, and **Topology**. Every other route
under `/next/` renders a "migration pending" card that links back to the legacy
console, so use `/` for Tasks, Reviews, Sessions, Bees, Worktrees, and Runs for
now.

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

### Dashboard

Shows runtime health, live bees, recent Flight Trails, failed runs, pending
reviews, honey pressure, and other items that need beekeeper attention.

Header plaques (Hive runtime, Live bees, Host, Git) and Reviews/Sessions tab
badges stay current over one Server-Sent Event stream (`GET /api/chrome/stream`).
The System and Git tabs still poll their full JSON APIs while those tabs are
open — in the preview, a route's store starts its poll on mount and stops it on
unmount, so an idle console holds none. Git status never fetches remotes on a
timer.

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

The feed does not refresh itself — it is recorded history, and a timer that
moved rows under you while you were reading one would be worse than useless.
Press **Refresh** when you want the newest events. Every row's *Raw event*
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

### Tasks

Lists task-ledger state and task details. Common CLI equivalents are
`paseka task list`, `paseka task show`, `paseka task start`, and
`paseka task retry`.

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

### Sessions and Runs

**Sessions** launches, attaches to, stops, and inspects interactive bees. Finished **Cursor or OpenCode** HITL sessions with a stored provider id offer **Resume** (optional continue line) — a new Paseka session in the same provider chat, without `create-chat` / pre-create.
**Runs** shows AFK and HITL run records, summaries, status, usage when the
adapter reports it, and the provider session id when available.

CLI equivalents are `paseka bee chat`, `paseka session resume`, `paseka session ...`, and
`paseka inspect usage`.

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
published. A worktree row links to the trail that owns it, and a branch row
carries one word — `current`, `leftover`, or `merged` — so a settled branch does
not repeat its flags. **Prune orphans** and **Delete N leftovers** remove local
state, so they ask first and the delete names every branch involved; a refusal
(say, a branch a live worktree still holds) is reported per branch instead of
being rounded up to a failure. Without an `origin` remote the three actions are
disabled and the page says why. The preview re-reads the clone after every
action and otherwise refreshes on a 15-second timer — slower than the legacy tab
because each read shells out to `git` several times — and it runs one action at
a time, so a second click is refused rather than queued behind a push. Nothing
here needs the Hive Runtime.

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
| Publish repository changes | Git | regular `git` commands |

## Troubleshooting

- **Dashboard says the runtime is down:** start `paseka run`; the Console does
  not start it automatically.
- **NATS is unreachable:** run `paseka doctor` and verify
  `PASEKA_NATS_URL` or machine-local `nats.url`.
- **A review has no merge diff:** confirm the proposal came from an isolated
  worktree and that the worktree still exists.
- **A session cannot attach:** check `paseka session list`; cross-process PTY
  attachment depends on the active session registry and terminal setup.
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
