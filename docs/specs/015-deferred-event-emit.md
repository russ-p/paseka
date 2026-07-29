# Spec 015: Deferred event emit buffer

## Status

**(Draft)**
Design exploration from the same conversation as [014-artifacts-protocol](014-artifacts-protocol.md). **014 MVP uses artifacts-only runtime flush on exit**, not this buffer. This spec records the general deferred-emit idea, how it could replace or complement 014 for artifacts, trade-offs, compromises, and proposed defaults — so the colony can adopt or reject it deliberately later.

## Problem Statement

Bees publish domain events through `paseka event emit`, which today goes to the bus immediately. That is correct for live control and for many narrative notes, but harmful when a bee is still mid-work: subscribers and `auto_invites` can wake on half-finished handoffs, burn honey on premature dispatches, and race the still-running producer.

Artifacts ([014](014-artifacts-protocol.md)) need “announce after success.” Other kinds share the same timing problem — bundles of `context.note`, `task.plan`, `spec.ready`, verifications — whenever the Beekeeper wants **one choreography tick at run boundary**, not N live publishes. Building a special flusher per kind does not scale; relying on prompt discipline (“only emit at the end”) is unreliable for LLM bees.

## Solution

Define an optional **deferred emit buffer**: the same CLI validate-and-accept path as live emit, but append to a per-run pending queue instead of JetStream. On successful run or session completion, runtime **flushes** the queue to the bus (and to the usual audit log) in FIFO order. Live emit remains the default for must-be-visible-now events.

This spec is the design record for that generalization. It does not require implementing the buffer before 014 ships. It explains when the buffer would be better than artifacts-only scan flush, proposed defaults, and explicit non-goals.

## User Stories

1. As a bee, I want to stage a bus event during a run without waking the swarm yet, so that I can finish related files and sibling events first.
2. As a bee, I want one CLI surface for live vs deferred publish, so that I do not learn a second event protocol.
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
13. As an implementer, I want validation at defer time (same schema as live), so that bees get fast feedback before exit.
14. As an implementer, I want re-validation at flush for kinds that depend on files-at-ref, so that missing files fail closed at announcement time.
15. As a Console user, I want queued vs published distinguished in UI when both exist, so that “on disk / staged” is not confused with “swarm has the scent.”
16. As a platform, I want to forbid or discourage defer for selected live-only platform kinds if any must remain synchronous, so that energy/kill/control paths cannot be accidentally delayed.
17. As a bee, I want defer to work for HITL chat the same way as AFK: queue during session, flush on successful session end, so that one mental model covers both.
18. As a beekeeper, I want failed runs to leave the pending queue unpublished by default, so that I can decide to manual-flush or discard.
19. As a documentation reader, I want this spec to state that 014 MVP does not depend on defer, so that implementers do not block artifacts on this design.
20. As a colony adopting both 014 and 015 later, I want a documented coexistence rule (scan flush vs explicit deferred `artifact.written`), so that double announcement does not happen.
21. As a bee emitting many deferred events of different kinds, I want FIFO flush without automatic cross-kind coalesce, so that behavior is predictable.
22. As a bee emitting multiple artifact refs, I want either one deferred `artifact.written` with an `artifacts` array or documented coalesce-only-for-that-kind, so that batching stays an artifacts concern (014) rather than a silent buffer feature.
23. As a beekeeper, I want energy accounting unchanged for deferred events (charge on flush publish, not on queue), so that queuing alone does not spend honey.
24. As an eval/oracle author, I want deterministic flush timing in tests (success → flush), so that harnesses can assert bus state after run.
25. As a multi-machine future user, I understand pending queues are local to the run directory until flush, so that I do not expect JetStream to show deferred events early.

## Implementation Decisions

### 1. Relationship to 014 (artifacts)

