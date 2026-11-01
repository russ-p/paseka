# Spec 046: Bee check gate

## Status

**(Implemented)**
The runtime stage shipped. A Bee's `checks:` block (`command`, `timeout`, `rejection_cap`) is validated at colony load and read from the resolved configuration snapshot; the runtime runs it after the adapter exits and before the deferred-event flush, fail-closed on non-zero exit, signal, timeout, or exec failure. A rejection marks the run `checks_failed`, writes `checks.log`, suppresses the proposal through the existing completion contract, records the gate result on the run and proposal archive records, and publishes a runtime-authored `verification.failed` that redispatches the builder on the same attempt and worktree; at the consecutive rejection cap the task fails with the gate summary. The reference colony now rolls this out: the builder declares a Go formatting/lint/build/test gate, and the guard audits the recorded verdict instead of re-running those checks. Queen Console run-page attribution of the gate result and the `paseka doctor` gate findings are not shipped yet.

## Problem Statement

A builder produces a diff, and the first actor to learn whether that diff compiles is a Guard Bee — an LLM. The guard re-runs formatters, linters, and tests, reads the output, and only then reviews requirements, scope, and design. Three things are wrong with that arrangement:

- **Cost.** A mechanical failure (a stray unused import, one failing assertion) is discovered by the most expensive actor in the colony, in a full review turn that is then thrown away. The builder is redispatched, fixes it, and the guard reviews again. A two-second deterministic check burns two model turns.
- **Attention.** The guard's review turn is spent interpreting compiler output it cannot improve on. Its comparative advantage is judgement — does the diff satisfy the acceptance criteria, is it in scope, is it safe — and that part gets less attention because the turn starts with lint noise.
- **Trust.** The builder is asked to run the same checks and report on itself ("Build passes", "Related tests pass" in its success criteria). That self-report is not evidence: the same model that wrote the diff also wrote the verdict. So the guard must re-run everything anyway, and the colony pays for the check twice while still having only the builder's word for the first run.

What the colony actually wants is a division that matches the work: the builder iterates against a deterministic gate as it works, the runtime runs that gate once as a precondition to publishing a proposal, and the guard spends its turn on things a script cannot decide.

## Solution

1. **The gate is a declared part of the Bee.** A Bee can declare `checks:` — a command, a timeout, a rejection cap, nothing else. Mechanical verification becomes colony configuration, not a prompt instruction.
2. **The runtime runs it, not a Bee.** After the adapter exits and before any proposal event leaves the run, the runtime executes the check command in the producing workspace with the producing run's environment. The exit code decides.
3. **Failure is a rejection, not a task failure.** A failing gate marks the run `failed` with a machine-readable reason, suppresses the proposal, and emits the same `VERIFICATION/verification.failed` a guard would emit — so the builder is redispatched through the existing rework loop. No new event kinds, no new routing. A repeated rejection that the builder cannot fix stops at a configured cap and fails the task instead of draining the honey reserve.
4. **Fail-closed on everything.** A missing binary, an unreadable command, a timeout, or a truncated log is a gate failure. A gate can never pass because it could not run.
5. **The guard stops re-running checks and audits the gate instead.** The gate result is recorded on the run and on the proposal record, so the guard reads one small file by the proposal author's identity and spends its turn on semantics. A colony may keep the old arrangement; the gate is opt-in per Bee.
6. **The builder still checks its own work.** The prompt keeps telling it to run the same command locally — that is how it avoids an expensive round trip — but its verdict is no longer treated as evidence by anyone.

## User Stories

