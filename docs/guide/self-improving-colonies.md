# Self-improving colonies in practice

> The applied half of [Self-improving colonies](../idea/self-improving-colony.md): how to run the loop on a Paseka colony today, which commands carry each step, and which decisions the platform deliberately leaves to the Beekeeper.

## 1. What this guide is

The idea note asks whether a colony can learn from its own trails. Its answer is a split. **The platform provides capabilities** — it records what a trail cost, delivers a procedure to a role, carries a proposed change as a reviewable artifact, forks a trail, isolates an experiment, and spends a budget. **The methodology provides decisions** — when a trail is worth studying, which failure deserves which change, how many runs count as evidence, what "better" means, and when a hypothesis becomes a rule.

This guide is the methodology. Nothing in it is a runtime feature, and none of it is required to use Paseka. Every mechanism it names is shipped and documented elsewhere; the guide's job is to show how they compose into a loop a Beekeeper can actually run. Treat it as a starting policy, not a doctrine: the loop is content, and content can be replaced.

The short version:

> The platform makes the comparison fair. The methodology decides what better means. The Beekeeper approves the change.

## 2. The idea in one page

An agent is a model plus a **harness** — the prompt, the guardrails, the procedures, the checks, the routing, the completion contract. With the model frozen, the harness is what a colony can improve. Three properties make that safe:

- **The harness is a file tree.** A change to how a role works is a change to committed files under `.paseka/`, diffable and revertible. It is not a mood or a memory.
- **The base stays stable.** Every experiment starts from the committed state; a fork never edits the live colony as a side effect. Only the lesson library carries forward.
- **Refusal is a normal outcome.** Most candidates are worse or unclear. "Keep what we had" is a result, and the library must be able to hold it without calling it a failure.

The loop is: **notice → understand → hypothesize → probe → decide → promote or refuse**. Paseka now records enough for each step, and the probe no longer needs a second checkout: a fork runs in the same colony with its own configuration, worktree, and honey.

## 3. What Paseka provides today

| Idea ref | Capability | Shipped surface | Spec |
| -------- | ---------- | --------------- | ---- |
| R1 | Read a trail as one object | `paseka inspect metrics --trace <id> [--json] [--write]`; `.paseka/runs/<traceId>/metrics.json` | [043b](../specs/043b-trail-metrics-and-work-completion.md) |
| R2 | Know the working result is ready | `SIGNAL/trail.work.completed`; optional direct reaction with `purpose: analysis` | [043b](../specs/043b-trail-metrics-and-work-completion.md) |
| R3 | Find the moment it diverged | `trail-events.ndjson`, run directories, completion checkpoint, `paseka export --include patches,transcript,reasoning,config,skills` | [040](../specs/040-trail-event-log.md), [043b](../specs/043b-trail-metrics-and-work-completion.md), [048](../specs/048-native-session-transcript.md), [041](../specs/041-skills-layer.md), [042](../specs/042-trail-fork.md) |
| R4 | Propose, never commit | proposal patch records (`proposals/<agentId>.patch`); `INSIGHT/skill.proposed`; `paseka skills proposals` | [039](../specs/039-proposal-patch-records.md), [041](../specs/041-skills-layer.md) |
| R5 | Probe the same task | `paseka trail fork --source <id> --delta <patch> \| --no-delta` | [042](../specs/042-trail-fork.md) |
| R6 | Isolate the experiment | pinned configuration, own worktree, own capped honey; same colony, bus, and Reactor | [042](../specs/042-trail-fork.md) |
| R7 | Spend a bounded budget | honey reserve, durable seed evidence, pause/resume observations | [016](../specs/016-cue-layer.md), [043b](../specs/043b-trail-metrics-and-work-completion.md), [044b](../specs/044b-standing-supersession-and-energy-observations.md) |
| R8 | Deliver a procedure to a role | `.paseka/skills/`, per-role gate, `paseka skills` group, trial into one trail | [041](../specs/041-skills-layer.md) |

R6 changed shape during implementation. The idea note described a separate bus, home, and checkout; the shipped fork keeps one colony, one Reactor, one bus, and one trail collection, and gets isolation from a pinned configuration, a separate worktree and branch, and its own honey reserve. The comparison is still fair, and the operator has one place to look.

Four recent ships make the loop substantially less noisy:

- **[043a — truthful run records](../specs/043a-truthful-run-records.md).** A run that broke its completion contract now reads `failed` on disk, with a separate `contract` verdict. A scan over run directories no longer reports success for a run the runtime rejected.
- **[045 — the closing phase](../specs/045-trail-closing-phase.md).** The wrap-up round is a real phase before the merge gate. `byPurpose.work` is the completion baseline; closing is measured separately as trail overhead.
- **[046 — the check gate](../specs/046-bee-check-gate.md).** A Bee can declare a mechanical `checks:` command. A failure is a recorded `checks_failed` rejection with a `checks.log`, not a judgment call buried in a review.
- **[050 — current attention](../specs/050-trail-attention-state.md).** A failure stays open only until a later success of the same task supersedes it. A healed trail is quiet; a damaged record is a separate `record error` axis.

