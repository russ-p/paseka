# Backlog

Deferred ideas, follow-ups, bugs, and implementation assumptions outside the active change.
Shipped work: [Changelog](changelog.md). Design drafts: [Specs index](specs-index.md).

## Deferred work

### Task ledger

#### `autorun` flag on `task.plan`

- **Kind:** idea
- **Source:** `task.ready` race fix planning
- **Summary:** Single `INSIGHT/task.plan` payload field (e.g. `autorun: true`) that runtime translates into a follow-up `SIGNAL/task.ready` on the bus after plan apply — instead of bees emitting two deferred events.
- **Why deferred:** CLI/cues/Telegram already use plan+ready as two events; deferred FIFO pair matches `task create --autorun` without new protocol surface. Ledger `pendingReady` covers out-of-order emits.
- **Revisit when:** Product wants one-event autostart from bees without relying on prompt discipline, with explicit bus observability for the synthesized ready.

### Energy and honey

MVP shipped per-trace honey (`defaults.energy_budget`, `energy.add` / `energy.consume`, reactor gating, `paseka energy show|add`). Loop protection is energy depletion → `blocked` (`Honey reserve exhausted`). These items need separate design or evidence before expanding the MVP.

#### `confidence` (Pollen Quality)

- **Kind:** idea
- **Source:** [Brief](../idea/brief.md); planning (energyToken)
- **Summary:** Filter or weight events by confidence level alongside honey.
- **Why deferred:** Needs protocol and UX design (event shapes, CLI/Console) beyond the anti-loop MVP.
- **Revisit when:** Product brief item is specified with event shapes and operator surfaces, or eval scenarios require confidence filtering.

#### New trace from interrupted worktree

- **Kind:** follow-up
- **Source:** planning (`system.kill` / hard kill)
- **Summary:** After a hard kill (or late-stage avalanche), good early work may already live in `.paseka/worktrees/<traceId>/`. Need an operator path to start a **new** `traceId` that reuses that worktree (or grafts its branch/diff) instead of discarding progress and redoing from `HEAD`.
- **Why deferred:** Orthogonal to kill protocol itself; needs worktree registry + trace bootstrap design (identity, honey budget, which tasks/events to carry).
- **Revisit when:** `system.kill` ships and operators hit “early stages were fine, last stage blew up” without a clean continue path.

#### Energy gate on `paseka bee run` / `bee chat`

- **Kind:** follow-up
- **Source:** planning (energyToken)
- **Summary:** One-shot `bee run` and `bee chat` bypass the reactor today; only paths through `paseka run` consume honey. Gate standalone invocations the same way.
- **Why deferred:** Requires adapter-layer changes without a running reactor.
- **Revisit when:** Operators need honey accounting for one-shot/interactive launches, or eval/product rules demand it.

#### Per-bee cost multipliers

- **Kind:** idea
- **Source:** planning (energyToken)
- **Summary:** Charge more than flat `1` per adapter dispatch (per-role or per-intent pricing in bee YAML or routing rules).
- **Why deferred:** Extra configuration surface before evidence that flat cost is too coarse.
- **Revisit when:** Operators report false positives/negatives from flat `1`-token cost or traces that block incorrectly.

#### Honey ↔ LLM token billing

- **Kind:** idea
- **Source:** planning (energyToken)
- **Summary:** Optionally relate honey spend to LLM `usage` on AFK `result.json`. Do not price honey from model tokens without a separate design.
- **Why deferred:** Orthogonal to anti-loop honey; mixing billing models needs an explicit decision.
- **Revisit when:** Product wants cost visibility tied to model usage, with a written design.

#### Interactive session usage

- **Kind:** follow-up
- **Source:** planning (energyToken / SessionAdapter)
- **Summary:** Surface Cursor stream-json `usage` from `bee chat` / SessionAdapter (AFK may already persist optional usage on `result.json`).
- **Why deferred:** Out of energy MVP scope; interactive path differs from AFK file IPC.
- **Revisit when:** Console/CLI need session token usage, or billing/observability work starts.

### Queen Console

API fields for energy and merge-diff exist; per-run proposal preview is still thin.

#### Per-run proposal diff in Reviews

- **Kind:** follow-up
- **Source:** [002-queen-console-mvp](../specs/002-queen-console-mvp.md); planning (reviews)
- **Summary:** Side-by-side preview of per-run `MUTATION/code.proposal.isolated` / `code.proposal.root` for `review: required` tasks. Final merge gate preview (`GET /api/traces/:traceId/merge-diff`) already ships.
- **Why deferred:** Final merge gate was enough for MVP; per-run preview is extra UI surface.
- **Revisit when:** Beekeepers need mid-trace proposal diffs without waiting for the merge gate.