- **014 ships first** with runtime **scan + flush** of the trail comb on successful exit. No pending-event file required for MVP artifacts.
- This spec (015) is the candidate generalization. It is **Approved/Implemented only when** the project explicitly chooses to build the buffer.
- **If 015 replaces artifacts announcement:** bees (or a tiny helper) write files and `event emit --defer` one batched `artifact.written`; runtime scan flush in 014 can be turned off or reduced to “files without pending entry” fallback.
- **If 015 complements 014:** coexistence rule — if a pending `artifact.written` exists for the run, skip scan-synthesized duplicate; else keep 014 scan flush. Prefer one announcement path per run.

### 2. CLI and wire behavior (proposed)

- Live (unchanged): validate → publish JetStream → append run audit log.
- Deferred: validate → append per-run pending queue → return ok with a `deferred: true` (or equivalent) result flag so bees/tools know it was not published yet.
- Manual: flush pending for a given run (operator recovery).
- Optional dry-run / validate-only remains orthogonal.

### 3. Pending store

- Per-agent run directory under the trail runs tree (same audit boundary as today’s event log).
- Format: append-only NDJSON of validated event envelopes (or the CLI input shape plus normalized ids). Durable across process crash of the bee; cleared or marked flushed after successful publish.
- Inspect/status projections may list pending count and kinds.

### 4. When flush runs (proposed defaults)

| Trigger | Default |
| ------- | ------- |
| AFK adapter `completed` | Flush pending FIFO, then continue existing post-run publishes (see ordering). |
| AFK `failed` / `cancelled` | **Do not** auto-flush; leave pending on disk; surface warning in run status. |
| Interactive session success end | Same as AFK completed. |
| Interactive failure / abandon | Same as AFK failed — no auto-flush. |
| Operator manual flush | Allowed regardless of terminal state (explicit recovery). |
| Crash before runtime exit hook | Pending remains; operator inspect + manual flush. |

Honey / dispatch: subscribers only see events after flush publish (no charge for defer alone beyond normal publish rules at flush time).

### 5. Ordering relative to runtime auto-publishes (proposed)

On successful completion:

1. Flush deferred bee events (FIFO).
2. Then 014-style artifacts scan flush **if** still enabled and not superseded by a deferred `artifact.written` in step 1 (coexistence rule).
3. Then auto `MUTATION/code.proposal` when applicable.
4. Then synthesized `INSIGHT/run.summary` when applicable.
5. Then `post_exec`.

Rationale: bee-intended scents and comb announcements precede machine proposal/summary synthesis. Document any change in one place if implementation discovers a deadlock with invite completion.

### 6. Coalesce policy (proposed)

- **Default: no cross-kind coalesce.** N deferred lines ⇒ N publishes on flush.
- **Artifacts exception (align with 014):** prefer a single `artifact.written` with `artifacts[]`. Achieved by bee emitting one batched event, or by an optional flush-time merge **only** of consecutive/pending items that are all `artifact.written` into one batch (must be explicitly chosen at implementation time; default suggestion: **do not auto-merge**, teach batched payload in partials).

### 7. Live vs defer guidance (prompt / docs)

| Use defer | Use live |
| --------- | -------- |
| Handoffs meant for after this bee finishes (`artifact.written`, `spec.ready`, bundled notes/plans) | Need another bee or human reaction **during** this run/session |
| Invite completion should wait for session/run success | True mid-session control signals |
| Eval stability (assert bus after exit) | Debugging with immediate timeline feedback |

Partials should state the rule in few lines; default examples for handoff kinds use `--defer` **only after 015 is implemented**. Until then, 014 scan flush covers artifacts; other kinds stay live.

### 8. Invite `done_when` implication

- Deferred kinds matching `done_when` complete the invite at **flush** time, not at queue time. That is desirable for grilling + deferred `spec.ready`.
- Document that mid-session live `spec.ready` remains possible and would complete immediately — partials should prefer defer once available.

### 9. Pros and cons (design record)

**Pros**

- One mechanism for any kind — not only comb files.
- Preserves explicit emit intent (unlike pure filesystem inference).
- Same validation path as live emit; small conceptual delta (`--defer`).
- Reduces mid-run choreography races and wasted dispatches.
- Improves LLM safety when partials default handoffs to defer.
- Pending file aids inspect/debug and crash recovery.
- Shared AFK and HITL model.
- Opt-in beside unchanged live emit.

