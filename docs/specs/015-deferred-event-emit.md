# Spec 015: Deferred event emit buffer

## Status

**Implemented.** General deferred `paseka event emit --defer` buffer with per-run `pending.ndjson`, success flush (FIFO, before `run.summary`), `event pending` / `event flush` / `--discard`, and live-only deny-list. **014 MVP does not depend on this buffer** — artifacts use scan flush first; 014/015 coexistence follow-up is in [backlog](../plans/backlog.md).

## Problem Statement

Bees publish domain events through `paseka event emit`, which today goes to the bus immediately. That is correct for live control and for many narrative notes, but harmful when a bee is still mid-work: subscribers and `auto_invites` can wake on half-finished handoffs, burn honey on premature dispatches, and race the still-running producer.

Artifacts ([014](014-artifacts-protocol.md)) need “announce after success.” Other kinds share the same timing problem — bundles of `context.note`, `task.plan`, `spec.ready`, verifications — whenever the Beekeeper wants **one choreography tick at run boundary**, not N live publishes. Building a special flusher per kind does not scale; relying on prompt discipline (“only emit at the end”) is unreliable for LLM bees.

## Solution

Define an optional **deferred emit buffer**: the same CLI validate-and-accept path as live emit, but append to a per-run pending queue instead of JetStream. On successful run or session completion, runtime **flushes** the queue to the bus (and to the usual audit log) in FIFO order. Live emit remains the default for must-be-visible-now events.

Beekeepers can inspect pending events, manually flush after failure/crash, or discard the queue without publishing (`event flush --discard`). Selected platform control kinds are hard-denied for defer so emergency and HITL control paths cannot be delayed by mistake.

## User Stories

1. As a bee, I want to stage a bus event during a run without waking the swarm yet, so that I can finish related files and sibling events first.
2. As a bee, I want one CLI surface for live vs deferred publish (`emit` + `--defer`), so that I do not learn a second event protocol.
3. As a beekeeper, I want deferred events to hit the bus only after the producer completes successfully by default, so that failed runs do not dispatch choreography.
4. As a beekeeper, I want to inspect queued (not yet published) events for a run, so that I can debug what a bee intended before flush.
5. As a bee author of prompt partials, I want clear guidance which kinds should use defer vs live, so that models do not mix modes randomly.
6. As a colony author, I want deferred `task.plan` + notes to flush as a bundle after breakdown finishes, so that ledger and narrative land together after success.
7. As a grilling Drone, I want deferred `spec.ready` until session success flush, so that invite `done_when` does not complete on a mid-session emit.
8. As a bee writing trail comb files under [014](014-artifacts-protocol.md), I want an optional future where I explicitly defer `artifact.written` instead of relying only on directory scan, so that intent is visible in the pending queue.
9. As a beekeeper, I want live emit to keep working unchanged, so that urgent interactive control paths do not wait for run end.
10. As a reactor, I want flush to be ordered and once-per-run, so that I do not see duplicate storms from double flush.
11. As a beekeeper after a crash before flush, I want pending events retained on disk for inspect / manual flush, so that work is not silently lost without a trace.
12. As a beekeeper, I want a manual flush command for a run when policy or crash left a non-empty queue, so that I can recover without re-running the bee.
13. As a beekeeper, I want to discard pending without publishing (`flush --discard`), so that bad staged handoffs do not require wiping the whole run tree.
14. As an implementer, I want validation at defer time (same schema as live), so that bees get fast feedback before exit.
15. As an implementer, I want re-check of file-at-ref on flush, so that missing files fail closed at announcement time.
16. As a Console user (later), I want queued vs published distinguished in UI when both exist, so that “on disk / staged” is not confused with “swarm has the scent.”
17. As a platform, I want to hard-deny defer for selected live-only platform kinds, so that energy/kill/invite control paths cannot be accidentally delayed.
18. As a bee, I want defer to work for HITL chat the same way as AFK: queue during session, flush on successful session end, so that one mental model covers both.
19. As a beekeeper, I want failed runs to leave the pending queue unpublished by default, so that I can decide to manual-flush or discard.
20. As a documentation reader, I want this spec to state that 014 MVP does not depend on defer, so that implementers do not block artifacts on this design.
21. As a colony adopting both 014 and 015 later, I want a documented coexistence rule (scan flush vs explicit deferred `artifact.written`), so that double announcement does not happen.
22. As a bee emitting many deferred events of different kinds, I want FIFO flush without automatic cross-kind coalesce, so that behavior is predictable.
23. As a bee emitting multiple artifact refs, I want batching only via one deferred `artifact.written` with an `artifacts` array (no silent merge), so that batching stays an artifacts concern (014).
24. As a beekeeper, I want energy accounting unchanged for deferred events (charge on flush publish, not on queue), so that queuing alone does not spend honey.
25. As an eval/oracle author, I want deterministic flush timing in tests (success → flush), so that harnesses can assert bus state after run.
26. As a multi-machine future user, I understand pending queues are local to the run directory until flush, so that I do not expect JetStream to show deferred events early.
27. As a bee or tool calling `--defer` outside a run, I want a clear error, so that staged events are never written without an audit boundary.
28. As a beekeeper when mid-flush publish fails, I want already-published events to stay on the bus and remaining pending to stay queued, so that a retry can resume FIFO without inventing false atomicity.

