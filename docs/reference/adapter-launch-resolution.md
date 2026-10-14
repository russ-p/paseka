# Adapter launch resolution

A bee is YAML; an adapter is an OS process. Everything in between — which binary, which argv, which env, which cwd — is decided by the **launch resolution** documented here. This is the cross-cutting view of `adapter`, `params`, `command`, profiles, and machine-local config; field-level schema lives in [bee config](../guide/bee-config.md) and [colony layout](../guide/colony-layout.md).

Each topic is split:

- **As-is** — verified current behavior with `file:line` anchors. Line numbers drift; symbol names are the stable handle.
- **Proposed** — the target model for the adapter launch rework. **No code implements it yet**; it is tracked in the [backlog](../plans/backlog.md#adapter-launch-resolution).

---

## 1. Launch modes

### As-is

| Mode | Entry | Builds argv/env in | Process owner |
| ---- | ----- | ------------------ | ------------- |
| AFK CLI | `paseka bee run <role>` → `cmd/paseka/bee.go:38` → `runtime.BeeRun` (`internal/runtime/bee_run.go:41`) | `internal/runtime/dispatch_stages.go` + per-adapter `buildArgs` | adapter (`exec.CommandContext`) |
| Reactor task | `internal/runtime/dispatch_task.go:79` → `DispatchColonyBee` | same as AFK | adapter |
| Reactor direct | `internal/runtime/dispatch_direct.go:60` → `DispatchColonyBee` | same as AFK | adapter |
| Console run | `GET /api/bees/:role/runs` (`internal/console/bees_run.go:123`) | same as AFK | adapter |
| Interactive | `paseka bee chat <role>` → `cmd/paseka/bee.go:160` → `sessions.Manager.launch` (`internal/sessions/manager.go:160`) | `SessionAdapter.SessionCommand` per adapter | runtime PTY (`internal/sessions/pty.go:22`) |
| Invite / resume | `internal/invites/service.go:60`, `internal/sessions/resume.go:76` | same as interactive | runtime PTY |

Two code paths resolve the same layered config independently:

- params merge: `dispatch_stages.go:180` **and** `manager.go:297` (identical call)
- `command:` render: `dispatch_stages.go:214` **and** `manager.go:314`
- system+prompt join (cursor/opencode): `dispatch_stages.go:189` **and** `manager.go:301`
- workspace/worktree pick: `colony_dispatch.go:143` **and** `manager.go:196`

Workspace (cwd) = colony root, or `.paseka/worktrees/<traceId>/` when `worktree: true` (or an isolated `MUTATION` proposal forces it), plus the sector subpath. `.paseka/runs/<traceId>/<agentId>/` IPC always lives under the colony root.

### Proposed

One resolver, one struct, two consumers:

```go
// internal/runtime/launch.go (planned)
type LaunchPlan struct {
    Binary   string            // resolved executable
    Args     []string          // argv already built (command mode) or flag-mapped (params mode)
    Env      []string          // complete child environment
    Dir      string            // cwd
    Timeout  time.Duration     // 0 = unbounded (interactive default)
    PlanMode PlanMode          // params | command | script
}
```

- `ResolveLaunch(bee, colonyContext, mode)` is the **only** place layers merge. It runs for AFK and interactive alike; mode only changes flag mapping and process ownership.
- Adapters become pure mappers: `BuildArgs(RunParams, RunRequest) []string` and nothing else. No adapter reads home config, profiles, or env again.
- Interactive keeps its extras (cursor `create-chat`, opencode `opencode serve` credential pair) as a post-map hook on the plan, not a second resolution.

Migration is mechanical: extract the `manager.go:165-323` body into the shared resolver first, then switch both callers.

---

## 2. Configuration layers

### As-is

Sources, lowest to highest precedence:

| # | Layer | Source | Loader |
| - | ----- | ------ | ------ |
| 1 | Adapter defaults | hardcoded (`agent`, `pi`, `claude`, `opencode`) | `internal/adapters/<name>/*.go` `defaultBinary` |
| 2 | Machine-local base | `~/.config/paseka/<slug>/adapters/<name>.yaml` — `binary`, `api_key_env` | `internal/colony/home.go:217-339` |
| 3 | Machine-local profile | `~/.config/paseka/<slug>/profiles/<name>/adapters/<name>.yaml` | `profile_merge.go:69-93` |
| 4 | Repo colony | `.paseka/colony.yaml` — `model_aliases`, `defaults.*`, `sectors` | `bee.go:96` via `ResolveContext` (`home.go:84`) |
| 5 | Committed profile | `.paseka/profiles/<name>.yaml` — `adapter`, `params`, `model_aliases`, `bees.<role>` | `profile_merge.go:14` |
| 6 | Bee YAML | `.paseka/bees/<role>.yaml` — `params` (applied *after* profile overlay via `LoadBee`) | `bee.go:162`, `profile_merge.go:261` |
| 7 | Machine-local overlay | `AdapterExtra` — binary + resolved API key, merged over bee params | `params.go:78`, `dispatch_stages.go:180` |
| 8 | Model aliases | colony + home + profile aliases, applied once after the merge | `model_aliases.go:81` |
| 9 | Process env | `PASEKA_PROFILE`, `PASEKA_NATS_URL`, `api_key_env` target var | `profile.go:125`, `home.go:37` |

Profile selection itself: `--profile` flag → `PASEKA_PROFILE` → sticky `profile:` in home `config.yaml` → none (`profile.go:115`). Unknown profile fails closed and lists what exists on both layers.

**Known distortions:**

1. **Machine-local defaults shadow repo config.** Every loader fills a default `binary`, and `MergeRunParams` lets any non-zero `over` win (`params.go:11`). Because `AdapterExtra` always carries a non-empty `Binary`, bee `params.binary` can never win — the repo-level override is dead. The loaders already document the root cause: "Every adapter loader fills a default for an absent key, so a loaded struct cannot say where a value came from" (`home.go:344-349`). The presence-aware reader exists (`AdapterConfigDeclared`, `home.go:350`) but is used only by the Console.
2. **Booleans merge one-way.** `MergeRunParams` (`params.go:23-31`) can set `trust`/`force` true but never false; a profile or machine layer cannot turn a colony default off.
3. **No param validation.** `RunParamsFromBee` (`params.go:42-75`) silently ignores unknown keys — a typo (`modle:`, `plan_mode:`) is a no-op with no warning.
4. **Profile remap has edge cases.** The global `adapter:` overlay only remaps LLM bees without `command:`; `script` bees and custom-argv bees are skipped silently (`profile_merge.go:197`, and see [colony layout](../guide/colony-layout.md)).

### Proposed

Presence-aware layering with an explicit ladder. A layer only contributes keys it **actually declares** — a loader default is not a declaration:

1. Adapter built-in defaults (`defaultBinary` constants; `RunParams` zero value with `trust/force` defaults documented per adapter).
2. Committed colony config — `colony.yaml` `model_aliases`, `defaults`, `sectors`.
3. Bee YAML — `params`, `command`, `worktree`, `sector`. The colony commit is the primary description of the work.
4. Selected profile — committed `.paseka/profiles/<name>.yaml`, then machine `profiles/<name>/`. A profile is a process-level switch and stays above the bee (that is its purpose); it contributes declared keys only.
5. Machine-local `adapters/<name>.yaml` — **declared keys only**, and only as a *fallback* for `binary` (repo wins when set). `api_key_env` is a machine fact and always wins; the key itself is never a launch parameter.
6. Model alias resolution — once, after all merging, before argv build.
7. Process env — `PASEKA_PROFILE`, `PASEKA_NATS_URL`, credential vars.

Consequences to accept deliberately: a machine that never writes `adapters/cursor.yaml` gets `agent` from (1); a bee that pins `params.binary: agent-next` finally wins over the absent machine file; `paseka doctor` gains a "resolved launch plan" dump per bee so operators can see layer attribution without reading Go.

---

## 3. Params (`RunParams`)

### As-is

`RunParamsFromBee` maps bee `params` (`params.go:42-75`); defaults `trust: true`, `force: true`. Keys: `model`, `output_format`, `trust`, `force`, `plan`, `binary`, `provider`, `thinking`. `model` passes through `ApplyModelAliases` once per launch. Per-adapter mapping is the materialization rule — e.g. cursor `buildArgs` (`cursor/cursor.go:234`), claude permission mode (`claude/claude.go:247`), pi `--mode` (`pi/pi.go:232`), opencode `--variant`/`--auto` (`opencode/args.go:41`).

**Shape smell — adapter flags wearing a bee schema.** Three keys are not colony decisions:

- `output_format` — the runtime is the only consumer (usage, `providerSessionId`, summary normalization). Every bee in this repo repeats `output_format: stream-json` (`.paseka/bees/*.yaml`); nothing differs per bee. Worse, the value sets are adapter-disjoint (cursor `text|json|stream-json`, pi `text|json|rpc`, opencode `default|json`) and a "wrong" choice degrades runtime parsing silently — e.g. claude requires `--verbose` for stream-json (`claude/claude.go:224`).
- `trust` / `force` — two booleans that collapse into a single flag on two of four adapters: claude `bypassPermissions` triggers on `force || trust` (`claude/claude.go:251`), opencode `--auto` on the same OR (`args.go:44`). They are cursor's two flags (`--trust`, `--force`) generalized into a schema.
- `thinking` — a per-adapter name for the same concept (pi `--thinking`, opencode `--variant`).

`MergeRunParams` is shallow and non-zero-wins, so confidence in a value depends on which side it arrived from. `Command` set on the bee overrides params entirely with only a warning (`dispatch_stages.go:207`).

### Proposed

The bee-visible surface carries **only colony-level decisions**; vendor flags are the adapter's business. Proposed keys:

| Key | Values | Decision it expresses | Per-adapter mapping |
| --- | ------ | --------------------- | ------------------- |
| `model` | alias or vendor id | unchanged — alias-resolved once before argv build | `--model` (+ `provider/` prefix for opencode) |
| `yolo` | bool | run without asking a human | cursor `--trust --force`; claude `--permission-mode bypassPermissions`; opencode `--auto`; pi: no equivalent (documented) |
| `plan` | bool | read-only planning; wins over `yolo` | cursor/pi `--plan`; opencode `--agent plan`; claude `--permission-mode plan` |
| `reasoning` | `low` \| `medium` \| `high` \| passthrough | reasoning effort | pi `--thinking`; opencode `--variant`; cursor/claude unmapped today (doctor reports) |
| `binary` | string | pin an executable for this bee | wins over machine-local; machine-local stays a fallback |

Removed from the bee schema: `output_format` (**runtime-owned**, below), `trust`/`force` (replaced by `yolo`), `thinking` (renamed to `reasoning`).

**Runtime-owned output format.** The runtime always requests each adapter's structured mode, because usage, `providerSessionId`, and summary normalization are parsed from it:

| Adapter | Runtime-requested structured mode |
| ------- | --------------------------------- |
| cursor | `--output-format stream-json` |
| pi | `--mode json` |
| claude | `--output-format stream-json --verbose` |
| opencode | `run --format json` |

Debugging escape hatch: `paseka bee run --raw` (per invocation, never committed). When `--raw` costs a runtime feature — cursor loses `usage`/`providerSessionId`, session-tracking degrades — the run log says so; today `output_format: text` degrades silently.

**Rules that come with the new surface:**

- Unknown keys fail at load time with the valid-key list (`paseka doctor` and dispatch) — typos stop being silent no-ops.
- Tri-state booleans (`unset | true | false`) so a profile can turn a value off, not only on.
- Attribution kept: the resolver returns which layer set each field (drives `paseka doctor --explain <bee>` and the Console adapter panel).
- `command` + `params` on one bee becomes a hard error unless the params are exact defaults; today it is a warning.
- **Migration:** `trust`/`force` accepted for one release with a deprecation warning mapping `trust || force` → `yolo: true`, then flagged by `paseka doctor`; `paseka init` templates and colony bees drop the boilerplate (each `.paseka/bees/*.yaml` loses three repeated keys).

**Provider dissolves into the model id** (decided direction — open question closed). `provider` today is "which vendor serves the model": a separate arg for pi (`--provider`, `pi.go:245`) and a `provider/model` prefix for opencode (`args.go:20-32`). Provider and model are one atomic pair — switching vendor while keeping the id produces a broken combination. So the vendor moves into the alias value and is no longer its own key:

- Alias values become full ids: `high: anthropic/claude-sonnet-4`, `small: openai/gpt-5.5` (`colony.yaml` / home / profile `model_aliases`). A bare id without `/` (`composer-2.5`) stays valid — no provider.
- The runtime maps the resolved id per adapter: cursor/claude pass it through; opencode consumes `provider/model` natively (the `resolveModel` join dies); pi splits at the **first** `/` → `--provider <vendor> --model <rest>`.
- `params.provider` is removed; deprecation release maps `provider: X` + `model: Y` → `X/Y` for that run, `paseka doctor` warns.
- **Profile override of the vendor needs no new machinery** — profiles already overlay `model_aliases` (`profile_merge.go:57-63`, spec 019 scenario 18). `.paseka/profiles/local.yaml` setting `model_aliases: { high: ollama/llama3 }` moves every bee on `high` to that vendor **and** model on this machine/repo profile, atomically.

**Verified against pi 1.0.0** (`@earendil-works/pi-coding-agent`; `dist/core/model-resolver.js` — `parseModelPattern`, `resolveCliModel`, `findExactModelReferenceMatch`). The `:<suffix>` collision is real and handled:

- `--model` documents "supports `provider/id` and optional `:<thinking>`" — thinking levels are `off, minimal, low, medium, high, xhigh, max`, so `:free` looks like a thinking level to pi.
- But pi **exact-matches the full pattern against its catalog first**, and its own catalog entries carry colon suffixes (`deepseek/deepseek-v4.1-flash:batch` appears in `pi --list-models`). Only on a catalog miss does it split at the *last* colon; the strict CLI parse then keeps an invalid suffix as part of the id, and `resolveCliModel` documents the case explicitly ("OpenRouter-style IDs like `openai/gpt-4o:extended` where `openai` looks like a provider but the full string is actually a model id").
- Split rule for the adapter: `vendor/rest` → `--provider <vendor> --model <rest>`, where **rest keeps everything after the first slash** — inner slashes and colon suffixes included. So `openrouter/deepseek-r1:free` must arrive as `--provider openrouter --model deepseek/deepseek-r1:free`; stripping to `deepseek-r1:free` loses the vendor path and, with a stale catalog, degrades to a wrong custom model id. A bare id (`composer-2.5`) passes through unchanged.
- Since Paseka passes `--thinking` explicitly for `reasoning:`, pi's suffix-as-thinking path is never consulted on Paseka launches (the fallback only reads the suffix when `--thinking` is absent).
- Known limitation: a vendor suffix that happens to equal a valid thinking word (no live example) and is missing from the catalog would be consumed as thinking. Mitigation is catalog freshness (`pi update`), not client-side parsing.

---

## 4. Command selection

### As-is

Three mutually exclusive shapes, chosen implicitly:

| Shape | Trigger | Behavior |
| ----- | ------- | -------- |
| Params mode | no `command:` | `params` → per-adapter flags; prompt/system placed by adapter convention |
| Command mode | `command:` set | full custom argv via `CommandVars` substitution (`colony/command.go:18`); params not mapped |
| Script mode | `adapter: script` | `command:` required (`bee_validate.go:11`); argv plus `PASEKA_*` env (`scriptenv.go:10`) |

`ResolveExec` (`adapters/command.go:6`) prefers `req.Command` when present, else the builder. cursor/opencode join system+prompt into one argument because they have no system-file flag (`cursor.JoinPrompt`, `cursor/cursor.go:223`, used at `dispatch_stages.go:189`). Claude and pi use `system.txt` via flags. `post_exec` runs after the run with its own narrower var set (`runtime/post_exec.go:13`).

### Proposed

- Selection stays implicit (it reads well) but the exclusivity is enforced: `command` + non-default `params` = error; `adapter: script` without `command` = error (already true at runtime today — move it to load time).
- One rendering pass: `command:` templates receive the same `LaunchPlan` values as params mode, including the plan's env, so custom argv and mapped argv can share path/var conventions (`{{.ResultFile}}` etc. stay as-is).
- `post_exec` inherits the plan env — today it gets bare `os.Environ()` (`colony/exec.go:22`), which means a credential passed to the bee does not reach its post-run hook.

---

## 5. Environment and secrets

### As-is

| Adapter | Child env | Source |
| ------- | --------- | ------ |
| cursor | `os.Environ()` + `CURSOR_API_KEY` when set | `cursor/cursor.go:94` |
| claude | `os.Environ()` + `ANTHROPIC_API_KEY` when set | `claude/claude.go:101` |
| pi | `os.Environ()` only — key rides on argv `--api-key` | `pi/pi.go:123` |
| opencode | `os.Environ()` | `opencode/opencode.go:103` |
| script | `os.Environ()` + `PASEKA_TRACE_ID/AGENT_ID/TASK_ID/WORKSPACE/COLONY_ROOT/RUN_DIR/BEE/EVENT_LOG/RESULT_FILE/PROMPT_FILE/SYSTEM_FILE` | `scriptenv.go:10` |
| opencode interactive | `os.Environ()` minus inherited `OPENCODE_SERVER_USERNAME/PASSWORD`, plus a freshly generated pair | `opencode/session.go:83` |

Credentials resolve through machine-local `api_key_env` (`home.go:243-315`): the name of a host env var, read at launch. Claude treats "empty" as valid (subscription auth). The whole host environment is inherited by every child, so any vendor key already exported wins regardless of `api_key_env`, and there is no way to hand a bee an extra variable without wrapping it in `command:`. Logs redact argv (`adapters/log.go:21`); the Console strips credentials before probing binaries (`console/adapters.go:236`).

### Proposed

`LaunchPlan.Env` is assembled, not inherited by accident:

1. `os.Environ()` as the base (needed for PATH, locale, provider by-convention vars).
2. Credentials: only the variable named by the resolved `api_key_env` is guaranteed; the plan records which key was injected for redaction.
3. Declared pass-through: bee/machine config lists env the bee may receive, e.g. `params.env: { OPENAI_BASE_URL: https://…, GITHUB_TOKEN: $HOST_GITHUB_TOKEN }` — values are literals or host-var references. Undeclared vendor keys keep flowing in (nothing is stripped), but the declared set becomes the documented, redacted, auditable surface.
4. `PASEKA_*` stays for script and gains the same treatment for `post_exec`.

Open question: whether *denied-by-default* env (strip all non-declared secrets from the child) is worth the breakage for operators who rely on `export ANTHROPIC_API_KEY` in their shell profile. Recommendation: keep inherit-by-default in the first iteration, default-deny as an opt-in `params.env_mode: strict`.

---

## 6. Process control

### As-is

No timeout exists on the adapter process itself — `exec.CommandContext` receives the caller context: unbounded in CLI (`cmd/paseka/bee.go:50`), `context.Background()` in the reactor, cancellable only through trace kill (`runtime/kill.go`). Bounded timeouts cover only auxiliary children: cursor `create-chat` 15s, opencode `serve` 20s, Console probe 5s, forge 60s, session usage 30s. A hung `agent -p` occupies a worker slot until killed by hand.

### Proposed

`params.timeout` (AFK; default unbounded to preserve behavior) measured from launch, with kill → short grace → kill -9, surfaced as `status: cancelled` with reason `timeout`. Interactive sessions stay unbounded — a human is the timeout. Emit the plan timeout on `status.json` so a post-mortem can tell "slow" from "stuck".

---

## 7. Summary of proposed changes

| Topic | As-is | Proposed |
| ----- | ----- | -------- |
| Resolution sites | 4 duplicated bodies (dispatch, sessions, ×prompt join) | one `ResolveLaunch` → `LaunchPlan` |
| Adapters | assemble env/argv per mode | pure argv mappers + hook for interactive extras |
| `binary` precedence | machine-local always wins (defaults shadow repo) | repo wins; machine fills gaps |
| Params shape | adapter flags as bee keys (`output_format`, `trust`, `force`) | semantic keys only (`yolo`, `reasoning`); output format runtime-owned |
| Provider | separate key + per-adapter join | vendor lives in the alias value (`vendor/model[:suffix]`); pi splits first `/` (rest keeps suffixes), others pass through |
| Booleans | one-way true, split per vendor (`trust` **and** `force`) | one semantic bool (`yolo`), tri-state |
| Unknown params | silently ignored | error with valid-key list |
| `command` + params | warning | error unless params are defaults |
| Env | full inheritance, one injected key | assembled plan + declared pass-through; redaction plan-aware |
| Timeout | none on adapter process | opt-in `params.timeout` |

Related: [bee config](../guide/bee-config.md) · [colony layout](../guide/colony-layout.md) · [architecture overview](../architecture/overview.md) · [interactive sessions](../guide/interactive-sessions.md)
