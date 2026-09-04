# Spec 027: Config profiles

## Status

**(Draft)**
Named settings overlays at colony + home layers so a Beekeeper can switch adapter and related settings for a process without editing committed bee YAML. Decisions locked: fail-closed profile names, global `adapter:` replaces LLM bee adapters, sticky default only in home `config.yaml`.

## Problem Statement

A colony binds each Bee to an adapter in committed role YAML. Machine-local home config holds credentials and binaries for those adapters, and `bees/<role>.local.yaml` may overlay prompt templates only. There is no named, opt-in way to run the **same** colony with different adapter settings.

To try Pi or OpenCode on an existing Cursor colony, the Beekeeper must rewrite every `bees/*.yaml`, re-run `paseka init --adapter …` (which preserves existing files), or abuse `command:`. Model aliases stay Cursor vendor ids. Home `adapters/<name>.yaml` never changes which adapter a bee uses.

Operators want `paseka --profile pi` to load a known overlay and run AFK, HITL, and the hive reactor against that overlay — then drop the flag and return to the committed colony.

## Solution

Introduce a **config profile**: a named overlay that is selected for the whole Queen Shell process and merged into colony + home settings before bees dispatch.

Two layers, matching the existing split:

| Layer | Location | Committed | Role |
| ----- | -------- | --------- | ---- |
| Colony profile | `.paseka/profiles/<name>.yaml` | yes | Shareable recipe: LLM adapter remap, params, `model_aliases`, per-bee exceptions |
| Home profile | `~/.config/paseka/<slug>/profiles/<name>/` | no | This machine: `config.yaml` (`nats`, `model_aliases`) and `adapters/*.yaml` overlays |

Selection (highest wins):

1. `--no-profile` (force the committed/home base, ignore sticky)
2. `--profile <name>`
3. `PASEKA_PROFILE`
4. `profile:` in home `config.yaml` (sticky default for this machine only)
5. None — today’s behavior

Fail closed: a selected name that matches no colony file and no home profile directory is an error. The error lists names found on both layers. There is no implicit profile synthesized from a known adapter name.

A process has exactly one effective profile (including none). `paseka run` and `paseka bee run` with different profiles are different hives, not a hot-swap.

## User Stories