Still on the drawing board, and therefore not assumed by this guide:

| Not shipped | Consequence for the loop |
| ----------- | ------------------------ |
| [Swarm mode](../specs/034-swarm-mode.md) — fan-out and a blind judge | You run arms manually and compare with metrics. There is no automatic best-of-N. |
| [Native skill bridge](../specs/049-native-skill-bridge.md) | Delivery is the system-prompt advertisement (`path` or `command`), not a client discovery directory. |
| [Delivery outcomes](../specs/047-delivery-outcomes.md) | Work completion precedes final approval and delivery; do not read a completion as a merge. |
| A lesson library or promotion service | Keep the journal in the colony; promotion is a reviewed commit. |

## 4. The loop at a glance

```text
      a trail ends
           │
           ▼
   notice ── is this trail worth a retrospective?
           │      paseka status · trail list · inspect metrics
           ▼
   understand ── where did the run diverge?
           │      trail show · inspect metrics --json · export --include ...
           ▼
   hypothesize ── one change, in the cheapest component
           │      prompt · skill · script · check · contract · routing
           ▼
   probe ── the same task, from the same base, with a recorded delta
           │      trail fork --no-delta (control) · --delta <patch> (candidate)
           ▼
   decide ── better, worse, unclear, or confounded?
           │      byPurpose.work · task retries · gate verdicts · outcome
           ▼
   promote ── a reviewed commit, or a skill approval + role list
      │
      └── refuse ── keep the base, record no-entry, spend no more honey
```

| Step | Question | Main command | Evidence left behind |
| ---- | -------- | ------------ | -------------------- |
| Notice | Is this trail interesting? | `paseka status`, `paseka trail list`, `paseka inspect metrics` | attention state, metrics warnings |
| Understand | Where did it go wrong? | `paseka trail show`, `paseka inspect metrics --json`, `paseka export` | events, run records, patches, transcripts |
| Hypothesize | What is the smallest change? | edit a skill, prompt, check, or contract | a proposal, not a commit |
| Probe | Did the change help? | `paseka trail fork --dry-run`, then create | `fork.json`, two metric projections |
| Decide | Is the difference real? | compare `byPurpose.work` and task dimensions | a decision in the journal |
| Promote | Where does it land? | `paseka skills approve`, a reviewed commit, `trail config apply` | store + role list, or a commit |
| Refuse | What did we learn? | `paseka skills reject`, `paseka trail drop` | a recorded refusal, retained evidence |

## 5. Notice: pick one trail, not a dashboard

A colony can produce more trails than a Beekeeper can study. The first discipline is to study few of them. A retrospective is worth it when a *pattern* is visible, not merely when a trail was expensive.

Cheap signals, in rough order of usefulness:

| Signal | Where to read it | Why it matters |
| ------ | ---------------- | -------------- |
| The same task was rebuilt twice or more | `tasks[].retries >= 2` | A retry is repeated work; the rework loop is the first thing to study. |
| The mechanical gate rejected the run | `runs.checksFailed`, `tasks[].gateRejections`, `checks.log` | A failing check is a concrete, reproducible defect, not a taste dispute. |
| Verification failed repeatedly | `verificationFailed`, `tasks[].verificationFailed` | The contract or the review rubric may be underspecified. |
| The contract could not be evaluated | `runs.contractUnavailable`, `runs.contractFailed` | The evidence is missing or unreadable; fix the record before learning from it. |
| Work honey is an outlier for the class | `byPurpose.work.honeySpent`, `.class.bee`, `.class.intent` | "Expensive for its kind" needs a kind; `class` is the entry task descriptor. |
| The trail ran more than one procedure | `configuration.multipleVersions`, `configuration.used` | One baseline cannot describe a multi-version trail; select the attempt, not the aggregate. |
| The trail's kind is unclear | `cueId`, `cueIngresses`, `paseka trail show` origin | A later cue is an addition, not the origin. Compare like with like. |
| Most of the wall time is waiting | `wallTimeSec`, `activeTimeSec` | Waiting for a human is often the real cost, and it is not fixed by a prompt. |
| The record is partial or damaged | `coverage`, `warnings`, `record error` attention | Do not build a lesson on evidence you cannot read. |

A small reader over all retained trails, sorted by work honey:

