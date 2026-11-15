---
name: paseka-event-emit
description: Publish one Paseka bus event and prove it landed. Use when a bee's prompt names a SIGNAL, INSIGHT, or VERIFICATION to publish — address the envelope from the run, pick live or deferred delivery by what the event is, and run the command that shows the record accepted it.
metadata:
    approved_at: "2026-11-15T07:38:06Z"
    procedure_hash: cb2b9430792ac306bba742fcd06bc88882d78abb35c81379f5da2711ccb74c36
    role: archivist
    source: proposal
    trail: trace-01a1da38673703e4
---

# Publish a Paseka event

An event is the colony's record; the summary pasted into chat is not. Publish
one event per decision, prove it landed, then write the summary.

The procedure travels: it names no role and no documentation path, so the same
file serves a bee in any colony. What differs between colonies is which kinds a
bee publishes, and that comes from the bee's prompt, not from here.

## Which kind, and who decides

The bee's prompt names the events its run must publish, and — when the role
declares one — its completion contract fails the run when a required event is
missing. Those two are the authority.

A role's `publishes` list is **advisory**. It decides what the runtime
auto-publishes on the run's behalf (a `MUTATION` proposal, a synthesized
`run.summary`) and which undeclared domain kind raises a warning. It does not
refuse a kind the prompt names, and a role that declares nothing still publishes
what its prompt requires. Read it to know what will be published *for* you, never
to decide whether you may publish what the prompt named. When the two disagree,
the prompt wins, and the disagreement is a note for the Beekeeper — not a reason
to drop the event.

`references/` covers the shared platform kinds a bee usually publishes. Open
only the file for the kind you are about to publish:

| Kind | Reference |
| --- | --- |
| `context.note`, `run.summary` | `references/insight-run.md` |
| `trace.title`, `trace.summary`, `worktree.branch`, `pr.body` | `references/insight-trail.md` |
| `task.plan` | `references/task-plan.md` |
| `task.ready` | `references/task-ready.md` |
| `verification.success`, `verification.failed`, `review.note` | `references/verification.md` |
| `task.completed` | `references/task-completed.md` |
| `skill.proposed` | `references/skill-proposed.md` |

Three rules keep the catalog honest:

- A kind the prompt does not name is another bee's job. Publishing it
  re-triggers that bee, and a second gate decision reopens finished work.
- A colony-defined kind — accepted as an envelope, absent from the platform
  catalog — exists only in the prompt and bee YAML that introduced it. This
  skill has nothing to add beyond the envelope and the delivery choice.
- Where a colony documents its own kinds, that document wins over these
  references. Nothing here depends on it existing.

`MUTATION` is normally synthesized by the runtime from the working tree after a
successful run. Hand-publish it only when the prompt says to.

## The three values every emit needs

An event cannot be addressed without them, and none of them is `$PWD` (in a
worktree, `$PWD` is the worktree):

```bash
ROOT=$(dirname "$(git rev-parse --path-format=absolute --git-common-dir)")  # the colony, worktree or not
TRACE=trace-…        # the Flight trail line of your prompt
AGENT=…              # your agent run id
SKILL_DIR=…          # the directory holding this SKILL.md
```

`SKILL_DIR` is the path the run's skill advertisement printed for this file,
minus `/SKILL.md`, or the `dir:` line that `paseka skills load` printed. Every
`references/…` and `scripts/…` path below is relative to it. That advertisement
is the route to this procedure; a client's own skill tool is not guaranteed to
resolve a colony skill, so read the advertised path when it cannot.

The same three ids are what the prompt header already prints. Fill `TRACE` and
`AGENT` from it before running anything below.

Only the **script** adapter exports these as `PASEKA_COLONY_ROOT`,
`PASEKA_TRACE_ID`, and `PASEKA_AGENT_ID`. An LLM bee reads them from its own run
header and passes them explicitly; a copied example's ids publish into a trail
that is already over.

## 1. Anchor at the colony root

Pass `--root "$ROOT"` (the script adds `-C`) or `-C "$ROOT"` to every subcommand.
A run working in `.paseka/worktrees/<traceId>/` cannot resolve the colony from
its cwd — the home config names the primary checkout — and the CLI answers
`colony: home config points to "…", but repo is "…"`. Compute the *commit* SHA
in the worktree; publish from the colony root.

