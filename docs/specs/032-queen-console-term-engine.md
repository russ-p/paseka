# Spec 032: Queen Console Selectable Terminal Engine

## Status

**(Draft)**
Queen Console session view can switch between the stable xterm engine and the experimental Ghostty WebGPU/WASM engine; xterm remains the default.

## Problem Statement

Queen Console renders interactive bee sessions with the xterm.js emulator over the PTY WebSocket relay. The Beekeeper has no choice of renderer: xterm is the only engine, even though the Ghostty terminal stack ships a WebGPU/WASM build that can be embedded page-side. Moving to Ghostty blindly would risk regressing the stable default, so the Beekeeper wants to try the experimental engine per-browser while keeping xterm as the safe fallback.

## Solution

Queen Console gains a terminal-engine toggle in the session view header. Clicking it switches between `xterm` and `ghostty`; the choice is remembered in `localStorage` per browser and reapplied on reload. The Ghostty engine is lazy-loaded on demand (dynamic import of the vendored WebGPU/WASM bundle, then async init) and only used when explicitly selected. On any engine switch during an attached session, the terminal is recreated and reconnected to the same session. A generation token guards the asynchronous recreate so that switching sessions or engines mid-load can never attach or stream the old session's PTY into the current view. If the experimental bundle fails to load, the Console falls back to xterm for that session and clears the persisted engine choice.

## User Stories

1. As a Beekeeper viewing an interactive session, I want a header control that shows which terminal engine is active, so that I can tell at a glance whether xterm or Ghostty rendered the view.
2. As a Beekeeper, I want to click the control to switch to the experimental Ghostty engine, so that I can try WebGPU/WASM rendering for the current session.
3. As a Beekeeper, I want to switch back to xterm with one more click, so that I can return to the stable renderer at any time.
4. As a Beekeeper, I want my engine choice remembered across page reloads, so that I do not re-select the engine every time I open the Console.
5. As a Beekeeper, I want the default to stay xterm, so that nothing changes for operators who never touch the toggle.
6. As a Beekeeper, I want Ghostty loaded only when selected, so that browsers that never use it do not pay the WASM download cost at Console boot.
7. As a Beekeeper who switches engines while attached to a session, I want the same session re-attached to the new engine, so that the conversation window survives the switch.
8. As a Beekeeper who switches sessions or engines while the Ghostty bundle is still loading, I want the newest request to win, so that a stale load cannot close the new session's relay or stream a previous session's terminal.
9. As a Beekeeper whose Ghostty bundle fails to load or initialize, I want an automatic fallback to xterm for the current session, so that the session stays usable.
10. As a Beekeeper after a Ghostty failure, I want the persisted engine reset to xterm, so that the next page load does not retry the broken engine.
11. As a Beekeeper, I want the vendor bundle served with a JavaScript Content-Type, so that the dynamic import works behind any proxy or mime table.
12. As a maintainer, I want the vendored Ghostty bundle covered by a static contract test, so that the embedded export surface and HTTP serving stay intact.

## Implementation Decisions

### 1. Engine model

- Two engines: `xterm` (stable, default) and `ghostty` (experimental WebGPU/WASM, vendored).
- Selection persisted in `localStorage` (`paseka.termEngine`); any read failure falls back to `xterm`.
- The engine toggle lives in the session view header next to the existing terminal controls and emits `aria-pressed` state for the current choice.

### 2. Lazy loading

- The Ghostty bundle is a vendored third-party asset under the SPA static root and is only fetched via dynamic `import()` when the engine is selected, with async WASM init before the terminal is constructed.
- The xterm UMD globals remain the eager default path.

### 3. Engine switching

- Switching engines with an attached session recreates the terminal bound to the same session (same exit callback, reconnected after recreate).
- Ghostty creates its canvas/hidden-textarea inside the recreated container and marks the container with an `ghostty` class for styling.

### 4. Async recreate race guard

- A module-level generation token is incremented on every detach (session change, engine switch, explicit detach).
- The asynchronous recreate chain (lazy import → terminal create → connect) captures the token before yielding and re-checks the token plus the target session id before connecting; the failure handler is also token-guarded so a superseded load cannot tear down a newer session.
- This invariant means the newest attach or engine switch always wins: a stale load never closes the current websocket or streams the old PTY.

### 5. Fallback

- Ghostty load/init failures log a console error, downgrade the engine to `xterm`, persist the downgrade, and recreate the terminal with xterm so the session stays usable.

## Testing Decisions

Good tests assert the SPA and HTTP static contracts, not vDOM or engine internals.

- Embedded SPA contracts in `internal/console/static_test.go`: terminal.js exposes `getEngine`/`setEngine` and references the vendored Ghostty module URL; index.html includes the engine toggle; app.js wires the toggle handler and engine-change render path.
- Vendor contract test in `internal/console/static_test.go`: the Ghostty bundle embeds the inline WASM, exports the expected surface and version, and `GET /lib/ghostty-web/ghostty-web.js` returns the bundle with a JavaScript `Content-Type` (guarding the dynamic `import()`).
- Manual/browser check: toggling engines mid-reconnect keeps the latest session attached and never mixes PTY streams.

Prior art: `TestXtermVendorStaticContract`, `TestCytoscapeVendorStaticContract` and the console static string needles in [002](./002-queen-console-mvp.md) / [029](./029-console-chrome-stream.md).

## Out of Scope

- Replacing xterm as the default engine.
- Ghostty link detection parity with xterm's WebLinksAddon (omitted).
- Server-side or per-colony engine selection (choice is browser-local).
- Any change to the PTY relay or session websocket contract.

## Further Notes

Related: [002](./002-queen-console-mvp.md) MVP baseline, [025](./025-cursor-session-resume.md) / [030](./030-opencode-session-resume.md) session views. Durable operator text lives in the Queen Console guide; the vendored bundle is embedded via the existing `static/lib` mechanism.