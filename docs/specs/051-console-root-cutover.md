# Spec 051: Queen Console Root Cutover

## Status

**Approved**
Removes the legacy vanilla-JS console and serves the Svelte console at `/`. The `/next` prefix becomes a permanent redirect. This is the root cutover that [035-queen-console-redesign](035-queen-console-redesign.md) deliberately left as a human decision.

## Problem Statement

Paseka ships one binary with two web consoles. The Svelte console is complete and every surface is migrated, but it is still published under a preview URL, while the legacy vanilla-JS console remains the default at the root. An operator has to know which URL is "the real one", and every document, bookmark, chat message, and bug report has to carry the distinction.

The continuation has real costs. Two frontends mean two build stories, two sets of route tests, two asset conventions, and two places where a UI fix could be applied to the wrong console. The legacy bundle also carries its own vendored copies of xterm, diff2html, and cytoscape, which the Svelte console no longer uses. Every release keeps paying for code that no surface needs, and every new operator is taught a URL that is scheduled to disappear.

The preview also makes the default console a subpath. That is a migration convenience, not a product shape: it puts the primary interface under a name that means "not yet", and it makes root-relative links, cookies, and reverse-proxy rules carry a prefix that has no reason to exist after parity.

## Solution

The Svelte console becomes the only Queen Console, served at `/`. The legacy bundle is deleted rather than kept behind a flag. `/next` and every path beneath it permanently redirect to the same path at the root, so older links still work while the address bar converges on the real URL. The buildless fallback moves to the root with it, and says plainly that the console bundle was not built instead of claiming a legacy console is still available.

Operators keep every current behavior: the same dashboard, routes, detail pages, theme, keyboard chords, last-route persistence, and JSON API. What changes is the address, the removal of the second UI, and the single asset pipeline that follows from it.

## User Stories

1. As a Beekeeper, I want `paseka console` to open the modern console at `/`, so that I do not have to know a preview URL to reach the primary interface.
2. As a Beekeeper, I want every route to live at a root path such as `/traces` or `/sessions`, so that links I share name the product rather than its migration state.
3. As a Beekeeper with a bookmark to `/next/traces`, I want it to redirect to `/traces` permanently, so that an old shortcut keeps working without me editing it.
4. As a Beekeeper following an old chat message or document link to `/next/traces/<id>`, I want the redirect to preserve the full path and query string, so that the trail or filter I was sent to still opens.
5. As a Beekeeper with a browser tab left open on an old `/next` page, I want its cached asset requests under `/next/_app/...` to redirect too, so that a reload during the transition does not land on a broken page.
6. As a Beekeeper, I want the root URL to send me to my last visited list, or to the Dashboard on a first visit, so that opening the console lands where I expect.
7. As a Beekeeper whose stored last route was saved under `/next`, I want the console to ignore that stale path and land safely on the Dashboard, so that a migration detail cannot produce a redirect loop or a blank page.
8. As a Beekeeper, I want deep links to a trail, run, task, proposal, session, or merge preview to work at the root, so that the console is linkable exactly as it was under the preview.
9. As a Beekeeper, I want `Escape` and the `g` route chords to keep working after the base path changes, so that the cutover does not quietly cost me the navigation I use.
10. As a Beekeeper, I want my theme and menu state to survive the cutover, so that the console still feels like mine on the first root visit even though the preview URL's last-route path is retired.
11. As a Beekeeper, I want the console title, favicon, and startup banner to name one console rather than a preview, so that I can tell which build answered the port.
12. As a Beekeeper, I want an unknown asset under the console to return a real 404 rather than the app shell, so that a missing script is diagnosed as missing instead of appearing as a broken page.
13. As a Beekeeper, I want `/api/*` to remain a JSON surface and never be answered with console HTML, so that an unknown endpoint fails as an API error rather than a page.
14. As a Beekeeper whose binary was built without the frontend bundle, I want the root URL to tell me how to build it, so that a missing console reads as a build instruction rather than a blank page or a resurrected legacy UI.
15. As a Beekeeper installing from a release archive or the container image, I want the console bundle included as it is today, so that the default path is also the supported path.
16. As a maintainer, I want the legacy HTML, JavaScript, CSS, and vendored libraries removed from the repository and the binary, so that releases stop shipping a console nobody uses.
17. As a maintainer, I want the legacy contract tests deleted with the bundle and replaced by root-route tests, so that the test suite measures the console that exists.
18. As a maintainer, I want the frontend build output checked for `/next` prefixes, so that a missed base-path configuration fails verification instead of shipping broken links.
19. As a maintainer, I want one documented build path for the console, so that source builds, release archives, the container image, and the development server all agree.
20. As a maintainer, I want no runtime switch for the old console, so that "temporarily go back" cannot become an untested second product.
21. As a maintainer, I want the JSON API left unchanged by this cutover, so that CLI, Telegram, exports, and external scripts are unaffected.
22. As a maintainer, I want the docs and agent instructions to describe one console at `/`, so that the next change does not reintroduce preview language.
23. As a maintainer, I want the cutover to be reversible only by reverting a release, so that the rollback story is honest rather than a hidden dual-UI mode.
24. As a Beekeeper, I want no visible behavior regression on any migrated surface, so that removing the legacy console is a path change rather than a feature loss.
25. As a Beekeeper, I want the documentation's troubleshooting steps to describe the real failure modes of a buildless binary, so that I do not follow advice about a console that no longer exists.
26. As a Beekeeper on a reverse proxy, I want the console to remain at the proxy root, so that existing forwarding rules keep working without a `/next` special case.
27. As a Beekeeper, I want the favicon and app mark to be served from the console bundle rather than a second static root, so that there is one asset pipeline to trust.
28. As a maintainer of a fork, I want API calls to stay root-relative rather than inherit a UI prefix, so that the JSON contract does not depend on where the console shell is mounted.

