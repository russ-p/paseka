# Queen Console Next

Frontend foundation for the Svelte-based Queen Console redesign described in [Spec 035](../docs/specs/035-queen-console-redesign.md). The development and production app is mounted by the Go console at `/next/`; the legacy console remains at `/`.

## Layout

| Path | Holds |
| ---- | ----- |
| `src/lib/api/` | `client.ts` — the only module that fetches; `types.ts` mirrors the Go JSON views |
| `src/lib/components/` | Shared components from the design-system inventory |
| `src/lib/stores/` | Runes stores: `consoleStatusStore` (chrome stream + the one dashboard poll), `themeStore`, `toastStore` |
| `src/lib/format.ts` | Pure formatters, unit-tested |
| `src/routes/` | One directory per route; unmigrated routes render `PagePlaceholder` |
| `src/tests/` | Test-only fixtures shared across suites |

## Development

```sh
pnpm install --frozen-lockfile
pnpm dev
```

Open `http://localhost:5173/next/`. Vite proxies `/api/*` to `http://127.0.0.1:8787`; set `PASEKA_CONSOLE_API` to use another Go console address.

## Verification

```sh
pnpm check
pnpm test
pnpm build
```

The production build is written to `internal/console/next/dist/` and committed for embedding by the Go binary. Rerun `pnpm build` whenever frontend sources change.
