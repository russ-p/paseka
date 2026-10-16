# Adapter launch resolution

A bee is YAML; an adapter is an OS process. Everything in between — which binary, which argv, which env, which cwd — is decided by the **launch resolution** documented here. This is the cross-cutting view of `adapter`, `params`, `command`, profiles, and machine-local config; field-level schema lives in [bee config](../guide/bee-config.md) and [colony layout](../guide/colony-layout.md).

The resolver is the single place layers merge: `launch.ResolveLaunch` (`internal/launch/launch.go`) returns a `LaunchPlan` used by both AFK runs and interactive sessions. Line numbers drift; symbol names are the stable handle.

```go
// internal/launch/launch.go
type LaunchPlan struct {
    Adapter string
    Mode    Mode              // afk | interactive
    Form    Form              // params | command | script
    Binary  string            // params.binary or the adapter's built-in default
    Params  adapters.RunParams // fully merged, aliases applied
    Command []string          // rendered bee command argv (command/script forms)
    Env     []string          // complete child environment, or nil to inherit
    RedactedEnv []string      // env keys logs must mask
    Dir     string            // cwd
    Timeout time.Duration     // AFK only; 0 = unbounded
    // ...
}
```

---

## 1. Launch modes

| Mode | Entry | Process owner |
| ---- | ----- | ------------- |
| AFK CLI | `paseka bee run <role>` → `runtime.BeeRun` | adapter (`exec.CommandContext`) |
| Reactor task | `runtime.DispatchColonyBee` (task) | adapter |
| Reactor direct | `runtime.DispatchColonyBee` (direct) | adapter |
| Console run | `GET /api/bees/:role/runs` | adapter |
| Interactive | `paseka bee chat <role>` → `sessions.Manager.launch` | runtime PTY |
| Invite / resume | `internal/invites/service.go`, `internal/sessions/resume.go` | runtime PTY |

AFK and interactive both call `launch.ResolveLaunch`; the mode only changes argv mapping and who owns the process. Interactive keeps its extras (cursor `create-chat`, opencode `opencode serve` credential pair) as a post-map hook on the plan, not a second resolution.

Workspace (cwd) = colony root, or `.paseka/worktrees/<traceId>/` when `worktree: true` (or an isolated `MUTATION` proposal forces it), plus the sector subpath. `.paseka/runs/<traceId>/<agentId>/` IPC always lives under the colony root.

---

## 2. Configuration layers

Presence-aware layering: a layer only contributes keys it **actually declares** — a loader default is not a declaration. Lowest to highest precedence:

1. Adapter built-in defaults — `defaultBinary` constants plus the documented `RunParams` zero value.
2. Committed colony config — `colony.yaml` `model_aliases`, `defaults`, `sectors`.
3. Bee YAML — `params`, `command`, `worktree`, `sector`.
4. Selected profile — committed `.paseka/profiles/<name>.yaml`, then machine `profiles/<name>/` (`profile_merge.go`). A profile is a process-level switch and stays above the bee; it contributes declared keys only.
5. Machine-local `adapters/<name>.yaml` — declared keys only, and only as a *fallback* for `binary` (repo wins when set). `api_key_env` is a machine fact and always wins; the key itself is never a launch parameter.
6. Model alias resolution — once, after all merging, before argv build (`ApplyModelAliases`).
7. Process env — `PASEKA_PROFILE`, `PASEKA_NATS_URL`, credential vars.

Profile selection: `--profile` flag → `PASEKA_PROFILE` → sticky `profile:` in home `config.yaml` → none (`profile.go`). Unknown profile fails closed and lists what exists on both layers.

**Binary asymmetry.** `AdapterExtraLayer` (`internal/colony/params.go`) declares `binary` only when the machine file actually set one, so a committed `params.binary` always wins and an absent machine file leaves the adapter default as the true fallback. `api_key` always comes from the machine layer. `MergeRunParamsLayers` is presence-aware and tri-state: a declared boolean applies even when false, so a profile can turn a colony default off.

---

## 3. Params (`adapters.RunParams`)

Bee-visible `params` carry **only colony-level decisions**; vendor flags are the adapter's business. The closed key set is `RunParamKeys` (`internal/colony/params.go`):

