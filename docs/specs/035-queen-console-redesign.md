# Spec 035: Queen Console Redesign

## Status

**Draft**
Initial design spec for migrating Queen Console to Svelte 5 + Tailwind v4 + DaisyUI with new layout, routing, and theming.

## Problem Statement

The current Queen Console uses an older frontend stack that limits developer experience, theming flexibility, and component composability. Operators need a modern, responsive interface with:
- Consistent theming (light/dark/custom) via DaisyUI, including Catppuccin Latte, Frappé, Macchiato, Mocha
- Clear navigation with right-side menu and top status panel
- Dedicated routes for Dashboard, Traces, Bees, Worktrees, and Settings
- Better TypeScript integration and component reuse
- Faster builds and hot-module replacement during development

## Solution

Rebuild Queen Console as a Svelte 5 application using Tailwind v4 and DaisyUI for styling. Adopt a file-based router (e.g., SvelteKit or vite-plugin-ssr) with explicit routes. Implement a persistent shell layout: top panel with cluster/NATS status indicators, right slide-out menu replacing current tabs, and a main content area per route. Extract shared UI components (cards, tables, forms, status badges) into a component library. Configure DaisyUI themes and expose a theme selector in Settings.

## User Stories

1. As a Beekeeper, I want to switch between light, dark, and custom DaisyUI themes, so that the console matches my environment and accessibility needs.
2. As a Beekeeper, I want a top panel that always shows NATS connection status, active trace count, and Queen health, so that I can assess system state at a glance.
3. As a Beekeeper, I want a right-side navigation menu with icons and labels for Dashboard, Traces, Bees, Worktrees, Settings, so that I can switch contexts without losing scroll position.
4. As a Beekeeper, I want a dedicated Dashboard route (/dashboard) showing colony overview, recent signals, and quick actions, so that I land on a useful summary after login.
5. As a Beekeeper, I want a Traces route (/traces) with a filterable, paginated list of traces and a detail drawer, so that I can inspect execution history efficiently.
6. As a Beekeeper, I want a Bees route (/bees) listing all registered bees with status, last run, and adapter info, so that I can monitor bee health.
7. As a Beekeeper, I want a Worktrees route (/worktrees) showing active worktrees, their branches, and associated traces, so that I can manage isolation contexts.
8. As a Beekeeper, I want a Settings route (/settings) to configure themes, NATS endpoints, API keys, and notification preferences, so that I can customize the console without editing files.
9. As a Beekeeper, I want the console to persist my last active route and theme in localStorage, so that my preferences survive reloads.
10. As a Beekeeper, I want keyboard shortcuts (e.g., `g d` for Dashboard, `g t` for Traces) to jump between routes, so that I can navigate quickly.
11. As a Beekeeper, I want the console to gracefully degrade when NATS disconnects, showing a reconnecting banner and queuing mutations locally, so that I don't lose work during transient outages.
12. As a Beekeeper, I want the new console to be served from the same Go binary via `paseka console`, so that deployment stays a single binary.
13. As a developer, I want TypeScript types generated from the Go event contracts (SIGNAL, INSIGHT, MUTATION, VERIFICATION), so that frontend consumes typed payloads.
14. As a developer, I want a component storybook or visual regression setup, so that UI changes are reviewed consistently.
15. As a Beekeeper, I want the console to be responsive down to 768px width, collapsing the right menu into a bottom sheet on mobile, so that I can check status on a phone.

## Implementation Decisions