1. As a Beekeeper, I want a diff that fails the build to be rejected before a model reviews it, so that I pay for a mechanical check instead of a review turn.
2. As a Beekeeper, I want a failed gate to send the task back to the builder, so that the repair loop works exactly as it does after a guard rejection.
3. As a Beekeeper, I want the check command to come from colony configuration, so that the builder's own diff cannot weaken the gate that judges it.
4. As a Beekeeper, I want a gate that cannot execute to be a failure and not a pass, so that a broken toolchain never turns into an approving silence.
5. As a Beekeeper, I want a hung linter to be bounded, so that one bad check cannot hold an entire trail.
6. As a Beekeeper, I want the check output preserved on the run, so that I can read why a proposal was rejected without re-running anything.
7. As a Beekeeper, I want a gate rejection distinguishable from a guard rejection in the trail, so that I can tell mechanical churn from design disagreement.
8. As a Beekeeper, I want the gate to be opt-in per Bee, so that a colony that prefers a reviewer-owned check keeps working unchanged.
9. As a Beekeeper, I want to see how often the gate rejects over a trail, so that a flaky or mis-scoped check command is visible as a pattern rather than as one-off surprise.
10. As a Beekeeper, I want to read the gate outcome in the Queen Console run page, so that the reason a proposal never appeared is one click away.
11. As a Beekeeper, I want the gate outcome to survive run pruning with the proposal record, so that an audit of an old proposal still says what passed.
12. As a Beekeeper, I want `paseka doctor` to name a Bee whose gate command cannot resolve, so that a typo is caught before a trail is spent on it.
13. As a Beekeeper, I want a gate on a Bee that never publishes code to be flagged, so that dead configuration is not copied into new colonies.
14. As a Builder Bee, I want the gate explained in my prompt, so that I know which command the runtime will run and can run it myself first.
15. As a Builder Bee, I want a gate rejection to arrive with the failing output, so that my repair turn starts from evidence instead of from a summary.
16. As a Guard Bee, I want to review semantics and not re-run formatters and tests, so that my turn is spent on requirements, scope, and risk.
17. As a Guard Bee, I want to read the producing run's gate result, so that I can confirm mechanical verification happened without repeating it.
18. As a Guard Bee, I want a diff with no gate result to be visible as such, so that I can treat an un-gated proposal as weaker evidence rather than assumed-green.
19. As a colony author, I want the check to receive the same workspace path the adapter had, so that what is checked is exactly the diff under review.
20. As a colony author, I want the check to inherit the run's declared environment, so that a check that needs a token does not need a second credential path.
21. As a colony author, I want to declare the check as a shell-like string or an argv list, so that it matches how I already declare an adapter command.
22. As a colony author, I want the gate to work for both isolated and root proposals, so that the reviewer-owned-check problem does not come back for root proposals.
23. As an operator, I want the run record to say `checks_failed` rather than a generic failure, so that triage does not require reading logs first.
24. As an operator, I want the trail's rework counter to include gate rejections, so that churn is measured against one number.
25. As a maintainer, I want no new event kinds and no routing changes, so that the gate is a runtime behaviour and not a protocol revision.
26. As a maintainer, I want gate failures to be counted as run outcomes with a reason, so that trail observability reports them truthfully.
27. As an implementer, I want Bee loading to reject an incomplete gate configuration, so that a half-declared gate cannot silently mean "no gate".
28. As an implementer, I want un-gated Bees to behave byte-for-byte as before, so that this ships without a migration.
29. As a Builder Bee, I want the path to the full check log in my rework task, so that I read the real failing output instead of a one-line paraphrase that may have been truncated.
30. As a Beekeeper, I want a gate the builder cannot satisfy to stop after a couple of round trips and fail the task with a named reason, so that a broken toolchain does not drain the honey reserve before anyone notices.
31. As a maintainer, I want the rejection to be authored by the platform, so that role-based self-dispatch suppression cannot silently swallow the very rework it is meant to trigger.
32. As an operator, I want a failed gate on a run without a task to fail quietly without dispatching anything, so that a taskless run cannot loop a builder against itself.
33. As a maintainer, I want a redelivered rejection to dispatch the builder once, so that a bus retry does not multiply repair turns.

## Implementation Decisions

### 1. Gate placement in the run lifecycle

