# Spec 022: Queen Console System Info

## Status

**(Implemented)**
Queen Console Host plaque and System tab: `GET /api/system` Linux `/proc` snapshot, graceful non-Linux identity-only degrade.

## Problem Statement

When a Beekeeper runs Queen Console on an apiary (often `paseka console` in Docker on a homelab box), the header already answers whether Hive runtime is up and whether an AFK or interactive bee process is alive. It does not answer whether that bee is currently burning the machine: tests, compilers, and runtimes (`node`, `java`, `go`, `pytest`, and similar) are almost always **child or grandchild processes**, not the adapter PID shown by Live bees.

Operators therefore SSH in or run `docker stats` / `top` to see CPU, RAM, and a process list. That is extra friction for a trusted, single-machine console that already polls the same host.

They do not want Docker APIs, cAdvisor, Prometheus, or a “tests are running” detector. They want the **OS view as seen by the `paseka console` process** (inside a container, that is the container PID namespace).

## Solution

Add a third observe-only header panel (**Host**) showing compact CPU and RAM. Clicking it opens a Queen Console tab (**System**) with cheap identity (hostname, kernel, OS/arch, CPU count, uptime, console PID), load averages, the same memory figures, optional colony-root disk usage, and a capped top-process table.

One read-only JSON API, Linux-first, polled with the existing header timer. Live bees stays the source of truth for adapter/session liveness; System Info does not replace it. Process names are a **hint** that work is happening, not a ledger or verification signal.

## User Stories

1. As a Beekeeper watching Queen Console during an AFK run, I want CPU and RAM in the header, so that I can see load without leaving the current tab.
2. As a Beekeeper, I want those numbers to reflect the machine (or container namespace) where `paseka console` runs, so that homelab Docker matches what I would see with `top` inside that container.
3. As a Beekeeper, I want to click the Host panel and land on a System tab, so that I get the same navigation pattern as Live bees → Runs/Sessions.
4. As a Beekeeper using a keyboard, I want Enter/Space on the Host panel to open System, so that the plaque is as operable as Live bees.
5. As a Beekeeper, I want a **System** tab in the tab list, so that I can open host detail without using the plaque.
6. As a Beekeeper on System, I want hostname and kernel release, so that I can confirm which box or image I am looking at.
7. As a Beekeeper, I want OS and architecture, so that a mixed fleet is identifiable at a glance.
8. As a Beekeeper, I want CPU count, so that load averages are interpretable.
9. As a Beekeeper, I want uptime, so that I can tell a freshly restarted container from a long-lived apiary.
10. As a Beekeeper, I want the PID of the console process, so that I can correlate `paseka console` in a process list.
11. As a Beekeeper, I want load averages (1/5/15), so that I have a stable load signal when instantaneous CPU percent is noisy.
12. As a Beekeeper, I want memory used, total, and available, so that I can see pressure before OOM, not only a used percentage.
13. As a Beekeeper, I want a process table sorted by CPU then RSS, so that `java`/`node` test runners surface when they dominate.
14. As a Beekeeper, I want each process row to show pid, RSS, CPU percent when available, short comm, and a truncated command line, so that I can tell jest from the Cursor agent.
15. As a Beekeeper, I want kernel threads omitted, so that the table is usable.
16. As a Beekeeper, I want a soft cap on process rows (about 25), so that the API and UI stay cheap.
17. As a Beekeeper with live AFK or session bees, I want those PIDs highlighted in the process table when present, so that adapter processes are easy to find without merging the two APIs.
18. As a Beekeeper, I want System Info **not** to claim that tests are running, so that process names remain a hint only.
19. As a Beekeeper, I want Live bees unchanged in meaning, so that a live `agent` PID is still the adapter liveness signal even if children are busy.
20. As a Beekeeper with no bees live but a compiler running, I want the Host plaque and process list still populated, so that host load is independent of hive choreography.
21. As a Beekeeper with Hive runtime stopped, I want System Info still available, so that I can inspect the box before Start.
22. As a Beekeeper with NATS down, I want System Info still available, so that this surface does not depend on the bus.
23. As a Beekeeper, I want header CPU/RAM to refresh on the same ~3s timer as runtime and Live bees, so that one poll loop stays honest.
24. As a Beekeeper on Dashboard or Traces, I want the Host plaque to keep updating, so that I do not have to stay on System to watch load.
25. As a Beekeeper on System, I want the process table to refresh on that same timer (or when the tab is visible), so that the list is not stale after a test starts.
26. As a Beekeeper, I want a Refresh control on System, so that I can force a snapshot without waiting for the timer.
27. As a Beekeeper on the first poll, I want load and memory even if CPU percent is not yet a delta, so that the plaque is not empty on first paint.
28. As a Beekeeper, I want CPU percent derived from `/proc/stat` deltas between polls, so that the number is actual utilization, not a fake instant.
29. As a Beekeeper whose console runs in Docker, I want the process list to be that container’s PID namespace, so that I see bees, `node`, and `java` without the rest of the NAS.
30. As a Beekeeper, I want no Docker Engine / Compose / cAdvisor client, so that System Info works the same on bare metal and in a container.
31. As a Beekeeper, I understand `/proc/meminfo` and `/proc/stat` may describe the **host** when cgroups do not hide them, so that I do not treat RAM as “this container’s limit” unless a later spec reads cgroup files.
32. As a Beekeeper, I want colony-root disk used/total when `statfs` succeeds, so that filling worktrees and `node_modules` is visible without a volume API.
33. As a Beekeeper, I want disk fields omitted or zero with a clear empty state when `statfs` fails, so that a network mount error does not blank the whole tab.
34. As a Beekeeper on non-Linux, I want identity fields that still work (OS/arch, console PID) and empty or unavailable load/process sections, so that Console does not crash.
35. As a Beekeeper, I want `/proc` read failures to yield an error message on System without taking down other tabs, so that a restricted environment is diagnosable.
36. As a Beekeeper, I want no kill, nice, or signal controls, so that this remains observe-only like Live bees MVP.
37. As a Beekeeper, I want command lines truncated, so that secrets in argv are less likely to dominate the page (best-effort, not a redaction product).
38. As a Beekeeper, I want no second Dashboard stat-card required for CPU/RAM, so that the header plaque is the compact signal (Dashboard may ignore this feature in MVP).
39. As a Beekeeper using Telegram `/status` or `paseka status`, I want those surfaces unchanged in this spec, so that CLI/gateway scope does not block Console.
40. As a Beekeeper, I want the Host plaque idle/zero styling when snapshot fails, so that a dead API is visible next to Hive runtime.
41. As a Beekeeper, I want Go runtime version as an optional cheap identity line, so that I can see which toolchain built the binary without a new subsystem.
42. As a Beekeeper with a noisy desktop (console not in Docker), I want top-N by CPU/RSS still usable, so that the same feature helps outside homelab containers.
43. As a platform contributor, I want one GET handler and a small snapshot helper, so that Console HTTP stays the only new product surface.
44. As a platform contributor, I want SPA tests to assert the Host plaque, System tab, and API path exist, so that the tab cannot silently disappear.
45. As a Beekeeper, I want documentation after ship (Console guide / homelab) to say this is the console host view, not a cluster control plane, so that operators do not expect remote multi-host metrics.

