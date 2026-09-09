# Spec 029: Queen Console Chrome Stream

## Status

**(Implemented)**
Queen Console header plaques and Reviews/Sessions tab badges over `GET /api/chrome/stream` (SSE). Domain Timeline stream remains deferred in [002](./002-queen-console-mvp.md).

## Problem Statement

Queen Console keeps four header plaques and two tab badges fresh by pulling several JSON APIs on timers: runtime, live bees, host, and git about every three seconds, plus invites and the review queue about every five seconds. The Beekeeper sees one chrome strip, but the browser issues many round-trips that can skip a tick when a previous batch is still in flight. Git status work (worktrees, leftover branches) rides the same header timer even when the Git tab is closed. Reverse proxies and extra Console tabs multiply that cost.

They do not want a live domain Timeline over the bus, a second WebSocket next to the PTY relay, or deletion of the existing REST GETs. They want one long-lived HTTP stream that the server uses to push the always-visible chrome.

## Solution

Add `GET /api/chrome/stream` as Server-Sent Events. A process-wide hub ticks while at least one browser is subscribed, builds a compact chrome envelope (runtime, live bees, host plaque without the process table, git plaque without worktree/branch lists, review and invite counts), and fans the same frame out to subscribers. REST GET handlers stay for tab Refresh and for System/Git/Reviews/Sessions lists. Those lists poll only while their tab is active. The Git stream never fetches remotes. A hidden document closes the EventSource; if SSE is unavailable the SPA falls back to the old header REST poll.

## User Stories