1. The gate is a stage of the AFK dispatch finalize path: it runs after the adapter process exits and **before the deferred-event flush**, so a queued proposal event cannot reach the bus ahead of the gate decision.
2. It also runs before run-summary synthesis, so the synthesized summary and the completion-contract evaluation see the gate's verdict rather than a stale `completed`.
3. `post_exec` keeps its current position after the gate and its current advisory semantics; it is not promoted into a gate. The two exist for different purposes: the gate decides, the hook notifies.
4. The gate runs for runtime-published proposals on both proposal paths. Its working directory is the dispatch workspace — the trace worktree for isolated proposals, the colony checkout for root proposals — so the checked bytes are the reviewed bytes.
5. Interactive sessions are not gated: a human is the gate.

### 2. Configuration

6. A Bee gains an optional gate block with a command and an optional timeout. Command parsing, quoting, argv-list form, and `$`/`${}` variable substitution reuse the existing colony command type used by adapter `command` and `post_exec` — one syntax, one implementation.
7. A gate block without a command is a load error, not "no gate". An unparsable timeout is a load error.
8. Absent timeout defaults to a bounded value (10 minutes); an explicit zero or negative timeout is a load error. There is no unbounded gate.
9. A rejection cap defaults to two consecutive gate rejections per task attempt; a negative value is a load error and an explicit zero means "no cap". The cap exists so a gate the builder cannot satisfy — a missing binary, a wrong working directory, a broken toolchain — surfaces as a failure instead of consuming the honey reserve.
10. Consecutive rejections are derived from the attempt's recorded gate results, not from in-memory counters, so a restarted runtime counts the same way. A passing gate clears the count.
11. The gate command is read from the resolved colony configuration for the dispatch — the same snapshot the Bee definition itself comes from — never from the workspace under review.

### 3. Execution contract

12. The command runs with the dispatch workspace as its working directory, with the launch plan's assembled environment plus the `PASEKA_*` run identity variables already provided to script Bees, so both declared `params.env` and injected credentials reach it.
13. The runtime runs the command directly by argv, without an intervening shell, exactly as it already runs an adapter command; a colony that wants a pipeline declares a shell as its command.
14. The exit status of the check process is the verdict, read directly by the runtime because the runtime owns the process. Nothing is inferred from the check's stdout or stderr, and the check command publishes no events — unlike a script Bee, where the runtime cannot know what "pass" means and the script must emit its own verification. Zero is pass; any non-zero code is fail; a process killed by a signal is fail with a signal reason; a command that cannot be started at all is fail with an exec reason.
15. Output is captured without a shell-level pipe that can swallow the exit code. The full captured output is written to the run's record under a hard size cap with an explicit truncation marker; a separately capped head of the output is appended to the run's error note and a one-line verdict goes to the rejection event, so rework prompts stay readable without turning a task summary into a log.
16. The gate is not run when the run has not reached `completed` for other reasons: a cancelled, timed-out, or contract-downgraded adapter run keeps its existing outcome and needs no mechanical verdict.
17. The gate does not consume additional honey: it is part of a dispatch already paid for, which is the point of putting it in the runtime rather than in a separate Bee.

### 4. Outcome semantics

18. A gate failure sets the run status to `failed` with reason `checks_failed`, records the gate result, and appends the failing output's head to the run's error note.
19. Proposal publication is suppressed by the existing completion contract: proposal archival and publication already refuse a run whose final status is not `completed`, so the gate needs no second suppression path. The diff stays on disk in the workspace exactly as an adapter failure leaves it.
20. **The rejection is authored by the runtime, not by the run.** It must not carry the producing run's agent identity, because direct dispatch suppresses a subscriber whose role matches the publisher's role resolved from that identity — a builder-authored rejection would therefore never redispatch a builder. Runtime authorship also states the truth: the verdict belongs to the platform, not to the model that wrote the diff.
21. The rejection is the existing `VERIFICATION/verification.failed` with an additive payload:
    - `taskId` — the task of the failed run; without it the event has no attempt context;
    - `reason` — an open string naming the cause, `checks_failed` today, so mechanical rejection is distinguishable from review rejection;
    - `summary` — a one-line verdict (`checks failed (exit 1): go test ./...`), never the log;
    - `details` — optional, colony-root-relative path to the captured gate log, following the same path convention as a proposal `ref`.