1. As a Beekeeper, I want a committed colony profile named `pi`, so that the recipe for running this colony on Pi travels with the repo.
2. As a Beekeeper, I want `paseka --profile pi bee run scout --body "…"`, so that one AFK invocation uses the Pi overlay without editing `bees/scout.yaml`.
3. As a Beekeeper, I want `paseka --profile pi bee chat builder`, so that HITL uses the same overlay as AFK.
4. As a Beekeeper, I want `paseka --profile pi run`, so that the hive reactor dispatches every bee through the overlay for the life of that process.
5. As a Beekeeper, I want omitting `--profile` (and having no sticky default) to leave bees and home config unchanged, so that existing colonies keep working.
6. As a Beekeeper, I want `--profile` as a persistent root flag, so that every subcommand (`bee`, `run`, `doctor`, `status`, `console`, …) sees the same selection.
7. As a Beekeeper in scripts, I want `PASEKA_PROFILE=pi`, so that I do not repeat the flag on every command.
8. As a Beekeeper on one machine, I want `profile: pi` in home `config.yaml`, so that this apiary defaults to Pi without a flag and without committing that choice.
9. As a Beekeeper, I do not want `profile:` in `colony.yaml` or in a colony profile file, so that CI and other machines are not silently remapped.
10. As a Beekeeper with a sticky home default, I want `--no-profile` to ignore it for one command, so that I can run the committed Cursor colony without editing home config.
11. As a Beekeeper, I want `--profile` to win over `PASEKA_PROFILE`, so that an explicit flag is unambiguous.
12. As a Beekeeper, I want `PASEKA_PROFILE` to win over home sticky, so that a shell export overrides the machine default.
13. As a Beekeeper, I want `--no-profile` to win over `--profile` and `PASEKA_PROFILE` only when they are not combined; combining `--no-profile` with `--profile` should error, so that conflicting intent is not silently resolved.
14. As a Beekeeper, I want an unknown profile name to fail before any adapter launch, so that a typo does not run the base colony and look like a successful Pi test.
15. As a Beekeeper on a missing profile, I want the error to list names found in `.paseka/profiles/` and in the home `profiles/` directory, so that I can see what exists on this machine.
16. As a Beekeeper, I do not want `--profile pi` to invent a profile just because `pi` is a known adapter, so that smoke tests cannot skip writing a real recipe (fail closed).
17. As a Beekeeper, I want a colony profile that exists only as `.paseka/profiles/pi.yaml` to be enough, so that I can share a recipe without a home overlay.
18. As a Beekeeper, I want a home profile that exists only under `profiles/pi/` to be enough, so that I can keep a private overlay without committing YAML.
19. As a Beekeeper, I want the same name on both layers to merge (home keys win on collision), so that a shared recipe can be tweaked per machine.
20. As a Beekeeper, I want a home profile to count as present only when its directory contains a loadable `config.yaml` and/or `adapters/*.yaml`, so that an empty folder is not a silent no-op.
21. As a Beekeeper, I want an empty but present colony profile file (no overlay keys) plus a valid name to succeed, so that I can reserve a name; it still must exist on disk.
22. As a Beekeeper, I want global `adapter: pi` in a colony profile to **replace** each LLM bee’s `adapter` field even when the bee YAML already says `cursor`, so that `paseka init` colonies are actually switchable.
23. As a Beekeeper, I want that global remap to apply to `cursor`, `pi`, `claude`, and `opencode` bees, so that any LLM role can be retargeted.
24. As a Beekeeper, I want `adapter: script` bees left unchanged by global `adapter:`, so that oracle/script guards do not become LLM CLIs.
25. As a Beekeeper, I want bees with `command:` left unchanged by global `adapter:`, so that a custom argv is not paired with a different parser/PTY.
26. As a Beekeeper, I want `paseka doctor` to warn when global `adapter:` skipped `command:` or `script` bees, so that I see which roles still use the committed adapter.
27. As a Beekeeper, I want `bees.<role>.adapter` in the profile to replace that role even if it has `command:` or would have been skipped, so that an explicit exception is possible.
28. As a Beekeeper, I want a per-bee adapter of `script` in a profile to be rejected, so that profiles cannot turn a role into a script bee (choreography stays in committed YAML).
29. As a Beekeeper, I want global `adapter: script` to be rejected at load, so that the remap cannot target the script adapter.
30. As a Beekeeper, I want an unknown adapter name in a profile to fail like an unknown bee `adapter`, so that typos do not fall back to Cursor.
31. As a Beekeeper, I want profile `params` merged onto each remapped LLM bee (profile keys win), so that I can set `provider`, `output_format`, or `thinking` for Pi/OpenCode without editing roles.
32. As a Beekeeper, I want `bees.<role>.params` to win over global profile `params` for that role, so that one bee can pin a different model.
33. As a Beekeeper, I want bees that were not remapped (`script`, skipped `command:`) to ignore global profile `params`, so that script/command roles do not pick up LLM flags.
34. As a Beekeeper, I want `*.local.yaml` to stay prompt-only, so that profiles do not reopen spec 019’s ban on local params overlays.
35. As a Beekeeper, I want profile `model_aliases` to overlay colony aliases, so that `params.model: high` can resolve to a Pi or OpenCode vendor id under that profile.
36. As a Beekeeper, I want the alias merge order to be colony → colony profile → home `config.yaml` → home profile, so that machine overlays still win over committed recipes.
37. As a Beekeeper, I want the same single-hop alias validation as spec 019 on the **fully merged** map, so that a profile cannot introduce alias-to-alias chains.
38. As a Beekeeper, I want home profile `adapters/<name>.yaml` to overlay the slug’s `adapters/<name>.yaml` (non-empty fields win), so that a profile can point at a nightly binary or a different `api_key_env`.
39. As a Beekeeper, I want adapter credentials to keep loading by **resolved** adapter name, so that after remap to `pi` the process uses `adapters/pi.yaml` (plus home-profile overlay) without duplicating keys into the colony profile.
40. As a Beekeeper, I want a colony profile that contains `adapters:` or API key fields to fail load, so that secrets cannot be committed “by accident” in the recipe.
41. As a Beekeeper, I want home profile `config.yaml` to overlay `nats.url`, so that I can point a test profile at another JetStream without editing the sticky home NATS.
42. As a Beekeeper, I want `PASEKA_NATS_URL` to still win over every yaml layer, so that containers and homelab keep one env override.
43. As a Beekeeper, I want home profile `config.yaml` to reject `colony_root`, `slug`, and `profile:`, so that a profile cannot retarget the colony or nest sticky defaults.
44. As a Beekeeper, I want a colony profile that sets `subscribes`, `publishes`, `completion_contract`, `worktree`, `sector`, `run_summary`, `prompt_template`, `system_template`, `command`, `post_exec`, or `role` to fail load, so that a profile cannot silently rewrite choreography or prompts.
45. As a Beekeeper, I want the same forbidden-field rejection on `bees.<role>` overlays, so that per-bee exceptions stay adapter/params only.
46. As a Beekeeper, I want prompt overlays to remain `bees/<role>.local.yaml`, so that profiles and local templates do not compete.
47. As a Beekeeper, I want profile names to reject `/`, `..`, empty string, and leading dots, so that names cannot escape the profiles directories.
48. As a Beekeeper, I want `--profile` with an empty value to error (not mean “none”), so that `--no-profile` is the only way to clear sticky.
49. As a Beekeeper, I want `paseka doctor` to print the effective profile name, which layers loaded, and remapped roles, so that I can confirm Pi will actually run before a long chat.
50. As a Beekeeper, I want `paseka status` to include the effective profile name, so that a running hive’s overlay is visible without reading flags.
51. As a Beekeeper, I want Queen Console started via `paseka --profile pi console` to use that overlay, so that Console-launched sessions match the CLI hive.
52. As a Beekeeper, I do not need an in-Console profile switcher in this slice, so that a long-lived Console cannot diverge from its process overlay.
53. As a Beekeeper, I want AFK `meta.json` to record the effective profile name (empty when none), so that Pi vs Cursor manual runs are distinguishable in inspect/export.
54. As a Beekeeper, I want HITL session records to store the same profile name, so that chat history is auditable the same way.
55. As a Beekeeper looking at export `--include bees`, I want the committed YAML files on disk, not the overlayed view, so that export stays a snapshot of the repo.
56. As a Beekeeper looking at EDA topology / bee list in a profiled process, I want **effective** adapters, so that the graph matches what will dispatch.
57. As a Nuc packer, I want profiles excluded from Nuc export/import, so that a Nuc stays bees, prompts, and cues.
58. As a Beekeeper after `paseka init`, I want no starter profiles created, so that profiles are opt-in recipes, not another init matrix.
59. As a Beekeeper, I want `init --adapter` unchanged (scaffold committed bees + home adapter yaml), so that a new colony still has a real default on disk.
60. As an operator of a long-lived `paseka run`, I want the profile fixed at process start, so that mid-flight bees cannot mix Cursor and Pi overlays.
61. As a Beekeeper, I want two concurrent processes with different `--profile` values to be allowed, so that I can compare adapters without stopping the other hive — with the understanding they must not share one reactor’s in-memory state (separate processes).
62. As a Beekeeper writing `bees.hivewright.adapter: cursor` under a global `adapter: pi` profile, I want hivewright to stay on Cursor, so that I can mix adapters by exception.
63. As a Beekeeper, I want a per-bee overlay for a role that does not exist in `.paseka/bees/` to fail load, so that typos in `bees.<role>` are not ignored.
64. As a Beekeeper, I want `paseka profile` subcommands (`list` / `show` / `init`) out of this slice if doctor + error lists cover discovery, so that MVP stays a load/merge flag rather than a new command family.
65. As a documentation reader, I want the technical word **profile** in CLI and docs, so that this is not given a bee-language alias that collides with Nuc or Forage Cue.
66. As a Beekeeper, I want Honey Reserve charging unchanged (one token per dispatch), so that a profile is not a pricing feature.
67. As a Beekeeper using `defaults.energy_budget` or cue honey, I want those untouched by profiles, so that adapter tests do not silently shrink reserves.
68. As a platform contributor, I want resolve/merge unit tests without launching a real LLM CLI, so that CI stays hermetic.
69. As a Beekeeper, I want a missing home `adapters/pi.yaml` after remap to keep today’s defaults (`binary: pi`), so that a colony-only profile still runs if Pi is on PATH.
70. As a Beekeeper, I want invalid YAML in a selected profile file to fail with the file path, so that a broken recipe is obvious.

