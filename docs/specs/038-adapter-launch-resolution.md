# Spec 038: Adapter launch resolution

## Status

**Implemented** — all eight phases shipped. The deprecation window (Phases 3–4) stays **open for one full release**: the retired keys (`trust`/`force`/`thinking`/`provider`/`output_format`) keep working with a `paseka doctor` deprecation warning, and removal is deferred to the next release.

Turns the launch model in [Adapter launch resolution](../reference/adapter-launch-resolution.md) into a phased implementation plan; that reference is the authoritative description of the shipped behavior and this spec is the execution record.

## Problem Statement

Launching an agent is the single most cross-cutting behavior in Paseka, and today four code paths resolve the same layered config independently while each adapter re-assembles argv and environment on its own. A Beekeeper therefore cannot trust a committed config:

- A committed `params.binary` never takes effect, because machine-local loaders fill a default and non-zero-merge lets the machine layer always win.
- A profile or machine layer cannot turn a colony boolean (e.g. `trust`) off — booleans merge one-way to true.
- A typo in `params` (`modle:`, `plan_mode:`) is a silent no-op with no operator feedback.
- The bee-visible params surface exposes vendor flags as colony decisions (`output_format`, `trust`, `force`, `thinking`, `provider`), so every bee repeats boilerplate and semantics drift per adapter.
- Interactive and AFK launches can diverge because they merge layers in two separate bodies.
- A credential handed to a bee does not reach its `post_exec` hook.

The cost is an operator who edits YAML and cannot predict, or explain, what process will run.

## Solution

One presence-aware config ladder, one shared launch resolver, and a vendor-neutral params surface.

- **One `LaunchPlan`.** `ResolveLaunch(bee, colonyContext, mode)` is the only place layers merge; AFK and interactive both consume the resulting plan. Adapters become pure argv mappers.
- **Declared keys only.** A layer contributes a key only when the file/config actually sets it; a loader default is not a declaration. Repo config wins over machine fallback for `binary`; `api_key_env` stays a machine fact.
- **Semantic params.** Bees describe colony decisions (`model`, `yolo`, `plan`, `reasoning`, `binary`); the adapter maps them to its own flags. Output format becomes runtime-owned.
- **Provider dissolves into the model id.** Alias values become `vendor/model[:suffix]`; the runtime maps them per adapter.
- **A plan, not an accident, for env and process control.** Assembled environment with declared pass-through and plan-aware redaction, plus an opt-in `params.timeout`.
- **Attribution is visible.** The resolver records which layer set each field; `paseka doctor` and the Console can explain a launch without reading Go.

## User Stories

