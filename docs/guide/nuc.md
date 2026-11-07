# Nuc — portable bee packs

A **Nuc** is a single-file portable pack of shareable colony config: bee role YAML, prompt templates, optional **Forage Cues**, and the **skills** those roles use. Use it to move tuned bees, prompts, ingress shortcuts, and procedures between Colonies without copying secrets, adapters, or `colony.yaml`.

Implementation: [`internal/nuc`](../../internal/nuc/).

Related: [bee config](bee-config.md), [prompt templates](prompt-templates.md), [Forage Cues](cues.md), [CLI](cli.md).

---

## 1. What is included

| Included | Excluded |
| -------- | -------- |
| `.paseka/bees/<role>.yaml` | `colony.yaml`, `*.local.yaml` |
| `.paseka/prompts/*.md` referenced by exported bees | `~/.config/paseka/` adapters and secrets |
| `.paseka/prompts/_partials/*.md` when any prompt is exported | NATS URL, machine-local state |
| `.paseka/cues/<id>.yaml` (all when present, or `--cues` filter) | |
| `.paseka/skills/<name>/**` resolved by the exported roles, plus `--skills` names | |

---

## 2. File format

Filename convention: `<name>.nuc.yaml` (extension `.yaml`, as elsewhere in the repo).

```yaml
apiVersion: paseka/v1
kind: Nuc
metadata:
  name: minimal
  description: optional human note
spec:
  bees:
    scout: |
      role: scout
      adapter: cursor
      prompt_template: scout.md
  prompts:
    scout.md: |
      You are Scout Bee...
    _partials/emit-insight.md: |
      ...
  cues:
    feature: |
      description: Intake an idea or bug for Scout classification
      emit: signal
      type: SIGNAL
      kind: feature.requested
      title: "{{.Title}}"
      body: "{{.Body}}"
  skills:
    release-proc:
      SKILL.md: |
        ---
        name: release-proc
        description: Run the release procedure.
        metadata:
          owner: release-team
        ---
        Run scripts/run.sh.
      scripts/run.sh: |
        echo hi
```

| Field | Required | Meaning |
| ----- | -------- | ------- |
| `apiVersion` | yes | Must be `paseka/v1` |
| `kind` | yes | Must be `Nuc` |
| `metadata.name` | yes | Pack identity (defaults to colony slug on export) |
| `metadata.description` | no | Optional note |
| `spec.bees` | yes | Map of role → raw bee YAML body |
| `spec.prompts` | no | Map of path (relative to `.paseka/prompts/`) → markdown body |
| `spec.cues` | no | Map of cue id → raw cue YAML body |
| `spec.skills` | no | Map of skill name → (relative path → file body). `SKILL.md` is required; supporting files travel with it |

Prompt paths must not be absolute or contain `..`. A skill name must be a concrete stored skill name (no pattern), and every skill file path must stay under that skill's directory.

---

## 3. CLI

```bash
paseka nuc export [-o file] [--bees role,...] [--cues id,...] [--skills ref,...] [--name name] [--description text] [-C path]
paseka nuc import <file|url|-> [--force] [--dry-run] [-v] [-C path]
```

### `paseka nuc export`

Reads the current Colony and writes a nuc document.

| Flag | Short | Default | Description |
| ---- | ----- | ------- | ----------- |
| `--output` | `-o` | stdout | Write nuc file; prints path when set |
| `--bees` | | all roles | Comma-separated bee roles to export |
| `--cues` | | all cues when present | Comma-separated cue ids to export |
| `--skills` | | none | Skill references added to the exported roles' resolved set |
| `--name` | | colony slug | `metadata.name` |
| `--description` | | | `metadata.description` |
| `--path` | `-C` | cwd | Colony resolution start directory |

When any prompt is exported, all `_partials/*.md` files are included (partials are shared; there is no dependency graph).

Every exported role's skill references are resolved against the live store with the same resolver a run uses, so a pattern expands exactly as it would at run time. `--skills` adds a skill no exported role references. A role with no skills adds no `spec.skills` section, so a pack without skills is unchanged. An invalid reference refuses the export rather than silently dropping a procedure.

```bash
paseka nuc export -o minimal.nuc.yaml
paseka nuc export --bees scout,builder -o scout-builder.nuc.yaml
paseka nuc export --cues feature,hotfix -o entry-points.nuc.yaml
paseka nuc export --skills docs-style -o with-docs.nuc.yaml
paseka nuc export | less
```

### `paseka nuc import`

Applies a nuc pack to the current Colony.

| Flag | Default | Description |
| ---- | ------- | ----------- |
| `--force` | off | Overwrite existing bee, prompt, cue, and skill entries |
| `--dry-run` | off | Show plan without writing |
| `--verbose` / `-v` | off | List created, skipped, and overwritten paths |
| `--path` / `-C` | cwd | Colony resolution start directory |

Source may be a local path, `https://…` URL, or `-` (stdin).

`--dry-run` writes nothing and lists the skill actions beside the bee, prompt, and cue actions.

Carried skills are installed into `.paseka/skills/<name>/` with every file. The whole pack is validated with the shared skill rule set before anything is written, so one invalid skill refuses the pack without a partly imported colony. A skill name or relative path that would escape the store is refused.

An imported skill carries provenance in its own `metadata`:

| Key | Value |
| --- | ----- |
| `source` | `nuc` |
| `procedure_hash` | the hash of the imported body and supporting files |

Proposal provenance (`trail`, `role`, `approved_at`) is removed because it describes another colony; every other metadata key is preserved. The hash excludes the frontmatter, so writing provenance cannot change it, and `doctor` can later detect a hand edit.

```bash
paseka nuc import ./minimal.nuc.yaml
paseka nuc import https://example.com/nucs/dev.nuc.yaml
paseka nuc import ./dev.nuc.yaml --dry-run -v
paseka nuc import ./dev.nuc.yaml --force
```

---

## 4. Conflict policy

Import is **per file**, not field-level:

| Mode | Flag | Behavior |
| ---- | ---- | -------- |
| skip | default | Existing path is left untouched; missing paths are created |
| override | `--force` | Existing path is replaced with nuc contents (bees, prompts, cues, skills) |

A skill directory is one unit: without `--force` an existing skill of the same name is skipped whole; with `--force` the old directory is replaced whole, so a file the pack does not carry does not survive.

There is **no merge**. Bee YAML lists (`subscribes`, `publishes`, `intents`) and markdown prompts are not merged field-by-field. Colonies keep `.paseka/` in git — use version control to review, diff, or revert imports.

After a real import (not `--dry-run`), imported bees are validated with the same load rules as `paseka bee run`.

---

## 5. Typical workflow

```bash
# In source Colony
paseka nuc export -o tuned-scout.nuc.yaml

# In target Colony (after paseka init)
paseka nuc import ./tuned-scout.nuc.yaml --dry-run -v
paseka nuc import ./tuned-scout.nuc.yaml
git diff .paseka/
```

Use `--force` when you intentionally replace local copies. Use git when you need a manual merge.
