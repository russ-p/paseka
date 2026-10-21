# Spec 042: Trail Configuration Revisions and Forks

## Status

**Draft**
Trail-owned configuration snapshots, explicit application of new revisions during ordinary work, and pinned experimental forks executed by the colony's existing Reactor. Admission policy and remaining scope questions are listed in Further Notes.

Related: [008 code proposal workspaces](008-code-proposal-workspaces.md),
[027 config profiles](027-config-profiles.md),
[034 swarm mode](034-swarm-mode.md),
[039 proposal patch records](039-proposal-patch-records.md),
[040 trail event log](040-trail-event-log.md),
[041 skills layer](041-skills-layer.md),
[043 trail observability](043-trail-observability.md),
[idea: self-improving colonies](../idea/self-improving-colony.md).

## Problem Statement

A Beekeeper can retry a failed task, but cannot conveniently test whether a different procedure would have produced a better result. A retry uses the same trail and its remaining honey. Changing the colony's prompts or bee definitions changes shared configuration rather than creating a recorded experiment against the original task.

A useful experiment needs the same task and starting code, a reviewable change to the procedure, and its own budget. The procedure can include bee definitions, prompts, skills, routing, and completion contracts. Adapter parameters alone cannot express it.

Putting an edited `.paseka/` in a worktree currently does not select it for execution. The runtime reads configuration from the colony root and uses the worktree as the adapter's working directory. Routing also uses one colony-wide bee registry. Changing only the prompt loader would leave subscriber selection, validation, and completion behaviour tied to another configuration.

Running another colony checkout with another Reactor, home, and bus namespace makes the experiment harder to operate. Its records and lifecycle are separated from the source, and the Beekeeper needs several control points to inspect and stop related work.

There is also a historical gap: today's configuration does not necessarily describe what an earlier trail received. Profiles, local templates, model aliases, and private skills may have changed. An experiment must inherit a saved configuration of its source before the platform can claim that only the candidate delta changed.

Ordinary work also needs controlled change within an existing trail. A Beekeeper may want later tasks to use a stronger model, apply an updated procedure, or retry failed work with different settings. Freezing the whole trail forever would require a new identity merely to change those settings. The configuration record must preserve what ran while allowing an explicit change to future work.

## Solution

A fork is a new Flight Trail in the **same colony**, with the source task, the source's starting code commit, an immutable configuration derived from the source's saved configuration, a separate execution worktree, and its own capped Honey Reserve.

The colony keeps one Reactor, home, bus namespace, and trail record collection. Queen Console and Queen Shell show the fork beside its source and use the same inspect, honey, kill, and cleanup operations for both.

The configuration overlay is prepared once: the runtime takes the source's saved configuration, applies a proposed delta, validates the result, and stores a complete snapshot under a stable `ConfigRef`. Every dispatch in the fork uses that snapshot. A worktree can be used to author the candidate; its changing contents do not become a live configuration source.

An ordinary trail supports the operator action **Apply configuration**. It prepares a new immutable snapshot, validates the remaining work, and changes the trail's active reference at a safe execution boundary. The initial reference and all previous versions remain in history. Running agents and their already-started verification chains retain their recorded version. Experimental forks are pinned in v1 so this action cannot silently change the candidate being evaluated.

Three bindings are independent:

| Binding | Responsibility |
| --- | --- |
| `ColonyRoot` | Colony identity, infrastructure, credentials, records, and machine-local registry |
| `ConfigRef` | An immutable configuration version; a trail has initial and active references, and each execution records the reference it actually used |
| `WorkspaceRoot` | The checkout in which the trail's agents execute and change code |

Source and fork records remain under the colony's `.paseka/runs/<traceId>/`, while the fork executes in its own `.paseka/worktrees/<traceId>/`. In v1 each trail stores its complete configuration beside its tasks and artifacts:

```text
.paseka/runs/<traceId>/
  artifacts/
  tasks/
  fork.json             # fork lineage, when present
  config/
    manifest.json       # initial/active references, revision, policy, and change history
    snapshots/
      <ConfigRef>/      # complete immutable inputs, resolved settings, and inventory
      <anotherRef>/     # later applied version or a fork's candidate
```

An ordinary trail starts with one snapshot and retains additional versions when the operator applies a change. A fork owns both its source and candidate configuration; base and effective are roles in its records rather than mutable directories. Deleting the source trail does not break fork execution or configuration comparison. Equal references, including a control arm's base and candidate, use one copy within the owning trail. These are runtime-owned records, not operator-editable overrides.

`ConfigRef` identifies content independently of storage location. Two trails can have the same reference while retaining separate physical copies. V1 deliberately accepts that duplication to keep trail backup, inspection, and deletion self-contained.

For an ordinary new trail, the creation source is the current colony configuration with the selected profile and applicable local contributions. The runtime captures it and initializes both `initialConfigRef` and `activeConfigRef` before configuration-dependent routing. A label such as `main`, `default`, or `colony` may describe the source, but is never the bound content identity or a request to keep reading live files.

For example, tasks 1 and 2 in trail A use X. The operator applies a stronger builder model, producing Y; subsequent tasks use Y in the same trail and worktree. Earlier runs still identify X. Editing colony files alone does not trigger the switch: applying current colony configuration is an explicit action. A fork selects the source task attempt's recorded configuration, which can be X even while A's active reference is Y.

The platform supplies reproducible configuration inputs and records the differences. The methodology decides whether the candidate improved the outcome.

## User Stories