1. As a Beekeeper, I want a committed `params.binary` to win over an absent machine-local file, so that a pinned executable in the repo actually runs.
2. As a Beekeeper, I want a profile to set a colony boolean to `false`, so that a role can opt out of an autonomous default.
3. As a Beekeeper, I want a typo in `params` to fail at load time with the list of valid keys, so that a silent no-op becomes a fixable error.
4. As a Beekeeper, I want AFK and interactive launches to resolve layers through the same code, so that a bee behaves the same whether run headless or in `bee chat`.
5. As a Beekeeper, I want to write `yolo: true` instead of guessing whether an adapter needs `trust`, `force`, or both, so that one semantic switch means "do not ask a human".
6. As a Beekeeper, I want `plan: true` to win over `yolo`, so that a planning run never mutates even when the role default is autonomous.
7. As a Beekeeper, I want `reasoning: high` to mean the same thing across adapters, so that I do not learn each vendor's flag name.
8. As a Beekeeper, I want `output_format` out of my bee files, so that each bee stops repeating a runtime concern.
9. As a Beekeeper, I want the runtime to always request structured output, so that usage and `providerSessionId` accounting never silently degrade.
10. As a Beekeeper, I want a `--raw` escape hatch on `paseka bee run`, so that I can debug a provider with plain text without committing it.
11. As a Beekeeper, I want a model alias to carry its vendor (`high: anthropic/claude-sonnet-4`), so that switching vendor and model is one atomic edit.
12. As a Beekeeper, I want switching a profile to switch the vendor for every bee on an alias, so that "run this repo against Ollama" is one profile edit.
13. As a Beekeeper, I want a runtime-owned split of `vendor/model` that preserves colons and inner slashes, so that OpenRouter-style ids survive the trip to Pi.
14. As a Beekeeper, I want `provider` as a separate key to be removed with a deprecation warning, so that old colonies keep working for one release.
15. As a Beekeeper, I want `trust`/`force` accepted for one release and mapped to `yolo`, so that existing bees do not break on upgrade.
16. As a Beekeeper, I want my machine-local `api_key_env` to keep winning, so that secrets stay a per-machine fact.
17. As a Beekeeper, I want to declare extra environment for a bee (`params.env`), so that a wrapper-only variable stops being the only way to pass one.
18. As a Beekeeper, I want the launch plan to record which credential it injected, so that logs and the Console can redact it.
19. As a Beekeeper, I want an opt-in strict env mode that strips undeclared secrets, so that a shared apiary can fence a child process without breaking shell-exported keys by default.
20. As a Beekeeper, I want `post_exec` to inherit the same environment as the run, so that a credential passed to a bee reaches its post-run hook.
21. As a Beekeeper, I want `params.timeout` on AFK runs, so that a hung `agent -p` releases its worker slot instead of waiting for a manual kill.
22. As a Beekeeper, I want interactive sessions to stay unbounded, so that a human remains the timeout.
23. As a Beekeeper, I want a timed-out run to report `cancelled` with reason `timeout`, so that a slow run is distinguishable from a stuck one.
24. As a Beekeeper, I want `paseka doctor --explain <bee>` to print the resolved launch plan with the layer that set each field, so that I can debug precedence without reading Go.
25. As a Beekeeper, I want the Console adapter panel to show declared-vs-default per setting, so that the UI stops implying a default was something I wrote.
26. As a Beekeeper, I want `command:` plus non-default `params` to be a load-time error, so that a contradictory bee fails on init rather than at dispatch.
27. As a Beekeeper, I want `adapter: script` without `command:` to fail at load time, so that the requirement surfaces before a run starts.
28. As a Beekeeper, I want `command:` templates and params mode to receive the same plan values, so that custom argv can use `{{.ResultFile}}` and the plan env alike.
29. As a Beekeeper, I want a profile with a global `adapter:` to report which bees it skipped and why, so that `script` and custom-argv bees are not silently ignored.
30. As a Bee author, I want the committed bee files to lose three repeated boilerplate keys, so that each bee describes only what is specific to it.
31. As a Bee author, I want a `reasoning` value the adapter does not map to be reported by `doctor` rather than silently dropped, so that I know it had no effect.
32. As a Bee author, I want an unknown `api_key_env` target to be visible, so that a credential that resolves to empty is not mistaken for valid auth.
33. As a Console operator, I want the adapter panel to reflect the same resolution the runtime uses, so that the UI cannot disagree with a run.
34. As a maintainer, I want one resolver so that adding a fifth adapter touches one argv mapper and one default, not four dispatch sites.
35. As a maintainer, I want a characterization test per launch path before the refactor, so that behavior-preserving steps are provably preserving.
36. As a maintainer, I want the machine-local overlay to use presence-aware reads everywhere, so that "default" and "declared" stop being conflated.
37. As a maintainer, I want the deprecation window to be a single release with tests, so that cleanup is mechanical rather than open-ended.

## Implementation Decisions

### One resolver, one plan

Introduce a `LaunchPlan` (binary, argv, env, cwd, timeout, plan mode, and per-field attribution). `ResolveLaunch` is the only function that merges layers; it runs for AFK (all four dispatch entries) and interactive alike, and `mode` changes only flag mapping and process ownership. Adapters expose a pure argv mapper (`BuildArgs`) and stop reading home config, profiles, or environment. Interactive-only extras (Cursor `create-chat`, OpenCode `serve` credential pair/pre-session) attach to the plan as a post-map hook rather than a second resolution. The `preparedDispatch`/`manager.launch` bodies shrink to plan consumption.

### Presence-aware layering (explicit ladder)

