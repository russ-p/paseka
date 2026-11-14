# `skill.proposed` — a skill discovery

Five required fields — `name`, `description`, `reason`, `bundle`,
`procedureHash` — and one missing field is refused at emit. `bundle` is relative
to the trail comb (`skill-proposals/<name>`), never absolute and never a store
path: the runtime is the store's only writer, and only after a Beekeeper decides.

`ROOT`, `TRACE`, `AGENT`, and `SKILL_DIR` are defined in `SKILL.md` § *The three
values every emit needs*.

Write the bundle first, then hash its body and supporting files with the
frontmatter excluded. The hash is the proposal's identity; provenance written
later must not change it.

```bash
NAME=paseka-event-emit
bundle="$ROOT/.paseka/runs/$TRACE/artifacts/skill-proposals/$NAME"

HASH=$(
  cd "$bundle" || exit 1
  awk 'BEGIN{fm=0} /^---[[:space:]]*$/ && fm<2 {fm++; next} fm>=2 {print}' SKILL.md
  find . -type f ! -path ./SKILL.md -print | LC_ALL=C sort | while IFS= read -r p; do
    printf '%s\n' "${p#./}"
    cat "$p"
  done
) | sha256sum | cut -d' ' -f1
```

Then publish, deferred — a proposal is a record for the Beekeeper, and nothing
needs to react to it mid-run:

```bash
printf '%s' "$(jq -cn --arg h "$HASH" --arg n "$NAME" \
  '{kind:"skill.proposed",name:$n,
    description:"…",
    reason:"runs A and B both … (cite run ids, task ids, files, events inline)",
    bundle:("skill-proposals/" + $n),
    procedureHash:$h}')" \
| sh "$SKILL_DIR/scripts/emit.sh" --root "$ROOT" --trace "$TRACE" --agent "$AGENT" \
    --type INSIGHT --defer
```

Leave `taskId` out: a proposal belongs to the trail, not to one task.

Good result — pending, and the bundle validates:

```bash
paseka -C "$ROOT" skills proposals "$TRACE/$NAME"
```

```text
TRAIL                    SKILL              PROPOSED    STATE    BUNDLE
trace-01a1d62ac0c152e7   paseka-event-emit  2026-11-14  pending  valid
```

`STATE pending` plus `BUNDLE valid` is the whole success condition. `missing`
means the comb path and the directory name disagree; `invalid: …` names the
`SKILL.md` rule the bundle broke.