1. As a Beekeeper on any Queen Console tab, I want Hive runtime, Live bees, Host, and Git plaques to stay current, so that I do not switch tabs just to see whether the hive or the clone moved.
2. As a Beekeeper, I want those plaques to update from one server stream, so that the browser is not issuing four header GETs every few seconds.
3. As a Beekeeper, I want Reviews and Sessions tab badges to update from the same stream, so that pending reviews and invites are visible without opening those tabs.
4. As a Beekeeper, I want the first chrome frame as soon as the page connects, so that the header is not empty until the next timer.
5. As a Beekeeper with Hive runtime stopped, I want Host and Git plaques still streamed, so that chrome does not depend on `paseka run`.
6. As a Beekeeper with NATS down, I want chrome still streamed, so that this surface does not require the bus.
7. As a Beekeeper, I want Live bees meaning unchanged from [004](./004-live-bees-indicator.md), so that AFK and session PIDs remain the liveness signal.
8. As a Beekeeper, I want Host plaque CPU and RAM on the stream, so that load is visible from Dashboard or Traces as today ([022](./022-console-system-info.md)).
9. As a Beekeeper, I do not want the process table on the chrome stream, so that a closed System tab does not scan top-N `/proc` for every subscriber.
10. As a Beekeeper on the System tab, I want the process table still refreshing, so that compilers and test runners appear without waiting on a manual Refresh.
11. As a Beekeeper, I want Git plaque ahead/behind/dirty/synced on the stream, so that unpublished merges stay visible from any tab ([023](./023-console-git.md)).
12. As a Beekeeper, I do not want worktrees, leftover branches, or unpublished commit lists on every chrome frame, so that closed Git tabs stay cheap.
13. As a Beekeeper on the Git tab, I want the full clone snapshot still refreshing, so that prune and leftover delete stay accurate.
14. As a Beekeeper, I want chrome git status never to `git fetch`, so that Gitea is not hammered from a header timer.
15. As a Beekeeper with two Console tabs open, I want one console process to share a hub, so that git status is not multiplied per EventSource.
16. As a Beekeeper, I want git plaque rebuilt less often than Host CPU, so that expensive git work is not tied to CPU deltas.
17. As a Beekeeper, I want a failed git or host collector to leave the other plaques intact, so that one error does not blank the whole header.
18. As a Beekeeper, I want an error signal on a failed block, so that a dead snapshot still looks idle/failed next to a healthy runtime.
19. As a Beekeeper who starts or stops Hive runtime from the header, I want the stream (or a one-shot REST refresh) to show the new status, so that Start/Stop still feel immediate.
20. As a Beekeeper on Reviews, I want the review queue list to keep polling while that tab is open, so that new waiting_review tasks appear without relying on a global invite/review pull.
21. As a Beekeeper on Sessions, I want pending invites to keep refreshing while that tab is open, so that Accept/Reject is not stale.
22. As a Beekeeper on Dashboard, I want dashboard polling unchanged, so that chrome streaming does not rewrite colony snapshot cards.
23. As a Beekeeper on Traces or Tasks, I want those tab polls unchanged, so that this spec stays chrome-only.
24. As a Beekeeper attached to a PTY session, I want the existing WebSocket relay unchanged, so that chrome SSE is not mixed with terminal bytes.
25. As a Beekeeper who switches away from the browser tab, I want the EventSource closed, so that a background laptop tab does not keep the hub ticking alone.
26. As a Beekeeper who returns to the tab, I want the stream to reconnect and paint a fresh frame, so that the header is not frozen.
27. As a Beekeeper whose browser has no EventSource, I want header REST polling as fallback, so that chrome still works.
28. As a Beekeeper whose stream errors repeatedly, I want fallback to header REST polling, so that a broken proxy does not leave plaques stuck.
29. As a Beekeeper behind nginx or Caddy, I want the stream unbuffered (including an explicit disable-buffering header), so that frames arrive without sitting in a proxy.
30. As a Beekeeper, I want periodic SSE comments as keep-alives, so that idle proxies do not drop a quiet connection.
31. As a Beekeeper who disconnects, I want the hub to stop ticking when no subscribers remain, so that an idle `paseka console` is not scanning git forever.
32. As a Beekeeper, I want existing `GET /api/runtime`, `/api/agents`, `/api/system`, `/api/git`, `/api/review-queue`, and `/api/invites` kept, so that Refresh buttons and tests do not depend on SSE.
33. As a Beekeeper, I want Timeline still on demand via `GET /api/events`, so that chrome is not a domain event firehose.
34. As a Beekeeper, I want chrome independent of JetStream consumers, so that filesystem and machine-local projections remain the source of truth.
35. As a Beekeeper on first Host paint, I want memory and load even if CPU percent is not yet a delta, so that the plaque is not empty ([022](./022-console-system-info.md)).
36. As a Beekeeper, I want Host CPU percent still derived from `/proc/stat` deltas shared with the System tab sampler, so that plaque and table stay one clock.
37. As a Beekeeper on non-Linux, I want identity-only Host on the stream, so that Console does not crash.
38. As a Beekeeper with no origin, I want Git plaque `no origin` behavior unchanged, so that a laptop-only repo is not an error.
39. As a platform contributor, I want Host and Git to remain console-local projections, so that hiveview is not forced to own OS or git snapshots.
40. As a platform contributor, I want static SPA tests to assert the stream URL and EventSource usage, so that chrome cannot silently revert to four header GETs only.
41. As a Beekeeper reading the Console guide, I want a note that the header is streamed and that Git GET still does not fetch, so that homelab proxy buffering is diagnosable.
42. As a Beekeeper, I want badge counts of pending reviews and pending invites, so that the Sessions badge still means invites, not live session processes.
43. As a Beekeeper, I want schemaVersion on the chrome envelope, so that a later field add is explicit.
44. As a Beekeeper with a slow EventSource, I want the hub to drop extra frames rather than block git collection, so that one stuck tab does not stall others.
45. As a Beekeeper after Fetch/Push/Pull on the Git tab, I want the tab snapshot from REST, so that mutations are not waiting on the slower git plaque cadence.

## Implementation Decisions

### 1. Product scope

- Queen Console HTTP + embedded SPA only.
- Chrome means always-visible header plaques and Reviews/Sessions tab badges.
- Do not implement [002](./002-queen-console-mvp.md) `GET /api/events/stream` in this spec.
- Do not move Host/Git into hiveview; those stay console-local.

### 2. Transport