Precedence, lowest to highest, with "declared only" semantics:

1. Adapter built-in defaults (`defaultBinary`, per-adapter `trust/force` defaults documented).
2. Committed `colony.yaml` (`model_aliases`, `defaults`, `sectors`).
3. Bee YAML (`params`, `command`, `worktree`, `sector`) — the primary description of the work.
4. Selected profile: committed `.paseka/profiles/<name>.yaml`, then machine `profiles/<name>/`. A process-level switch stays above the bee.
5. Machine-local `adapters/<name>.yaml` — declared keys only; `binary` is a **fallback** (repo wins), `api_key_env` always wins.
6. Model alias resolution — once, after all merging, before argv build.
7. Process env (`PASEKA_PROFILE`, `PASEKA_NATS_URL`, credential vars).

The declared-key reader generalizes the existing `AdapterConfigDeclared` idea: a loader returns both the config and the set of keys the file actually declared, so defaults never masquerade as declarations. Merge becomes presence-aware and tri-state (a boolean can be explicitly `false`).

### Semantic params surface

Bee-visible keys reduce to colony decisions:

| Key | Values | Per-adapter mapping |
| --- | ------ | ------------------- |
| `model` | alias or vendor id | `--model` (opencode gets `provider/model`) |
| `yolo` | tri-state bool | cursor `--trust --force`; claude `bypassPermissions`; opencode `--auto`; pi none (doctor-reported) |
| `plan` | tri-state bool, wins over `yolo` | cursor/pi `--plan`; opencode `--agent plan`; claude `permission-mode plan` |
| `reasoning` | `low`/`medium`/`high`/passthrough | pi `--thinking`; opencode `--variant`; cursor/claude unmapped (doctor-reported) |
| `binary` | string | pins executable for this bee; wins over machine fallback |

`output_format` is removed from the bee schema and becomes runtime-owned: the runtime always requests each adapter's structured mode (cursor `stream-json`, pi `mode json`, claude `stream-json --verbose`, opencode `run --format json`). A per-invocation `paseka bee run --raw` degrades to plain text and logs the lost runtime features. `trust`/`force` and `thinking` are accepted for one release with deprecation mapping (`trust || force` → `yolo`; `thinking` → `reasoning`).

### Provider dissolves into the model id

Alias values become full ids (`high: anthropic/claude-sonnet-4`, `small: openai/gpt-5.5`); a bare id without `/` stays valid. The runtime maps the resolved id per adapter: cursor/claude pass through; opencode consumes `provider/model` natively (the `resolveModel` join is deleted); pi splits at the **first** `/` into `--provider <vendor> --model <rest>`, where `rest` keeps all inner slashes and colon suffixes (`openrouter/deepseek/deepseek-r1:free` → `--provider openrouter --model deepseek/deepseek-r1:free`). `params.provider` is removed after a deprecation release that maps `provider: X` + `model: Y` → `X/Y` and warns via `doctor`. Profile vendor switching needs no new machinery: profiles already overlay `model_aliases`.

### Command selection and exclusivity

Selection stays implicit (params mode with no `command`; command mode with `command`; script mode with `adapter: script`). Exclusivity moves to load time: `command` + non-default `params` is an error (today a runtime warning), and `adapter: script` without `command` is a load-time error (today runtime-only). Command-mode templates and params mode receive the same plan values, including the plan env, so custom argv and mapped argv share path/var conventions.

### Environment and secrets

`LaunchPlan.Env` is assembled, not inherited by accident: `os.Environ()` base; the credential named by the resolved `api_key_env` guaranteed and recorded for redaction; bee/machine `params.env` declared pass-through (literal values or host-var references, e.g. `GITHUB_TOKEN: $HOST_GITHUB_TOKEN`). Undeclared vendor keys keep flowing in by default. `params.env_mode: strict` (opt-in) strips undeclared secrets. `PASEKA_*` keeps its script meaning and extends to `post_exec`.

### Process control and timeout

Add opt-in `params.timeout` for AFK runs (default unbounded to preserve behavior), measured from launch, enforced as SIGTERM → short grace → SIGKILL, surfaced as `status: cancelled` with reason `timeout`, and emitted on `status.json` so a post-mortem can tell "slow" from "stuck". Interactive sessions stay unbounded.