```bash
for t in .paseka/runs/trace-*; do
  id=$(basename "$t")
  paseka inspect metrics --trace "$id" --json |
    jq -r '[.traceId,
            (.class.bee // "-"),
            (.class.intent // "-"),
            (.byPurpose.work.honeySpent // 0),
            (([.tasks[] | select(.purpose == "work") | (.retries // 0)] | add) // 0),
            .runs.checksFailed,
            .runs.failed] | @tsv'
done | sort -k4,4nr
```

The runtime deliberately does not compute class medians, percentiles, or "is this expensive" — those are reader concerns. The loop above is a reader. Keep it in a script if you use it often; do not expect the runtime to grow a baseline.

What **not** to study:

- A trail whose failures were all superseded. In Queen Console and Telegram the badge is current; a healed trail is history, not an open problem.
- A trail with a `record error`. Repair or discard the evidence first; a lesson built on a damaged record poisons the journal.
- A single expensive trail with no visible pattern. It may be a one-off. Wait for the second instance.
- A trail whose closing round is still running. The work baseline is not final yet.

Pick **one** trail and write down **one** candidate lesson before opening the editor. The Archivist's own guidance is the right shape: recurring, procedural, and not already written down.

## 6. Understand: read the trail, not the log pile

Start with the trail's own account:

```bash
paseka trail show trace-abc123
paseka inspect metrics --trace trace-abc123 --json > /tmp/trail.json
jq '{class, cueId, work, honey, configuration, byPurpose, runs, verificationFailed, coverage, warnings}' /tmp/trail.json
```

Read in this order:

1. **`work`** — state, stage, revision, and whether a completion exists. A missing completion means the working result was never certified; a `failed` or `blocked` state is not a completion.
2. **`byPurpose.work`** — the baseline for this trail's goal work. Root totals include closing rounds and analysis, so they are the wrong baseline for a completion comparison.
3. **`tasks`** — which task was rebuilt, how often, and whether a gate or a verification rejected it. `executions` and `retries` are per-task and role-independent; a Guard review is not a task retry.
4. **`runs`** — failures with their dimensions kept apart. `failed`, `cancelled`, `contractFailed`, `contractUnavailable`, `checksFailed`, and `checksPassed` overlap; do not add them into a single "failures" number.
5. **`coverage` and `warnings`** — the honest edges. A `null` budget or token count is a missing measurement, not zero.

A trail can have more than one completion. Accepted rework advances `work.revision` and mints a new `completionId`; the older completion stays under `completions/` as history. When you study a trail, pick the completion that matches the work revision you care about, not the newest directory by reflex.

Then locate the divergence:

```bash
paseka export trace-abc123 --include patches,transcript --format md
```

The export is a report, not the archive. The primary evidence stays under the trail:

```text
.paseka/runs/<traceId>/
├── trail-events.ndjson              # the whole-trail record, one line per event
├── origin.json                      # which cue started the trail, if any
├── config/
│   ├── manifest.json                # initial/active ConfigRef, revision, history
│   └── snapshots/<ConfigRef>/       # the exact procedure the trail ran
├── <agentId>/
│   ├── prompt.txt  system.txt       # what the Bee was told
│   ├── request.json                 # model, profile, skills offered, task join
│   ├── status.json result.json      # the truthful outcome and contract verdict
│   ├── checks.log                   # the mechanical gate's output, if declared
│   ├── summary.md                   # the Bee's own account
│   └── skills-used.ndjson           # which procedures the run opened
├── proposals/<agentId>.patch        # the diff each proposal attempt captured
└── completions/<completionId>/      # immutable work-completion evidence
    ├── completion.json
    ├── result.json
    ├── patch.diff
    └── metrics.json
```

For a run that failed its check gate, `checks.log` is usually the whole lesson. For a run that passed its checks but produced the wrong change, `prompt.txt` and `system.txt` tell you what the role was actually told, and `proposals/*.patch` tells you what it actually did. For a run whose *review* went wrong, the trail events carry `review.note` with a closed severity and `human.feedback` with a verdict — both are countable, which is why [043a](../specs/043a-truthful-run-records.md) closed those vocabularies.

If the trail already completed its work, you can study it without touching the worktree:

```bash
jq -r '.work.completion.completionId' /tmp/trail.json
paseka bee run archivist \
  --trace trace-abc123 \
  --purpose analysis \
  --analysis-of <completionId> \
  --body "Read this trail and propose at most one reusable procedure."
```

Analysis runs in the original trail, at the colony (or fork) root, from the retained completion evidence. Its honey, tokens, failures, and notes land in `byPurpose.analysis` and never in `byPurpose.work`; it does not reopen the work revision, hold completion, or block merge or PR publication. A failed analysis is an analysis failure, not a retroactive failure of the work.

You can also let the runtime start the analysis when the milestone fires:

```yaml
# .paseka/bees/steward.yaml
subscribes:
  - type: SIGNAL
    kind: trail.work.completed
    dispatch: direct
    purpose: analysis
```