**Cons / risks**

- Second visibility state (queued vs published) — Console and operators must learn it.
- Failure policy must be crisp or behavior feels magical.
- Mixed live + deferred in one run complicates timeline mental models.
- Double-publish risk (defer + live same logical event; defer + 014 scan).
- Crash before flush needs operator story.
- Prompt cognitive load (two emit modes).
- Defer alone does not batch multi-artifact payloads — that remains 014 shape design.
- Pending is local until flush (no early multi-apiary visibility).
- Implementation surface: CLI, runtime hooks, inspect, tests, partials, Console — larger than artifacts-only flush.

### 10. Compromises (proposed)

| Tension | Compromise |
| ------- | ---------- |
| Safety vs recovering partial success | Default no flush on failure; manual flush + disk retention for recovery. |
| Universal buffer vs small 014 | Ship 014 scan flush first; treat 015 as follow-up, not a blocker. |
| Auto-merge vs predictability | No cross-kind merge; no silent artifact merge unless explicitly approved later. |
| Validate now vs at flush | Validate schema on defer; re-check file-at-ref on flush for kinds that require files. |
| HITL mid-session needs | Keep live emit; do not force defer for all SIGNAL kinds. |

### 11. Modules (indicative, when built)

- Event emit CLI — `--defer` flag and result shape.
- Pending queue read/write under run dir; flush API used by runtime and manual CLI.
- Runtime completion hooks — success flush; failure skip.
- Inspect / Console — queued events.
- Prompt `emit-howto` — live vs defer table.
- Coexistence switch with 014 scan flush.
- Tests mirroring emit validation + runtime publish ordering tests.

### 12. Decision checklist before implementation

Before moving this spec to Approved/Implemented, confirm:

1. Failure default remains “no auto-flush.”
2. Coexistence rule with 014 (replace vs skip-scan-when-pending).
3. Whether any platform kinds are hard-denied for defer.
4. Whether artifact pending lines auto-merge (recommend no).
5. Manual flush UX (CLI required; Console optional).

## Testing Decisions

When implemented, good tests cover external behavior of queue/flush/policy — not NDJSON helper internals.

- Defer validates bad payloads without publishing.
- Defer then success → events appear on bus in FIFO order; audit log matches.
- Defer then fail → bus unchanged; pending still inspectable.
- Manual flush after failure publishes and clears pending.
- Double flush is a no-op or safe empty.
- File-at-ref kinds fail flush if file missing after defer.
- Coexistence: deferred `artifact.written` suppresses duplicate 014 scan publish for that run (if that rule is chosen).
- Live emit during a run with also-deferred events: live visible immediately; deferred only after success.

Prior art: `ProcessEventInput` publish/validate paths; runtime auto-publish ordering tests; invite completion timing tests.

## Out of Scope

- Implementing the buffer as a dependency of [014](014-artifacts-protocol.md) MVP.
- Replacing JetStream with a global delayed message system.
- Cross-host pending replication.
- Automatic rewrite of bee live emits into deferred (no silent interception).
- Charging honey at defer time.
- MCP wrappers (may follow the same backend later).
- Changing semantics of adapter result `Artifacts` or Object Store offload.

## Further Notes

- Closest shipped analogy: runtime auto-publish of `code.proposal` **after** adapter exit — deferred emit extends that “boundary visibility” idea to **bee-authored** events.
- Naming: user-facing “queued events” / “flush on success”; wire stays standard domain events after publish.
- If 015 is rejected, keep this document as **Deprecated** with pointer to 014-only flush, so the trade-off discussion is not lost.
- Open product question (do not invent in code until answered): should synthesized `run.summary` move behind defer too, or always stay runtime-owned after flush? Recommendation: keep runtime synthesis after deferred flush, and discourage bees from deferring duplicate `run.summary` when auto-synthesis applies.