1. As a Beekeeper, I want to fork a trail with a proposed configuration change, so that I can test a different procedure against the same task.
2. As a Beekeeper, I want a fork with no delta, so that I can obtain a control run from the source configuration.
3. As a Beekeeper, I want the fork to start from the source's initial code commit, so that the source's completed solution does not become the experiment's starting point.
4. As a Beekeeper, I want an explicit alternative code base recorded as a difference, so that a continuation is not presented as the same-base comparison.
5. As a Beekeeper, I want the source's saved task input carried into a new task identity, so that the specification survives independently of the original task's execution state.
6. As a Beekeeper, I want ambiguous root tasks and unsupported dependencies reported before launch, so that the platform does not silently choose different work.
7. As a Beekeeper, I want to change prompts, bee definitions, skills, and completion contracts as a file delta, so that the experiment can express a complete procedure.
8. As a Beekeeper, I want additions and deletions preserved, so that removed roles or procedures do not reappear through fallback to the live colony.
9. As a Beekeeper, I want every run in a fork to use the same configuration reference, so that a multi-step result has one inspectable procedure.
10. As a Beekeeper, I want live-colony and execution-worktree edits to leave an admitted fork's configuration unchanged, so that concurrent work does not alter the experiment.
11. As a Beekeeper, I want the source profile and local contributions preserved, so that the baseline reflects what the source actually used.
12. As a Beekeeper, I want missing historical configuration reported honestly, so that current settings are never substituted for unknown past settings.
13. As a Beekeeper, I want to preview the task, code base, configuration difference, workspace, and budget, so that I can review the fork before creating it.
14. As a Beekeeper, I want invalid patches and configurations explained without starting an agent, so that unusable candidates spend no honey.
15. As a Beekeeper, I want forks and ordinary trails in one Console, so that I can manage the colony from one point.
16. As a Beekeeper, I want all trails executed by the same running Reactor, so that an experiment needs no extra runtime process.
17. As a Beekeeper, I want to inspect and stop a fork by its trail identity, so that existing operational controls remain useful.
18. As a Beekeeper, I want each fork to spend its own honey under colony limits, so that it cannot drain the source's reserve.
19. As a Beekeeper, I want candidate changes unable to raise their own spending or delivery permissions, so that experimental behaviour stays within colony policy.
20. As a Beekeeper, I want experimental proposals inspectable without merging or publishing them, so that internal verification can run without delivering the candidate.
21. As a Beekeeper, I want root-working roles confined to the fork checkout, so that a workspace setting cannot select the primary checkout.
22. As a Bee, I want fork events routed by its saved subscriptions, so that the candidate choreography is what actually runs.
23. As a Bee, I want publishing and helper commands to retain my colony and trail context, so that a worktree does not become another colony identity.
24. As a Beekeeper, I want concurrent configurations to remain independent, so that parallel dispatch cannot exchange prompts or routing rules.
25. As a Beekeeper, I want retries and interactive continuation to retain the trail configuration, so that changing the entry point does not change the procedure.
26. As a Beekeeper, I want a Reactor restart to recover configuration bindings, so that correctness does not depend on an in-memory cache.
27. As a Bee permitted to request experiments, I want an idempotent fork signal, so that duplicate delivery does not create duplicate work or budgets.
28. As a Beekeeper, I want lineage, recorded inputs, configuration references, delta, and budget together, so that the comparison is auditable.
29. As a methodology author, I want fork identity in metrics and work-completion reads, so that I can compare experiments and avoid recursively studying my own probes.
30. As a Beekeeper, I want to list a source's forks, so that I can discover previous experiments without scanning checkouts.
31. As a Beekeeper, I want configuration evidence to survive execution-worktree cleanup, so that I can still explain a retained result.
32. As a Beekeeper, I want private inputs to remain private when snapshots are retained or exported, so that reproducibility does not publish machine-local content or credentials.
33. As a Beekeeper, I want a retained fork to own both configuration versions, so that removing the source trail does not prevent execution or inspection of the configuration difference.
34. As a Beekeeper, I want configuration content stored with the trail, so that backing up or deleting a trail does not require managing references in another store.
35. As a Beekeeper, I want to apply the current colony configuration to an existing ordinary trail, so that future tasks can use updated procedures without losing their history or workspace.
36. As a Beekeeper, I want to override a role's model within one trail, so that later work can use another model without changing the colony defaults.
37. As a Beekeeper, I want to reactivate a retained configuration, so that I can roll back a change without rewriting past executions.
38. As a Beekeeper, I want running agents and their verification chains to keep their original configuration, so that an update does not mix execution and review contracts.
39. As a Beekeeper, I want the apply action to show when it is waiting and let me cancel it, so that pending reviews do not hide an unexplained configuration change.
40. As a Beekeeper, I want an explicit retry after a configuration change to record a new attempt on the active version, so that I can recover failed work using different settings.
41. As a Beekeeper, I want every configuration application recorded with its actor, source, old and new references, and outcome, so that a trail containing several versions remains auditable.
42. As a Beekeeper, I want conflicting and duplicate apply requests handled predictably, so that concurrent controls do not silently replace each other's settings.
43. As a Beekeeper, I want applying configuration to preserve code, task progress, and honey, so that changing procedure does not restart or clone the trail.
44. As a Beekeeper, I want a fork to name the source task attempt it inherits, so that a later active configuration is never substituted for the procedure being studied.

## Implementation Decisions

### 1. A fork belongs to its source colony

The fork gets a new `traceId`, a new `taskId`, and recorded lineage. The existing Reactor executes it using the colony's existing transport, ledger store, home binding, and lifecycle registry. There is no fork-specific slug, stream, subject prefix, home, or Reactor process.