## Implementation Decisions

### One console, mounted at the root

- The Svelte application is the only console UI. Its SvelteKit base path becomes empty, so generated routes and assets are root-relative.
- The route table remains the same set of relative paths. Existing path-joining helpers already accept a base argument; the root build passes the empty base, and no route is renamed.
- The root entry route keeps today's behavior: redirect to the last known list path, otherwise to the Dashboard. A stored path that still carries the retired prefix does not match and is ignored.
- The page title and app shell name the product, not the migration: "Queen Console", not "Queen Console Next".

### The old prefix redirects permanently

- `/next` and `/next/` redirect to `/`.
- `/next/<rest>` redirects to `/<rest>`, preserving the query string.
- The redirect is permanent (308) and handled by the Go server before the SPA fallback, so it covers routes, assets, and deep links without any frontend logic.
- Because the rule matches the whole prefix, a browser holding an old shell that requests `/next/_app/...` still resolves its assets.
- There is no expiry and no feature flag. The redirect is a stability promise for links, not a second mount point.

### One root SPA handler

- The Go server serves the embedded bundle at `/` and uses the bundle's own single-page entry as the fallback for unknown non-asset paths.
- A path whose last segment has a file extension is an asset request: if absent, it returns 404 rather than the shell. The existing rule that prevents an SPA from answering HTML where JavaScript was requested is retained.
- `/api` and `/api/*` are never answered with console HTML. Unknown API paths return an API error, matching the pre-cutover behavior of unknown preview-adjacent API paths.
- The legacy root handler and its embed are removed.

### One asset pipeline

- The favicons and touch icon used by the console move into the frontend's static assets, so the built bundle serves them at the root and the development server serves them without a proxy. The frontend shell declares the same icon set the legacy shell declared.
- The legacy vendored libraries disappear with the legacy bundle. The built console's `_app` assets become the only script and style assets the server carries.
- The development server drops its favicon proxy and serves the app at `http://localhost:5173/`; the API proxy remains unchanged.

### The buildless binary explains itself at the root

- The checked-in fallback page remains committed and remains the only content in a source-only embed. A binary built without the bundle answers any console route with that page, and any missing asset with 404.
- The fallback copy drops "preview" and the promise that the legacy console is unaffected. It keeps the two-step frontend-then-Go build, names release archives and the container image as paths that already do it, and states that `go install` does not carry a usable console.
- The embed tree loses its `next` segment. The generated bundle, the checked-in fallback, the ignore rules, the container copy, and the build output path move together so no part of the build names a retired UI.
- The checked-in fallback stays inside the embedded directory rather than beside it. The embed pattern must match at least one file in a fresh clone where the generated bundle does not exist, and the handler detects a build by looking for the SPA entry rather than by the directory's presence.
- Release archives and the container image continue to build the frontend first. A missing bundle in the container remains a build error, not a silently console-less image.

### The legacy console is deleted, not retired behind a switch

