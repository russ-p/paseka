# Colony skills

A **skill** is a reusable procedure the colony owns: a portable [Agent Skills](https://agentskills.io) directory, version-controlled in the repository, that a bee may be given when its role lists it. The format is the industry's, not Paseka's, so a skill written here is readable by Cursor, OpenCode, and Claude Code, and a skill written for one of them can be dropped in unchanged.

Design record: [spec 041](../specs/041-skills-layer.md). This guide covers the store, its validation, the per-role selection gate, the committed ledger, delivery through the run's system prompt and run record in both delivery modes, the script bee's environment manifest, the reading commands (`list`, `show`, `load`, `validate`, `proposals`), installation of an approved proposal (`approve`), the store mutations (`disable`, `enable`, `remove`), the proposal decision (`reject`), the Archivist's proposal intent, attribution, and the trial destination (`approve --into`). Approval writes the store; a trial writes one trail's pinned configuration instead.

---

## 1. The store

One root, in the repository:

```text
.paseka/
├── skills/
│   ├── paseka-release/
│   │   ├── SKILL.md          # required
│   │   ├── scripts/          # optional
│   │   ├── references/       # optional
│   │   └── assets/           # optional
│   └── docs-style/
│       └── SKILL.md
└── skills.index.json         # committed state file, beside the store
```

- **Each immediate directory is one skill**, named for that directory. The store is flat: a category folder would be a second level, so a family is a name prefix (`paseka-*`), not a directory.
- `scripts/`, `references/`, and `assets/` are supporting content and are optional. Any other file or folder inside a skill directory is allowed too; the budgets below apply to all of it.
- The ledger sits **beside** the store, not inside it, so a directory under the store is always a skill and never a bookkeeping file a client might load.
- The store is plain files in the repository. It is not inside a worktree, a run directory, or a cache, so trail and worktree cleanup leave it alone. A trail's configuration snapshot carries it for reproducibility.

## 2. `SKILL.md`

YAML frontmatter, a Markdown body, and nothing Paseka-specific:

```markdown
---
name: paseka-release
description: Run the colony release procedure. Use when shipping a version.
metadata:
  source: hand
---

# Release

1. Run `scripts/checks.sh`.
2. Tag after the merge, not before.
```

| Field | Required | Rule |
| ----- | -------- | ---- |
| `name` | yes | Lowercase letters, digits, and hyphens; no leading, trailing, or doubled hyphen; at most 64 characters. Must equal the directory name. |
| `description` | yes | 1–1024 characters. This is what a bee sees before it opens the file, so make it say when to use the procedure. |
| `metadata` | no | A string-to-string map. Provenance and other colony facts live here; a non-string value is a validation error. |
| body | yes | The procedure itself. It is never inlined into a prompt — a bee reads the file when the task calls for it. |

Other frontmatter keys are ignored, so a skill authored for another client stays readable here unchanged.

The name rules exist because a name is an identity other files and roles refer to: a rename is a new skill and a removed one.

## 3. References

A reference names a skill. It is the same syntax in the shell as in a bee's configuration, so a command cannot disagree with the runtime about what a name refers to.

| Reference | Means |
| --------- | ----- |
| `paseka-release` | That one skill (a bare name). |
| `paseka-*` | Every skill whose name matches the glob. |
| `*` | Every skill in the store. |

`*` is the only wildcard; it matches any run of characters. These are **invalid references**, never literal names:

- a tier prefix — `shared:foo`, `local:foo` (there is one root, so there is one namespace)
- a path — `./foo`, `../foo`, `.paseka/skills/foo`
- a foreign wildcard — `?`, `[]`, `{}`

A pattern that matches nothing is a failure, because a role or a command that names a procedure must not silently receive nothing. `*` alone is allowed to match an empty store.

## 4. Budgets

Fixed constants, not colony settings. They price context, not disk: an advertised skill costs tokens in every run, and that price is only acceptable while the count and the bytes stay small.

| Budget | Bound |
| ------ | ----- |
| One file in a skill directory (`SKILL.md` or a supporting file) | 256 KiB |
| The whole store, ledger excluded | 8 MiB |

A breach is a configuration error. The runtime never resolves a breach by dropping a reference silently.

## 5. Selecting skills for a role

A bee receives a skill only when its role lists it. The list is a gate, not a hint: a skill that is not listed for a bee does not reach it, whatever any client would discover on disk.

`skills` is **presence-aware** — absent, empty, and non-empty mean three different things:

| The configuration says | The role receives |
| ---------------------- | ----------------- |
| no `skills` key | the colony default (`defaults.skills` in `colony.yaml`) |
| `skills: []` (explicit empty list) | nothing — the role opts out, including of the colony default |
| `skills: [a, b]` | exactly `a` and `b`; the colony default does not apply |

A role's list **replaces** the colony default rather than extending it, so "remove this procedure from this one role" is expressible. With neither a default nor a role list, a bee receives nothing: opting in costs one line in a reviewed file.

```yaml
# .paseka/colony.yaml
defaults:
  skills:
    - paseka-release
  skills_delivery: path   # path (default) or command
```

```yaml
# .paseka/bees/builder.yaml
skills:
  - paseka-*        # a family pattern
```

A list entry is a bare name, a family pattern, or `*`. Resolution deduplicates the result and sorts it by name. A missing name, a non-`*` pattern that matches nothing, an invalid skill, or a role resolving to more than 32 skills is a configuration error: the run does not start and `doctor` fails. `*` is allowed on an empty store.

`skills_delivery` selects how the runtime advertises the resolved set — `path` (default) advertises each `SKILL.md` location; `command` advertises `paseka skills load`. It is colony-wide, not per role.

Profiles ([colony layout](colony-layout.md), [spec 027](../specs/027-config-profiles.md)) may override `skills` and `skills_delivery`, with the same tri-state, at the top level (the default list) and under `bees.<role>` (that role's list). Precedence is **home profile > colony profile > committed**. A profile chooses among skills that are in the committed store; it cannot add one from outside.

`skills` in `<role>.local.yaml` is a **load error**, not a silent no-op. A profile is the supported per-machine override, and a misplaced key must not look like a gate.

## 6. Delivery: the system-prompt advertisement

Before a run starts, the runtime resolves the effective set once and renders it into the run's system prompt as a section of its own, appended after the role's instructions. `defaults.skills_delivery` chooses which of two forms the section carries.

**`path`** (the default) advertises the frozen `SKILL.md` location, and the bee reads the file itself:

```text
## Available skills

Each skill below is a reusable procedure this role may use. The procedure itself is not included here; read the file at its path when the task calls for it.

- paseka-release: Run the colony release procedure. Use when shipping a version.
  path: <frozen-config>/tree/.paseka/skills/paseka-release/SKILL.md
```

**`command`** advertises the runtime command instead, and no path at all:

```text
## Available skills

Each skill below is a reusable procedure this role may use. Load one with the command shown when the task calls for it: the command prints the procedure body.

- paseka-release: Run the colony release procedure. Use when shipping a version.
  load: paseka skills load paseka-release --trace trace-abc123 --agent agent-7f2c
```

```yaml
# .paseka/colony.yaml
defaults:
  skills_delivery: command   # path (default) or command
```

- `command` mode makes the runtime the only route to a body: a load is a deterministic fact, recorded on the run, rather than an inference from a client transcript. `path` mode keeps the file readable directly. Both modes are the same advertisement otherwise.
- The mode is colony-wide, not per role. It is validated at load (`path` or `command`; anything else fails the configuration), read from the trail's **frozen manifest**, and a profile may override it — the selected profile is frozen with the trail, so a running trail cannot change mode. A configuration application changes it for later runs only.
- The section carries metadata only — name, description, and per mode either the absolute path or the load command. A body is never inlined: progressive disclosure is what keeps a procedure cheap until a bee needs it.
- The runtime appends the section; it is never written into a template file. A bee whose role declares no `system_template` receives the section as its whole system prompt, and a run whose effective set is empty receives no section at all.
- The composed system prompt is written to `.paseka/runs/<traceId>/<agentId>/system.txt` before the adapter starts, so a script bee has the same auditable file even when its adapter writes no system prompt of its own.
- A script bee also receives the resolved set as data in `PASEKA_SKILLS`: a JSON array of `{"name", "description", "path"}` entries, one per skill, with the same names and descriptions as the advertisement and the absolute frozen `SKILL.md` path in **both** delivery modes. It is absent when the set is empty. A path with a space is one JSON string, so a script parses the value instead of word-splitting it; a script that ignores the variable is unaffected. See [bee config](bee-config.md#4-adapters) for the full script environment.
- In `path` mode the advertised path is inside the trail's **frozen configuration**, not the live store, so a running trail keeps reading the bytes it started with. A store or role-list edit after trail start changes the next trail, not this run; it reaches a trail only through an explicit configuration application.
- The resolved names are recorded in the run's `request.json` as `skills` (omitted when the set is empty). Both the unattended dispatcher and the interactive session manager write it, so an AFK run and a HITL session carry the same field, and it stays readable after the role's list changes.
- A resumed provider session keeps the first turn's context; the runtime records the set but does not re-inject the section. A configuration application changes later runs and new sessions only.
- `load` works in either mode: a `path`-mode run that loads is still recorded, and its used set is the union of transcript attribution and load records, deduplicated by name.

A run that cannot resolve its declared set does not start the adapter: a missing reference, an invalid skill, a pattern that matches nothing, a 33rd skill, a skill file over 256 KiB, or a store over 8 MiB fails the dispatch and names the offending skill or reference. A disabled skill is the one skip: it is omitted and reported, because the disable is the Beekeeper's own recorded decision.

## 7. The committed ledger

`.paseka/skills.index.json` records the one thing a skill file cannot: whether a skill is offered at all. It carries a schema version and, per skill name, when and why the skill was disabled:

```json
{
  "schema_version": 1,
  "disabled": {
    "paseka-release": {
      "at": "2026-11-06T12:00:00Z",
      "reason": "release step 2 is wrong"
    }
  }
}
```

- A **missing** ledger is an empty ledger.
- A **corrupt** ledger — bad JSON, an unknown schema version, or a disabled record without `at`/`reason` — fails every skill command and `doctor`. It is never silently replaced.
- A **disabled** skill stays in the store, keeps its history, and is never offered. A role that still lists it is skipped and reported, not failed: an emergency stop must not become a colony-wide outage.
- Provenance lives in the skill's own `metadata`, and the inventory is the directory listing, so the ledger has no mirror to reconcile.
- The only writers are `paseka skills disable` and `paseka skills enable` (plus `remove`, which deletes a record). They write atomically and never commit: the change is left in the working tree for the Beekeeper.

## 8. Listing the store

```bash
paseka skills list
paseka skills list --json
```

The table's columns are **name, state, roles, description, provenance**:

```text
NAME            STATE                                            ROLES                         DESCRIPTION                       PROVENANCE
legacy          disabled: step 2 is wrong                        allrounder                    The old procedure.                hand
paseka-release  ok                                               allrounder, builder, shipper  Run the colony release procedure. proposal
paseka-ship     invalid: name "paseka-shipped" does not match its directory "paseka-ship"  allrounder, shipper  Run the ship procedure.  hand
```

- **state** is one of `ok`, `invalid` (with the reason from the same rule set `validate` uses), or `disabled` (with the ledger's reason). A disabled skill reads as disabled even if its bytes have since gone stale; `validate` and `doctor` still report the invalidity.
- **roles** is computed from the role configurations on every run, never remembered: it names every role whose selection resolves to the skill. A role that lists `*` or a matching pattern (`paseka-*`) is named against every skill it actually resolves to, because that is what the gate does — so the column answers "who carries this procedure, and is removing it safe?" for a wildcard role too. A profile override is reflected, because the role lists are the effective ones. `-` means no role carries the skill.
- **description** is the skill's `description`, the same line a bee sees in the advertisement.
- **provenance** is read from the skill's own `metadata.source`: `proposal` for a skill installed by `approve` (§18), `nuc` for an imported Nuc, and `hand` when there is no record. It is not mirrored in the ledger.

A store-wide finding — a stray entry at the store root, or the 8 MiB budget — has no row, so it is printed after the table as a `store: …` line. The per-skill findings are already in the state column.

`--json` prints the same inventory as a stable contract:

```json
{
  "store": "/repo/.paseka/skills",
  "skills": [
    {
      "name": "paseka-release",
      "state": "ok",
      "roles": ["builder"],
      "description": "Run the colony release procedure.",
      "provenance": "proposal"
    },
    {
      "name": "legacy",
      "state": "disabled",
      "reason": "step 2 is wrong",
      "roles": ["allrounder"],
      "description": "The old procedure.",
      "provenance": "hand"
    }
  ],
  "issues": []
}
```

`skills` and `issues` are always arrays, `reason` is present only when there is one, and `state` never carries the reason — a script reads the fields, not prose.

## 9. Showing one skill

```bash
paseka skills show paseka-release          # metadata, provenance, and body
paseka skills show paseka-release --path   # only the resolved SKILL.md path
paseka skills show paseka-release --json   # the same facts as a contract
```

`show` resolves one procedure, so it refuses a pattern (`paseka-*`, `*`): a pattern names a set, and `list` or `validate` is the command for a set. An unknown name is reported by name.

```text
name: paseka-release
description: Run the colony release procedure. Use when shipping a version.
path: /repo/.paseka/skills/paseka-release/SKILL.md
provenance: proposal
metadata:
  source: proposal
  trail: trace-abc123

# Release

1. Run `scripts/checks.sh`.
2. Tag after the merge, not before.
```

- **provenance** is read from the skill's own `metadata.source`; a skill with no provenance record is `hand`. It is not mirrored in the ledger.
- The **body** is printed without its frontmatter, exactly as a bee would read it.
- `--path` prints one line, the absolute `SKILL.md` path, so a script can consume the location without parsing prose. In this one command `-C`/`--colony` names the colony root, because `--path` is the path-only mode.
- An invalid skill is still shown, with its findings under `issues:`, so a Beekeeper can read a broken procedure while fixing it.

Both `list` and `show` read the **repository store**, never a trail's frozen copy: no trail is in scope when a Beekeeper runs a shell command. `load` is the deliberate exception — it takes a run's identifiers and reads that run's frozen copy (section 10).

## 10. Loading a skill

A bee in `command` mode gets the body by calling the runtime:

```bash
paseka skills load paseka-release --trace trace-abc123 --agent agent-7f2c
```

The reference is resolved against the configuration the run was **offered**, read from the run's recorded configuration reference — not the live store. A store edit or configuration application after the run started therefore cannot make `load` disagree with the advertisement the run received.

On success it prints the absolute `SKILL.md` path, the skill directory, and the body without frontmatter:

```text
path: /repo/.paseka/runs/trace-abc123/config/tree/.paseka/skills/paseka-release/SKILL.md
dir:  /repo/.paseka/runs/trace-abc123/config/tree/.paseka/skills/paseka-release

# Release

1. Run `scripts/checks.sh`.
2. Tag after the merge, not before.
```

- The **directory** is printed so a body that refers to `scripts/`, `references/`, or `assets/` can be followed. The files are reachable under `dir`, and an agent learns the location only by loading — which is what keeps the path out of the advertisement in `command` mode without making a bundled procedure unusable.
- A reference the run's role does not carry is refused with exit **2** and `skill_not_offered`. A pattern (`paseka-*`, `*`) is refused the same way: it names a set, and `load` serves one procedure. An invalid reference is also a refusal.
- Each successful load appends one `{name, contentHash, at}` line to `.paseka/runs/<traceId>/<agentId>/skills-used.ndjson`, where `contentHash` is the SHA-256 of the served `SKILL.md` bytes, frontmatter included. It identifies the file as served; it is not the install-time provenance hash.
- At finalize the runtime folds that file into the run's `result.json` as `skillsUsed`, deduplicated by name with the first load winning. The log keeps one line per load; only the result is a set.
- `load` works in either delivery mode. It speaks prose, not JSON, because it prints a body for a model and has a side effect on the run's own record.

**Resolving the colony.** The command is advertised by name, like `paseka event emit`; the colony does not promise an absolute binary path. It finds the colony root in this order:

1. `-C <dir>` when given;
2. `PASEKA_COLONY_ROOT`;
3. the nearest ancestor of the working directory that holds **both** `.paseka/colony.yaml` and `.paseka/runs/<traceId>/<agentId>/`.

The run directory is part of the test because a git worktree carries the committed `.paseka/` but not the gitignored runs; walking up from `.paseka/worktrees/<traceId>/` therefore reaches the primary checkout instead of stopping at the worktree. When no candidate holds the run, the command exits 1 and names the roots it tried, the trace and agent, and the `-C <colonyRoot>` that would fix it.

## 11. Validating

```bash
paseka skills validate          # the whole store
paseka skills validate paseka-* # the skills a pattern names
paseka skills validate "*"      # every skill, explicitly
```

The command resolves the live repository store and exits non-zero when anything is invalid: a broken skill, a reference that matches no skill, a budget breach, or a ledger that cannot be read. It prints each finding beside the skill it belongs to, then the count.

```text
skill "paseka-release": name "paseka-ship" does not match its directory "paseka-release"
skills: 1 issue(s) found
```

On a clean store it prints one line:

```text
skills: ok (2 skill(s), 8.3 KiB of 8.0 MiB)
```

Use it as a pre-commit hook or in CI; it needs no bus and no running colony.

`paseka skills` **alone** prints the group's help. It is a namespace, not an action — the verbs that change the store are deliberate, and guessing which one was meant is not harmless.

`paseka doctor` runs the same scan and the same resolver, and reports their findings:

- an unresolvable reference, an unmatched non-`*` pattern, an invalid skill, or the 32-skill breach is an **error**;
- an explicitly empty `skills` list is a **warning** (it is a legitimate opt-out, but easy to lose in a diff);
- a disabled skill is a **warning** naming the roles that still list it.

```bash
paseka doctor
```

## 12. Changing the store

Four verbs change the store. `approve` installs a proposed procedure (§18); `disable`, `enable`, and `remove` change one already in the store. All four act on one procedure — a pattern (`paseka-*`, `*`) is refused — speak prose, and never write a commit. They read the committed ledger first, so a corrupt ledger fails the command instead of being replaced, and no store entry is touched.

```bash
paseka skills disable paseka-release --reason "release step 2 is wrong"
paseka skills enable paseka-release
paseka skills remove paseka-release --dry-run
paseka skills remove paseka-release --force
```

- **`disable`** writes a disabled record (when and why) to `.paseka/skills.index.json`, names the roles whose resolved set changes, and leaves the skill in the store and in the listing. It is the emergency stop: it needs no preview.
- **`enable`** clears that record and restores the offer. Disable then enable is a complete undo and leaves no record behind.
- **`remove`** deletes the store entry and its ledger record, leaving no orphan. It refuses while a role still references the skill and names those roles, because the removal would silently strip a procedure from them; `--force` proceeds anyway. `--dry-run` prints the plan and leaves the store and ledger byte-identical.

A removal that must be shared is a change the Beekeeper commits. The commands write the working tree and stop there. Removing a skill does not delete a trail's decisions about it.

## 13. Same rules everywhere

Validation is one rule set, exposed once and reused by every later path — the store scan, dispatch, a trail's frozen copy, approval, and `doctor`. The shell and the runtime call the same scan, so they cannot report a different verdict about the same bytes; a second copy of these rules would drift.

An invalid skill is an error even when nothing else mentions it, so a typo in frontmatter cannot sit in the store unnoticed.

`doctor` also re-checks an installed procedure's **recorded hash**: a skill approved from a proposal carries `metadata.procedure_hash`, and a hand edit to the body or a supporting file changes the recomputed hash. That is an error, because the file is still well-formed but no longer the procedure the Beekeeper approved. A hand-authored skill without a recorded hash is not checked — there is no approved identity for it to disagree with.

## 14. Attribution and the trail export

`paseka export --include skills` carries both halves of a run's skill story, and both are read from that run's pinned configuration snapshot, so a later store edit or removal cannot rewrite an old export.

- **Skills offered** — the procedures the run's role resolved to, with the frozen `SKILL.md` path.
- **Skills used** — the union of transcript evidence and the run's load records, deduplicated by name with the first load winning.

A transcript attributes a skill when a tool call names a file under the run's frozen skill directory: its `SKILL.md`, a `scripts/` entry, or a `references/` or `assets/` file. The export reads the raw tool input beside the Agent log's summarized projection, so a path the summary would drop is still seen; the Agent log itself is unchanged.

The section shows the evidence that attributed each used skill and the body read from the frozen copy. If the body cannot be read, the entry says why instead of disappearing. A run whose usage cannot be determined reports the capability's own reason — `no providerSessionId`, `unsupported`, `store not found`, `parse error`, `resolve error`, or `timeout` — rather than an empty set, because absence of evidence is not absence of use. A run with a load record still attributes in command mode and on adapters without a transcript capability; a note says when the used set came from load records alone.

Attribution is a measurement, not a proof: a recorded read is evidence that a skill's file was reached for, not that its instructions were followed.

## 15. Proposing a skill

The Archivist Bee can turn a finished trail's friction into a proposed procedure. That is the `skill` intent, run deliberately against one trail rather than on every flight, because a proposal waits for a human anyway:

```bash
paseka bee run archivist --trace trace-abc123 --intent skill \
  --body "Propose the skill this trail proved the colony needs"
```

- The intent is discovered from `.paseka/prompts/_partials/archivist-intent-skill.md`, the same way the role's other intents are discovered. `--intent skill` renders the skill guidance instead of the `task` or `chronicle` guidance.
- The role reads the named trail's runs, tasks, verdicts, and review notes and applies three tests: the shape is **recurring**, **procedural**, and **not already written down** in the store or `docs/`. If none survives, it publishes nothing and its contract fails the run — a skill intent that produced no skill did not do its job.
- The bundle lives in that trail's artifact comb, never in the store:

  ```text
  .paseka/runs/<traceId>/artifacts/
  └── skill-proposals/
      └── <name>/
          ├── SKILL.md          # required
          ├── scripts/          # optional
          ├── references/       # optional
          └── assets/           # optional
  ```

- The run publishes exactly one `INSIGHT/skill.proposed` discovery. It carries the skill `name`, its `description`, a `reason` citing the runs, files, and events that motivated it inline, the comb-relative `bundle` path, and the bundle's `procedureHash`. The hash covers the `SKILL.md` body and the supporting files in a stable order with the frontmatter excluded, so writing provenance later does not change the proposal's identity.
- **The role never writes `.paseka/skills/`.** Nothing installs a proposal on its own; approval is a Beekeeper action, and the runtime is the store's only writer.
- The completion contract is per intent: `task` and `chronicle` must publish exactly one `context.note`, and `skill` must publish exactly one `skill.proposed`. A skill-intent run that publishes nothing, a note, or more than one proposal fails, and a note-intent run that publishes a proposal fails.

Proposals are operational `INSIGHT` events: the trail timeline and export summarize them with the skill name and its reason, and the kind is never projected into `{{.Insights}}` prompt memory.

## 16. Reviewing proposals

```bash
paseka skills proposals                          # every proposal every trail holds
paseka skills proposals paseka-release           # a bare name, across trails
paseka skills proposals trace-abc123/paseka-release
paseka skills proposals --trail trace-abc123     # every proposal on one trail
paseka skills proposals --json
```

A proposal exists because an Archivist skill-intent run published `INSIGHT/skill.proposed` and wrote a bundle into that trail's comb (section 15). `proposals` is the inventory of those discoveries: one row per proposal with the source trail, the skill, when it was proposed, its state, and the bundle's verdict.

| State | Meaning |
| ----- | ------- |
| `pending` | no decision yet; a matching, valid bundle is marked `valid` |
| `installed` | a Beekeeper approved the bundle into the shared store |
| `refused` | a Beekeeper turned it down; the reason is shown, and the refusal is terminal for that exact bundle |
| `trialled` | the bundle was applied to a trail's candidate configuration; the destination trail is named |
| `missing` | the discovery exists but its bundle is gone; it is reported, never hidden and never pending |

The **bundle** column re-reads the bytes rather than trusting the discovery: the bundle must contain a `SKILL.md` that passes the one rule set, every entry must be a regular file, and the recomputed procedure hash must equal the discovery's `procedureHash`. A hash mismatch, an invalid `SKILL.md`, or a symbolic link is reported with its reason and the row is not valid; a matching valid bundle reads `pending` and `valid`.

Decisions come from the runtime-owned, machine-local `.paseka/runs/<traceId>/skill-proposals/decisions.ndjson` — one line per decision carrying `skill`, `trail`, `procedureHash`, `destination`, `decision`, optional `reason` and `roles`, and `at`. Nothing about proposals is in the committed store ledger: the decision is a recorded state beside the proposal, not an absence, and the file is not shared through git. It disappears with the trail. A decision is keyed by the bundle's procedure hash, so a changed bundle is a new proposal and an earlier decision does not apply to it. The newest line per destination is the one the row shows, so a trial and a later store install both remain visible, while a refusal is terminal for the exact bundle.

A reference is a bare skill name or `<trail>/<skill>`; `--trail` supplies the trail for a bare name and, with no reference, filters the listing to one trail. A bare name that matches more than one trail is refused, naming every candidate as `<trail>/<skill>`, because choosing between two trails is not a listing's decision. A pattern is refused — a decision names one procedure — and a named selection that matches nothing is an error.

A malformed decision line, an unreadable event record, and an unreadable trail are printed after the table (and carried in the JSON `issues` array); none is silently skipped.

`--json` emits the same rows as a contract: `trail`, `name`, `description`, `proposedAt`, `state`, optional `reason`/`destination`/`decidedAt`, `bundle`, `bundleMissing`, `procedureHash`, `recomputedHash`, `valid`, `validation`, and `decisions` (the newest line per destination). `proposals` and `issues` are always arrays.

## 17. Refusing a proposal

```bash
paseka skills reject paseka-release --reason "not a procedure"
paseka skills reject trace-abc123/paseka-release --trail trace-abc123
paseka skills reject trace-abc123/paseka-release --dry-run
```

`reject` records a Beekeeper's refusal for exactly one proposal. It resolves its reference the same way `proposals` does — `<skill>` or `<trail>/<skill>`, with `--trail` supplying the trail for a bare name — so a pattern, an unknown name, and a bare name that appears in more than one trail are refused, and an ambiguity names every candidate as `<trail>/<skill>`.

The command appends one `refused` line to the proposing trail's `.paseka/runs/<traceId>/skill-proposals/decisions.ndjson`, carrying the trail, the skill, the bundle's procedure hash, the destination (`store`), the decision, the optional `--reason`, and the time. It refuses a bundle that is missing or invalid, a proposal already marked `installed`, and a name the live store already holds. The append is serialized under the trail's proposal-decision lock, so a concurrent writer cannot interleave a partial line. The decision is machine-local trail data: nothing is committed and it disappears with the trail.

A refusal is terminal for that exact bundle. The same bytes cannot later be approved or trialled, and rejecting them again reports the existing decision and time without adding a second line. A re-proposed bundle with a different `procedureHash` is a new proposal and can still be decided.

`--dry-run` prints the exact line that would be appended and changes nothing: the store, the ledger, the decision file, and the lock file are untouched. `paseka skills proposals` then shows the row as `refused` with the reason and the decision time.

## 18. Approving a proposal into the store

```bash
paseka skills approve paseka-release
paseka skills approve paseka-release --trail trace-abc123
paseka skills approve trace-abc123/paseka-release trace-abc456/another-skill
paseka skills approve trace-abc123/paseka-release --dry-run
paseka skills approve paseka-release --force
```

`approve` installs one or more proposed skills into the repository store. It resolves every reference the same way `proposals` does — `<skill>` or `<trail>/<skill>`, with `--trail` supplying the trail for a bare name — so a pattern, an unknown name, and a bare name that appears in more than one trail are refused, and an ambiguity names every candidate as `<trail>/<skill>`.

**The batch is atomic.** Every reference is resolved, every bundle is re-read and hash-verified against its discovery, and every `SKILL.md` is validated against the destination name before a single byte is written. One invalid, missing, refused, already-installed, or conflicting reference writes none of the good ones. Two proposals in one batch that would install the same store name are a conflict, because the store has one directory per name.

The command installs the bundle's `SKILL.md` and every supporting file, then runs the shared rule set over the result before it becomes visible. An existing store entry of the same name is refused without `--force`; with `--force` the whole batch still validates first, the old entry is replaced as a unit, and an existing disable record is **kept**: the replaced skill remains disabled and the command says so. `doctor` and `skills list` keep reporting it as disabled.

Installation writes provenance into the skill's own `metadata`, preserving every other metadata key and the body byte-for-byte:

| Key | Value |
| --- | ----- |
| `source` | `proposal` |
| `trail` | the trail that proposed the skill |
| `role` | the proposing bee's role (the discovery producer, resolved through its run record) |
| `approved_at` | the approval time in RFC 3339 |
| `procedure_hash` | the procedure-content hash the bundle was approved with |

The hash covers the `SKILL.md` body and the supporting files in a stable order and excludes the frontmatter, so writing provenance cannot change it. It is the identity `doctor` checks later: an installed skill whose body or supporting file was edited by hand is reported as a `procedure hash mismatch`, while a hand-authored skill without a recorded hash is not checked.

The command appends one `installed` decision to each source trail's `.paseka/runs/<traceId>/skill-proposals/decisions.ndjson`, carrying the destination (`store`), the skill, the bundle's procedure hash, and the time. The decision is machine-local trail data and disappears with the trail; the skill itself is a repository change. `proposals` then shows the row as `installed`.

Approval makes the procedure **available**, not active: it reaches a bee only when that bee's role lists it (§5).

`--dry-run` prints the same plan — source trail, destination, procedure hash, provenance, every path that would be written, and the exact decision line it would append — and leaves the store, the ledger, and the decision files byte-identical. It validates the real batch, so a reference the real command would refuse is refused here too.

An install leaves **untracked files in the working tree**. The command names every path it wrote and says that committing is still the Beekeeper's decision; it never writes a commit.

## 19. Trialling a proposal on one trail

```bash
paseka skills approve trace-abc123/paseka-release --into trace-def456 --role builder
paseka skills approve trace-abc123/paseka-release --into trace-def456 --role builder --role shipper
paseka skills approve trace-abc123/paseka-release --into trace-def456 --role builder --dry-run
```

`--into <trace>` sends the same batch to one trail's **pinned configuration** instead of the store. The command builds one candidate from that trail's active snapshot, adds the bundles under the candidate's store, and makes every named role resolve them in the same candidate. It validates the whole candidate with the same skill rule set and with the configuration procedure validation, then applies exactly one new configuration revision. It never edits the active snapshot, the live store, or the working tree.

The role-list change is part of the trial's diff, and it is deliberately narrow:

| The role currently… | The candidate writes |
| --- | --- |
| inherits the colony default | the resolved default list plus the skill, as the role's own list |
| has its own list | that list with the skill appended |
| has an explicit empty list | the one-skill list |
| lists `*` or a pattern that matches the skill | nothing — the gate already resolves the skill and is not narrowed |

All skills in one invocation go to the same role set; a different mapping is a separate invocation. Several references in one invocation are the recommended form: one candidate, one validation, one revision, all or nothing. Two sequential trials work when each applies, because each builds from the then-active snapshot; a second trial is refused while a configuration change is pending.

A trial appends one `trialled` decision to each proposing trail's decision file, naming the **destination trail**, the roles, the procedure hash, and the time. The proposal stays pending for the store: a trial is evidence, not a store approval. A later `approve` into the store records `installed` and leaves the trial record in place, so `proposals` shows both destinations.

The command refuses:

- without `--role`, because a trial that adds a skill no role carries changes nothing;
- a proposal that was already refused, is missing, invalid, or no longer hashes to its discovery;
- a candidate that fails the shared skill rules or procedure validation, before any revision or decision is recorded;
- a trail with another configuration change pending, or one where an execution blocker holds activation, because a queued trial is not a result.

`--dry-run` prints the same plan — destination, revision, file diff, and the exact decision lines — and writes nothing. The store and the working tree are byte-identical before and after either form.