Honey and task state remain keyed by the new trail identity. Sharing infrastructure does not share the source's mutable tasks, honey, run records, or comb.

### 2. Configuration is an explicit trail binding

The resolver returns an immutable execution context containing the canonical colony context, selected `ConfigRef` and configuration revision, effective definitions and settings, the corresponding bee registry, and `WorkspaceRoot`. Selecting a context for new work and preserving a context for already-started work are distinct operations.

`ConfigRef` is a deterministic content identifier computed from a versioned canonical inventory and effective non-secret settings. Executable modes and referenced procedure content are included where they affect execution. Trail identity, timestamps, and physical storage paths do not affect the identifier. A versioned manifest records the binding, snapshot inventories, content digests, local locations, and resolution provenance.

V1 stores the complete snapshot inside its owning trail. The same `ConfigRef` may identify separate copies in different trails; it is not a pointer into the source trail or a colony-wide snapshot store. The resolver owns location lookup and validates identity separately, allowing a later storage backend to preserve the runtime interface.

The runtime materializes files when needed and validates them against the stored identity. Missing or corrupt content blocks dispatch. A bound trail never falls back to live colony files.

### 3. Initial capture and explicit configuration application

New trails created after this feature is enabled capture their initial configuration before their first configuration-dependent event validation or routing. Concurrent first events converge on one durable binding. The initial code base and task inputs accepted for execution are recorded as well; later edits to task projections cannot rewrite past execution evidence.

The trail manifest stores `initialConfigRef`, `activeConfigRef`, a monotonic `configRevision`, and configuration policy. Initial capture creates revision 1 with equal initial and active references. Ordinary trails use `editable` policy and accept explicit application; experimental forks use `pinned` policy and reject application in v1. Initial identity and snapshot contents are immutable. Applying another version changes only the active binding and appends history. Editing files, topping up honey, or continuing work never implicitly refreshes that binding. Standing trails can apply revisions between their work cycles under the same rules.

Existing unbound trails retain legacy behaviour. An operator may explicitly capture configuration for future work at a safe boundary, using expected revision 0 and establishing revision 1, but earlier executions remain marked as lacking snapshots. Applying a version cannot retroactively supply historical evidence. A fork needing an unavailable source snapshot, task input, or code base is refused in v1; best-effort historical reconstruction is deferred.

#### Apply configuration sources

One apply action selects exactly one source:

| Source | Preparation |
| --- | --- |
| `colony` | Capture current colony definitions and selected profile/local contributions; preview and record the resolved selection |
| `overrides` | Derive a new snapshot from the expected active version with validated execution overrides, such as a builder model change |
| `snapshot` | Select a complete immutable version already retained by this trail, including a previous version for rollback |

Execution overrides form a recorded trail-local layer above captured behavioural settings and use existing adapter/parameter validation. They cannot change identity, transport, credential sources, budget ceilings, delivery permissions, or configuration policy. The operator preview shows the resulting resolved change. Applying colony configuration explicitly replaces the previous trail-local behavioural selection with the newly captured input; applying a retained snapshot restores that snapshot's complete selection.

Preparation captures source content at request time and validates it, including the roles and sectors needed by remaining unstarted tasks. Source profile selection cannot retarget the owning colony's transport or home; infrastructure values in a profile remain separate from the procedure being captured. A pending action never re-reads changing files at activation. The candidate is revalidated against current remaining accepted work before activation; incompatibility rejects the action and leaves the old version active. Applying configuration never edits application code, resets task state, creates a worktree, replenishes honey, launches an agent, or automatically retries a task.

#### Cutover and execution ownership

A new task attempt or independent direct-dispatch chain captures the active reference and revision when admitted to execution. Each attempt has a runtime-owned `attemptId`; independent chains likewise have durable execution ownership. Planned or ready tasks that have not started do not pin an execution version merely by existing in the ledger. All runs in an admitted attempt, including builder, guard, rework, and associated HITL review, retain that attempt's version. Run records and event provenance preserve this ownership through completion and deferred publication.

V1 activates every apply action at a safe boundary. While an action is pending, the Reactor stops admitting new task attempts and independent work chains, but continues dispatch and control needed to finish existing chains under their old context. Safe means no executing or suspended agent invocation, live task attempt, outstanding proposal verification, or human review requiring the old context remains. Operator review, cancellation, honey top-up, and kill remain available; pending application must not prevent an old guard from running and thereby deadlock its own cutover.

If the boundary is already safe, activation is immediate. Otherwise the action reports `pending` with its blocking attempts or reviews; it never cancels them automatically. The operator may cancel the apply action and continue on the old version. A failed or explicitly cancelled attempt is closed once its invocations and required side effects have drained. An explicit retry after activation starts a new attempt on the active version; rework inside an open attempt keeps its existing version. Continuing an already-open interactive session keeps its captured version and blocks cutover until that invocation is closed.

Activation serializes with admission of new work, so no run can be routed with one version and launched with another. Duplicate or late events from a closed attempt retain their recorded ownership and normal deduplication semantics; they cannot be reinterpreted as new work under the latest active registry or reopen a closed attempt implicitly. Restoring an earlier configuration changes future procedure only; it does not undo completed code, task results, emitted events, or external effects.

#### Action record and policy

Apply configuration is an operator action in v1, exposed through the colony's normal CLI and Console controls. `SIGNAL/trail.config.apply` represents the request with `requestId`, target trail, `expectedRevision`, and source selection. Its recorded actor must come from the operator control path; candidate bee declarations do not grant authority to reconfigure a trail. Requests for killed trails are refused. Agent-initiated application and changing pinned policy are deferred.

