# Colony skills

A **skill** is a reusable procedure the colony owns: a portable [Agent Skills](https://agentskills.io) directory, version-controlled in the repository, that a bee may be given when its role lists it. The format is the industry's, not Paseka's, so a skill written here is readable by Cursor, OpenCode, and Claude Code, and a skill written for one of them can be dropped in unchanged.

Design record: [spec 041](../specs/041-skills-layer.md). This guide covers the store, its validation, the per-role selection gate, the committed ledger, delivery through the run's system prompt and run record, the reading commands (`list`, `show`, `validate`), and the store mutations (`disable`, `enable`, `remove`); attribution and approval build on the same rules.

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

Before a run starts, the runtime resolves the effective set once and renders it into the run's system prompt as a section of its own, appended after the role's instructions:

```text
## Available skills

Each skill below is a reusable procedure this role may use. The procedure itself is not included here; read the file at its path when the task calls for it.

- paseka-release: Run the colony release procedure. Use when shipping a version.
  path: <frozen-config>/tree/.paseka/skills/paseka-release/SKILL.md
```

- The section carries metadata only — name, description, and the absolute path of the `SKILL.md`. A body is never inlined: progressive disclosure is what keeps a procedure cheap until a bee needs it.
- The runtime appends the section; it is never written into a template file. A bee whose role declares no `system_template` receives the section as its whole system prompt, and a run whose effective set is empty receives no section at all.
- The composed system prompt is written to `.paseka/runs/<traceId>/<agentId>/system.txt` before the adapter starts, so a script bee has the same auditable file even when its adapter writes no system prompt of its own.
- The advertised path is inside the trail's **frozen configuration**, not the live store, so a running trail keeps reading the bytes it started with. A store or role-list edit after trail start changes the next trail, not this run; it reaches a trail only through an explicit configuration application.
- The resolved names are recorded in the run's `request.json` as `skills` (omitted when the set is empty). Both the unattended dispatcher and the interactive session manager write it, so an AFK run and a HITL session carry the same field, and it stays readable after the role's list changes.
- A resumed provider session keeps the first turn's context; the runtime records the set but does not re-inject the section. A configuration application changes later runs and new sessions only.

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

The table's columns are **name, state, roles, description**:

```text
NAME            STATE                                            ROLES                         DESCRIPTION
legacy          disabled: step 2 is wrong                        allrounder                    The old procedure.
paseka-release  ok                                               allrounder, builder, shipper  Run the colony release procedure.
paseka-ship     invalid: name "paseka-shipped" does not match its directory "paseka-ship"  allrounder, shipper  Run the ship procedure.
```

- **state** is one of `ok`, `invalid` (with the reason from the same rule set `validate` uses), or `disabled` (with the ledger's reason). A disabled skill reads as disabled even if its bytes have since gone stale; `validate` and `doctor` still report the invalidity.
- **roles** is computed from the role configurations on every run, never remembered: it names every role whose selection resolves to the skill. A role that lists `*` or a matching pattern (`paseka-*`) is named against every skill it actually resolves to, because that is what the gate does — so the column answers "who carries this procedure, and is removing it safe?" for a wildcard role too. A profile override is reflected, because the role lists are the effective ones. `-` means no role carries the skill.
- **description** is the skill's `description`, the same line a bee sees in the advertisement.

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
      "description": "Run the colony release procedure."
    },
    {
      "name": "legacy",
      "state": "disabled",
      "reason": "step 2 is wrong",
      "roles": ["allrounder"],
      "description": "The old procedure."
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

Both `list` and `show` read the **repository store**, never a trail's frozen copy: no trail is in scope when a Beekeeper runs a shell command.

## 10. Validating

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

## 11. Changing the store

Three verbs change the store. All three act on one procedure — a pattern (`paseka-*`, `*`) is refused — speak prose, and never write a commit. They read the committed ledger first, so a corrupt ledger fails the command instead of being replaced, and no store entry is touched.

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

## 12. Same rules everywhere

Validation is one rule set, exposed once and reused by every later path — the store scan, dispatch, a trail's frozen copy, approval, and `doctor`. The shell and the runtime call the same scan, so they cannot report a different verdict about the same bytes; a second copy of these rules would drift.

An invalid skill is an error even when nothing else mentions it, so a typo in frontmatter cannot sit in the store unnoticed.
