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
open. Git status never fetches remotes on a timer.

### Traces and Timeline

**Traces** groups tasks, runs, insights, usage, and artifacts by `traceId`.
**Timeline** exposes the event stream for diagnosing routing and handoffs.
Use `paseka replay <traceId>` for the CLI equivalent.

Trails bound to a standing Forage Cue carry a **standing** badge, and their
honey reads `remaining / stipend` — see [Forage Cues](cues.md).

### Tasks

Lists task-ledger state and task details. Common CLI equivalents are
`paseka task list`, `paseka task show`, `paseka task start`, and
`paseka task retry`.

### Reviews

Lists tasks in `waiting_review`. Final isolated gates can open a merge preview
with per-file diffs and line-anchored comments.

- **Approve** merges an isolated final-gate worktree when applicable.
- **Request changes** writes `review-comments.md` to the trail comb, publishes
  `INSIGHT/human.feedback`, and plans rework on the same trail.
- Plain reject records feedback without merging.

CLI equivalents are `paseka proposal approve` and `paseka proposal reject`.
Review approval does not push the default branch to its remote.

### Sessions and Runs

**Sessions** launches, attaches to, stops, and inspects interactive bees. Finished Cursor HITL sessions with a stored provider id offer **Resume** (optional continue line) — a new Paseka session in the same Cursor chat, without `create-chat`.
**Runs** shows AFK and HITL run records, summaries, status, usage when the
adapter reports it, and the provider session id when available.

CLI equivalents are `paseka bee chat`, `paseka session resume`, `paseka session ...`, and
`paseka inspect usage`.

### Topology

Visualizes bee subscriptions, publications, dispatch mode, and automatic
invites from colony YAML. Generate the same graph as Mermaid with
`paseka colony topology`.

### System

Shows the OS view of the Console process: host identity, CPU, memory, uptime,
load, colony disk, and a capped process list. In a container this is the
container's PID namespace; no Docker API is queried.

### Git

Shows colony root status relative to `origin`, managed worktrees, and leftover
merged branches. Fetch only updates remote-tracking refs; Pull is
fast-forward-only; Push is explicit and never uses `--force`.

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
- [Homelab deployment](homelab-deployment.md)