22. `summary` is deliberately not a log dump. The rework task text is derived from it by concatenating a fixed prefix and the summary, the same text also lands in task status, the Console, and gateway messages, and the whole direct task body is capped — so a wall of compiler output there is both truncated and unreadable. The log is referenced, not inlined.
23. The runtime appends the rejection to the same synthesized-event list it already uses for a synthesized run summary, so the existing publication pass publishes it, records it in the run's event log, and the trail log records it by provenance. No new publication path.
24. The rework task text gains one line naming the `details` path when present, so the builder's repair turn starts from the exact failing output instead of a paraphrase. The rework dispatch reuses the attempt resolved by `taskId`, hence the same worktree the failed diff is in.
25. A gate failure is a proposal rejection, not a task lifecycle transition, mirroring guard rejection: the task keeps the status its dispatch had (typically `running`) and the rework is a direct dispatch. The runtime must therefore not take its generic "non-completed run" task-failure branch for a gate failure that already emitted a rework signal.
26. The rework signal is emitted whenever the failed run has a task, including a gate failure on a rework dispatch — the repair loop must be able to iterate, exactly as a guard rejection can reject a rework proposal. Without a task, the run simply fails: signalling a taskless builder would loop it against itself.
27. After the configured number of consecutive gate rejections for one task attempt, the runtime stops signalling rework, fails the task with the gate summary, and leaves the last gate result in place. An environment problem then reads as a failed task with a named reason instead of as an exhausted honey reserve.
28. Repeated gate rejections up to that cap consume honey like any other dispatch. This spec adds no other attempt limit.
29. Redelivery of the same rejection does not dispatch twice: the existing dedupe keys rework-cycle failures by event identity, so each pass runs once and each builder→gate pass gets a new one.

### 5. Visibility

30. The run record gains a gate result: command as executed, exit code, duration, timed-out flag, truncation flag, and the path to the captured log. The status snapshot's reason stays `checks_failed`.
31. The proposal archive record carries the gate result too, so an audit of a proposal outlives the run directory that produced it.
32. Gate rejections are visible in existing surfaces: run outcome and reason in the trail, the console run page, the trail export, the rejection event itself (reason, short summary, log path), and the existing rework counter. No new event kind is introduced for observability.
33. A Bee without a gate produces no gate result and no gate field anywhere in its records — absence stays distinguishable from a pass.

### 6. Validation and colony rollout

34. `paseka doctor` reports a gate whose command cannot be resolved on the host, and warns when a Bee declares a gate but publishes no code proposal.
35. The reference colony's builder declares a gate covering Go formatting, lint, the module build, and the module tests, inline in the Bee config so the diff it judges cannot weaken it; the guard prompt drops the re-run and gains the audit step instead. Frontend checks stay outside the gate because a trace worktree carries no installed dependencies, so the guard runs them itself when the diff touches the web tree.
36. The guard prompt's success criteria change from "linting passes" to "the producing run's gate result is present and passing, or its absence is called out".

## Testing Decisions

Good tests here assert run outcomes and bus effects, not internal call order: a proposal is published or not, a run record says what happened, a rework dispatch happens or does not. They should use a real command (a small shell script or a stub binary) and a fake adapter, the way the existing dispatch tests already exercise `post_exec` and completion contracts.