## Implementation Plan

Phases are ordered by dependency. Each phase is independently shippable and leaves the tree green; a phase that changes behavior carries its own migration. Phases 1 and 2 are the backbone; 3–8 build on them.

### Phase 0 — Characterization baseline

- **Goal:** pin observable behavior of all four AFK entries and the interactive entry before the refactor.
- **Steps:** add end-to-end launch tests that assert resolved binary, argv, cwd, and child env for cursor/pi/claude/opencode/script on AFK dispatch, reactor task/direct, Console run, and interactive. Capture with a fake adapter that records the request; reuse the existing dispatch test harness.
- **Deliverable:** green characterization suite that fails loudly if a "behavior-preserving" refactor changes argv/env.
- **Acceptance:** every adapter has at least one argv assertion and one env assertion per launch mode.
- **Risk:** low.

### Phase 1 — Presence-aware machine-local overlay

- **Goal:** declared keys only; `binary` precedence fixed; tri-state booleans; unknown-param validation.
- **Steps:**
  1. Generalize the declared-key reader (`AdapterConfigDeclared`) into a loader result that carries declared keys, and have each adapter loader preserve declared-ness instead of filling defaults into the loaded struct.
  2. Make `AdapterExtra`/adapter layer carry a declared `Binary` only when the machine file set it; make `api_key_env`/key resolution explicit.
  3. Replace non-zero-wins `MergeRunParams` with a presence-aware, tri-state merge; document each adapter's default for `yolo` equivalents.
  4. Validate bee `params` keys at load time; unknown keys error with the valid-key list.
  5. Update `paseka doctor`/Console attribution to use the declared-key reader for all adapters (it currently uses it only for one path).
- **Deliverable:** repo `params.binary` wins; a profile/machine layer can set `false`; typo fails at load.
- **Acceptance:** tests for (a) declared machine binary overridden by bee, (b) absent machine file falling back to default, (c) profile `false` beating colony `true`, (d) unknown key error.
- **Rollback:** behavior-changing; gate behind a release note. Revert by restoring default-filling loaders.
- **Risk:** medium — this is the precedence change operators rely on.

### Phase 2 — `ResolveLaunch` → `LaunchPlan`

- **Goal:** one resolver, two consumers.
- **Steps:**
  1. Introduce `LaunchPlan` and `ResolveLaunch` in the runtime layer; extract the interactive launch body as the seed implementation.
  2. Port AFK `prepareDispatch` to consume the plan; keep adapter `Run` reading plan-provided binary/args/env/cwd.
  3. Give adapters a pure `BuildArgs(params, request)` mapper; move env assembly into the plan.
  4. Attach interactive-only extras as a post-map hook (Cursor `create-chat`, OpenCode serve/credential pair).
  5. Retire duplicated merge/prompt-join/workspace-pick bodies so there is exactly one of each.
- **Deliverable:** single resolution site; identical behavior on the Phase 0 suite.
- **Acceptance:** characterization suite passes unchanged for unchanged inputs; grep shows one params-merge and one prompt-join call site.
- **Rollback:** mechanical; both callers can temporarily keep old bodies.
- **Risk:** medium-high — touches every launch. Mitigated by Phase 0.

### Phase 3 — Semantic params reshape

- **Goal:** `yolo`/`plan`/`reasoning`/`model`/`binary`; runtime-owned output format; `--raw`.
- **Steps:**
  1. Add the new keys and per-adapter mapping; keep `trust`/`force`/`thinking` as deprecation aliases for one release.
  2. Make structured output runtime-owned for all adapters; implement `paseka bee run --raw` and log the lost runtime features.
  3. Add `plan` wins over `yolo` precedence and tri-state handling.
  4. Update committed `.paseka/bees/*.yaml` and `paseka init` templates to drop the boilerplate keys.
  5. Update bee-config docs and `doctor` reporting for unmapped `reasoning` and missing `yolo` equivalents.