## Implementation Decisions

### 1. Product scope

- Queen Console only for MVP.
- Observe-only snapshot of the **OS view of the console process**.
- Do not extend Live bees ([004](./004-live-bees-indicator.md)) payload or liveness rules.
- Do not infer test/build activity as domain events.

### 2. Header: Host panel

- Place a third header panel beside Hive runtime and Live bees.
- UI label: **Host** (API path may use `system`).
- Show CPU percent and memory used/total (human GiB or similar). Optional third muted line: 1-minute load.
- No Start/Stop or other controls.
- Click / keyboard activates **System** tab (not a popover).

### 3. Tab: System

- New tab named **System** (panel title may be **System Info**).
- Single-panel layout in the Topology style: identity + load/memory (+ disk) + process table.
- Refresh button; no graph libraries.
- Highlight process rows whose pid appears in the current Live bees item list (client-side join).

### 4. API: `GET /api/system`

- Read-only JSON; no query parameters required for MVP.
- One response may include both summary (header) and process list (tab). Scanning `/proc` for a capped top-N every ~3s is acceptable on a homelab.
- Soft-cap process array (recommend **25**).
- Truncate each `cmd` (recommend **200** runes).
- Do not require NATS, runtime registry, or colony bees YAML.

Suggested response fields (names may be camelCase JSON as elsewhere in Console):

- Identity: `hostname`, `kernel`, `os`, `arch`, `cpus`, `uptimeSeconds`, `consolePid`, optional `goVersion`
- Load: `load1`, `load5`, `load15`, `cpuPercent` (may be omitted or null on first sample)
- Memory: `memUsedBytes`, `memTotalBytes`, `memAvailableBytes`
- Disk (optional): `diskUsedBytes`, `diskTotalBytes` for colony workspace root
- `processes[]`: `pid`, `rssBytes`, optional `cpuPercent`, `comm`, `cmd`

