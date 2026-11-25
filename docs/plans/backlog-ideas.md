# Backlog — ideas

Deferred product and platform ideas — useful, near-term, not part of the change that
produced them. This file is the parking lot; an idea that grows enough for user stories
becomes a spec (promote per the backlog workflow), it is not designed here.
Deferred follow-ups of shipped work and durable caveats live in [Backlog](backlog.md);
deferred defects in [Bugs](backlog-bugs.md) — the `Kind` field decides the file.
Shipped work: [Changelog](changelog.md). Design drafts: [Specs index](specs-index.md).

## Deferred work

### Trail attention state

The [050-trail-attention-state](../specs/050-trail-attention-state.md) projection and every
read surface shipped; the notification-policy question it deliberately left is the idea held
here. The damaged-record semantic edge from the same spec is a follow-up in
[Backlog](backlog.md#trail-attention-state).

#### Resurfacing a resolved failure as a notification is an operator decision

- **Kind:** idea
- **Source:** [050-trail-attention-state](../specs/050-trail-attention-state.md) § Further Notes; implementation trail `trace-01a1b9851c0161f5`
- **Summary:** A healed trail's badge goes quiet, and its detail keeps a plain `resolved failures` hint. Nothing tells the operator that a previously failing task recovered, and the spec deliberately leaves that out: whether a recovery should ever be a push, for which surfaces, and at what threshold is an operator decision rather than a badge rule. Implementing it needs a notification policy (per failure, per trail, first recovery only, quiet mode) and a way to distinguish "resolved since the last notification" from "resolved long ago" — the current `hasFailures`/`openFailures` pair says what is open now, not what changed since a prior poll.
- **Why deferred:** Spec 050 puts notification rules beyond the badge out of scope. The quieter badge is the shipped behavior, and adding a recovery push before anyone asks for it would recreate the noise the change removed.
- **Revisit when:** An operator asks to be told when a trail recovers, or a failed task's recovery is missed because the trail row simply went quiet.

### Channels and ingress

How work enters the colony and how the colony asks its beekeeper. The CLI, Queen Console, and Telegram are the live channels; these items extend the set.

#### `human.ask` — typed ask forms for operator decisions

- **Kind:** idea
- **Source:** planning (HITL / attention surfaces; Archon v0.12–v0.14 attention-queue research)
- **Summary:** Every operator decision today is binary or surface-specific: review gates approve or reject, fork requests approve/reject/cancel, and Telegram renders energy pauses as cards with top-up buttons. A typed ask event (`SIGNAL/human.ask` carrying a form — `approval`, `select`, `multi_select`, or free text — with declared options, a recommendation, and a per-option explanation) plus its answer (`beekeeper.answer` returning `{decision, text}` into prompt context) would generalize these into one mechanism, answerable from any surface (CLI, Console, Telegram) with staleness refusal by ask id. It builds on what already exists — 050's attention projection and `paseka status`'s attention list collect what waits on the beekeeper; this decides the *shape* of the question.
- **Why deferred:** A protocol addition that touches the event contract, every answering surface, and the resume path; the binary gates cover the decisions MVP colonies actually make. Needs its own spec for event shapes and per-surface behavior.
- **Revisit when:** A colony decision cannot be expressed as approve/reject — a guard that wants to ask "rework, skip, or drop" with a recommendation — or the same class of question is answered from Telegram often enough that per-gate UI work starts repeating.

#### Signed webhook ingress as a cue channel

- **Kind:** idea
- **Source:** planning (ingress channels; Archon v0.12 webhook-ingress research)
- **Summary:** A third ingress channel beside the CLI and Telegram: a signed webhook (Standard Webhooks `v1` HMAC, timestamp window, replay dedup by `webhook-id`) received by the console/apiary publishes `cue.ingress` with `source: webhook` and starts the cue-mapped trail — a Gitea/GitHub issue opens an intake trail, a PR event a review trail. The durable invocation identity and replay semantics already live in the cue subsystem; the new work is the HTTP receiver, per-source secrets, and a fail-closed mapping from webhook payload to the cue's declared inputs.
- **Why deferred:** The apiary is a homelab deployment and the Gitea sidecar already pulls on webhook, so nothing is blocked today. Ingress needs a receiving surface, secret management per source, and a payload-mapping design before it can be safe by default.
- **Revisit when:** An operator wants trails started by forge events (issue triage, PR review) without hand-running cues, or a second external system (CI, forms) needs to trigger the colony.

### Colony roles

#### `archivist` bee — audit a finished trail

- **Kind:** idea
- **Source:** planning (trail audit of `trace-01a10d602db8d6f6`)
- **Summary:** A read-only bee that audits a trail the agents have already finished and writes down what should be remembered. Named from the glossary's **Archivist Bee / Knowledge Agent** (`docs/idea/glossary.md:46`), next to Honeycomb and Wax Storage — it builds nothing and repairs nothing, it curates what survives into the archive. Kept distinct from `scout` by direction: scout looks forward at what to build, archivist looks back at what happened and what is owed. Settled shape — `worktree: false` (reads `.paseka/runs/` at colony root, and with no `code.proposal*` in `publishes` the runtime skips auto-mutation, so it cannot trigger a guard in response to itself), **no `subscribes`** (manual launch only, which is also the safety property), `params.model: medium`, publishes exactly one `INSIGHT/context.note` under a `completion_contract` so a silent no-op run fails loudly instead of reading as a clean audit. Two intents: **`task`** — per task, catch work that was deferred and draft it into the backlog; **`chronicle`** — per trail, find Return Flights and Bees Flying in Circles and propose skills. Writes its backlog entries to `{{.ArtifactsDir}}/backlog-draft.md`, never to a backlog file (`docs/plans/backlog*.md`) itself — an LLM rewriting a hand-maintained file unsupervised can drop existing items and will violate the backlog field rules, and `hivewright` is already the only root-editing bee and is scoped to `.paseka/`.
- **Why deferred:** No spec — this adds colony config and prompts, not platform code. `chronicle` ships offline first, with "not measurable without NATS" stated plainly in its report rather than papered over; the NATS-backed version (`paseka replay`, `energy show`) is a later pass. Offline, the collector reads `meta.json` (bee, adapter, profile, startedAt), `status.json` (finishedAt, state), `result.json` (`usage.*`, `summary`) and `request.json` (taskId) — no NATS, no git.
- **Revisit when:** Someone runs the bee by hand and the draft is good enough to paste, or the offline report's blind spots stop being acceptable.

#### A queue bee that drains a durable work list

- **Kind:** idea
- **Source:** planning (refactor-tick standing trail; Archon v0.12 SDLC queues)
- **Summary:** Generalize the refactor-tick pattern (Nurse Bee reads `scripts/refactor-targets.py`, picks one target, dispatches one refactor bloom) into a reusable colony role: a standing bee that claims exactly one item from a durable work list (backlog, advisory report, issue list), drives it to a reviewed proposal, asks only what an operator must decide, and writes the answer back into the source document as a dated ruling. Claims must survive overlapping ticks — the standing-trail supersession machinery is the natural substrate, with the claim recorded on the trail. The complement of the deferred `archivist`/`chronicle` bee: archivist fills the work list, this bee drains it.
- **Why deferred:** Colony config and prompts rather than platform code, but the claim semantics (one item per tick, supersession, write-back into a hand-maintained file such as `docs/plans/backlog.md`) need the same safety argument the archivist entry already makes, and refactor-tick is the only live instance — too specialized to prove the general shape.
- **Revisit when:** A second standing trail wants the claim-one-and-finish pattern (backlog grooming, dependency sweeps), or refactor-tick's target selection is demonstrably reusable as-is.

### Task ledger

#### `autorun` flag on `task.plan`

- **Kind:** idea
- **Source:** `task.ready` race fix planning
- **Summary:** Single `INSIGHT/task.plan` payload field (e.g. `autorun: true`) that runtime translates into a follow-up `SIGNAL/task.ready` on the bus after plan apply — instead of bees emitting two deferred events.
- **Why deferred:** CLI/cues/Telegram already use plan+ready as two events; deferred FIFO pair matches `task create --autorun` without new protocol surface. Ledger `pendingReady` covers out-of-order emits.
- **Revisit when:** Product wants one-event autostart from bees without relying on prompt discipline, with explicit bus observability for the synthesized ready.

#### `paseka doctor` cannot tell a paused trail with pending work from an idle one

- **Kind:** idea
- **Source:** Trail audit `trace-01a1982659da17c2`, guard `01a198494399dd29` (task 003) — “hiveview/diagnose operator surfaces deferred to later staged slice per 044b breakdown”; [044b](../specs/044b-standing-supersession-and-energy-observations.md) §7
- **Summary:** 005 and 006 delivered the `hiveview` half of that deferral — the shared `taskledger.AdmissionOf` projection, trail badges, board counts, `energy show`, and export — and no task on the trail touched a `diagnose` surface, so `paseka doctor` has no finding on the admission axis. A paused trail whose pending queue is waiting on honey therefore reads exactly like a trail that has nothing to run. Add a doctor finding that walks the ledger and reports each paused trail with its `pauseReason`, `pendingCount`, and remaining/budget, distinct from an idle trail with no pending work, reading `taskledger.AdmissionOf` rather than growing a second scheduler. This is the diagnose surface the 044b breakdown named, not a duplicate of `paseka status` attention (which lists paused trails but does not explain one quiet trail) or `energy show` (which needs the trace id).
- **Why deferred:** 003 staged diagnose out of the supersession slice, then 005 and 006 scoped themselves to `hiveview`, CLI, and export. The admission projection they shipped is the missing input, but a doctor finding still needs its own copy and a decision on how a paused trail groups beside the existing stuck-dependency and closing-round findings.
- **Revisit when:** An operator asks `paseka doctor` why a trail is idle, or a support case needs “paused with pending work” distinguished from “nothing planned”.

### Energy and honey

Ideas that would expand the shipped per-trace honey MVP (`defaults.energy_budget`,
`energy.add` / `energy.consume`, pause/resume episodes). Shipped state, follow-ups, and
known defects stay in [Backlog](backlog.md#energy-and-honey).

#### `confidence` (Pollen Quality)

- **Kind:** idea
- **Source:** [Brief](../idea/brief.md); planning (energyToken)
- **Summary:** Filter or weight events by confidence level alongside honey.
- **Why deferred:** Needs protocol and UX design (event shapes, CLI/Console) beyond the anti-loop MVP.
- **Revisit when:** Product brief item is specified with event shapes and operator surfaces, or eval scenarios require confidence filtering.

#### Global concurrent invocation cap

- **Kind:** idea
- **Source:** [042-trail-fork](../specs/042-trail-fork.md) § Deferred for v1 (active-fork cap); planning (runtime concurrency)
- **Summary:** Add a colony-wide limit on concurrent agent invocations rather than counting active trails or forks. Acquire the slot before `gateDispatchEnergy` so queued work does not spend honey, release it on every terminal path including kill and restart, and add a per-trail fairness limit so one long trail cannot occupy the whole colony budget. Adapter-specific or provider limits can layer on top. A future fork-specific cap in spec 042, when added, is a secondary experimental admission/fairness control, not a provider or monetary ceiling. Today the Reactor has an `inflight` map for dedup and kill only; there is no concurrency semaphore.
- **Why deferred:** Orthogonal to spec 042 and cross-cutting: dispatch, task and direct paths, HITL, retry, kill, recovery, and Console/runtime status need one queueing and fairness policy; introducing it inside fork admission would leave ordinary trails uncontrolled.
- **Revisit when:** Concurrent ordinary trails or forks produce provider rate limits, CPU or memory pressure, or an operator needs a predictable queue instead of unbounded parallel dispatch; or spec 042 fork admission is implemented and needs the colony-wide guard underneath it.

#### Fork admission caps and cumulative experiment allowance

- **Kind:** idea
- **Source:** [042-trail-fork](../specs/042-trail-fork.md) § Deferred for v1
- **Summary:** Decide and implement experimental admission limits for forks: an optional colony-wide cap on active forks, an optional per-source fan-out limit, and an optional cumulative allowance across repeated experiments. V1 deliberately enforces none: human approval gates agent-originated requests, and each fork has its own capped honey reserve. When added, an active fork should mean an approved fork trail with a reserved execution slot and unfinished work; pending and terminal forks hold no slot, and a paused fork releases its slot until resume or top-up. The cap is an admission and fairness control, not a provider or monetary ceiling.
- **Why deferred:** There is no v1 need and no data for a default: human approval prevents agent-initiated self-spending, while automatic campaigns and pre-authorized requests are out of scope. A fork cap would not bound provider load, which the colony-wide invocation cap owns instead.
- **Revisit when:** Policy pre-authorizes bounded agent requests, automatic campaigns are enabled, or concurrent experiments produce provider rate limits, worktree pressure, or starvation of ordinary work.

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

#### Auto-pause a trail until a provider rate-limit reset

- **Kind:** idea
- **Source:** planning (adapter failure handling; Archon v0.12 research)
- **Summary:** Classify adapter failures at the dispatch boundary — rate limit with a known reset, transient overload with capped backoff, misconfigured (never retried), auth — and pause the trail until the reset instead of burning retries and honey on a limit the adapter cannot beat. Implement as a new `reason` (e.g. `provider_limit`) on the existing `energy.paused`/`energy.resumed` episodes, so the trail holds no dispatch slot, keeps task lifecycle status, and wakes itself at reset — the episode machinery 044a/044b already built, with honey semantics unchanged. Complements the planned colony-wide invocation cap rather than duplicating it: the cap bounds load, this bounds waste when the provider already refused.
- **Why deferred:** Adapters do not report structured limit/reset information uniformly (Cursor, Pi, OpenCode differ), and a misclassified hard failure would become a silent sleep. Needs a failure-class contract at the adapter boundary first.
- **Revisit when:** A provider rate limit burns real honey through retries on a standing trail, or an adapter gains a structured reset signal worth honoring.

### Queen Console

Console ideas beyond the shipped routes; feature follow-ups stay in [Backlog](backlog.md#queen-console).

#### No mutation queue while NATS is down

- **Kind:** idea
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (user story #12)
- **Summary:** User story #12 asked the console to queue mutations locally when NATS disconnects. The banner half shipped — the topbar names `reconnecting` and then `unavailable`, and the transport icon reads the same fact — and the queue did not, deliberately: a queued approve or delete that fires minutes later is not the action the operator took, and the console's store contract already answers `busy` rather than queueing a second mutation. The open question is narrower than the story was: a *draft* the operator was typing (a review comment, a task form) is worth keeping across a reconnect, and a *committed* mutation is not.
- **Why deferred:** The console has no offline story to preserve, so the cost is a lost draft rather than a lost action, and the page already says the stream is down.
- **Revisit when:** An operator loses real typing to a reconnect, or a hive restart becomes routine enough to matter.

#### No form to create a bee or a worktree

- **Kind:** idea
- **Source:** [035-queen-console-redesign](../specs/035-queen-console-redesign.md) (user story #17)
- **Summary:** User story #17 asked for forms to open in a drawer rather than a column, and named four: new task, new bee, new worktree, settings edits. The task form and the session launch form both shipped and proved the primitive; the new-bee and new-worktree forms never existed. Both write committed colony YAML (`bees/*.yaml`) and the worktree case also creates a git checkout, which is the same class of risk [036-console-config-write](../specs/036-console-config-write.md) refuses for project config — a browser writing a tracked file is a different decision from a browser writing this machine's configuration.
- **Why deferred:** The routes that display both lists shipped read-only, and starting work has two supported paths today: a launch session, and **Run bee** on `/bees` for a headless run.
- **Revisit when:** An operator hand-edits `bees/*.yaml` often enough to want a form for it, or the worktrees route needs a create path for work that has no trail yet.

#### Bee local overlay is invisible and unwritable in Console

- **Kind:** idea
- **Source:** [036-console-config-write](../specs/036-console-config-write.md) (Out of Scope — per-bee editing)
- **Summary:** `.paseka/bees/<role>.local.yaml` is the machine-local overlay (`prompt_template` and `system_template` only, prompt-only by design) that wins over the committed `<role>.yaml` at resolve time, and the console neither reads nor writes it. `BeeView` has no overlay field, so a bee whose prompt is overridden on this machine renders the committed value with no hint that an override exists — the same missing-`source` half that `/settings` reports for configuration values. Reporting it is a per-bee `{value, source}` pair on `GET /api/bees`; writing it is the 036 writer (merge one key, temp+rename, preserve mode) pointed at a two-key gitignored file, which is a smaller surface than a full bee form.
- **Why deferred:** 036 puts per-bee editing out of scope on the rule that the console writes what is this machine's and nothing the repository shares, and it never names `*.local.yaml` — so the overlay was left undecided rather than refused. It sits in the repository directory, only its `.gitignore` entry makes it machine-local, so making it writable needs that distinction argued rather than assumed. Until then the overlay is invisible in the product: `LoadAllBees` skips it, and export `--include bees` and Nuc omit it.
- **Revisit when:** Per-bee or per-cue editing is reconsidered, or an operator is surprised by a prompt that is not the one `/bees` shows.

#### Flow of one trail's activity

- **Kind:** idea
- **Source:** planning (LayerChart graph primitives evaluated for [052-token-usage-overview](../specs/052-token-usage-overview.md))
- **Summary:** A trail view that draws what happened **inside one trail** as a flow rather than a list: each task and the Bee dispatched for it, the events that Bee published, verification failures and returns, closing rounds and analysis, with edge weight taken from observed counts (executions, retries, tokens, honey). LayerChart ships `Sankey`, `Chord`/`Ribbon`, `Link`, and a `Dagre` layout helper, so the shape could be a sankey of `task → bee → event kind → task` for that trail, or a node-link of the ledger's task dependencies plus who produced what. It is neither a colony-wide "who talks to whom" screen nor a Topology replacement: Topology stays on cytoscape with pan/zoom and saved positions, because it answers what is wired, not what happened.
- **Why deferred:** The raw material is already trail-local — the trail event log and the task ledger — but the axes and labels need a question first. Timeline gives chronology, trail detail gives stages, tasks and runs, and the metrics projection gives totals, so a flow view earns its place only when a real trail is unreadable from those: a long rework chain, many Bees on one task, a standing trail's ticks. 052 is about token volume and does not own this.
- **Revisit when:** An operator cannot explain a trail's shape from the timeline and task list — "why did this trail take 40 runs", "which step spawned this Bee" — and the retained event log is shown to carry those edges cheaply.

#### Mermaid fences in artifact previews

- **Kind:** idea
- **Source:** planning (artifact preview; Pele evaluation)
- **Summary:** The artifact modal renders `.md` bodies from the server's goldmark HTML, so a fenced Mermaid block arrives as `<pre><code class="language-mermaid">` and stays source. Hydrate those blocks in the browser with [Pele](https://github.com/obsidianmd/pele), an MIT-licensed Mermaid-syntax renderer that emits themeable SVG, has no runtime dependencies, and is ~230 kB gzip for the full entry point with per-type lazy chunks. No production API or Go change is needed; the existing HTML already carries the block. Keep the source behind a `<details>` fallback, isolate the engine behind a small wrapper, and start with links/images disabled because a comb artifact can contain model-generated text. Exports and other Markdown surfaces would still show the fence as source and are out of scope.
- **Why deferred:** Pele is a young 1.x library (1.13.0 today) and diagram rendering is a reading convenience, not a trail-correctness or review-gate need. It needs an exact version pin and a small security test for the SVG insertion path; neither is large, but neither belongs in the current change.
- **Revisit when:** An operator opens a trail artifact whose diagram is unreadable as Mermaid source, or diagrams are wanted in review notes/exports as well as the artifact modal.

### Pull-request delivery

One delivery idea from [024-pull-request-delivery](../specs/024-pull-request-delivery.md); the
follow-up stays in [Backlog](backlog.md#pull-request-delivery).

#### Auto-detect forge from origin

- **Kind:** idea
- **Source:** [024-pull-request-delivery](../specs/024-pull-request-delivery.md)
- **Summary:** Infer `tea` vs `gh` (or a bundled wrapper) from the `origin` host instead of requiring explicit `forge.command`.
- **Why deferred:** Fail-closed explicit argv avoids guessing the wrong CLI or a missing login; v1 wants a visible home-config choice.
- **Revisit when:** Beekeepers routinely have `tea`/`gh` on PATH and trip over the commented init example.

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

#### Fork marker on shared-bus events

- **Kind:** idea
- **Source:** [042-trail-fork](../specs/042-trail-fork.md) § Consumer decision
- **Summary:** Add an optional NATS message header such as `Paseka-Trail-Kind: fork` and `Paseka-Fork-Of: <sourceTrace>` so an external side-effecting subscriber can filter experimental trails without querying trail lineage. Keep the event envelope and payload unchanged. Persist the header in the trail event record only if replay or audit needs that provenance. V1 has no confirmed external consumers and uses lineage lookup by `traceId` instead.
- **Why deferred:** Headers are not part of the persisted event record and existing subscribers ignore them; adding durable provenance and a cross-language contract costs more than the current unknown risk. Fork events are ordinary trail events by design, with no separate subject prefix.
- **Revisit when:** A concrete external subscriber cannot query lineage but must avoid side effects from fork trails, or broker-level filtering of experimental events becomes a requirement.
