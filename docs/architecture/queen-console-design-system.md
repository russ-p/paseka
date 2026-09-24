# Queen Console Design System

Short contract for agents that write Queen Console UI. Targets the Svelte 5 + Tailwind v4 + DaisyUI 5 stack from [Spec 035](../specs/035-queen-console-redesign.md). There is no bespoke design: **consistency and predictability over originality**. Read this before writing any `.svelte` file.

## Principles

1. Use ready-made DaisyUI primitives and Tailwind utilities. Never invent a new visual style when a DaisyUI class already covers it.
2. Colors, spacing, and typography come from DaisyUI themes and Tailwind — never from raw CSS literals in components.
3. A page is a **composition**: shared shell + components from `lib/components/`. New screens add composition, not new CSS.

## Theming

- Curated themes only (registered in the Tailwind config `daisyui.themes` map):
  `catppuccin-latte`, `catppuccin-frappe`, `catppuccin-macchiato`, `catppuccin-mocha`, `light`, `dark`.
  Do **not** add more — bundle size and predictability are the reason.
- Theme is applied via `data-theme` on `<html>`, driven by `themeStore` (Svelte store), persisted to `localStorage`.
- Dark mode is a theme switch, not `@media (prefers-color-scheme)`.
- Semantic color classes only: `base-100/200/300`, `primary`, `secondary`, `accent`, `neutral`,
  `info`, `success`, `warning`, `error`, `*-content`. No hex/RGB literals in Svelte files.

## Layout shell

- `+layout.svelte` renders `<Header />` (top status panel), `<RightMenu />`, and `<main>`.
- Right menu is a fixed right-side panel; below **768px** it becomes a bottom sheet.
- Routes: `/dashboard`, `/traces`, `/bees`, `/worktrees`, `/settings`. Last route + theme persist in `localStorage`.
- Header shows NATS status (connected / reconnecting / disconnected), active trace count, Queen version, current user/colony — fed by the `paseka.console.status` WebSocket subscription.

## Component inventory (`lib/components/`)

Use these. If a page needs a missing element, add the component here and extend this table — do not duplicate one-off markup across pages.

| Component | Use for | Notes |
| --------- | ------- | ----- |
| `StatusBadge` | Any status or state label | `badge` + one semantic class; see mapping below |
| `DataTable` | Tabular lists: traces, bees, runs, worktrees, branches | filtering / pagination via props |
| `Modal` | Short, focused forms | Focus trap, ESC closes, restores focus to trigger |
| `Drawer` | Wide or multi-step forms (launch session, task create) | Slide from right; same focus rules |
| `Toast` | Transient notifications | For action results, not persistent state |
| `SignalCard` | SIGNAL / INSIGHT / MUTATION / VERIFICATION presentation | In feeds and detail blocks |
| `TraceRow` | Trace list rows | Title, status, energy |
| `BeeCard` | Bee / worker cards | Status, last run, adapter |
| `WorktreeCard` | Worktree rows | Branch, associated trace |
| `ThemeSelect` | Theme picker | Settings route, optional header |

## Status → semantic colors

Map domain status to a DaisyUI semantic badge. Do not pick colors per context.

| Domain status | Semantic class |
| ------------- | -------------- |
| running, interacting, connected, live | `badge-info` |
| success, approved, merged, ready, open | `badge-success` |
| waiting_review, pending, reconnecting | `badge-warning` |
| failed, rejected, killed, disconnected, error | `badge-error` |
| idle, archived, unknown | `badge-neutral` |

## Hard rules (agent contract)

1. Create/edit forms open in `<Modal>` or `<Drawer>` triggered by a button — never a full-column form replacing the list view.
2. Every status is a `StatusBadge` with a semantic class from the mapping above.
3. No inline styles, no raw color literals, no custom fonts. Tailwind + DaisyUI only.
4. Lists are `DataTable`s (filter, paginate) — not hand-rolled `<table>` markup per page.
5. Icon-only buttons must carry an `aria-label`; rely on DaisyUI/Tailwind focus-visible outlines.
6. Render loading (skeleton), empty, and error states — not just the happy path.
7. Stay responsive to **768px**: right menu collapses, tables never force horizontal scroll on mobile.
8. Keyboard shortcuts across routes (e.g. `g d` dashboard, `g t` traces).
9. On NATS disconnect show a reconnecting banner (`badge-warning`) and queue mutations locally; do not lose operator input.
10. Consume typed payloads generated from the Go event contracts; no `any`-typed event handling.

## Agent workflow

1. Read this doc + [Spec 035](../specs/035-queen-console-redesign.md) before writing UI code.
2. Compose from the shell and `lib/components/`; only add a component if the inventory genuinely lacks it (and extend the table).
3. Verify with component tests (Vitest + @testing-library/svelte) and Playwright visual regression on Dashboard, Traces, Settings.
4. Theme store and route persistence must be covered by unit tests (hydration, reload persistence).

## Scope

This contract covers the **new** Svelte-based console. The current vanilla-js console under `internal/console/static/` is being replaced per Spec 035 and is out of scope for new features. Operator-facing behavior of the console lives in [docs/guide/queen-console.md](../guide/queen-console.md).