Each request records a `changeId`, source provenance, previous and proposed references, expected revision, actor, optional reason, requested time, outcome, and applied time when relevant. Outcomes distinguish `pending`, `applied`, `unchanged`, `rejected`, and `cancelled`. Only one action may be pending per trail in v1. A stale expected revision or a competing request is refused rather than replacing an action; duplicate delivery returns the recorded result, and identity reuse with different contents is refused.

The validated snapshot is durable before activation. Switching the active reference, advancing the revision, recording the transition, and releasing the admission barrier form one recoverable operation. A request selecting the already-active content records `unchanged` without a new configuration revision. Notification/replay cannot apply the transition twice. Killing the trail cancels a pending action and prevents it from reopening work after restart.

### 4. A snapshot contains the complete procedure

Inputs include the portable manifest, bee and cue definitions, prompt templates and partials, the selected profile recipe, applicable local template overrides, effective model aliases, and the skills and supporting files the configuration makes available. Resolution preserves existing profile and local-override precedence. Cue definitions captured here do not register new colony-wide schedules or ingress handlers.

The snapshot retains both inputs needed to derive a candidate and the resolved execution view, including any recorded trail-local execution overrides. A profile name alone is insufficient: relevant contents and non-secret contributions are frozen. Private skills and other private inputs are retained with the trail under appropriate access restrictions and private classification; they are not added to shared Git history or ordinary exports. A fork retains its own copies of these inputs as well.

Runtime records, worktrees, caches, checkpoints, credentials, and unrelated machine files are excluded. Credential values remain supplied by the colony's machine-local mechanism; environment references can be recorded without copying secret values. Snapshot identity fixes the procedure and effective non-secret configuration, not every property of the host or external model service.

### 5. Apply a file delta, then freeze the result

Supported delta forms are `patch`, naming an existing proposal patch record from spec 039, and `none`, creating a control arm. The runtime copies and identifies the patch bytes and records proposal identity and approval/publication state at creation. Later deletion or state changes of the proposal do not rewrite the evidence.

The runtime retains a complete local copy of the source snapshot as the fork's base, and derives the effective snapshot separately. It applies the patch to the source snapshot's portable tree, then resolves it with the source's frozen machine-local contributions and the same precedence rules. It does not reselect the current process profile or re-read local overrides. Candidate changes shadowed by a frozen higher-priority layer are reported in the plan and resolved difference; they cannot be advertised as effective behavioural changes.

The delta may add, replace, or delete configuration files and referenced procedures. It may not touch application source, runtime data, or machine-local credentials. Paths escaping the allowed tree, including through links, are refused. The patch's recorded code base is provenance, not permission to substitute that checkout's configuration for the source snapshot.

A patch that cannot apply to the captured tree is refused without conflict resolution or agent launch. The complete result must pass configuration validation, including role, template, skill, adapter, and completion-contract references. Deleted files have no fallback into the live colony.

The effective snapshot's content determines its `ConfigRef`; `none` retains the source reference. Identical base and effective snapshots may use one physical copy within the fork, with both roles recorded in its manifest. The fork never depends on links into the source trail, live configuration, or execution worktree. Patch bytes and comparison inputs are retained locally as well. A commit on the execution branch is not required for activation or retention.

### 6. Configuration and code bases have separate identities

The default `baseSha` is the source trail's initial code commit. The fork worktree starts there on its own branch. The source's final changes, dirty workspace, runs, comb, and honey are not copied. V1 requires evidence that the source's initial application-code state matched its recorded base; an unrecorded dirty code state is refused as an unsupported source. Uncommitted configuration can still be captured by the configuration snapshot.

The selected source attempt's `ConfigRef` is authoritative even when it differs from configuration committed at `baseSha`, the source trail's initial reference, or its current active reference. The runtime records the code base, source configuration, effective configuration, and delta separately. A code range alone does not identify the effective procedure.

An explicit alternative base is allowed, but its difference is recorded and removes the same-base comparison claim. Missing commits are reported before admission; the runtime never silently uses current HEAD.

### 7. Carry one recorded task input

The fork carries one task's recorded input: title, body, target bee, sector, intent, and review policy, plus `cueId` provenance when present. When the source omitted a bee, its resolved target is captured explicitly rather than re-derived from current defaults. The fork mints a new task identity and keeps source identity separately. The task-input digest excludes execution identities, execution status, and completed-work output.

The default is the recorded root task when there is exactly one unambiguous root; otherwise the caller must select a task. An unavailable target bee or sector in the effective candidate is a validation failure.

The fork selects a recorded attempt of that task. If there is one eligible attempt, it is the default. Otherwise `sourceRun` must name a run belonging to the desired attempt; validation checks task ownership and uses that attempt's captured task input and `ConfigRef`. A guard run identifies its owning attempt rather than replacing the task with the guard's event-derived prompt. The runtime must not choose the source trail's latest active version or an arbitrary attempt merely because several versions exist.

Selecting an attempt chooses recorded task/configuration evidence; it does not restore the source at that execution point. Code-base selection keeps the rules above. A fork that resumes a graph of completed tasks and artifacts from a checkpoint is a separate deferred capability.

V1 refuses a selected task with dependencies on other tasks, and refuses a synthetic final-review task. It does not copy dependency identities into another trail or silently drop prerequisites. Whole-plan forks and captured prerequisite inputs are deferred.

### 8. Resolve configuration before routing or validation

Every configuration-dependent entry point first resolves execution ownership. Work belonging to an admitted attempt or direct chain uses its recorded reference and revision; new independent work uses the trail's active binding when admitted. This covers task-plan validation, default bee and sector selection, task and direct subscriptions, prompts and intents, skill delivery, publish checks, proposal synthesis, completion contracts, review gates, and auto-invites.

