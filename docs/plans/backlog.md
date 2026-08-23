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

#### Task retry with edit

- **Kind:** follow-up
- **Source:** [003-hive-evals](../specs/003-hive-evals.md); planning (task ledger / Console)
- **Summary:** Allow changing bee, intent, body, or sector when retrying a failed task (CLI flags or Console form). Today `paseka task retry` and Console Retry reuse the ledger snapshot as-is.
- **Why deferred:** Snapshot reuse was enough for MVP retry; edit-on-retry needs UX and ledger rules. Eval colony has no case that needs a corrected retry.
- **Revisit when:** Operators or eval cases need corrected retries without creating a new task.

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
- **Source:** planning (`system.kill` / hard kill); [013-system-kill](../specs/013-system-kill.md)
- **Summary:** After a hard kill (or late-stage avalanche), good early work may already live in `.paseka/worktrees/<traceId>/`. Need an operator path to start a **new** `traceId` that reuses that worktree (or grafts its branch/diff) instead of discarding progress and redoing from `HEAD`.
- **Why deferred:** Orthogonal to kill protocol itself (`paseka kill` shipped); needs worktree registry + trace bootstrap design (identity, honey budget, which tasks/events to carry).
- **Revisit when:** Operators hit “early stages were fine, last stage blew up” without a clean continue path.

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

### Releases

#### Windows release builds

- **Kind:** idea
- **Source:** planning (GoReleaser / cross-compile)
- **Summary:** Make `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./cmd/paseka` work (Unix-only PTY/HITL today: e.g. `SIGWINCH`, review `Setsid`), then add `windows` to GoReleaser `builds.goos` so release assets include `.exe` archives.
- **Why deferred:** Pipeline already ships linux/darwin from Ubuntu with `CGO_ENABLED=0`; Windows needs build tags/stubs before CI/release changes.
- **Revisit when:** Local/CI Windows cross-build succeeds and release should publish `windows/amd64` (optionally `windows/arm64`).

### NATS / hive substrate

Laptop onboarding still requires an external JetStream (`nats.url` in home config, or `PASEKA_NATS_URL`). Homelab already assumes a shared server ([homelab deployment](../guide/homelab-deployment.md)).

#### Optional embedded NATS

- **Kind:** idea
- **Source:** planning (laptop `paseka init` / `paseka run` DX)
- **Summary:** Opt-in in-process NATS+JetStream for a single-machine hive so first `paseka run` does not need Docker or a separately installed broker. Keep `PASEKA_NATS_URL` / home `nats.url` as the override for shared LAN/homelab JetStream. Store embedded server files under machine-local state (not `.paseka/` in the repo). Choreography contracts stay unchanged.
- **Why deferred:** Orthogonal to bee contracts; needs a written split (embedded vs external, bind address, data dir, one-consumer-per-prefix, shutdown) before changing `paseka init` defaults.
- **Revisit when:** Operators bounce on NATS as the first-run blocker, or we want a zero-dependency laptop path without weakening the homelab “bring your own JetStream” story.

## Assumptions and gotchas

### Eval colony

Wiring the side eval colony (`paseka-eval-colony`) and `runner/run-case.sh` against real NATS + `paseka run`. See [003-hive-evals](../specs/003-hive-evals.md).

Tier B in that repo already covers cases `01`–`14` (scripted loop, energy block, first-pass, inject-mutation, kill, human reject, cue hotfix, deferred emit, kill no-redispatch, signal direct, ready-before-plan, artifact scan-flush, deferred-artifact skip, artifact handoff). `runner/reset.sh` purges with `--reseed-energy` (skipped for cue ingress so the cue’s `energy_budget` seeds honey). `check_replay_event_chain` scores `case.yaml` `expect_event_chain` against `paseka replay`. Remaining eval work is Tier C (live LLM) and optional platform helpers (`paseka eval`, seeded `agentId`) — owned by spec 003, not this backlog.

- **Always pass `-C` to `paseka` from runner scripts** — resolving from cwd alone can target the wrong git repo (e.g. parent `paseka` platform) and `purge` the wrong colony. Use `paseka … -C "${EVAL_ROOT}"`.
- **Worktrees are created from `HEAD`, not the working tree** — seed code (`go.mod`, `pkg/`, …) must be **committed** before a trace worktree is created. Do not gitignore materialized seed files at the colony root. Also relevant outside eval; see [008](../specs/008-code-proposal-workspaces.md).
- **Script bees run from the worktree checkout** — `scripts/*.sh` and bee YAML come from git `HEAD`. Uncommitted script changes are invisible inside `.paseka/worktrees/<traceId>/`.
- **`paseka event emit` from script bees needs `-C "$PASEKA_COLONY_ROOT"`** — when cwd is the worktree, emit without `-C` fails colony/home resolution. Guard/receiver scripts must pass colony root explicitly.
- **`paseka event emit` can fail after a successful bus publish** — if audit log append to `.paseka/runs/<traceId>/<agentId>/events.ndjson` fails, emit exits non-zero and the adapter run is marked failed. Normal adapter runs have a run dir; ad-hoc manual emits need a matching run dir.
- **Fixed `trace` + JetStream state accumulates** — reusing case traces leaves ledger KV, depleted honey, and replay history. Stop `paseka run` first, then `paseka purge --bus --trace <case-trace> --reseed-energy` (eval `reset.sh` does this; cue-ingress cases omit `--reseed-energy`). See [CLI](../guide/cli.md) § `paseka purge`.
- **Only one `paseka run` consumer per colony subject prefix** — a second reactor logs `consumer is already bound to a subscription`. Stop the previous runtime before `run-case.sh` starts another.
- **Builder rework is async** — `verification.failed` → builder fix-up via direct dispatch can continue while the task is `waiting_review` or after `completed` (e.g. honey exhausted). Allow time for the guard→builder loop; treat `blocked` as terminal when honey runs out.
- **Oracle scope** — `go test ./...` in the worktree also picks up packages under `cases/…/expect/`. Prefer a narrow path (e.g. `go test ./pkg/...`) in `case.yaml` `oracle.command` and in script-guard bees.