Automatic reactions are deduplicated per milestone and reaction identity, including after a restart. They are also easy to overuse: a retrospective after every flight trains the Beekeeper to ignore retrospectives. A colony can subscribe the steward and still decide, in its prompt, that most trails produce no entry.

The output of this step is not a fix. It is a **lesson candidate** with three parts:

- **`cue`** — the situation that should trigger the procedure next time ("a builder run whose declared check fails twice").
- **`lesson`** — the smallest rule that would change the behaviour ("read `checks.log` before editing; claim done only after the declared command exits zero").
- **`evidence`** — the run ids, task ids, event kinds, and numbers that support it.

If you cannot fill `evidence` with identifiers, you do not have a lesson yet. You have an impression.

## 7. Hypothesize: one change, in the cheapest component

The [Harness²](https://github.com/google-research/harness2) ablations are blunt: refining a harness without probes and without a library produced results worse than the base. The same discipline applies here. A candidate that changes several things at once is **confounded** — no attribution is claimed, and no lesson is written.

"One change" means one hypothesis, not one file. A skill plus the role-list line that activates it is one change; a skill plus a model upgrade plus a new check command is three.

Route the failure to the cheapest component that can fix it:

| Symptom | Dimension | Cheapest effective component | Where it lives |
| ------- | --------- | ---------------------------- | -------------- |
| The role did not know a flag, command, or file existed | INV — how the agent works | one line of tool guidance | the role's prompt template |
| The same mechanical step is rebuilt by hand | INV | a bundled script, or a declared `checks:` command | a skill's `scripts/`, or the Bee's `checks` block |
| The role repeatedly ignores a rule | INV | a completion contract that requires the event | `completion_contract` in the Bee YAML |
| A failure is specific to one kind of task | DOM — what a practitioner knows | a skill with a name and a "use when" description | `.paseka/skills/<name>/SKILL.md` |
| Two roles need different procedures | DOM | a per-role skill list | `skills:` in the Bee YAML |
| The orchestration misses a hand-off | DOM | routing or an invite | `subscribes`, `publishes`, `auto_invites` |
| The model is too weak for the task | — | a model or reasoning change | `params.model`, `model_aliases`, or a trail-local `--set` |
| The review rubric is vague | DOM | a review prompt with explicit criteria | the Guard's prompt template |
| The check is flaky or mis-scoped | INV | fix the command, not the prompt | `checks.command` |

Why this order:

- A **prompt line** is always-on and costs context in every run. It is right for a rule that applies everywhere, wrong for a procedure used twice a month.
- A **skill** is progressive disclosure: the advertisement carries only name, description, and either a frozen path or a `load` command. The body costs nothing until the Bee opens it. That makes it the right home for a procedure, and the wrong home for a one-line rule.
- A **check** is enforcement, not guidance. If the failure is "the code does not build", a skill that says "make sure it builds" is weaker than a `checks:` command that runs the build and rejects the run.
- A **contract** is for a missing artifact or event, not for quality. It can demand that a summary exists; it cannot demand that the summary is wise.
- A **model change** is a different hypothesis from a procedure change. Do not bundle it. If you want to know whether a better model fixed the task, that is its own experiment.

Write the candidate down before you create it. A dry-run fork preview is the cheapest place to discover that the delta is not what you thought:

```bash
paseka trail fork --source trace-abc123 --delta <agentId-or-ref> --dry-run
```

The preview names the selected source task and attempt, the source configuration and revision, the task digest, the base, the raw/effective/shadowed differences, the workspace, the branch, the budget, and the comparison limits. If the *effective* diff is empty, a frozen higher-priority layer shadowed your change; the candidate would measure nothing. If the patch touches application code, credentials, or colony policy, the fork refuses it. That is the platform protecting the comparison, not a bug.

A delta is a recorded [proposal patch](../specs/039-proposal-patch-records.md), repurposed as a configuration delta. In practice you obtain one by having a run edit the candidate files under `.paseka/` and finish with a proposal; the fork retains the patch bytes and applies them to its own copy of the source snapshot. The proposal is not the experiment. The fork is.

## 8. Probe: the same task, from the same base

A fork is a new trail in the **same colony** with:

- the source task's recorded input, under a new task identity;
- the source attempt's recorded configuration, plus only the recorded delta;
- the source trail's **initial** code commit, not its finished solution;
- its own worktree, branch, and capped honey reserve;
- a pinned policy: configuration cannot be applied to it, and the platform will not merge, push, or publish its branch.

That is the platform's comparability guarantee. It has explicit limits: an explicit `--base` removes the equal-base guarantee; a trail that applied several configurations has no single procedure for its whole history; and models are nondeterministic. One run is not a statistical experiment.

### 8.1 Run a control arm first

Before you believe a candidate, measure the noise. A control fork uses the selected source attempt's configuration and no delta:

```bash
paseka trail fork \
  --source trace-abc123 \
  --no-delta \
  --title "control: no procedure change" \
  --honey 10 \
  --dry-run
```

Then create it (drop `--dry-run`). The reactor must be running for the fork to execute — `paseka run` in another terminal, or an already-running apiary:

```bash
paseka run
```

When the control completes, read it. If the control also fails the same way, the source failure is reproducible and the task is a fair test. If the control passes, the original failure may have been noise, an external service, or a one-off; a candidate that also passes has proved little.

Two limits belong to the interpretation, not the tooling. A fork carries **one task**, so a failure that needed a previous task's output will not reproduce — that is a different experiment. And the fork starts from the source trail's **initial** code commit, so if the failure depended on code produced later in the source trail, the probe is not replaying that condition either. When either limit applies, say so in the journal and do not claim a clean contrast.

### 8.2 Run the candidate

```bash
paseka trail fork \
  --source trace-abc123 \
  --source-run <agentId-of-the-source-attempt> \
  --delta <agentId-or-.paseka/runs/<trace>/proposals/<agentId>.patch> \
  --title "candidate: read checks.log before editing" \
  --honey 10
```

Use `--source-run` when the source task has more than one eligible attempt. The fork records the attempt you selected; a later retry on the source does not change the evidence. Use `--task` when the root task is ambiguous; v1 refuses a task with dependencies, a synthetic final-review task, or a source base it cannot prove. A bare agent id is resolved in the **source** trail's proposal store; when the patch came from another trail, pass the full archive ref `.paseka/runs/<trace>/proposals/<agentId>.patch`. If creation fails after the request identity exists, retry with the same `--request-id` instead of minting a second fork.

### 8.3 Compare the arms

```bash
paseka inspect metrics --trace <control-id> --json > /tmp/control.json
paseka inspect metrics --trace <candidate-id> --json > /tmp/candidate.json

jq -n --slurpfile c /tmp/control.json --slurpfile k /tmp/candidate.json '
  def arm($m; $label): {
    arm: $label,
    outcome: $m.work.state,
    workHoney: $m.byPurpose.work.honeySpent,
    workRuns: $m.byPurpose.work.runs,
    workRetries: (([$m.tasks[] | select(.purpose == "work") | (.retries // 0)] | add) // 0),
    checksFailed: $m.runs.checksFailed,
    verificationFailed: $m.verificationFailed,
    wallMin: ((($m.wallTimeSec // 0) / 60) | floor)
  };
  [arm($c[0]; "control"), arm($k[0]; "candidate")]'
```

Read the comparison in this order:

1. **Outcome first.** Did the task complete, and did the declared checks pass? A cheaper candidate that failed is not an improvement.
2. **The target dimension.** If the lesson was about mechanical rejection, compare `checksFailed` and `workRetries`. If it was about review churn, compare `verificationFailed`, `reviewNotes`, and `humanFeedback`. Pick the dimension before you look, so you do not rationalize afterward.
3. **Cost second.** `byPurpose.work.honeySpent` and `byPurpose.work.tokens` are the work baseline. `wallTimeSec` includes waiting for a human; it is a different question.
4. **Noise.** With one control and one candidate, a difference on a noisy dimension is unresolved. A hard outcome (failed, checks failed, contract unavailable) is not noise in the same way, but one run still does not prove a rule.

The fork's own metrics also carry `forkOf` and `configuration`, so you can prove after the fact which source attempt and which delta produced the arm:

```bash
jq '{forkOf, configuration, work}' <(paseka inspect metrics --trace <candidate-id> --json)
```

### 8.4 When a fork is the wrong tool

- **The change is only a skill and you want a quick trial on a live trail.** `paseka skills approve <proposal> --into <trace> --role <role>` applies one configuration revision to that trail. It is cheaper than a fork, but it is a **before/after** observation, not a same-task comparison: it changes future work, not a replay. It also refuses a pinned trail, a pending configuration change, or an execution blocker. Use it to gather evidence, then fork if the claim matters.
- **The change is a model override on an ordinary trail.** `paseka trail config apply <trace> --set bee.builder.model=<alias>` changes future work in that trail at a safe boundary. It is an operator action with a history, not an experiment. Compare the trail before and after the revision, and say so in the journal.
- **The change needs several tasks or a whole plan.** A fork carries one task and refuses dependencies. The platform does not yet offer a controlled whole-plan replay. Test the smallest task that exposes the procedure, or use a small cue-started trail and label the comparison as uncontrolled.
- **You need best-of-N with a blind judge.** That is [spec 034](../specs/034-swarm-mode.md), still a draft. Today you are the judge, and `inspect metrics` is the score sheet.

## 9. Decide: better, worse, unclear, or confounded

Make the decision before looking at the cost, and write it down even when it is "no".

| Outcome | Decision | What is written |
| ------- | -------- | --------------- |
| The candidate is no worse on outcome and clearly better on the target dimension, and the control shows the failure is reproducible | **Promote as a hypothesis** | a lesson entry with `verdict: promoted`; the change lands in the base only after a reviewed commit |
| The candidate is better but the evidence is one run, or the target moves within control-to-control variation | **Park** | `verdict: parked`; keep the probe, run a second instance before promoting |
| The candidate is worse on a hard outcome, or no better at higher cost | **Refuse** | `verdict: refused`; keep the base, keep the evidence |
| The delta changed several things, the base was overridden, or the arms differ in model/adapter/external inputs | **Confounded** | `verdict: no-entry`; no attribution, no lesson, no store write |
| The trail's record is damaged or the measurement is missing | **No entry** | fix the record first; do not learn from it |

A decision rule that fits small colonies:

1. **Hard outcomes are gates.** If the candidate failed a task, a check, or its contract where the control did not, refuse it regardless of cost.
2. **Promote a procedure, not a fluke.** A procedure can be promoted after one clean contrast, but label it a hypothesis and re-check on the next trail of the same class. A *rule* — "always use model X", "always route this task to this role" — needs a second independent instance.
3. **A difference smaller than the control's own variation is not evidence.** With one control, that means you cannot claim it. Run another control before promoting a cost claim.
4. **Confounded means no entry, not a weaker entry.** An invented attribution poisons the journal for every later trail. The idea note is right to make this the one unforgivable outcome.

Before you promote, all of these should be true:

- [ ] The candidate is one hypothesis, not a bundle.
- [ ] The control arm reproduced the failure, or the claim does not depend on the source failure.
- [ ] The candidate is no worse on the accepted outcome and the declared checks.
- [ ] The target dimension moved beyond the control's variation.
- [ ] The base, model, adapter, and external inputs are the same, or the difference is recorded.
- [ ] The change is reviewable, revertible, and small enough to explain.
- [ ] The evidence is in the journal with identifiers.

Refusal is not failure. The Archivist's shipped `skill` intent encodes this: if a finished trail offers no reusable procedure, it says so and lets the completion contract fail rather than publishing a fabricated proposal. That failure is the truthful result — "no entry" — and should be read as such, not as a broken colony.

## 10. Promote, or refuse: where the change lands

Promotion is the composition step. Harness² composes a final harness from the behavioural contrast between runs; here the composition is a reviewed change to the base — the winning delta and nothing else. The losing arms stay as evidence, not as a synthetic merge of everything that looked good.

### Promote a skill

A skill has two separate acts: **approval** makes it available, **activation** makes a role carry it. Find pending proposals with `paseka skills proposals`; nothing installs itself.

```bash
# 1. Install an approved proposal into the committed store.
paseka skills approve trace-abc123/paseka-check-first

# 2. Make the role carry it (a reviewed edit, then a normal commit).
#    .paseka/bees/builder.yaml
#    skills:
#      - paseka-check-first

# 3. Validate the result before the next trail starts.
paseka skills validate
paseka doctor
```

`approve` writes the store and leaves an uncommitted change; it never commits for you. `approve` does not activate: the role list is the gate, and it is reviewed in the same diff that changes behaviour. That separation is the whole safety story — the blast radius of a bad skill is the set of roles that opted in.

If the skill came from a proposal, its `metadata` carries the source trail, the proposing role, the approval time, and the procedure hash. `paseka doctor` reports a hand edit that no longer matches the hash. Keep the provenance: it is what makes a surprising skill explainable later.

### Promote a prompt, check, contract, or routing change

These are normal colony files, so the promotion is a normal reviewed commit. There is no special "promote" command, and that is deliberate: the base is the committed state, and a human approves the diff.

- **Prompt or contract:** edit `.paseka/prompts/...` or `.paseka/bees/...`, run `paseka doctor`, commit.
- **Check command:** add or fix `checks:` in the Bee YAML. A check is read from the resolved configuration snapshot, never from the workspace under review, so a builder cannot weaken the gate in the diff it is judged on.
- **Model or reasoning:** change `model_aliases` or the Bee's `params.model`, commit, and start a new trail. An existing trail keeps its captured configuration until an explicit `paseka trail config apply`.
- **A trail already in flight:** use `paseka trail config apply <trace> --from-colony` after the commit, or `--snapshot <ConfigRef>` to roll back. Applying configuration never retries, clones, resets task state, or replenishes honey; it changes future work only.

### Refuse a skill proposal

```bash
paseka skills reject trace-abc123/paseka-check-first --reason "the control passed without it"
```

A refusal is terminal for that exact bundle, is recorded in the proposing trail's decision file, and is not shared through git. A changed bundle with a new hash is a new proposal. The refusal belongs with the trail that motivated it.

### Retire a probe

```bash
paseka trail drop <fork-id> --reason "candidate refused"
```

`trail drop` stops the fork, removes its worktree and branch, and keeps its configuration, lineage, task and run records, and comb. The evidence survives; the disk does not. Do not purge the source trail until you are done reading the comparison.

### Make a trial permanent

A trial (`approve --into`) changed one trail's captured configuration. That is evidence, not a promotion. To make the procedure part of the colony, approve it into the store and add it to the role list as above. If the trial failed, apply the previous snapshot to the trail:

```bash
paseka trail config show <trace>
paseka trail config apply <trace> --snapshot <previous-ConfigRef> --reason "trial did not help"
```

### Roll back a promoted rule

```bash
# Neutralise a procedure without deleting its history or its provenance.
paseka skills disable paseka-check-first --reason "did not survive the next class instance"

# Or remove it from the role's list and commit. Remove the store entry only
# when no role references it; the listing shows the blast radius.
paseka skills list
paseka skills remove paseka-check-first --dry-run
```

A prompt, check, contract, or routing rollback is a normal `git revert`. A trail already in flight can apply a previous configuration snapshot; a pinned fork cannot be changed at all. Disabling is usually the right first move: it is reversible, it names the roles whose resolved set changes, and it keeps the evidence for the next retrospective.

## 11. The lesson journal

The platform has no lesson store, and this guide does not invent a runtime one. Keep the journal where the colony keeps methodology notes — a plain Markdown directory in the repository is enough, for example `docs/lessons/`. It is content: reviewable and diffable, and it travels with the repository. If you want a pack to carry the distilled procedure, promote the lesson into a skill or a prompt; those are the units a Nuc carries.

One record per lesson, with the three fields the idea note borrows from Harness²:

```markdown
---
date: 2026-11-08
class: builder/feature
trail: trace-abc123
cue: A builder run whose declared check fails twice in a row.
lesson: Read checks.log before editing; claim done only after the declared command exits zero.
evidence:
  - "trace-abc123: task-002 executions=3 retries=2 checksFailed=2; checks.log names an unused import"
  - "control trace-def456: same task, no delta, checksFailed=1"
  - "candidate trace-ghi789: same task, delta adds paseka-check-first to builder, checksFailed=0, completed"
verdict: promoted
---

The control reproduces the failure, so the task is a fair test. The candidate
passes on its first run with one fewer execution. The skill is installed and
builder lists it; the next builder trail is the re-check. Not yet a rule: one
candidate run is a hypothesis.
```

`verdict` is one of:

| Verdict | Meaning |
| ------- | ------- |
| `promoted` | The change landed in the base (store + role list, or a commit). Re-check on the next class instance. |
| `parked` | The evidence points the right way but is too thin or too noisy. Keep the probe; run another. |
| `refused` | The change did not help, or made something worse. The base stayed. |
| `no-entry` | The comparison was confounded or the evidence was unreadable. Nothing is claimed. |

Two rules keep the journal useful:

- **One lesson per retrospective.** A trail with five findings is a backlog, not a retrospective. The other four can wait for their own evidence.
- **Every claim cites a trail, a task, or a number.** "The build was slow" is not evidence. "`trace-abc123` task-002 retries=2, `checks.log` names an unused import" is.

## 12. Noise, cost, and the settings that are yours

These are methodology choices, not platform guarantees. The values below are a starting policy for a small colony; change them when your models or your domain demand it.

| Decision | Starting policy | Why |
| -------- | --------------- | --- |
| How many trails to study | At most one per retrospective; at most one or two per week | Attention is the scarce resource. A retrospective after every flight trains the Beekeeper to ignore them. |
| What counts as evidence | One control + one candidate for a procedure; two independent instances for a rule | Models are nondeterministic. One run cannot separate a procedure from a lucky sample. |
| What "better" means | Hard outcome first, then the pre-declared target dimension, then cost | A cheap failure is still a failure. |
| How much a probe may spend | Set `--honey` explicitly, below the colony ceiling, and stop at the first hard failure | A fork has its own reserve; it cannot drain the source's, but it can drain the colony's attention. |
| When to refuse | When the candidate is worse, unclear, or confounded | Refusal is the normal result. The base is the safe default. |
| When to roll back a promoted rule | When the next class instance does not improve, or a later trail regresses | A rule that does not survive contact with new work is a hypothesis that lost. |
| Whether to study analysis cost | `byPurpose.analysis` is visible; decide whether it is worth the spend | Analysis is real honey. Studying every trail can cost more than the rework it prevents. |

Two measurement habits matter more than any threshold:

- **Use `byPurpose.work`, not root totals, for a work comparison.** A colony with a closing round enabled pays for a wrap-up on every trail. Folding closing into the work baseline makes the same goal look more expensive on the trails that review more, which is exactly the wrong lesson.
- **Treat a missing measurement as missing.** A `null` budget, token count, or retry count is not zero. The metrics projection reports coverage and warnings for this reason; a partial trail is still readable, but it is not a clean baseline.

## 13. Ship the methodology as a Nuc

The idea note's split says the methodology ships as a **Nuc pack** — a role, its prompts, its skills, its thresholds — installed into a colony like any other pack. The runtime has no feature called "self-improvement"; it only sees a bee, its prompts, and its skills. That is still the right shape, and the pieces are now portable:

```text
steward.nuc.yaml
├── bee: steward (or reflector)
├── prompts: steward.md, steward-intent-reflect.md, steward-intent-skill.md
├── skills: the procedures the role uses to read and judge trails
└── cue: an optional `retro` ingress with its own honey budget
```

A minimal shape, illustrated rather than prescribed:

```yaml
# .paseka/bees/steward.yaml
role: steward
adapter: cursor
prompt_template: steward.md
default_intent: reflect
params:
  model: thinker
worktree: false
publishes:
  - type: INSIGHT
    kind: context.note
  - type: INSIGHT
    kind: skill.proposed
completion_contract:
  required:
    - type: INSIGHT
      kind_one_of: [context.note]
      count: 1
  intents:
    propose-skill:
      required:
        - type: INSIGHT
          kind_one_of: [skill.proposed]
          count: 1
subscribes:
  - type: SIGNAL
    kind: trail.work.completed
    dispatch: direct
    purpose: analysis
```

Export and import it like any other pack:

```bash
paseka nuc export --bees steward --name self-improving -o self-improving.nuc.yaml
paseka nuc import ./self-improving.nuc.yaml --dry-run -v
```

What the pack can do: read a trail's metrics and records, write a lesson or a proposal into the trail's comb, propose a skill, and request a fork if the colony grants `SIGNAL/trail.fork`. What it cannot do: approve itself, write the skill store, raise its own honey above the colony ceiling, or deliver an experimental branch. The Beekeeper remains the only path from a proposal to the base. That is the same line the idea note draws, and it is still the line that keeps a self-improving colony from becoming a self-modifying one.

## 14. Limits and failure modes

- **A completion is not a delivery.** `SIGNAL/trail.work.completed` fires before final approval, merge, and PR publication. The platform never merges, pushes, or publishes a fork's branch; promotion is a separate human act. Do not write "shipped" in the journal because a completion exists.
- **A fork is one task.** It refuses dependencies, ambiguous roots, final-review tasks, a dirty or unrecorded base, and missing source evidence. It is a probe, not a campaign.
- **A fork is pinned.** `trail config apply` refuses it, and editing its checkout does not change its procedure. Testing another candidate means another fork.
- **A used skill is not a followed skill.** Attribution means a file was read, not that the instructions were obeyed. The run record says what was offered; the transcript and load records say what was reached for. Neither proves compliance.
- **Analysis is not free.** It stays in the trail's honey and totals; `byPurpose.analysis` keeps it out of the work baseline, but it still spends the reserve. A colony that studies everything can spend more than it saves.
- **A trial is not a promotion.** `approve --into` writes one trail's configuration and a `trialled` decision. The store is untouched. A trial is evidence, not approval.
- **A refusal is not a gap.** `reject` records a decision; a `no-entry` records a judgement. Neither is an absence of work. A proposal nobody looked at is the only unexamined state.
- **A record error is not a lesson.** `record error` (a conflicting or incomplete `status`/`result` pair) means the evidence is damaged. A later success does not repair it. Fix or discard it before studying the trail.
- **Most candidates are worse.** That is the expected result, not a sign the loop is broken. The base is the safe default, and the library's value is partly the list of things that did not work.

## 15. Where to go next

| Goal | Doc |
| ---- | --- |
| The idea and the platform/methodology split | [Self-improving colonies](../idea/self-improving-colony.md) |
| The skill store and the per-role gate | [Colony skills](skills.md) |
| Forking a trail, configuration revisions, and apply | [CLI — `paseka trail`](cli.md#paseka-trail) |
| Metrics, completion evidence, and analysis | [CLI — `paseka inspect`](cli.md#paseka-inspect), [task ledger](../reference/task-ledger.md#work-revision-and-completion-workrevision-workcompletion) |
| The completion milestone and analysis reactions | [Bee routing](../reference/bee-routing.md#analysis-reactions) |
| A portable methodology pack | [Nuc packs](nuc.md) |
| What the runtime records about a trail | [Spec 043b](../specs/043b-trail-metrics-and-work-completion.md) |
| Fork lineage and configuration evidence | [Spec 042](../specs/042-trail-fork.md) |