A fork event is matched against its pinned registry only. Other attempt-owned events use their recorded registry, including after a later active binding has been applied. The Reactor must not also route them through the current colony or active trail registry. Run metadata and runtime-generated, queued, or deferred event records retain enough provenance to recover the owning attempt/chain, `ConfigRef`, and revision; caller-supplied provenance cannot override the recorded origin. Read-only colony views may observe all versions; cross-trail reactions that launch work require an explicit supported scope rather than an accidental second routing pass.

While application is pending, new independent work is held before configuration-dependent validation and routing. Already-planned but unstarted tasks are revalidated against the candidate before cutover; existing attempt-owned events continue under their captured context. Configuration application and ordinary operator controls use canonical policy and cannot be blocked by missing candidate subscriptions.

The same context passes through prepare, run, and finalize. Caches may share immutable parsed content and routing rules by `ConfigRef`. Materialization paths, workspace, and record ownership remain trail-specific: a shared cache must not make a fork read files from another trail's directory. Dispatch must not temporarily replace the Reactor's colony, registry, dispatcher registry, or process-wide profile: another trail may execute concurrently.

### 9. Every execution path preserves the binding

AFK, direct dispatch, manual bee runs, interactive sessions, retries, and helper commands all resolve explicit execution ownership. A new independent invocation captures the active version; continuation inside an open attempt keeps its version. An eligible explicit retry of a closed attempt records a new attempt using the active version; this cannot revive a killed trail. A process profile never replaces a binding during ordinary execution; the operator selects a different profile through Apply configuration. Status and inspection can read a trail without selecting its profile.

Child processes receive canonical colony identity and trail/run context explicitly. Publishing, deferred flush, task operations, and skill resolution from the checkout still target the owning colony and pinned configuration. The runtime does not globally change environment variables to select another trail.

Prompt and command contexts expose the resolved configuration location separately from workspace and record locations. Runtime-delivered templates, skills, and procedures resolve from the immutable materialization. Arbitrary relative reads by an external tool follow that tool's working-directory semantics; they do not select another configuration binding.

### 10. The fork owns its execution root

All fork runs execute within its checkout, with sectors resolved underneath it. A bee with `worktree: false` uses the fork's `WorkspaceRoot`; a bee with `worktree: true` reuses that checkout. Neither chooses the primary colony checkout, and the runtime creates no nested colony or per-role worktree.

Proposal affinity is evaluated in this execution context. Root and isolated declarations retain their validation rules, while the root for a fork's root proposal is its own execution root. Guards, diff collection, task-review validation, and HITL review must resolve the same scope. The original bee declaration is recorded rather than rewritten to force another proposal kind.

Configuration files edited in the fork's execution checkout are candidate output. They do not alter later dispatches in the pinned experiment. Testing that new configuration requires another admitted experiment. An ordinary trail can explicitly apply a new version through the operator action; file edits alone never activate it.

### 11. Separate experimental behaviour from colony policy

The delta may change prompts, bee parameters, skill lists, subscriptions, publications, completion contracts, sectors, and the default bee. It may change the guard; the configuration difference must make that visible.

Identity, transport, home binding, credential sources, admission, budget ceilings, and delivery permissions come from canonical colony policy. A delta attempting to alter those controls is refused. In particular, changing the manifest's slug, NATS configuration, energy default, or delivery policy cannot redirect events, grant honey, or enable publication.

Declaring a capability in a candidate cannot grant authority to create further forks or bypass colony limits. A fork of a fork is refused in v1.

### 12. Internal proposals are observable; delivery is disabled

Fork runs may publish proposal and verification events within their trail so guards and rework can execute. Events and captured patches remain visible in the shared Console and records with experimental lineage.

Every platform delivery entry point checks experimental policy before merge, branch push, or pull-request publication, including manual approval and reconciliation. Internal review can complete experimental work without delivering its branch. Promotion is a separate proposal reviewed through the normal colony workflow.

### 13. Shared transport requires trail-aware consumers

The Reactor provides logical separation by resolving each event's trail context. Console, metrics, and audit reads can observe all trails; work-producing consumers respect experimental scope and policy. Trail lookup exposes lineage for this purpose.

Built-in gateways and side-effecting consumers apply the same rules before acting. Independent NATS subscribers still receive matching subjects and must implement this contract themselves. This design does not promise broker-level separation from arbitrary subscribers or filesystem sandboxing of external agents. Separate transport namespaces and operating-system sandboxes are outside v1.

### 14. Admission, honey, and concurrency belong to the colony

An agent request is eligible only when its sender resolves to a recorded bee run and canonical colony policy grants that role `SIGNAL/trail.fork` publication. A claimed role or a grant added by the candidate is insufficient. Empty or absent declarations grant no eligibility. This is an enforced action check; ordinary publish declarations retain their advisory behaviour.

The fork gets a fresh reserve. The initial budget defaults to the canonical colony's resolved energy default and cannot exceed that ceiling; source honey is untouched. Operator top-ups remain explicit ordinary trail controls. Configuration deltas cannot seed or raise a reserve.

The colony enforces a cap on concurrently active forks before making a prepared fork runnable. Slot reservation and any reacquisition on resume must be atomic with admission to dispatch. Exhausting honey pauses dispatch; it does not establish an unlimited new-fork allowance. Concurrency bounds active work, not total monetary spending. Automatic campaigns are outside this spec.

Human admission of bus requests, the concrete active-fork cap, and a possible cumulative experiment allowance remain policy questions below. Until an admission policy is selected, a publication declaration must not be treated as implicit approval to spend.