- **Deliverable:** bees carry only colony decisions; deprecation warnings on old keys.
- **Acceptance:** a bee with `yolo: true` produces the same argv as the old `trust+force`; a `--raw` run logs degraded parsing; unknown/removed key handling is tested.
- **Rollback:** keep the alias path; revert bees to old keys.
- **Risk:** medium — schema change with a migration window.

### Phase 4 — Provider dissolves into model id

- **Goal:** remove `params.provider`; alias values carry vendor.
- **Steps:**
  1. Extend alias validation to accept `vendor/model[:suffix]` and bare ids.
  2. Map resolved id per adapter: opencode native; pi split at first `/` preserving the rest; cursor/claude pass through.
  3. Remove `params.provider`; deprecation release maps `provider` + `model` → `X/Y` and warns.
  4. Add the Pi split-rule tests (`openrouter/deepseek/deepseek-r1:free` → provider + full rest).
  5. Update `colony.yaml`/profile alias examples and reference docs.
- **Deliverable:** one atomic alias edit moves vendor and model; profile vendor switch works through existing alias overlay.
- **Acceptance:** table-driven adapter mapping tests including colon suffixes and bare ids; deprecation mapping test.
- **Rollback:** re-introduce `provider` key; alias values can stay combined.
- **Risk:** medium — interacts with Phase 3.

### Phase 5 — Command/script exclusivity and plan-driven env

- **Goal:** load-time exclusivity; assembled env; `post_exec` inheritance; declared pass-through.
- **Steps:**
  1. Move `command`+params and `script`-requires-`command` checks to bee load/validate.
  2. Route `command` templates through the plan values, including env.
  3. Assemble `LaunchPlan.Env`: base + guaranteed credential + `params.env` pass-through; record injected key for redaction; update log redaction to be plan-aware.
  4. Give `post_exec` the plan env (today `os.Environ()`).
  5. Add `params.env_mode: strict` as an opt-in.
- **Deliverable:** contradictory bees fail at load; credentials reach post-run hooks; redaction is plan-aware.
- **Acceptance:** tests (a) command+params load error, (b) script without command load error, (c) declared env reaching child and post_exec, (d) redaction of injected key, (e) strict mode strips undeclared secret.
- **Rollback:** keep the old warning path behind a flag for one release.
- **Risk:** medium — load-time errors can break existing colonies; announce.

### Phase 6 — Timeout and process control

- **Goal:** opt-in `params.timeout`.
- **Steps:**
  1. Add `Timeouts` to the plan; default unbounded for AFK and interactive.
  2. Implement kill → grace → SIGKILL and status transition to `cancelled` with reason `timeout`.
  3. Emit the plan timeout on `status.json`.
  4. Document `params.timeout` in bee-config.
- **Deliverable:** hung runs release workers; post-mortems can distinguish slow from stuck.
- **Acceptance:** a fake adapter that sleeps is killed and reports `cancelled`/`timeout`; interactive unaffected.
- **Rollback:** remove the key; behavior reverts to unbounded.
- **Risk:** low.

### Phase 7 — Attribution surfaces (doctor / Console)

- **Goal:** make precedence explainable.
- **Steps:**
  1. Have `ResolveLaunch` return per-field layer attribution.
  2. Add `paseka doctor --explain <bee>` printing the plan and the layer that set each field.
  3. Feed the same attribution to the existing Console adapter panel so it shows declared-vs-default per setting.
- **Deliverable:** operator can explain any launch without reading Go.
- **Acceptance:** golden output for a bee whose `binary`/`model`/`yolo` come from different layers.
- **Rollback:** additive.
- **Risk:** low.

### Phase 8 — Migration cleanup and docs