## Implementation Decisions

### 1. Naming

- Technical term: **profile**. CLI: `--profile`, `--no-profile`, env `PASEKA_PROFILE`, home key `profile`.
- No bee-language synonym. Do not call this a Cue, Nuc, or hive.
- Profile **name** = colony filename stem (`.paseka/profiles/pi.yaml` → `pi`) and the home directory name (`profiles/pi/`). Names on both layers for the same process must match the selected name; there is no aliasing.

### 2. Layout

Colony (one file per name):

- `.paseka/profiles/<name>.yaml` — committed. Init gitignore unchanged (`*.local.yaml` still ignores bee locals, not these files).
- No `.paseka/profiles/<name>.local.yaml` in this slice.

Home (directory mirroring slug layout):

- `~/.config/paseka/<slug>/profiles/<name>/config.yaml` — optional overlay of `nats` and `model_aliases` only.
- `~/.config/paseka/<slug>/profiles/<name>/adapters/<adapter>.yaml` — optional overlay of that adapter’s existing home schema (`binary`, `api_key_env` as today).
- Sticky default: `profile: <name>` on the slug’s `config.yaml` only.

A selected name is **found** when at least one of: the colony file exists; the home profile directory contains at least one of those loadable files. Otherwise fail closed.

### 3. Colony profile schema