### 15. Creation is durable and idempotent

`SIGNAL/trail.fork` is a platform control request with required `requestId`, `sourceTrace`, and explicit delta selection; optional fields are `sourceTask`, `sourceRun`, `baseSha`, `budget`, and `title`. The CLI creates a request identity before submission. The colony records one result per identity; repeated delivery returns it rather than minting another trail or reseeding honey. Reusing an identity with different request contents is refused.

Creation resolves recorded inputs, validates and snapshots the delta and effective configuration, prepares the worktree and branch, persists lineage and binding, and passes admission before publishing the new task plan and ready signal. The existing Reactor executes the runnable fork. Preparation never starts another daemon.

Binding and lineage must be durably discoverable before any event can route on the new trail. Complete local snapshot contents and their manifest are published atomically; partially copied directories cannot become a usable binding. Preparation, pending admission, runnable execution, and refusal are distinct states. Interrupted creation or publication recovers under the same request identity without exposing a fork as an ordinary unbound trail, resetting honey, or duplicating the initial task. Refusal launches no agent and records the reason and any resources retained for cleanup.

### 16. Records expose the actual comparison

The versioned fork record and configuration binding expose at least:

| Field | Meaning |
| --- | --- |
| `requestId`, `forkId`, `traceId` | Request identity, experiment identity, and new trail identity |
| `sourceTrace`, `sourceTask`, source attempt and run identity | Source lineage and the selected execution evidence |
| `sourceTaskDigest` | Identity of the carried task input |
| `sourceBaseSha`, `baseSha` | Source's initial code commit and chosen execution base |
| `sourceConfigRef`, source revision, `configRef` | Selected source attempt's configuration and the pinned fork candidate, both retained locally |
| `delta` | Form, immutable patch identity when present, and proposal provenance |
| `profile` | Source profile provenance; snapshot identity captures its contents |
| `budget`, admission state, timestamps | Initial reserve and creation/admission outcome |
| Workspace and branch identity | Execution location and retained branch |
| Comparison differences | Changed base, effective configuration differences, and evidence limitations |

Every trail binding records its initial and active references, revision, policy, retained snapshot inventory, pending action when present, and append-only transition history. Every attempt, run request, session, and review context records its actual `ConfigRef` and revision; old records are never re-derived from the active pointer. Apply records are visible in the trail's event history with old/new references, actor, and outcome.

Metrics carry `forkOf` as specified by spec 043 and reference the configuration history. A trail using several versions is marked as such, with execution records identifying which runs used each version; one current reference must not stand in for the entire history. Metrics and export readers find this evidence without another checkout. A derived lineage index lists a source's forks without rewriting its historical record.

### 17. Comparability has explicit limits

For the default fork, the platform guarantees equal selected task-attempt input, the same recorded initial code base unless overridden, and inheritance of that attempt's configuration with only the allowed recorded delta applied. Both trails use the same metrics schema. Resolved launch provenance exposes relevant differences.

A source trail that applied several configurations has no single procedure for its entire history. The fork records its selected task attempt and source revision. Aggregate source-trail metrics cannot be presented as a baseline that differs only by the candidate delta; a methodology must select the relevant execution evidence. Previous attempts may also have changed the task's starting code or inputs, so selecting a run alone does not establish an equivalent execution checkpoint.

An explicit base override removes the equal-base guarantee. A changed verifier, adapter, model, or skill set is an observable difference, not evidence of improvement. Credentials, external services, tool versions, client-discovered instructions, timing, and model nondeterminism can affect behaviour. One run is not guaranteed to be a statistically fair experiment or produce identical outputs.

Legacy sources lacking captured inputs are refused in v1. Repetition counts, external-input control, scoring, and lesson promotion remain methodology responsibilities.

### 18. CLI and Console use the same trail collection

`paseka trail fork` accepts `--source`, optional `--task`, `--source-run`, and `--base`, exactly one of `--delta` or `--no-delta`, optional `--honey` and `--title`, and `--dry-run`. Preview reports the selected task attempt, source configuration and revision, effective differences, code base, workspace and branch, budget, admission requirements, and comparison limitations. Dry run creates no durable resources and launches no agents.

`paseka trail config apply <traceId>` accepts exactly one source: `--from-colony`, repeatable `--set <key=value>` for validated behavioural overrides, or `--snapshot <ConfigRef>` for a retained version. `--reason`, `--expected-revision`, and `--dry-run` are available. The CLI obtains and submits the expected revision when omitted; an explicitly chosen profile is part of colony-source capture. Preview identifies the exact candidate, effective changes, remaining-work validation, and any blocking execution scope. Existing work keeps running under its old version during preparation.

`paseka trail config show <traceId>` shows initial/active references, policy, revision history, and pending action with blockers. `paseka trail config cancel <traceId> <changeId>` cancels a pending action; it cannot undo an applied transition. Rollback uses apply with a retained snapshot. Console exposes equivalent Apply configuration, history, pending status, and cancellation actions through the same service and revision checks.

`paseka trail show <traceId>` exposes lineage, binding, and state. `paseka trail list --forks` lists experiments; `--source <traceId>` filters by source. `paseka trail drop <traceId>` stops a fork and removes its execution resources under the retention rules below. These operations use the same collection as Console. There is no isolation-tag or fork-plane run flag; execution uses the ordinary colony runtime lifecycle.

Console marks experimental trails and pinned policy, links source and fork, exposes configuration differences, and preserves inspect, kill, honey, and read-only review controls. Pinned experiments reject apply with a clear reason. Delivery controls enforce experimental policy. A dedicated experiment editor or comparison dashboard is deferred.