On partial failure, return HTTP 200 with whatever succeeded plus an `error` string, **or** HTTP 500 only when the snapshot cannot be built at all. Prefer 200 + `error` so the SPA can still show identity. Pick one policy in implementation and test it; do not mix silently.

### 5. Linux collection (no extra product integrations)

- Memory and load: `/proc/meminfo`, `/proc/loadavg` (and/or equivalent stdlib / `x/sys` helpers already in module).
- CPU percent: deltas of `/proc/stat` aggregate CPU between successive handler calls (in-process last sample). First response may omit `cpuPercent`.
- Processes: `/proc/[pid]/{stat,status,cmdline}`; skip kernel threads; rank by sampled CPU then RSS.
- Kernel/hostname: `uname` / hostname syscalls.
- Disk: one `statfs` (or equivalent) on the colony root already known to Console.
- **Do not** call Docker, containerd, cgroup exporters, or cloud metadata APIs.
- **Do not** require cgroup v2 files for MVP. A later spec may add `memory.current` when present.

### 6. Non-Linux

- Handler remains registered.
- Fill `os`, `arch`, `consolePid`, and `goVersion` when possible; leave load, memory, disk, and processes empty or unavailable without panicking.

### 7. Polling

- Extend the existing header poll (~3s) to load `GET /api/system` together with runtime and agents.
- System tab reuses that state; Refresh triggers the same GET.
- No WebSocket/SSE for this feature.

### 8. Security and trust model

- Same as Queen Console: no authentication; trusted network (homelab / VPN).
- Process listing is sensitive; acceptable because Console already runs on the operator’s machine.
- Truncate argv; do not add a redaction engine.

### 9. Explicit non-goals encoded as decisions

- No process-tree walk from bee PIDs in MVP (nice-to-have later).
- No `paseka status` / Telegram fields in this spec.
- No Dashboard duplicate cards required.
- No kill from UI.

## Testing Decisions

- Test **external behavior**: HTTP JSON shape, caps, truncation, non-crash on missing `/proc` pieces, SPA markup/API wiring — not `/proc` parser internals as the only coverage.
- Console API tests: `GET /api/system` returns 200 and required identity keys in CI (Linux builders); process array length ≤ cap; `cmd` truncation if a fixture supplies a long line.
- Prefer a **snapshot helper** with injectable reads or testdata so tests do not depend on the builder’s live `java` processes.
- First-sample `cpuPercent` omitted/zero vs second sample: unit-test the delta logic with two fake stat snapshots.
- Non-Linux: build tags or a stub path so `go test` on all platforms still compiles the handler.
- SPA static tests (same style as Live bees / Topology): Host panel ids, System tab, `setTab('system')`, `api('/api/system')`.
- Prior art: [004](./004-live-bees-indicator.md) header poll + `internal/console` handler tests; Topology tab static assertions.

## Out of Scope

- Docker / Compose / Kubernetes APIs; cAdvisor; Prometheus; node_exporter.
- Cgroup-only “this container’s CPU/RAM” as the primary metric (optional later if files exist).
- GPU, thermals, network interface counters, inode-only alerts.
- Kill, stop, nice, or attach to arbitrary PIDs.
- Mapping processes to `traceId` / `agentId` via PID trees or cgroups.
- Declaring “tests running” as a boolean product state.
- Changing Live bees, Hive runtime Start/Stop, or NATS diagnostics.
- `paseka status`, Telegram `/status`, or CLI process dumps.
- Multi-host / remote metrics (Console is not a cluster control plane).
- WebSocket push, historical charts, or persisted time series.
- Windows/macOS process tables of equal fidelity (graceful degrade only).

## Further Notes

- Homelab mental model: bind-mounted colony + `paseka console` as the container PID 1 (or child of the entrypoint). The process table **is** the interesting set; `/proc/meminfo` may still be the **host** — document that after ship in the homelab guide.
- Related: [002](./002-queen-console-mvp.md) Console baseline, [004](./004-live-bees-indicator.md) Live bees, [homelab deployment](../guide/homelab-deployment.md).
- Follow-ups (not this spec): cgroup memory.current when present; PID-tree rooted at live bee PIDs; optional `paseka status` host line.
- After implementation: set this spec **Implemented**, changelog + Console/homelab docs, and a Related-specs line on 002 if not already added during drafting.
