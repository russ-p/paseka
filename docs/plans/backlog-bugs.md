# Backlog — bugs

Known defects, deferred — found, recorded, and not fixed in the change that surfaced them.
This list should trend to zero: read it as a whole rather than search it, and every entry
is a commitment to fix it or retire it explicitly.
Deferred follow-ups and assumptions live in [Backlog](backlog.md); ideas in
[Ideas](backlog-ideas.md) — the `Kind` field decides the file.
Shipped work: [Changelog](changelog.md).

## Deferred work

#### Two `trace.summary` events for one trail step

- **Kind:** bug
- **Source:** trail analysis (`trace-01a10d602db8d6f6`)
- **Summary:** JetStream held bus sequence 10 and 11 as consecutive `INSIGHT/trace.summary` with the same synthetic agent id `agent-<traceId>`, between `task.ready` and `task.status` — a summary that looks hand-published twice, since no Go code in the tree mints that agent id. `paseka verify <traceId>` now settles whether the two are one event seen twice or two events, by identity; it cannot yet compare their payloads, which is what "duplicate" actually turns on.
- **Why deferred:** Unverified and harmless on its own: `runs.ResolveTraceSummary` takes the last non-empty summary, so a duplicate changes nothing downstream.
- **Revisit when:** A trail shows a summary that disagrees with what actually happened, or trail tooling needs to diff two bus payloads rather than match them.

#### An analysis admission left `running` after a crash is never reconciled

- **Kind:** bug
- **Source:** [043b-trail-metrics-and-work-completion](../specs/043b-trail-metrics-and-work-completion.md) § 4.3; `internal/runtime/dispatch_direct.go` (`prepareAnalysisAdmission`)
- **Summary:** A configured analysis reaction writes `.paseka/runs/<traceId>/analysis/<admissionId>.json` as `running` before the adapter starts and finishes it as `completed`/`failed` afterwards. If the reactor dies mid-run, the record stays `running` forever; restart treats it as already taken and never redelivers, so the analysis silently never happens. Only an explicit operator action recovers it, and there is no surface that names the stuck admission. Add a startup reconciliation that marks an abandoned `running` admission `failed` with a diagnostic (or records it as retryable), so the trail does not hide an analysis it never finished.
- **Why deferred:** The at-most-once choice is deliberate — redelivering on every restart would duplicate expensive analysis — and the failure mode loses one retrospective, not work. Changing it needs a policy for when a `running` record is stale and a test that pins restart behavior.
- **Revisit when:** An operator expects an analysis that never appears, or reactor restarts mid-analysis become routine (homelab redeploy, laptop sleep).

#### `receiver.md` calls `rtk`, a machine-local binary

- **Kind:** bug
- **Source:** trail audit of `trace-01a10d602db8d6f6`
- **Summary:** `.paseka/prompts/receiver.md:11` instructs every Receiver Bee to read the staged diff with `rtk git diff --staged`, while line 13 tells it to commit with plain `git commit`. `rtk` is not a Paseka dependency: it appears nowhere in `docs/`, `bee-config.md`, `cli.md`, or any bee YAML, and on this machine it is `~/.local/bin/rtk`, a third-party LLM-output proxy. On any machine without it the Receiver cannot read what it is about to commit — and it is the bee that writes the merge commit.
- **Why deferred:** Fixing it is a one-word prompt edit, but it belongs with whoever next touches the receiver prompt; nothing about the trail audit depends on it.
- **Revisit when:** The receiver prompt is edited for any other reason.

#### Barrier-held direct work is not resumed at startup

- **Kind:** bug
- **Source:** [044a-direct-pending-dispatch](../specs/044a-direct-pending-dispatch.md) review, follow-up to its startup resume
- **Summary:** A direct reaction held by a pending configuration change is deliberately excluded from the energy drain (`resumableDirectDispatch`) and is released only by `tryActivateConfig` → `resumeBarrierHeldDispatches`, which runs when an event reaches that trail. `RecoverPendingConfigs` on startup walks only manifests that still have a pending change, so when the change is activated or cancelled while the reactor is down, the held entry has no drainer: it stays `pending` until the trail's next event, and a trail that goes quiet never runs it. The entry stays visible in the ledger, so nothing is lost silently, but a restart alone does not resume it.
- **Why deferred:** Not part of 044a's direct-energy scope, and the barrier path already resumes on the next event, so it is a quiet-trail startup gap rather than a lost-ship defect. Closing it needs a startup enumeration of barrier-held work plus a decision on what a cancelled change does with it after a restart.
- **Revisit when:** A resolved configuration change on a quiet trail leaves a guard or receiver unrun, or reactor restarts become routine (homelab redeploy, laptop sleep) while operators apply configuration mid-trail.

#### `energy.resumed` can be lost after the pause episode closes

- **Kind:** bug
- **Source:** Trail audit `trace-01a1982659da17c2`, finding 7; guard `01a19857812aacf6` `summary.md` (task 004); [044b](../specs/044b-standing-supersession-and-energy-observations.md) §6.6
- **Summary:** Observed on the branch: a stipend applies, supersedes the previous tick's work, and clears `EnergyPause` in the same ledger mutation (`internal/taskledger/apply.go`, `SignalEnergyStipend` → `CloseEnergyPause`). The runtime publishes `energy.resumed` from the returned `PauseClose` after that write (`internal/runtime/reactor.go` → `publishResumeObservation`). A crash in the gap leaves no episode to settle — startup republishes only an open unpublished pause and closes only an episode that is still open — so the resume is **lost**, not misattributed. The audit's original claim (source `energy.add` instead of `energy.stipend`, with exactly one card still delivered) was never reproduced and is recorded here as **unverified**; it does not match the reducer. Test-first design: (1) a memory-ledger characterization test applies a stipend and stops before the publish, then runs startup settlement and asserts the current outcome for the episode; (2) a durable pending-resume record on `TraceSnapshot` (episode id, source, remaining/budget, pending before/after, dispatched, abandoned, claimed flag) is written in the same mutation that closes the episode; (3) startup claims the record through the ledger CAS and publishes it once with the original source, clearing the claim after the send, and keeps the existing at-most-once trade-off rather than adding a retry loop.
- **Why deferred:** Low severity: one notification can be lost, the episode still closes, and no work is stranded. The existing design deliberately trades at most one notification for no duplicates; changing it needs a durable outbox and a test that pins the actual behavior, and this is a decision record, not permission to start a ledger-shape change before 044b merges.
- **Revisit when:** A restart between a stipend or `energy.add` close and its resume publish is observed, or an operator relies on the resume card's source/`abandoned` count to account for a standing tick's work; or the platform moves from at-most-once to at-least-once notifications.