| Key | Values | Decision it expresses | Per-adapter mapping |
| --- | ------ | --------------------- | ------------------- |
| `model` | alias or vendor id | which model to run; alias-resolved once before argv build | cursor/claude pass it through; opencode consumes `provider/model` natively; pi splits the vendor (below) |
| `yolo` | bool | run without asking a human | cursor `--trust --force`; claude `--permission-mode bypassPermissions`; opencode `--auto`; pi: no equivalent (doctor warns) |
| `plan` | bool | read-only planning; wins over `yolo` | cursor/pi `--plan`; opencode `--agent plan`; claude `--permission-mode plan` |
| `reasoning` | `low` \| `medium` \| `high` \| `passthrough` | reasoning effort | pi `--thinking`; opencode `--variant`; cursor/claude unmapped today (doctor warns) |
| `binary` | string | pin an executable for this bee | wins over machine-local; machine-local stays a fallback |
| `env` | map | declared pass-through environment | values are literals or `$HOST_VAR` references; declared keys are redacted in logs |
| `env_mode` | `strict` | deny undeclared environment | child keeps only process essentials, declared `env`, and the injected credential |
| `timeout` | duration | bound an AFK run | runtime process control, not an adapter flag |

Unknown keys fail at load time with the valid-key list (`ValidateRunParams`), so typos stop being silent no-ops. `LayerFromBee` maps the block and records the declared set so merging and attribution stay tri-state (`yolo` drives the internal `trust`/`force` pair the adapters still take).

**Runtime-owned output format.** The runtime always requests each adapter's structured mode, because usage, `providerSessionId`, and summary normalization are parsed from it:

| Adapter | Runtime-requested structured mode |
| ------- | --------------------------------- |
| cursor | `--output-format stream-json` |
| pi | `--mode json` |
| claude | `--output-format stream-json --verbose` |
| opencode | `run --format json` |

Debugging escape hatch: `paseka bee run --raw` (per invocation, never committed). When `--raw` costs a runtime feature — cursor loses `usage`/`providerSessionId`, session tracking degrades — the run log says so.

**Migration is over.** `trust`, `force`, `thinking`, `provider`, and `output_format` are removed from the schema. A bee or profile that still declares one fails to load; `RemovedRunParamKeys` names the replacement (`trust`/`force` → `yolo`, `thinking` → `reasoning`, `provider` → fold the vendor into `model`, `output_format` → runtime-owned). `paseka doctor` loads the bee leniently (`LoadAllBeesForDiagnosis`) so it can report the leftover as a warning instead of just failing the strict load.

**Attribution** is kept: `ResolveLaunch` returns which layer set each field, driving `paseka doctor --explain <bee>` and the Console adapter panel.

### Provider lives in the model id

`provider` is no longer its own key. Provider and model are one atomic pair — switching vendor while keeping the id produces a broken combination — so the vendor moves into the alias value:

- Alias values are full ids: `high: anthropic/claude-sonnet-4`, `small: openai/gpt-5.5` (`colony.yaml` / home / profile `model_aliases`). A bare id without `/` (`composer-2.5`) stays valid — no provider.
- `launch.MapModel` (`internal/launch/modelid.go`) maps the resolved id per adapter: cursor/claude pass it through; opencode consumes `provider/model` natively; **pi splits at the first `/`** → `--provider <vendor> --model <rest>`, where the rest keeps inner slashes and colon suffixes.
- Profile override of the vendor needs no new machinery — profiles already overlay `model_aliases` (spec 019). `.paseka/profiles/local.yaml` setting `model_aliases: { high: ollama/llama3 }` moves every bee on `high` to that vendor **and** model atomically.

**Verified against pi 1.0.0** (`@earendil-works/pi-coding-agent`; `dist/core/model-resolver.js` — `parseModelPattern`, `resolveCliModel`, `findExactModelReferenceMatch`). The `:<suffix>` collision is real and handled:

- `--model` documents "supports `provider/id` and optional `:<thinking>`" — thinking levels are `off, minimal, low, medium, high, xhigh, max`, so `:free` looks like a thinking level to pi.
- But pi **exact-matches the full pattern against its catalog first**, and its own catalog entries carry colon suffixes (`deepseek/deepseek-v4.1-flash:batch` appears in `pi --list-models`). Only on a catalog miss does it split at the *last* colon; the strict CLI parse then keeps an invalid suffix as part of the id, and `resolveCliModel` documents the case explicitly ("OpenRouter-style IDs like `openai/gpt-4o:extended` where `openai` looks like a provider but the full string is actually a model id").
- Split rule for the adapter: `vendor/rest` → `--provider <vendor> --model <rest>`, where **rest keeps everything after the first slash** — inner slashes and colon suffixes included. So `openrouter/deepseek/deepseek-r1:free` must arrive as `--provider openrouter --model deepseek/deepseek-r1:free`; stripping to `deepseek-r1:free` loses the vendor path and, with a stale catalog, degrades to a wrong custom model id. A bare id (`composer-2.5`) passes through unchanged.
- Since Paseka passes `--thinking` explicitly for `reasoning:`, pi's suffix-as-thinking path is never consulted on Paseka launches (the fallback only reads the suffix when `--thinking` is absent).
- Known limitation: a vendor suffix that happens to equal a valid thinking word (no live example) and is missing from the catalog would be consumed as thinking. Mitigation is catalog freshness (`pi update`), not client-side parsing.