## Implementation Decisions

### 1. Relationship to 014 (artifacts)

- **014 ships first** with runtime **scan + flush** of the trail comb on successful exit. No pending-event file required for MVP artifacts.
- This spec is **Approved** as the generalization contract; implement when the project explicitly schedules it.
- **Coexistence (complement + skip):** keep 014 scan flush enabled. If a pending `artifact.written` exists for the run when scan would run, **skip** scan-synthesized duplicate for that run. Prefer one announcement path per run. Do not require cutover to defer-only artifacts.

### 2. CLI and wire behavior

- Live (unchanged): validate → publish JetStream → append run audit log.
- Deferred: same emit path + **`--defer`**: validate → append per-run pending queue → return `ok` with **`deferred: true`** (or equivalent) so callers know it was not published yet.
- Manual recovery: **`paseka event flush`** bound to a run (`--trace` + `--agent` or equivalent; exact flags at implementation). Publishes pending FIFO using the same flush path as runtime.
- Discard without publish: **`paseka event flush --discard`** clears the pending queue for that run.
- Inspect pending: CLI must show pending count and kinds for a run (under `event` or `inspect`); operators must not rely on reading the raw file by hand.
- Console queued-vs-published UI is **out of scope for the first 015 ship** (follow-up).
- Optional dry-run / validate-only remains orthogonal.
- **`--defer` requires an existing run directory** for the event’s `traceId`/`agentId`. Missing run dir → fail closed (do not create ad-hoc, do not silently fall back to live).

### 3. Pending store

- Per-agent run directory under the trail runs tree (same audit boundary as today’s event log).
- Format: append-only NDJSON of validated event envelopes (or CLI input shape plus normalized ids). Durable across process crash of the bee.
- After each successful publish during flush, remove or mark that line so a retry resumes with remaining items only.
- After full successful flush or `--discard`, pending is empty.
- Double flush on empty pending is a safe no-op.

### 4. When flush runs

| Trigger | Policy |
| ------- | ------ |
| AFK adapter `completed` | Flush pending FIFO, then continue existing post-run publishes (see ordering). |
| AFK `failed` / `cancelled` | **Do not** auto-flush; leave pending on disk; surface warning in run status. |
| Interactive session success end | Same as AFK completed. |
| Interactive failure / abandon | Same as AFK failed — no auto-flush. |
| Operator manual flush | Allowed regardless of terminal state (explicit recovery). |
| Crash before runtime exit hook | Pending remains; operator inspect + manual flush or discard. |

Honey / dispatch: subscribers only see events after flush publish (no charge for defer alone beyond normal publish rules at flush time).

### 5. Ordering relative to runtime auto-publishes

On successful completion:

1. Flush deferred bee events (FIFO).
2. Then 014-style artifacts scan flush **if** still enabled and not superseded by a deferred `artifact.written` in step 1 (coexistence rule).
3. Then auto `MUTATION/code.proposal` when applicable.
4. Then synthesized `INSIGHT/run.summary` when applicable.
5. Then `post_exec`.

Rationale: bee-intended scents and comb announcements precede machine proposal/summary synthesis.

### 6. Mid-flush failure

- **Stop on first error.** Already published events remain on the bus; the failed item and all later pending items stay queued.
- Flush returns error; run/status surfaces the failure.
- A later manual (or retry) flush continues FIFO from what remains.
- Do not invent all-or-nothing rollback of JetStream publishes.
- Do not best-effort skip a failed item and continue silently.

### 7. Coalesce policy

- **Default: no cross-kind coalesce.** N deferred lines ⇒ N publishes on flush.
- **No auto-merge of multiple `artifact.written` lines.** Batching is achieved only by the bee emitting one event with `artifacts[]` (014 shape). Teach that in partials.

### 8. Live vs defer guidance (prompt / docs)