Allowed top-level keys only:

- `adapter` — string; LLM adapter name; **replaces** `Bee.adapter` for every LLM bee without `command:`
- `params` — map; merged onto those remapped bees (profile keys replace same keys)
- `model_aliases` — map; same shape and validation as spec 019
- `bees` — map of role name → `{ adapter?, params? }`

Any other key is a load error. Per-bee entries allow only `adapter` and `params`.

`adapter` / `bees.*.adapter` must be one of `cursor`, `pi`, `claude`, `opencode`. `script` is rejected. Empty `adapter` on a per-bee entry means “do not replace this bee’s adapter” (params-only exception). Empty top-level `adapter` means no global replace.

### 4. Adapter replace rules

Effective adapter for a bee:

1. If profile `bees.<role>.adapter` is set → use it (even on `command:` bees).
2. Else if profile global `adapter` is set **and** the bee’s committed adapter is an LLM adapter **and** the bee has no `command:` → use the global profile adapter (replace, do not default-fill).
3. Else → committed `Bee.adapter` (empty still defaults to `cursor` as today).

LLM adapters: `cursor`, `pi`, `claude`, `opencode`. After replace, `ResolveAdapter` / `AdapterExtra` run on the **effective** name so home credentials follow the remap.

Global profile `params` apply only to bees remapped by step 2 (or that already matched the global adapter and are LLM without `command:`). Per-bee `params` apply to that role whenever the role exists, including `command:` bees (params remain ignored at argv build when `command:` is set, same as today; doctor may warn).

### 5. Merge order

Highest wins within each concern:

1. Committed colony + bees
2. Colony profile
3. Home slug `config.yaml` + `adapters/*.yaml`
4. Home profile `config.yaml` + `adapters/*.yaml`
5. `bees/<role>.local.yaml` (prompt templates only, unchanged)
6. Env (`PASEKA_NATS_URL`, existing adapter key env vars)

`model_aliases`: colony map, then colony profile, then home config, then home profile; then spec 019 validate on the result.

Adapter yaml: load slug adapter file (with today’s missing-file defaults), then overlay home-profile adapter file field-by-field where the overlay value is non-empty.

