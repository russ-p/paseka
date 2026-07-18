# Nuc — portable bee packs

A **Nuc** is a single-file portable pack of shareable colony config: bee role YAML and prompt templates. Use it to move tuned bees and prompts between Colonies without copying secrets, adapters, or `colony.yaml`.

Implementation: [`internal/nuc`](../../internal/nuc/).

Related: [bee config](bee-config.md), [prompt templates](prompt-templates.md), [CLI](cli.md).

---

## 1. What is included

| Included | Excluded |
| -------- | -------- |
| `.paseka/bees/<role>.yaml` | `colony.yaml`, `*.local.yaml` |
| `.paseka/prompts/*.md` referenced by exported bees | `~/.config/paseka/` adapters and secrets |
| `.paseka/prompts/_partials/*.md` when any prompt is exported | NATS URL, machine-local state |

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
```

| Field | Required | Meaning |
| ----- | -------- | ------- |
| `apiVersion` | yes | Must be `paseka/v1` |
| `kind` | yes | Must be `Nuc` |
| `metadata.name` | yes | Pack identity (defaults to colony slug on export) |
| `metadata.description` | no | Optional note |
| `spec.bees` | yes | Map of role → raw bee YAML body |
| `spec.prompts` | no | Map of path (relative to `.paseka/prompts/`) → markdown body |

Prompt paths must not be absolute or contain `..`.

---

## 3. CLI

```bash
paseka nuc export [-o file] [--bees role,...] [--name name] [--description text] [-C path]
paseka nuc import <file|url|-> [--force] [--dry-run] [-v] [-C path]
```

### `paseka nuc export`

Reads the current Colony and writes a nuc document.

| Flag | Short | Default | Description |
| ---- | ----- | ------- | ----------- |
| `--output` | `-o` | stdout | Write nuc file; prints path when set |
| `--bees` | | all roles | Comma-separated bee roles to export |
| `--name` | | colony slug | `metadata.name` |
| `--description` | | | `metadata.description` |
| `--path` | `-C` | cwd | Colony resolution start directory |

When any prompt is exported, all `_partials/*.md` files are included (partials are shared; there is no dependency graph).

```bash
paseka nuc export -o minimal.nuc.yaml
paseka nuc export --bees scout,builder -o scout-builder.nuc.yaml
paseka nuc export | less
```

### `paseka nuc import`

Applies a nuc pack to the current Colony.

| Flag | Default | Description |
| ---- | ------- | ----------- |
| `--force` | off | Overwrite existing bee and prompt files |
| `--dry-run` | off | Show plan without writing |
| `--verbose` / `-v` | off | List created, skipped, and overwritten paths |
| `--path` / `-C` | cwd | Colony resolution start directory |

Source may be a local path, `https://…` URL, or `-` (stdin).

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
| override | `--force` | Existing path is replaced with nuc contents |

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