- `GET /api/chrome/stream` Server-Sent Events.
- Event name `chrome`; `data` is one JSON object.
- Headers: `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `X-Accel-Buffering: no`.
- Comment heartbeat about every 15 seconds (`: ping`).
- Cancel on request context; require `http.Flusher`.
- No EventSource auth headers (Console remains unauthenticated localhost/trusted network).

### 3. Envelope

- `schemaVersion` is `1`.
- `runtime`: existing runtime view.
- `agents`: existing live-bees view.
- `host`: host snapshot without `processes`.
- `git`: plaque fields only (`branch`, SHAs, dirty, default branch, origin, ahead/behind, last fetch age, `note`). Omit worktrees, branches, unpublished lists.
- `attention.reviews` and `attention.sessions`: pending review count and pending invite count.
- Per-block error strings (`runtimeError`, `agentsError`, `hostError`, `gitError`, `attentionError`) so one collector failure does not drop the frame.

### 4. Hub

- One hub per Queen Console process.
- Ticker about every 3 seconds only while subscriber count is greater than zero.
- Rebuild runtime, agents, host plaque, and attention counts every tick.
- Rebuild git plaque at most about every 9 seconds; cache last good plaque; never fetch remotes.
- Marshal canonical JSON; skip fan-out when identical to the last published frame; always send the current frame once on subscribe.
- Slow subscribers: non-blocking send, drop if the per-connection buffer is full.

### 5. Host sampler

- Plaque collection updates host CPU totals/idle without wiping per-process tick maps used by the System tab.
- Full `GET /api/system` still includes the capped process table.

### 6. SPA

- Prefer `EventSource` for chrome; stop the global four-GET header interval and the global 5s invite/review pull on the happy path.
- Merge host/git plaques into existing state without clearing System process rows or Git tab arrays when those keys are absent.
- Drive tab badges from `attention` counts.
- `visibilitychange`: close the EventSource when hidden; reopen when visible.
- Fallback: missing EventSource or repeated stream errors → restore ~3s header REST poll (`/api/runtime`, `/api/agents`, `/api/system`, `/api/git`).
- While System is active: poll `GET /api/system` about every 3 seconds.
- While Git is active: poll `GET /api/git` about every 3 seconds.
- While Reviews is active: poll the review queue about every 5 seconds.
- While Sessions is active: poll invites on an interval (not globally).
- Dashboard, traces, tasks, selected session transcript, and run events keep their existing polls.
- PTY stays on WebSocket.

### 7. REST stability

- Existing GET/POST chrome-adjacent APIs remain for tests, Refresh, and mutations.
- Stream is additive.

## Testing Decisions

Good tests assert HTTP and SPA contracts, not goroutine internals of the hub.

- Console handler tests (same `httptest` + `NewServer` style as runtime/git/system): first SSE frame is `event: chrome` with JSON `schemaVersion` 1; host has no process table; git plaque has no worktrees/branches; request cancel ends the handler; a second concurrent client also receives a frame.
- Host unit tests: plaque collection omits processes; full collection still ranks processes; CPU sampler totals still advance.
- Embedded SPA string contracts: EventSource and `/api/chrome/stream` present; tab REST paths for `/api/system` and `/api/git` remain; header fallback helpers may still mention the old GETs.

Prior art: `internal/console` API handler tests, `static_test.go` panel contracts, [022](./022-console-system-info.md) / [023](./023-console-git.md) static needles.

## Out of Scope

- Domain bus / Timeline SSE or WebSocket (`/api/events/stream`).
- Replacing dashboard, traces, tasks, merge-diff, topology, or PTY polling/streaming.
- NATS or filesystem watchers as the chrome trigger (hub remains a timer).
- Dropping REST GETs.
- Authentication on EventSource.
- Multi-console or multi-host chrome.
- Telegram or `paseka status --watch`.

## Further Notes

Related: [002](./002-queen-console-mvp.md) MVP baseline, [004](./004-live-bees-indicator.md), [022](./022-console-system-info.md), [023](./023-console-git.md). Durable operator text belongs in the Queen Console guide after ship.
