Propose **one** procedure this finished trail proved the colony needs. You are not auditing the trail for its own sake here: the audit is the evidence for a reusable skill, and the trail you are reading is `{{.TraceID}}`.

The Task section may name the trail to read. It must be `{{.TraceID}}`. If it names a different trail, stop and say so: this run writes only into its own trail's comb, and a proposal from another trail's evidence would be filed in the wrong place.

### Method

1. Confirm the trail is finished. In `TIMELINE`, `last_task_completed` must be set and `lag_merge_after_completed_s` must say whether the work reached `main` (a number, `merged`, or `unmerged`) — never `no-completed-task`. A trail that is still running is not evidence of a repeatable procedure; stop and say so.
2. Read the trail's own account of what it did (`summary.md` per run) and the `REVIEW-NOTES`, `VERDICTS`, and `DEFERRED-IN-SPEC` rows that explain the friction.
3. Look for a shape that recurs **inside this trail** and would recur on the next one: the same correction raised twice, the same checklist rebuilt by hand, the same merge gate missed, the same review note left unresolved across tasks.
4. Check what the colony already knows before proposing anything. Read `.paseka/skills/` for an existing skill, and search `docs/` for an existing procedure. A finding that duplicates a written procedure is a pointer to that procedure, not a new skill.
5. Apply all three tests. A proposal is justified only when the shape is **recurring** (could happen on the next trail), **procedural** (the answer is steps or a rule, not a fact to record), and **not already written down** (no existing skill, guide, or spec states it).
6. Cite the evidence inline in `reason`: the run ids, task ids, files, and event kinds that show the shape. A reason that says "the build was slow" is not evidence; a reason that says "runs A and B both re-ran `scripts/checks.sh` by hand after task 002 (verification.failed, then rework) — the procedure belongs in a skill" is.

### Judging

Report the selection honestly. One strong proposal with cited evidence is the correct output; three weak ones are not. If nothing survives the tests, **do not publish a `context.note` to satisfy the completion contract** — say in the run log that this trail offered no reusable procedure and stop. The run is then failed by its contract, which is the truthful result: a skill intent that produced no skill did not do its job.

### Writing the bundle

Write the bundle under the artifact comb, at `{{.ArtifactsDir}}/skill-proposals/<skill-name>/`:

```text
{{.ArtifactsDir}}/
└── skill-proposals/
    └── <skill-name>/
        ├── SKILL.md          # required
        ├── scripts/          # optional
        ├── references/       # optional
        └── assets/           # optional
```

- **Never** write into `.paseka/skills/` or any repository path. The store's only writer is the runtime, on a Beekeeper's approval; a Bee that writes there has installed instructions for its successors.
- The directory name and the frontmatter `name` must be identical. Name shape: lowercase letters, digits, and hyphens; no leading, trailing, or doubled hyphen; at most 64 characters.
- `description` is one or two sentences that say when to use the procedure, 1–1024 characters. The body is the ordered procedure and the judgement it needs.
- Keep the bundle small and self-contained. Put a script or template the procedure needs beside `SKILL.md`, under the standard directories, rather than describing it in prose.

```markdown
---
name: paseka-release
description: Run the colony release procedure. Use when shipping a version.
---

# Release

1. Run `scripts/checks.sh`.
2. Tag after the merge, not before.
```

### The procedure-content hash

The discovery carries the bundle's procedure-content hash: the `SKILL.md` body and every supporting file, in a stable order, with the `SKILL.md` frontmatter excluded so provenance written later cannot change the identity. Compute it after the bundle is final:

```bash
bundle="{{.ArtifactsDir}}/skill-proposals/<skill-name>"
(
  cd "$bundle" || exit 1
  awk 'BEGIN{fm=0} /^---[[:space:]]*$/ && fm<2 {fm++; next} fm>=2 {print}' SKILL.md
  find . -type f ! -path ./SKILL.md -print | LC_ALL=C sort | while IFS= read -r path; do
    printf '%s\n' "${path#./}"
    cat "$path"
  done
) | sha256sum | cut -d' ' -f1
```

Use the hash that command prints in `payload.procedureHash`, and use `skill-proposals/<skill-name>` (relative to the comb) in `payload.bundle`.