- **Framework**: Svelte 5 (runes mode) with SvelteKit for file-based routing, SSR disabled (SPA mode), adapter-static for embedding in Go binary.
- **Styling**: Tailwind v4 (CSS-first config) + DaisyUI 5 for themed components. DaisyUI `themes` array in `tailwind.config.js` includes built-in themes (`light`, `dark`, `cupcake`, `bumblebee`, `emerald`, `corporate`, `synthwave`, `retro`, `cyberpunk`, `valentine`, `halloween`, `garden`, `forest`, `aqua`, `lofi`, `pastel`, `fantasy`, `wireframe`, `black`, `luxury`, `dracula`, `cmyk`) plus custom Catppuccin themes: `catppuccin-latte`, `catppuccin-frappe`, `catppuccin-macchiato`, `catppuccin-mocha` (defined in `tailwind.config.js` via `daisyui.themes` extension).
- **Theme switching**: `data-theme` attribute on `<html>` toggled via Svelte store persisted to localStorage; DaisyUI handles CSS variables automatically.
- **Layout shell**: `+layout.svelte` renders `<Header />` (top panel), `<RightMenu />` (slide-out), `<main>` slot. Right menu uses `<aside>` with `fixed inset-y-0 right-0 w-64 transform transition-transform` and `translate-x-full` when closed; mobile breakpoint switches to bottom sheet.
- **Top panel**: Shows NATS status (connected/reconnecting/disconnected), active trace count, Queen version, current user/colony. Uses WebSocket subscription to `paseka.console.status` subject for live updates.
- **Routing**: Routes map to `(app)/dashboard/+page.svelte`, `(app)/traces/+page.svelte`, `(app)/bees/+page.svelte`, `(app)/worktrees/+page.svelte`, `(app)/settings/+page.svelte`. Auth guard redirects to `/login` if no session cookie.
- **API layer**: Central `api.ts` with typed fetch wrappers around `/api/v1/*` endpoints. Generates TypeScript types from Go `internal/console/api` via `go run ./cmd/paseka-gen-types` during build.
- **State management**: Svelte 5 runes (`$state`, `$derived`, `$effect`) for local component state; cross-route stores in `stores/` (e.g., `themeStore`, `statusStore`, `traceStore`).
- **Component library**: `lib/components/` with `DataTable`, `StatusBadge`, `SignalCard`, `TraceRow`, `BeeCard`, `WorktreeCard`, `Modal`, `Drawer`, `Toast`, `ThemeSelect`. All styled with DaisyUI classes + Tailwind utilities.
- **Build integration**: `pnpm build` outputs to `internal/console/embed/dist`; Go `//go:embed` serves assets. `paseka console` runs `pnpm dev` in watch mode for development.
- **Testing**: Vitest + @testing-library/svelte for unit/component tests; Playwright for E2E against running `paseka console`. Prior art: `internal/console/api/*_test.go` patterns.
- **Accessibility**: Semantic HTML, ARIA labels on icon-only buttons, focus-visible outlines, color-contrast compliant DaisyUI themes.

## Testing Decisions

- Unit test each store (`themeStore`, `statusStore`) for persistence, hydration, and reactive updates.
- Component test `RightMenu` open/close, keyboard navigation, mobile breakpoint toggle.
- Component test `Header` status indicators reflect WebSocket events.
- Integration test route guards redirect unauthenticated users.
- E2E test happy path: login → dashboard → traces filter → open detail → switch theme → reload → theme persists.
- Visual regression on key pages (Dashboard, Traces list, Settings) using Playwright screenshot comparison.
- Contract test: generated TS types decode sample SIGNAL/INSIGHT/MUTATION/VERIFICATION payloads without error.

## Out of Scope

- Real-time collaborative editing or multi-user cursors.
- Plugin/extension system for third-party panels.
- Historical analytics charts (Grafana/Prometheus integration remains separate).
- Mobile app or PWA offline support beyond localStorage persistence.
- Migration of existing console code — this is a clean rewrite.

## Further Notes

- DaisyUI theme list can be trimmed to 6–8 curated themes before release to reduce bundle size; Catppuccin variants are strong candidates for defaults.
- Consider `svelte-put/clickaway` and `svelte-put/escape` for menu/drawer interactions.
- Go binary size impact: embedded SPA ~2–3 MB gzipped; acceptable for single-binary distribution.
- Follow-up spec may cover "Console Plugin API" once shell is stable.