### 19. Recovery and retention preserve evidence

Durable active bindings, attempt ownership, and transition records are authoritative after restart. Recovery restores pending application and its admission barrier, or a completed transition, without mixing revisions or applying it twice. Missing content blocks the execution scope that needs it instead of falling back to another snapshot. Pending, killed, or refused forks cannot resume through ordinary-event fallback.

Worktrees and branches are disposable under existing cleanup mechanisms. Removing them does not remove retained records or configuration evidence. Ordinary trails retain initial, active, and previously applied snapshots for audit and rollback until whole-trail cleanup; intermediate revision collection is outside v1. Each retained fork owns its selected source and candidate snapshots and delta bytes. Removing the source trail therefore cannot remove the fork's configuration evidence or invalidate execution, even when both trails have identical `ConfigRef` values.

Whole-trail run-data deletion removes that trail's configuration along with its other records. Other trails require no reference updates. V1 has no shared snapshot garbage collector, cross-trail reference counting, or dependency on a global content store. Trail copying and backup can include the complete local configuration; standard export still applies private-input rules and does not promise source metrics or logs that were deleted with another trail.

Dropping an active fork first stops execution and prevents further dispatch, then removes its execution resources. Bus history follows ordinary trace purge and retention. Export honours private-input classification and reports omissions rather than claiming a self-contained snapshot with hidden missing inputs.

### 20. Module responsibilities and compatibility

Colony configuration resolution owns capture, validation, provenance, and snapshot reads. The resolver separates content identity from physical location; runtime consumers do not construct snapshot paths themselves. Runtime owns version selection, Apply configuration, the admission barrier, execution ownership, routing, and dispatch. Worktree management owns explicit-base creation and execution roots. Launch and sessions preserve context across AFK and HITL. Review and delivery enforce attempt ownership and experimental restrictions. Shared trail read models serve CLI, Console, gateways, metrics, export, and cleanup.

Profiles select input for initial capture or an explicit colony-source apply action; this spec does not widen their schema into routing or prompt overlays. An unrelated process profile cannot silently change a bound execution. New finite and standing trails preserve snapshots while allowing explicit application during ordinary work. Legacy unbound trails retain their compatibility path and can opt into future snapshot capture without backfilling history. Ordinary trails need no extra infrastructure.

## Testing Decisions

Tests assert externally visible behaviour: which procedure runs, which events cause dispatch, which checkout changes, which budget is charged, and which evidence survives restart and cleanup. They must not depend on cache layout or helper names. Storage assertions cover trail ownership, complete local copies, and integrity rather than a particular copying or hashing implementation.

Cover at least:

- **Capture:** profiles, local templates, aliases, shared/private skills, supporting files, and executable modes are captured; secrets and runtime data are excluded. Concurrent first events bind one snapshot.
- **Ordinary creation:** a normal new trail initializes initial/active references from captured content rather than a symbolic live selector. Colony edits alone do not update existing trails; explicit application does. Unchanged inputs yield the same `ConfigRef` across independently stored copies.
- **Identity and local ownership:** equal configuration in different trail directories has the same `ConfigRef` despite different trail identities and timestamps; changed content or executable modes changes the identity. Forks own complete base/effective inputs; equal versions can share one copy within that fork.
- **Stability:** later primary-checkout, profile, skill-source, or worktree edits do not alter dispatch. Missing or corrupt content blocks instead of falling back.
- **Apply sources:** colony capture, role/model overrides, and retained-snapshot rollback produce the previewed configuration. Operator overrides take their defined precedence; policy/credential changes and incompatible remaining tasks are refused. A pending action uses prepared content even if its source files later change.
- **Ordinary continuation:** tasks admitted before and after application use their respective references; initial identity, code, workspace, task results, and honey remain unchanged. Rollback restores future settings without undoing completed work. Reapplying active content records an unchanged outcome without another revision.
- **Safe cutover:** pending application holds new work but lets old builders, guards, rework, operator reviews, cancellation, top-up, and kill progress. Blockers are visible; cancel leaves the old reference active. Failed attempts can drain and an explicit retry after activation uses a new attempt on the new reference.
- **Change concurrency:** competing requests, stale expected revisions, repeated request delivery, and reused identities with altered payloads cannot overwrite an action or activate twice. Admission and cutover interleavings never route and launch the same run with different versions.
- **Attempt provenance:** delayed results, suspended or continuing HITL, queued/deferred events, and proposal review preserve their owning configuration through an application. Closed-attempt duplicates cannot reopen work under another registry. Pinned experiments refuse application.
- **Delta:** additions and deletions work; application-source, runtime-data, credential, policy, and path-escaping edits are refused. Conflicts and broken references prevent launch. Frozen higher-priority overrides retain precedence and appear in preview.
- **Provenance:** a source whose effective settings differed from its code commit is reproduced from its snapshot; a control retains its `ConfigRef`; raw and effective differences are distinguishable.
- **Source inputs:** task input and initial code commit get new execution identities; later task-projection edits do not rewrite captured input. A selected source run resolves its owning attempt and version even after the trail's active reference changes; ambiguous attempts require selection. Changed bases are labelled; missing history, unrecorded dirty code, ambiguous roots, dependencies, and final-review tasks fail clearly.
- **Routing:** ordinary and fork trails with different subscribers for the same kind dispatch only their own roles. Include task-plan validation, defaults, direct dispatch, auto-invites, proposals, and completion.
- **Concurrency:** interleaved task/direct runs with different configurations do not exchange registries, profiles, prompts, or finalization behaviour.
- **Workspace:** worktree-enabled and root-working roles, including guards, operate in the fork checkout. Platform workspace operations leave the primary checkout and source worktree unchanged.
- **Execution paths:** manual runs, HITL, retries, emit, and deferred flush preserve colony, selected attempt/version, audit location, and workspace. New independent executions use the active binding; in-chain continuation keeps the old one. A profile selected outside the apply action cannot replace either implicitly.
- **Admission and energy:** sender identity and canonical grants are enforced; candidate grants cannot escalate authority; source honey is untouched; initial budgets and active-fork limits are atomic.
- **Delivery:** internal proposal/verification choreography works, while CLI approval, Console approval, and reconciliation cannot merge, push, or publish the fork branch.
- **Recovery:** duplicate fork/apply requests, interrupted snapshot publication or active-reference transitions, and Reactor restarts preserve action outcomes and the admission barrier without exposing partial snapshots, losing attempt ownership, duplicating tasks/honey, or applying a transition twice.
- **Controls:** apply, cancellation, history, kill, top-up, source/fork listing, consumer scope, and settled-trail handling use the common collection. Multi-configuration trails expose execution/version associations and cannot claim one current configuration for aggregate source metrics.
- **Retention:** worktree pruning leaves initial, active, and historical configuration evidence readable. Deleting the source after a fork is prepared leaves its selected source/candidate versions and delta available, including with a previously populated shared cache. Whole-trail deletion removes only that trail's copies; active drop stops dispatch first; private export omissions are explicit.
- **Preview and compatibility:** dry run makes no durable changes; no extra home, stream, prefix, or Reactor is created; legacy unbound trails retain their behaviour.