### Code proposal workspaces

Leftovers from [008-code-proposal-workspaces](../specs/008-code-proposal-workspaces.md).

#### `proposal_paths` allowlist

- **Kind:** follow-up
- **Source:** [008-code-proposal-workspaces](../specs/008-code-proposal-workspaces.md)
- **Summary:** Restrict which paths may appear in a code proposal.
- **Why deferred:** Not required for dual isolated/root proposal MVP.
- **Revisit when:** Colonies need path policy to limit proposal scope.

#### Untracked files in proposal delta

- **Kind:** follow-up
- **Source:** [008-code-proposal-workspaces](../specs/008-code-proposal-workspaces.md)
- **Summary:** Include or define behavior for untracked files in proposal deltas.
- **Why deferred:** Deferred from 008 ship to keep delta semantics simple.
- **Revisit when:** Real proposals lose important untracked files, or operators ask for explicit rules.

#### Alias removal for bare `code.proposal`

- **Kind:** follow-up
- **Source:** [008-code-proposal-workspaces](../specs/008-code-proposal-workspaces.md)
- **Summary:** Timeline and migration to remove the bare `code.proposal` alias (today → isolated).
- **Why deferred:** Alias keeps older colonies working while `.isolated` / `.root` settle.
- **Revisit when:** Docs and colonies have moved to explicit kinds and the alias is a liability.

### Eval harness