Apply profile during context resolve for the process so AFK dispatch, HITL session start, reactor, doctor, and status share one effective view.

### 6. CLI and env

- Root persistent flags: `--profile string`, `--no-profile` (bool).
- `--no-profile` with `--profile` set → usage error.
- `--profile` with empty or whitespace name → usage error.
- `PASEKA_PROFILE` empty/unset → not a selection (fall through to sticky). Non-empty invalid characters → error (do not treat as none).
- Home sticky invalid or missing profile → `ResolveContext` fails (fail closed), including commands that load context.

### 7. Observability

- Doctor: effective profile (or none); layers that loaded; remapped roles; skipped `script` / `command:` roles; available profile names.
- Status snapshot: effective profile name field (empty string if none).
- AFK run `meta.json` and HITL session handle/record: profile name (empty if none).
- Queen Console uses the process overlay; no SPA profile picker in this slice.
- Export of bee YAML remains files on disk. In-process topology uses effective bees.

### 8. Init, Nuc, Honey

- `paseka init` does not create profiles or a sticky `profile:` key.
- Nuc export/import does not include `.paseka/profiles/`.
- Honey, cues, worktrees, routing contracts unchanged.

### 9. Modules

- Colony load/merge owns profile file discovery, schema validation, bee remap, alias merge, home-profile adapter overlay.
- Context carries the effective profile name (empty if none) and already-merged home adapter structs / alias map.
- Queen Shell root flags populate the name before `ResolveContext`.
- Runtime AFK + session manager persist the name on run/session metadata; they do not re-parse profile files.
- Doctor and status read the name from context.

## Testing Decisions

Good tests assert **external behavior**: selected name, fail-closed errors, effective `Bee.adapter` / `RunParams` after merge, merged alias map, NATS URL, adapter binary/`api_key_env`, skipped script/`command:` bees, meta/status fields. Do not assert helper function names.

Cover at least:

- Colony load/merge: missing name lists available; replace vs skip script/`command:`; per-bee exception; forbidden keys; `script` rejected; unknown adapter; per-bee unknown role; alias merge order and 019 validation; home-only and colony-only profiles; empty home dir not found.
- CLI selection: flag vs env vs sticky vs `--no-profile`; conflict `--no-profile` + `--profile`; empty `--profile`.
- Dispatch/session: remapped bee launches the resolved adapter extra (binary/key) with a fake binary; `meta.json` / session record has the name.
- Doctor/status: effective name present.
- Init: still no profile files.

Prior art: spec 019 merge/validate tests; home adapter load tests; `BeeRun` fake-binary adapter tests (Pi/OpenCode style).

## Out of Scope

- Implicit profile synthesized from a known adapter name when no files exist
- Sticky default in `colony.yaml` or in a profile file
- `paseka profile list|show|init` command family
- `.paseka/profiles/<name>.local.yaml`
- Queen Console profile picker / hot-swap on a running hive
- Overlay of routing, worktree, sector, prompts, `command:`, `post_exec`, energy, cues
- Turning bees into `script` via profile
- Forking a second home slug directory per profile
- Nuc packing of profiles
- Per-adapter alias tables (still one map, spec 019)
- Changing `paseka init --adapter` to write profiles instead of bee YAML
- Windows-specific path behavior beyond existing home-dir helpers

## Further Notes

- Durable docs after ship: colony layout (two-tier + profiles), bee config (effective adapter), CLI root flags, architecture overview (context resolve). Glossary: no new bee-language term.
- Concurrent processes with different profiles share git worktrees and home `state.json`; operators comparing adapters should use separate traces. If that bites, a later spec can isolate state — not this one.
- `LoadAllBees` consumers used for **committed** topology export should keep reading files; in-process doctor/status/reactor must use the profiled view. Implementers should not silently mix the two.
- Related: [019-model-aliases](019-model-aliases.md) (alias overlay extended by one hop), [001-pi-integration](001-pi-integration.md) / [026-opencode-adapter](026-opencode-adapter.md) (adapter testing is the motivating use), bee local overlays remain prompt-only.