Prior art: configuration/profile merge tests, task-ledger KV and honey tests, task/direct Reactor dispatch tests, proposal-workspace tests, launch/session fake-adapter tests, proposal-record tests, trail-event reads, and review/delivery tests. Real model calls are unnecessary.

## Out of Scope

- A separate colony, Reactor, bus namespace, home, or stream per fork or experiment plane.
- Broker-level tenant isolation and operating-system sandboxing of external agents.
- Mutating an existing snapshot, automatic live reload from colony/worktree edits, or replacing the context of an in-flight execution.
- Agent-initiated configuration application, changing pinned policy, and bypassing the safe boundary for selected fast-changing fields in v1.
- Widening the existing profile schema.
- A shared configuration store, cross-trail file deduplication, reference counting, and snapshot garbage collection in v1.
- Whole-plan forks, dependency snapshots, copied combs, or replaying a source's completed solution.
- Continuation forks from execution checkpoints: restoring task-ledger state, artifacts, intermediate code, or suspended agent processes at an arbitrary point is a separate capability.
- Best-effort reconstruction of legacy historical configuration.
- Moving Git refs as live selectors; released-configuration import can later prepare a snapshot.
- Forks of forks, automatic campaigns, experiment queues, and schedulers.
- Merge-back, push, pull-request publication, or automatic promotion of experimental output.
- Scoring, pass@k, benchmarks, repetition policy, and lesson-selection methodology.
- A dedicated experiment editor, comparison dashboard, or colony-wide snapshot catalog/management group.

## Further Notes

### Open decisions

1. **Human admission of bus requests.** A canonical publication declaration establishes eligibility. Should every request require release by a Beekeeper, or may explicit colony policy pre-authorize bounded requests? Recommended initial behaviour: CLI creation is an operator action; agent requests remain pending for human admission. An unapproved proposal may be tested after admission; proposal approval and experiment spending are separate acts.
2. **Active-fork cap and spending policy.** Choose the configuration surface and default cap before implementation. Concurrency and honey do not establish a monetary ceiling or cumulative allowance; repeated automatic experiments would need another policy.
3. **Independent consumers.** Inventory built-in gateways, notifiers, and external side-effecting integrations. Define their explicit experimental scope before enabling shared-bus forks; a NATS subscription alone does not implement the resolver contract.

### Storage decision

V1 uses a local collection of immutable snapshots inside each trail. Its manifest identifies initial, active, source-baseline, and candidate roles by content reference and records application history; those roles do not require duplicate mutable directories. This keeps inspection, backup, rollback, and deletion local to the record and avoids shared-store lifecycle machinery. The tradeoff is duplicated files across trails and retention of old versions within a trail.

Content identity remains independent of location, so equality checks and caches can reuse a `ConfigRef` without physical sharing. A shared store can be considered later if measured snapshot sizes and trail counts justify it; introducing one must preserve resolver semantics, private-input handling, and retained-fork independence.

### Relationship to adjacent work

- [Spec 043](043-trail-observability.md) supplies metrics, truthful outcomes, and work-completion observation before final approval/delivery, with separately accounted analysis in the same trail. This spec adds configuration/revision evidence and experimental lineage, including the fact that one ordinary trail may contain several procedures.
- [Spec 041](041-skills-layer.md) supplies skill resolution and attribution. Captured skill bytes and provenance are inputs to a pinned procedure; a name or mutable absolute path alone is insufficient.
- [Spec 034](034-swarm-mode.md) owns fan-out and winner selection. A fork supplies one task with one pinned configuration; campaigns remain separate work.
- The self-improvement idea's R5 is a new trail with a recorded configuration delta. R6 is fulfilled here by a trail-owned execution context and workspace under the existing colony. Earlier descriptions of a mandatory separate plane, bus, and home are superseded by this model.
- The underlying capability is versioned configuration with explicit application during ordinary work and immutable execution provenance. Forking adds pinned experimental policy, selected source evidence, lineage, delta preparation, and admission; runtime does not need to know the methodology requesting the experiment.