---

## 4. Command selection

Three mutually exclusive shapes, selected from what the bee declares:

| Shape | Trigger | Behavior |
| ----- | ------- | -------- |
| Params form | no `command:` | `params` → per-adapter flags; prompt/system placed by adapter convention |
| Command form | `command:` set | full custom argv via `CommandVars` substitution (`colony/command.go`); params not mapped |
| Script form | `adapter: script` | `command:` required (`bee_validate.go`); argv plus `PASEKA_*` env (`scriptenv.go`) |

The exclusivity is enforced at load: `command` + non-default `params` is an error (`Bee.ParamsAreDefault` / `NonDefaultParamKeys`), and `adapter: script` without `command` is an error. `timeout` is exempt from the params/command exclusivity — it is process control, not an adapter flag. `ResolveLaunch` sets `LaunchPlan.Form` accordingly.

`adapters.ResolveExec` prefers `req.Command` when present, else the builder. cursor/opencode join system+prompt into one argument because they have no system-file flag (`cursor.JoinPrompt`). Claude and pi use `system.txt` via flags. `post_exec` runs after the run with its own narrower var set (`runtime/post_exec.go`).

---

## 5. Environment and secrets

`LaunchPlan.Env` is assembled, not inherited by accident (`resolveEnv`):

1. `os.Environ()` as the base (PATH, locale, provider by-convention vars).
2. Credentials resolve through machine-local `api_key_env` (`home.go`): only the variable named by the resolved `api_key_env` is guaranteed; the plan records which key was injected (`RedactedEnv`) for redaction. Claude treats "empty" as valid (subscription auth).
3. Declared pass-through: `params.env` lists env the bee may receive, e.g. `{ OPENAI_BASE_URL: https://…, GITHUB_TOKEN: $HOST_GITHUB_TOKEN }` — values are literals or host-var references. Undeclared vendor keys keep flowing in (nothing is stripped by default).
4. `params.env_mode: strict` opts into deny-by-default: the child keeps only process essentials (`PATH`, `HOME`, locale, `PASEKA_*`), declared `env`, and the injected credential.
5. `PASEKA_*` stays for script and reaches `post_exec`, which now receives the plan environment rather than bare `os.Environ()`.

Logs redact argv and the plan's `RedactedEnv` keys (`adapters/log.go`); the Console strips credentials before probing binaries.

---

## 6. Process control

`params.timeout` is AFK-only and opt-in (`LaunchPlan.Timeout`), measured from launch, surfaced as `status: cancelled` with reason `timeout` so a post-mortem can tell "slow" from "stuck". Absent means unbounded (the default, preserving prior behavior). The kill path is SIGTERM → short grace → SIGKILL. Interactive sessions stay unbounded — a human is the timeout.

---

## 7. Summary

| Topic | Behavior |
| ----- | -------- |
| Resolution sites | one `launch.ResolveLaunch` → `LaunchPlan`, shared by AFK and interactive |
| Adapters | pure argv mappers plus a post-map hook for interactive extras |
| `binary` precedence | committed bee wins; machine-local fills the gap; adapter default is the floor |
| Params shape | semantic keys only (`yolo`, `reasoning`, `plan`, `model`, `binary`, `env`, `env_mode`, `timeout`); output format runtime-owned |
| Provider | vendor lives in the model id (`vendor/model[:suffix]`); pi splits the first `/` (rest keeps suffixes), others pass through |
| Booleans | one semantic bool (`yolo`), tri-state across layers |
| Unknown / retired params | error at load with the valid-key list and the replacement |
| `command` + params | error unless params are defaults (timeout exempt) |
| Env | assembled plan, declared pass-through, credentials recorded for redaction |
| Timeout | opt-in `params.timeout` on AFK runs |

Related: [bee config](../guide/bee-config.md) · [colony layout](../guide/colony-layout.md) · [architecture overview](../architecture/overview.md) · [interactive sessions](../guide/interactive-sessions.md)