- The legacy HTML, application script, terminal script, stylesheet, vendored libraries, and their contract tests are removed in the same change as the new root mount.
- No `--legacy` flag, environment toggle, or second route is added. A dual UI is the condition this spec removes; rollback is a release rollback.
- The Go JSON API, handlers, projections, and CLI are untouched. Pruning endpoints that happen to have no remaining first-party caller is a separate change with its own evidence and is not implied here.
- Legacy browser storage left behind by the old console (route prefixes, topology node positions) is not migrated or cleaned up. It is inert, and a migration would add code for a one-time cosmetic gain.

### Operator-facing surface

- The startup banner keeps the listening URL and build line and drops the preview line. The build stamp remains, because it is what an operator names in a bug report.
- The console remains unauthenticated and local by default. The cutover changes neither the listen address nor the security posture.
- Documentation treats `/` as the only console. Historical specs keep their historical `/next` references as a record; the current guide, design-system contract, README files, CLI reference, container notes, and agent instructions are updated to the root.

### Verification and rollout

- The cutover lands in one change: the legacy removal, the root mount, the redirect, the asset move, the embed-path rename, and the test/doc updates. Splitting the removal from the mount would leave a release with no console.
- The frontend build is verified to contain no `/next` route or asset prefix. This is the mechanical guard for the one failure this change can cause at every link.
- The release notes call out the address change and the new redirect. No data migration is needed.
- The visual-regression gap recorded in the UI migration backlog is not closed by this spec: the cutover moves paths rather than layouts, and the existing Vitest and Go route coverage plus the build-output assertion address the breakage this change introduces. A Playwright suite remains separate work.

## Testing Decisions

Good tests here assert externally visible HTTP and navigation behavior, not the internal shape of the server. The prior art is the existing Go route tests that exercise the console handler with `httptest`, and the Vitest suites that render routes and assert links.

- **Go route tests** cover: the root serves the built bundle; an unknown non-asset path serves the SPA entry; an unknown asset returns 404; `/api` and `/api/<unknown>` never return console HTML; `/next`, `/next/`, `/next/traces`, `/next/traces/<id>?q=x`, and `/next/_app/<asset>` redirect to their root equivalents with the query preserved; and a buildless embed serves the fallback page at the root and deep routes while still returning 404 for missing assets.
- **Legacy contract tests are deleted** rather than adapted. They assert markup and script contents of a bundle that no longer exists; keeping them would test deleted code.
- **Vitest and `svelte-check`** run against the root base. Existing route, store, and component tests are updated from preview-prefixed expectations to root expectations. A test that asserts a `/next` link is a failure, not a variant.
- **Build-output check**: after the production frontend build, verify the emitted entry document and asset references contain no `/next` prefix and that `_app` assets are emitted at the root. This catches a base-path regression that unit tests may not.
- **Build verification** follows the repository gate: formatting and lint for Go, Go build and tests, then frontend check, tests, production build, and a final Go build that embeds the bundle. The same script remains the single source for local and AFK checks, and its frontend trigger already covers a change under `web/`.
- **Manual browser pass** covers the things no current automated suite does: every menu route at the root, one detail route per family, reload on a detail page, theme switching, `Escape` back to a list, a `g` chord, and an old `/next/traces/<id>` bookmark. This pass is a release checklist item, not a substitute for the automated route tests.

## Out of Scope

- Removing or redesigning any JSON API endpoint, handler, projection, or CLI command.
- Changing any route's content, layout, filters, actions, or component behavior beyond what the base-path move requires.
- Adding authentication, TLS, or a reverse-proxy configuration.
- Supporting `go install` as a way to get the console, or committing the generated bundle.
- Migrating browser storage from the legacy console or the preview URL.
- Adding Playwright, screenshot comparison, or a component storybook. The UI migration backlog keeps that gap.
- Rewriting historical specs and changelog entries that describe the preview period. Their `/next` references are accurate for the time they record.
- Removing stale exports or artifacts left by the legacy console outside the repository, such as browser localStorage.

## Further Notes

The cutover should be treated as called parity rather than inferred parity: [035](035-queen-console-redesign.md) records every legacy tab as migrated, and this spec's acceptance is that no legacy-only operator workflow remains. The one behavior that changes is the URL; the one behavior that disappears is the second console.

The main risk is a missed base-path reference. That risk is mechanical, so it is answered mechanically: an empty SvelteKit base, the build-output assertion, and Go route tests that name the old prefix. A second risk is a stale external link; the permanent prefix redirect makes it a non-event.

If the cutover must be undone, revert the release. Reintroducing the legacy console as a runtime mode would restore exactly the maintenance cost this spec removes and would leave the root mount untested in the configuration most operators use.