- **Config loading**: gate without command rejected; bad timeout rejected; zero timeout rejected; negative rejection cap rejected; un-gated Bee unchanged; command parsed identically to adapter `command` (shared command-type tests are the prior art).
- **Dispatch finalize**: pass → proposal published, status `completed`, gate result recorded; non-zero → no `MUTATION` published, status `failed` with reason `checks_failed`, gate log written, output head appended to the error note; timeout → failed, not hung; command that cannot start → failed, never pass; killed-by-signal → failed with the signal reason; un-gated Bee → records contain no gate field.
- **Rejection payload**: reason, one-line summary, and the log path are present; the summary does not contain the full log; the referenced log file exists and matches the run's gate record.
- **Ordering**: a deferred proposal event emitted by a failing gated run does not reach the bus; a synthesized run summary for a failing run says failed, not completed.
- **Reactor**: the rejection is authored by the runtime and a builder still receives it — the regression this pins is that role-based suppression must not swallow the rework because the publisher resolved to the subscriber's own role; a gate failure with a task triggers exactly one builder rework dispatch, reuses the same attempt and worktree, and leaves task status unchanged (prior art: the existing rework-redispatch and direct-pending tests).
- **No task**: a failing gate on a run without a task fails the run, publishes no rework signal, and dispatches nothing.
- **Cap**: gate rejections up to the cap keep producing rework; the next one fails the task with the gate summary and dispatches nothing further; a passing gate resets the count; the count survives a runtime restart because it is read from recorded gate results.
- **Root path**: a gated root proposal is checked in the colony checkout and rejected the same way.
- **Records**: gate result present on the run record and on the proposal record; survives run-directory removal in the proposal case.
- **Doctor**: unresolvable command reported; gate without a code-proposal publish warned.
- **Regression**: colonies with no gate behave exactly as before, and the existing guard/builder/receiver trail tests pass unchanged.

## Out of Scope

- Replacing or narrowing the guard's semantic review. The guard keeps requirement matching, scope isolation, and design judgement; only the mechanical rerun moves.
- Auto-fixing gate failures, retry budgeting, or classifying linter output by cause.
- Per-file or per-package scoping heuristics. The colony declares the command; the runtime does not infer scope from the diff.
- Gating interactive sessions, merges, or the commit-gate receiver.
- Caching check results across runs, parallel checks, or per-check servers.
- A separate mechanical-guard Bee. Rejected: direct subscribers of one event are dispatched concurrently, so a second Bee cannot gate a reviewer without a new event kind and a new contract; and each Bee run costs a hive token regardless of how fast it exits. See Further Notes.
- A Console redesign for gate results beyond the run page and the existing trail metrics.
- Any change to how proposals are collected, stored, or merged.

## Further Notes

**Why not a mechanical-guard Bee.** It is the obvious choreography-shaped answer and it does not work with today's routing: the reactor dispatches every direct subscriber of an event in its own goroutine, so a script Bee and the guard would race on the same proposal, and the guard could approve while the checks are still running. Making it sequential requires the script Bee to emit a kind the guard subscribes to — a new event contract, topology entry, dedupe rule, and a second `VERIFICATION` producer competing with the receiver's commit gate. It also spends a hive token per proposal on a five-second process.

**Why not keep it inside the guard as a script.** A colony can do this today with no runtime change — guard runs a committed script first and short-circuits on failure — and it is a reasonable stopgap. What it cannot give is a guarantee: the guard can forget the step, skip it while "being efficient", or be replaced by a Bee without it, and the check command lives in the workspace under review, so a builder can weaken it in the same diff. A runtime stage is not skippable and reads its command from configuration outside the diff.

**Why not promote `post_exec`.** It already runs an arbitrary command after the adapter and already receives the run environment, but its failures are deliberately advisory and must stay so: it is the notification hook, and making it fatal would turn every broken webhook into a failed proposal. A gate needs a different contract — fail-closed, timed, recorded, and wired to the rework signal.

**Why the rejection must not look like a message from the builder.** Direct dispatch already refuses to run a Bee on an event published by the same Bee role, to stop a reviewer from re-triggering itself. That rule makes the obvious implementation of the rejection event — stamp it with the failed run's own identity — silently useless: the producer is a builder, the subscriber is a builder, no rework happens, and the trail shows a failed run with no explanation. The rejection has to be platform-authored. Worth stating loudly because the failure mode is invisible in a unit test that only checks "the event was published".

**Why there is a rejection cap at all.** The guard loop is self-limiting because a model eventually changes its mind or the operator intervenes. A mechanical gate has no such property: if `golangci-lint` is not installed, every round trip fails identically and the only visible end state is an exhausted honey reserve. The cap converts that into a failed task with `checks_failed` — the same class of honesty the rest of the run record aims for.

**The honest asymmetry that remains.** The builder still self-checks. That redundancy is intended and cheap: an author running the same command before asking for review is how the loop stays short. The difference this spec makes is that the self-check is no longer the only evidence.