| Use defer | Use live |
| --------- | -------- |
| Handoffs meant for after this bee finishes (`artifact.written`, `spec.ready`, bundled notes/plans) | Need another bee or human reaction **during** this run/session |
| Invite completion should wait for session/run success | True mid-session control signals |
| Eval stability (assert bus after exit) | Debugging with immediate timeline feedback |

Partials should state the rule in few lines; default examples for handoff kinds use `--defer` **only after 015 is implemented**. Until then, 014 scan flush covers artifacts; other kinds stay live.

### 9. Hard-deny list for `--defer`

CLI validation rejects `--defer` for these platform kinds (live-only):

- `system.kill`
- `energy.add`
- `energy.consume`
- `session.invite`
- `beekeeper.ready`
- `task.status`

All other validated kinds may be deferred (including `task.plan`, `task.ready`, `spec.ready`, `artifact.written`, colony SIGNAL kinds, verifications, explicit `code.proposal*`). Expand the deny-list only by explicit product decision — do not blanket-deny all `task.*`.

### 10. Validation timing

- Full schema validation at defer time (same as live).
- On flush, **re-check file-at-`ref` existence** for kinds that require files (`spec.ready`, `artifact.written`, and any similar). Missing file → fail that flush step (stop-on-first-error).
- Full envelope re-validation on flush is not required if the queued envelope was already normalized at defer time.

### 11. Invite `done_when` implication

- Deferred kinds matching `done_when` complete the invite at **flush** time, not at queue time.
- Mid-session live `spec.ready` remains possible and would complete immediately — partials should prefer defer once available.

### 12. `run.summary`

- Keep runtime synthesis of `INSIGHT/run.summary` after deferred flush (ordering step 4).
- Soft guidance: bees should not defer a duplicate `run.summary` when auto-synthesis applies.
- Not a hard-deny for MVP.

### 13. Modules (indicative, when built)

- Event emit CLI — `--defer` flag and result shape; deny-list enforcement; require run dir.
- Pending queue read/write under run dir; flush API shared by runtime and `event flush` (including `--discard`).
- Runtime completion hooks — success flush; failure skip + status warning.
- Inspect / event CLI — pending count and kinds.
- Prompt `emit-howto` — live vs defer table (after implementation).
- Coexistence switch with 014 scan flush (skip when pending `artifact.written` present).
- Tests mirroring emit validation + runtime publish ordering + mid-flush resume.

## Testing Decisions

When implemented, good tests cover external behavior of queue/flush/policy — not NDJSON helper internals.

- Defer validates bad payloads without publishing; deny-list kinds reject `--defer`.
- Defer without run dir fails closed.
- Defer then success → events appear on bus in FIFO order; audit log matches; pending empty.
- Defer then fail → bus unchanged; pending still inspectable.
- Manual flush after failure publishes and clears pending.
- `flush --discard` clears pending without bus publish.
- Double flush is a no-op or safe empty.
- Mid-flush failure: earlier events published; remainder stays pending; retry continues.
- File-at-ref kinds fail flush if file missing after defer.
- Coexistence: deferred `artifact.written` suppresses duplicate 014 scan publish for that run.
- Live emit during a run with also-deferred events: live visible immediately; deferred only after success.
- No auto-merge: two deferred `artifact.written` lines → two publishes.

Prior art: `ProcessEventInput` publish/validate paths; runtime auto-publish ordering tests; invite completion timing tests.

## Out of Scope

- Implementing the buffer as a dependency of [014](014-artifacts-protocol.md) MVP.
- Queen Console queued-vs-published UI in the first 015 ship (follow-up).
- Replacing JetStream with a global delayed message system.
- Cross-host pending replication.
- Automatic rewrite of bee live emits into deferred (no silent interception).
- Charging honey at defer time.
- Hard-deny of `run.summary` defer (soft guidance only).
- Auto-merge of pending `artifact.written` lines.
- MCP wrappers (may follow the same backend later).
- Changing semantics of adapter result `Artifacts` or Object Store offload.

## Further Notes

- **014 coexistence follow-up:** scan-flush skip when a deferred `artifact.written` is present is tracked in [backlog](../plans/backlog.md#014-scan--deferred-artifactwritten-coexistence), not the first 015 implementation slices.
- Closest shipped analogy: runtime auto-publish of `code.proposal` **after** adapter exit — deferred emit extends that “boundary visibility” idea to **bee-authored** events.
- Naming: user-facing “queued events” / “flush on success”; wire stays standard domain events after publish.
- If the colony later rejects building 015, move status to **Deprecated** with pointer to 014-only flush so the trade-off discussion is not lost.
- Pros/cons from the design exploration remain valid: one mechanism for any kind and explicit intent vs second visibility state and prompt cognitive load — mitigated by live default, deny-list, and CLI inspect/flush/discard.