## 2. Validate, then emit

`validate` writes nothing. Run it before `emit`: a bad field then costs one
round trip instead of a half-published event.

```bash
paseka -C "$ROOT" event validate --stdin <<EOF
{"traceId":"$TRACE","agentId":"$AGENT","type":"INSIGHT","payload":{"kind":"context.note","summary":"…"}}
EOF
```

`event validate --stdin` reads the **whole envelope**. `scripts/emit.sh` reads
the **payload only** and validates the envelope it builds. Piping a payload-only
object into `validate` fails on the missing envelope fields; that is a
copy-paste, not a payload defect — either wrap it in the envelope or let
`emit.sh` do both.

`scripts/emit.sh` does both, builds the envelope, and prints proof:

```bash
printf '%s' '{"kind":"context.note","summary":"The bundle guard runs from web/package.json build, so every build path inherits it"}' \
  | sh "$SKILL_DIR/scripts/emit.sh" --root "$ROOT" --trace "$TRACE" --agent "$AGENT" \
      --type INSIGHT --defer
```

Every envelope carries `traceId`, `agentId`, `type`, and `payload.kind` — nothing
else is added by hand.

## 3. Live or deferred

The choice follows what the event *is*, not which bee publishes it. The same
kind takes the same mode in every run of a colony.

| Use `--defer` | Use live |
| --- | --- |
| A record the trail reads after this run succeeds: `context.note`, `run.summary`, `trace.summary`, `pr.body`, `worktree.branch`, `task.plan`, `task.ready` (when the kick belongs after exit), `skill.proposed`, explicit `artifact.written` | A gate decision, or a fact already true however this run exits: `verification.success`, `verification.failed`, `task.completed`, `review.note` beside a live gate |
| The event is a record, not a trigger | The event is a trigger or a commit record, not prose |
| Runtime flushes FIFO on successful exit; a failed or cancelled run leaves it queued | Published immediately, even if the run later fails |

Why the two live exceptions:

- `task.completed` records a commit that already exists. A run that fails after
  the commit must not take the commit record with it, so do not queue it behind
  the run's exit.
- `verification.*` is the review's outcome: the rework or approval is dispatched
  as part of this run, and a decision left queued looks like an unanswered
  review.

The merge copy is the other way round: `trace.summary` and `pr.body` say the
trail is ready, so a run that fails after publishing them must not leave that
claim behind. Defer them.

Live-only platform kinds refuse `--defer` with `defer_denied`: `system.kill`,
the `energy.*` family, `cue.ingress`, `session.invite`, `beekeeper.ready`, and
`task.status`. Do not try to queue them.

Deferring needs an existing run directory `.paseka/runs/<traceId>/<agentId>/`.
A trail-level run with no run directory publishes live.

## 4. Prove it landed

| Delivery | Command | Good result |
| --- | --- | --- |
| deferred | `paseka -C "$ROOT" event pending --trace "$TRACE" --agent "$AGENT"` | `{"ok":true,…,"count":1,"kinds":["context.note"]}` |
| live | `jq -r 'select(.payload.kind=="context.note")|.createdAt' "$ROOT/.paseka/runs/$TRACE/$AGENT/events.ndjson" \| tail -1` | a timestamp, not empty |
| live, bus up | `paseka -C "$ROOT" verify "$TRACE"` | the event in both record and stream |

A deferred emit's `ok:true` proves the event is **queued**, not delivered. The
flush is proven after the run exits, by the kind appearing in the trail record:

```bash
jq -r 'select(.payload.kind=="context.note")|.createdAt' \
  "$ROOT/.paseka/runs/$TRACE/trail-events.ndjson" | tail -1
```

Say "queued for flush" — never "broadcast" — until that line shows a timestamp.

`ok:true` without `subject` means the event was recorded locally and never
broadcast: the colony is still correct and the bus was down. Say that; never
claim delivery the result does not show.

## 5. `ok:false` is a fix, not a retry loop

Read `error` and `details[].path`, correct that field, publish once more.
`references/troubleshooting.md` maps every code on this path to its cause.

## 6. Boundaries

- Do not write event JSON to a file; pipe it to the CLI.
- Do not print raw event JSON in the final answer — the summary is prose.
- Do not invent a kind to reach a workaround. A missing capability is a task,
  not a payload.