- **Goal:** ship the semantic surface, keep the deprecation window open for one full release, and record the ship.
- **Steps:**
  1. Keep `trust`/`force`/`thinking`/`provider` and `output_format` in the schema for one full release so an operator upgrading from the previous release has a migration path; `paseka doctor` warns with the replacement (`trust`/`force` → `yolo`, `thinking` → `reasoning`, `provider` → fold the vendor into `model`, `output_format` → runtime-owned). Removal is **deferred / release-gated** — it lands in the release after one ships with the keys deprecated (see the [backlog](../plans/backlog.md#remove-the-legacy-params-aliases-after-a-release-ships-with-them-deprecated)).
  2. Update [Adapter launch resolution](../reference/adapter-launch-resolution.md) from "Proposed" to shipped and update bee-config/colony-layout guides.
  3. Add a changelog entry, mark this spec Implemented, re-add the deferred removal to the backlog.
- **Deliverable:** the semantic surface is authoritative and documented; the legacy keys keep loading with a deprecation warning until the next release.
- **Acceptance:** legacy keys load and `doctor` names the replacement; docs reference shipped behavior.
- **Rollback:** none needed (additive).
- **Risk:** low.

## Testing Decisions

- **External behavior only.** Tests assert the resolved process contract — binary, full argv, child env, cwd, and reported status — never internal merge order. Prior art: the dispatch tests that exercise `Dispatch` with a fake adapter, and the existing params/adapter unit tests.
- **Characterization first.** Phase 0 freezes argv/env per adapter per launch mode; later phases must keep those assertions green except where the phase's own acceptance changes them.
- **Table-driven adapter mapping.** One table per adapter maps `LaunchPlan`/params to expected argv, covering `yolo`/`plan`/`reasoning`, structured output, and the Pi provider split (colons, inner slashes, bare ids).
- **Layering matrix.** A table asserting which layer wins per field, including declared-vs-default and tri-state `false`, plus unknown-key errors.
- **Interactive parity.** A test that resolves the same bee in AFK and interactive mode and asserts the shared fields are identical, differing only by the documented interactive extras.
- **Doctor/Console attribution.** Golden-file test for `doctor --explain` and a handler test for the adapter panel's declared-vs-default pairs.
- **Env/redaction.** Tests for injected-credential redaction, declared pass-through, strict mode, and `post_exec` inheritance.

## Out of Scope

- Deny-by-default environment for all runs — `env_mode: strict` is the opt-in first iteration; flipping the default is a separate decision.
- Adding a fifth adapter — the resolver makes it cheaper but a new adapter is its own spec.
- Provider-side token billing/honey coupling (tracked in the backlog under "Energy and honey").
- Migrating the Console roster/`GET /api/system/adapters` contract beyond the attribution changes.
- Per-bee or per-cue editing UI in the Console.

## Further Notes

**Implementation anchors (non-durable; the reference doc is authoritative while the code moves).** Resolution sites to collapse: `dispatch_stages.go` (params merge, prompt join, command render) and `sessions/manager.go` (same). Layer loaders: `colony/home.go` (machine overlay + `AdapterConfigDeclared`), `colony/profile_merge.go` (profile overlay, `ApplyBeeProfile`), `colony/model_aliases.go`. Params: `colony/params.go`. Adapters: `adapters/{cursor,pi,claude,opencode}` `buildArgs` plus `adapters/script`. Env: `adapters/scriptenv.go`, `colony/exec.go` (`os.Environ()` in `ExecArgv`). Workspace pick: `runtime/colony_dispatch.go`. Doctor entry: `cmd/paseka/nats.go`. These will drift; symbol names are the stable handle.

**Deliberate consequences.** A machine with no `adapters/cursor.yaml` gets `agent` from built-in defaults; a bee pinning `params.binary: agent-next` finally wins over an absent machine file. Operators relying on machine-local-always-wins will see a change in Phase 1 and should be told in that release's notes.

**Resolved decisions.** The deprecation window is a single release and stays **open**: `trust`/`force`/`thinking`/`provider`/`output_format` still load with a `paseka doctor` deprecation warning, and Phase 8 removal is **deferred / release-gated** so a release ships with the keys deprecated before they are removed. `--raw` is `paseka bee run` only; interactive `bee chat` owns its TTY and never opts out of structured output.

Related: [Adapter launch resolution](../reference/adapter-launch-resolution.md) · [bee config](../guide/bee-config.md) · [colony layout](../guide/colony-layout.md) · [config profiles](027-config-profiles.md) · [model aliases](019-model-aliases.md) · [architecture overview](../architecture/overview.md).