Follow-ups for [003-hive-evals](../specs/003-hive-evals.md) and the side colony `paseka-eval-colony`. Gotchas from standing up Phase 2 are under [Assumptions and gotchas](#assumptions-and-gotchas).

#### Task retry with edit

- **Kind:** follow-up
- **Source:** [003-hive-evals](../specs/003-hive-evals.md); planning (task ledger / Console)
- **Summary:** Allow changing bee, intent, body, or sector when retrying a failed task (CLI flags or Console form). Today `paseka task retry` and Console Retry reuse the ledger snapshot as-is.
- **Why deferred:** Snapshot reuse was enough for MVP retry; edit-on-retry needs UX and ledger rules.
- **Revisit when:** Operators or eval cases need corrected retries without creating a new task.

#### Trace reset helper

- **Status:** shipped as `paseka purge --bus --trace <id> --reseed-energy` (seeds colony `defaults.energy_budget` after bus wipe). For filesystem artifacts, combine with `--runs`, `--worktrees`, and `--state` as needed.
- **Kind:** follow-up (narrowed)
- **Source:** [003-hive-evals](../specs/003-hive-evals.md); planning (eval harness)
- **Summary:** ~~One command to seed energy and clear ledger for a fixed `--trace`~~ Covered by `purge --bus --reseed-energy`; optional dedicated alias or eval-runner wrapper remains polish.
- **Why deferred:** Core wipe+seed path shipped; a friendlier eval helper or custom budget override is optional polish.
- **Revisit when:** Eval runners need a single named command beyond `purge --bus --reseed-energy`.

#### Event-chain scorer in runner

- **Kind:** follow-up
- **Source:** [003-hive-evals](../specs/003-hive-evals.md)
- **Summary:** Assert `case.yaml` `expect_event_chain` against `paseka replay` output (today: oracle + human replay inspection only).
- **Why deferred:** Phase 2 focused on colony wiring and oracle; automated chain scoring is Phase-adjacent polish.
- **Revisit when:** Eval cases rely on event-chain assertions beyond manual replay.

### Flight trail export

`paseka export` today writes a self-contained HTML report from `hiveview` trail data (overview, tasks, runs, event timeline). Split render format from payload depth so agent-friendly exports can grow without tying UI chrome to audit content.

#### Export data scope / richer payload

- **Kind:** idea
- **Source:** planning (trace export / agent analysis)
- **Summary:** Separate flag (e.g. `--scope` / `--include`) controls how much data enters the export, independent of `--format`. Default stays trail-only (no colony configs). Expand payload for analysis: surface existing `usage` and run durations; optional higher scopes may embed bee/colony/cue snapshots for bottleneck and misconfiguration review. Default must remain without configs.
- **Why deferred:** Needs a clear scope ladder and size/privacy trade-offs; not required for a format-only ship.
- **Revisit when:** `--format md` exists (or is shipping) and agent/operator audits need usage, timings, or config context alongside the trail.

### Releases

#### Windows release builds

- **Kind:** idea
- **Source:** planning (GoReleaser / cross-compile)
- **Summary:** Make `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./cmd/paseka` work (Unix-only PTY/HITL today: e.g. `SIGWINCH`, review `Setsid`), then add `windows` to GoReleaser `builds.goos` so release assets include `.exe` archives.
- **Why deferred:** Pipeline already ships linux/darwin from Ubuntu with `CGO_ENABLED=0`; Windows needs build tags/stubs before CI/release changes.
- **Revisit when:** Local/CI Windows cross-build succeeds and release should publish `windows/amd64` (optionally `windows/arm64`).

### Colony configuration

Shareable colony defaults vs machine-local overlays. Bee roles stay in `.paseka/bees/`; home config stays for secrets and this-machine overrides ([colony layout](../guide/colony-layout.md)).

#### Model aliases (`small` / `medium` / `high`)

- **Kind:** idea
- **Source:** planning (bee `params.model` after Cursor grok 4.6 slug churn)
- **Summary:** Colony-owned aliases (committed, e.g. in `colony.yaml`) map stable names like `small`, `medium`, `high` to the real adapter model id. Home config should override that map when possible so one machine can remap without editing the repo. Bees keep using `params.model` (alias or raw vendor id); this must not replace per-bee YAML or `.paseka/bees/<role>.local.yaml`. Goal: change the id passed to the adapter in one place, with a light local remap.
- **Why deferred:** Orthogonal to bumping current bee model slugs; needs a short precedence design (colony map vs home map vs bee/`*.local.yaml` params) before it is a spec.
- **Revisit when:** Vendor model ids churn again, or operators keep editing every `bees/*.yaml` for the same rename.

### Deferred emit and artifacts

General deferred `event emit` buffer ([015](../specs/015-deferred-event-emit.md)) and trail comb protocol ([014](../specs/014-artifacts-protocol.md)).

#### 014 scan ↔ deferred `artifact.written` coexistence

- **Kind:** follow-up
- **Source:** [015-deferred-event-emit](../specs/015-deferred-event-emit.md) (decision §1 / US 8, 20, 21, 23); planning (015 task breakdown)
- **Summary:** When both 014 scan flush and 015 deferred emit ship, skip scan-synthesized `artifact.written` for a run if a deferred `artifact.written` is already pending/flushed for that run; no silent merge of multiple deferred artifact lines (batch only via one event with `artifacts[]`).
- **Why deferred:** First 015 ship is the general defer/flush path; 014 scan flush is not a dependency and may land on a different schedule. Coexistence needs both mechanisms present to verify.
- **Revisit when:** 014 scan flush and 015 deferred emit are both implemented (or one is about to land on top of the other).

## Assumptions and gotchas

### Eval colony

Wiring the side eval colony (`paseka-eval-colony`) and `runner/run-case.sh` against real NATS + `paseka run`. See [003-hive-evals](../specs/003-hive-evals.md).

- **Always pass `-C` to `paseka` from runner scripts** — resolving from cwd alone can target the wrong git repo (e.g. parent `paseka` platform) and `purge` the wrong colony. Use `paseka … -C "${EVAL_ROOT}"`.
- **Worktrees are created from `HEAD`, not the working tree** — seed code (`go.mod`, `pkg/`, …) must be **committed** before a trace worktree is created. Do not gitignore materialized seed files at the colony root. Also relevant outside eval; see [008](../specs/008-code-proposal-workspaces.md).
- **Script bees run from the worktree checkout** — `scripts/*.sh` and bee YAML come from git `HEAD`. Uncommitted script changes are invisible inside `.paseka/worktrees/<traceId>/`.
- **`paseka event emit` from script bees needs `-C "$PASEKA_COLONY_ROOT"`** — when cwd is the worktree, emit without `-C` fails colony/home resolution. Guard/receiver scripts must pass colony root explicitly.
- **`paseka event emit` can fail after a successful bus publish** — if audit log append to `.paseka/runs/<traceId>/<agentId>/events.ndjson` fails, emit exits non-zero and the adapter run is marked failed. Normal adapter runs have a run dir; ad-hoc manual emits need a matching run dir.
- **Fixed `trace` + JetStream state accumulates** — reusing case traces leaves ledger KV, depleted honey, and replay history. Use `paseka purge --bus --trace <case-trace>` (stop `paseka run` first); see [CLI](../guide/cli.md) § `paseka purge`.
- **Only one `paseka run` consumer per colony subject prefix** — a second reactor logs `consumer is already bound to a subscription`. Stop the previous runtime before `run-case.sh` starts another.
- **Builder rework is async** — `verification.failed` → builder fix-up via direct dispatch can continue while the task is `waiting_review` or after `completed` (e.g. honey exhausted). Allow time for the guard→builder loop; treat `blocked` as terminal when honey runs out.
- **Oracle scope** — `go test ./...` in the worktree also picks up packages under `cases/…/expect/`. Prefer a narrow path (e.g. `go test ./pkg/...`) in `case.yaml` `oracle.command` and in script-guard bees.
