# Spec 019: Model aliases

## Status

**Implemented.** Colony-owned `model_aliases` map in `.paseka/colony.yaml`; home `config.yaml` overlays the same keys. Bees keep `params.model` as alias or raw vendor id; runtime resolves once before `--model` is passed to the adapter.

## Problem Statement

Vendor model ids churn (Cursor grok slugs, composer versions, Claude ids). Beekeepers today edit every `bees/*.yaml` when the same rename applies colony-wide. Per-bee YAML and `.paseka/bees/<role>.local.yaml` are the wrong place for a shared rename table — local overlays are for prompt templates, not secrets or machine-local adapter binaries.

Operators want stable names (`small`, `medium`, `high`, or arbitrary keys) in bee config and a single committed map (plus optional home remap on one machine) that translates to the real adapter model id at dispatch time.

## Solution

Add a global **`model_aliases`** map:

- **Committed:** `.paseka/colony.yaml` → `model_aliases: { alias: vendor-id }`
- **Machine-local overlay:** `~/.config/paseka/<slug>/config.yaml` → same shape; home keys win on collision

Bees continue to set `params.model` to either an alias key or a raw vendor id. At AFK dispatch and HITL session start, after `RunParamsFromBee` / `MergeRunParams`, runtime looks up `params.model` in the merged map:

- **Hit** → substitute value (vendor id) for `--model`
- **Miss** → pass through unchanged (treat as vendor id)

Single hop only: alias values must not be another alias key (fail closed at load/merge). Empty `params.model` stays empty (adapter omits `--model`).

## User Stories

1. As a colony author, I want `model_aliases` in `colony.yaml`, so that vendor ids live in one committed place.
2. As a Beekeeper, I want bees to use `params.model: high`, so that role YAML stays stable across vendor renames.
3. As a Beekeeper, I want `params.model: composer-2.5` to still work, so that unmapped values are raw vendor ids.
4. As a Beekeeper on one machine, I want home `model_aliases` to override colony keys, so that I can remap without editing the repo.
5. As a colony author, I want arbitrary alias keys (`low`, `high-fast`, …), so that I am not limited to three tiers.
6. As a colony author, I want one global map (not per adapter), so that the same alias name resolves the same way for all bees on that colony.
7. As a Beekeeper, I want alias resolution at dispatch only, so that adapters stay dumb and pass `--model` as today.
8. As a Beekeeper running `paseka bee run` / reactor dispatch, I want the resolved id on the adapter argv, so that AFK and HITL behave the same.
9. As a colony author, I want invalid maps to fail at load, so that typos and alias chains do not silently mis-route models.
10. As a Beekeeper with `command:` on a bee, I want params ignored as today, so that aliases do not rewrite custom argv.
11. As a Beekeeper with script bees, I want no alias magic in script adapter, so that behavior is unchanged.
12. As a Beekeeper, I want empty `params.model` to omit `--model`, so that adapter defaults still apply.
13. As a colony author, I want trimmed keys and values, so that YAML whitespace does not break lookup.
14. As a Beekeeper, I want per-bee exceptions via that bee's `params.model`, so that one role can pin a raw id without forking the map.
15. As a Beekeeper, I do not want `*.local.yaml` to override `params`, so that local overlays stay prompt-only.
16. As a Nuc packer, I expect aliases to travel with the repo (`colony.yaml`), not inside a Nuc, so that Nuc semantics stay bees/prompts/cues.
17. As a Beekeeper after `paseka init`, I want no forced alias map, so that empty map means current behavior.
18. As an operator when home remaps `high` to a different vendor id, I want colony bees using `high` to pick up the home value on that machine only.
19. As a colony author, I want alias-to-alias rejected (value equals another key), so that there are no chains or cycles.
20. As a Beekeeper, I want load errors for empty alias keys or empty values, so that half-filled maps fail early.

## Implementation Decisions

### 1. Config fields

- `Colony.ModelAliases map[string]string` — YAML `model_aliases` in `.paseka/colony.yaml`
- `HomeConfig.ModelAliases map[string]string` — YAML `model_aliases` in home `config.yaml`

### 2. Merge and resolve

- `MergedModelAliases(colony, home)` — copy colony map, overlay home keys (home wins)
- `ValidateModelAliases(merged)` — reject empty keys/values after trim; reject any value that equals a key in the merged map
- `ResolveModel(name, merged) (resolved string, ok bool)` — trim name; if key in map return value and `ok=true`; else return name and `ok=false`

Validation runs on merged map in `ResolveContext` so cross-layer alias chains are caught.

### 3. Application point

After `MergeRunParams(colony.RunParamsFromBee(bee), extra)` in:

- AFK `prepareDispatch` (`internal/runtime/dispatch_stages.go`)
- HITL session start (`internal/sessions/manager.go`)

If `params.Model != ""`, apply `ResolveModel` with merged map from `colony.Context` (or manifest + home at call site).

Adapters unchanged. `command:` bees unchanged (params ignored when command set).

### 4. Out of scope

- Per-adapter alias tables
- `bees/<role>.local.yaml` params overlay
- Recursive alias chains
- Queen Console / `paseka status` model column
- Rewriting `command:` `--model` flags

## Testing Decisions

- Unit tests in `internal/colony` for merge, overlay, pass-through, trim, empty key/value errors, alias-to-alias rejection
- Dispatcher test: bee `model: high` + colony map → adapter receives vendor id; unmapped slug unchanged
- Session test optional mirror of dispatch behavior
- Test external behavior (resolved `RunParams.Model`), not YAML parser internals

## Out of Scope

See Implementation Decisions §4.

## Further Notes

- Doctor: invalid maps fail at `ResolveContext`; optional future advisory listing bees whose `params.model` is a known alias key
- Durable docs: [Colony layout](../guide/colony-layout.md), [Bee config](../guide/bee-config.md), [Architecture overview](../architecture/overview.md)
