/* global Terminal, FitAddon, WebLinksAddon */

(function () {
  // xterm: UMD builds expose { FitAddon: class } / { WebLinksAddon: class } on the global.
  const FitAddonCtor = FitAddon.FitAddon || FitAddon;
  const WebLinksAddonCtor = WebLinksAddon.WebLinksAddon || WebLinksAddon;

  // Experimental ghostty-web engine (WebGPU/WASM) is lazy-loaded on demand.
  const ENGINE_STORAGE_KEY = 'paseka.termEngine';
  const ENGINE_XTERM = 'xterm';
  const ENGINE_GHOSTTY = 'ghostty';
  const GHOSTTY_MODULE_URL = '/lib/ghostty-web/ghostty-web.js';

  const RECONNECT_BASE_MS = 500;
  const RECONNECT_MAX_MS = 5000;

  const TERM_OPTIONS = {
    cursorBlink: true,
    fontSize: 13,
    fontFamily: "'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
    theme: {
      background: '#111318',
      foreground: '#e8eaed',
      cursor: '#f5c518',
    },
    scrollback: 5000,
  };

  let engine = readEngine();
  let curEngine = null; // engine of the currently-created terminal
  let lastContainer = null;
  let onEngineChange = null;

  let term = null;
  let fitAddon = null;
  let ws = null;
  let sessionId = null;
  let reconnectTimer = null;
  let reconnectAttempt = 0;
  let intentionalClose = false;
  let resizeObserver = null;
  let onExitCallback = null;
  let terminalGen = 0;

  function readEngine() {
    try {
      return localStorage.getItem(ENGINE_STORAGE_KEY) === ENGINE_GHOSTTY ? ENGINE_GHOSTTY : ENGINE_XTERM;
    } catch (_) {
      return ENGINE_XTERM;
    }
  }

  function writeEngine(value) {
    try {
      localStorage.setItem(ENGINE_STORAGE_KEY, value);
    } catch (_) {
      // localStorage unavailable; engine is session-only.
    }
  }

  function getEngine() {
    return engine;
  }

  function setEngine(next) {
    if (next !== ENGINE_XTERM && next !== ENGINE_GHOSTTY) next = ENGINE_XTERM;
    writeEngine(next);
    engine = next;
    if (onEngineChange) onEngineChange(engine);
    if (sessionId && lastContainer) {
      const id = sessionId;
      const cb = onExitCallback;
      detach();
      const gen = terminalGen;
      intentionalClose = false;
      sessionId = id;
      onExitCallback = cb;
      createTerminal(lastContainer, gen)
        .then(() => {
          if (gen !== terminalGen || sessionId !== id) return;
          connect(id);
        })
        .catch((err) => {
          if (gen !== terminalGen) return;
          console.error('Failed to recreate terminal engine', err);
          detach();
          if (onExitCallback) onExitCallback(engineErrorText(err));
        });
    }
  }

  function setOnEngineChange(cb) {
    onEngineChange = typeof cb === 'function' ? cb : null;
  }

  function engineErrorText(err) {
    return `engine error: ${err && err.message ? err.message : String(err)}`;
  }

  function wsBaseURL() {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${proto}//${location.host}`;
  }

  function sendResize() {
    if (!ws || ws.readyState !== WebSocket.OPEN || !fitAddon || !term) return;
    fitAddon.fit();
    const msg = JSON.stringify({
      type: 'resize',
      cols: term.cols,
      rows: term.rows,
    });
    ws.send(msg);
  }

  function clearReconnect() {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
  }

  function scheduleReconnect() {
    if (intentionalClose || !sessionId) return;
    clearReconnect();
    const delay = Math.min(RECONNECT_BASE_MS * 2 ** reconnectAttempt, RECONNECT_MAX_MS);
    reconnectAttempt += 1;
    reconnectTimer = setTimeout(() => {
      connect(sessionId);
    }, delay);
  }

  function connect(id) {
    if (!term || sessionId !== id) return;
    if (ws) {
      ws.onopen = null;
      ws.onclose = null;
      ws.onerror = null;
      ws.onmessage = null;
      if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
        ws.close();
      }
      ws = null;
    }

    const url = `${wsBaseURL()}/api/sessions/${encodeURIComponent(id)}/pty`;
    ws = new WebSocket(url);
    ws.binaryType = 'arraybuffer';

    ws.onopen = () => {
      reconnectAttempt = 0;
      sendResize();
    };

    ws.onmessage = (ev) => {
      if (typeof ev.data === 'string') {
        try {
          const msg = JSON.parse(ev.data);
          if (msg.type === 'status' && msg.state === 'exited') {
            intentionalClose = true;
            if (onExitCallback) onExitCallback(msg.reason || '');
          }
        } catch (_) {
          // ignore malformed status
        }
        return;
      }
      if (ev.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(ev.data));
      }
    };

    ws.onerror = () => {
      // onclose handles reconnect
    };

    ws.onclose = () => {
      ws = null;
      if (!intentionalClose) {
        scheduleReconnect();
      }
    };
  }

  let ghosttyInitPromise = null;
  async function initGhostty() {
    if (!ghosttyInitPromise) {
      ghosttyInitPromise = import(GHOSTTY_MODULE_URL).then((mod) => {
        if (typeof mod.init === 'function') {
          return mod.init().then(() => mod);
        }
        return mod;
      });
    }
    return ghosttyInitPromise;
  }

  function disposeTerminal() {
    if (resizeObserver) {
      resizeObserver.disconnect();
      resizeObserver = null;
    }
    if (term) {
      try {
        term.dispose();
      } catch (_) {
        // ignore dispose errors
      }
      term = null;
      fitAddon = null;
      if (curEngine === ENGINE_GHOSTTY && lastContainer) {
        // ghostty-web renders into a canvas + hidden textarea appended to the
        // container and mutates container attributes; undo both on detach.
        lastContainer.replaceChildren();
        for (const attr of ['tabindex', 'contenteditable', 'role', 'aria-label', 'aria-multiline']) {
          lastContainer.removeAttribute(attr);
        }
        lastContainer.className = lastContainer.className
          .split(/\s+/)
          .filter((c) => c !== 'ghostty')
          .join(' ')
          .trim();
      }
    }
    curEngine = null;
  }

  async function ensureTerminal(container, gen) {
    lastContainer = container;
    if (term && curEngine === engine) return;
    disposeTerminal();
    curEngine = engine;

    if (engine === ENGINE_GHOSTTY) {
      const mod = await initGhostty();
      // After the await a competing attach/setEngine may have claimed the
      // session; a stale chain must not touch the shared terminal state.
      if (gen !== terminalGen) return;
      term = new mod.Terminal(TERM_OPTIONS);
      fitAddon = new mod.FitAddon();
      term.loadAddon(fitAddon);
      // ghostty-web has no xterm WebLinksAddon; link detection is omitted.
      term.open(container);
      container.classList.add('ghostty');
    } else {
      term = new Terminal(TERM_OPTIONS);
      fitAddon = new FitAddonCtor();
      term.loadAddon(fitAddon);
      term.loadAddon(new WebLinksAddonCtor());
      term.open(container);
    }
    fitAddon.fit();

    term.onData((data) => {
      if (ws && ws.readyState === WebSocket.OPEN) {
        // Binary frames are PTY input; JSON text frames are control (resize).
        ws.send(new TextEncoder().encode(data));
      }
    });

    resizeObserver = new ResizeObserver(() => {
      if (!term || !fitAddon) return;
      fitAddon.fit();
      sendResize();
    });
    resizeObserver.observe(container);
  }

  function detach() {
    intentionalClose = true;
    clearReconnect();
    terminalGen += 1;
    sessionId = null;
    onExitCallback = null;

    if (ws) {
      ws.onopen = null;
      ws.onclose = null;
      ws.onerror = null;
      ws.onmessage = null;
      if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
        ws.close();
      }
      ws = null;
    }

    disposeTerminal();
  }

  async function createTerminal(container, gen) {
    try {
      await ensureTerminal(container, gen);
    } catch (err) {
      if (gen !== terminalGen) return; // superseded during engine init
      console.error('Terminal engine failed', err);
      if (engine === ENGINE_GHOSTTY) {
        // Experimental engine did not load; fall back to xterm so the session
        // stays usable. The chosen engine is persisted for next reload.
        engine = ENGINE_XTERM;
        writeEngine(ENGINE_XTERM);
        if (onEngineChange) onEngineChange(engine);
        await ensureTerminal(container, gen);
        return;
      }
      throw err;
    }
  }

  async function attach(id, container, options = {}) {
    if (!id || !container) return;
    if (sessionId === id && term) {
      fitAddon?.fit();
      return;
    }

    detach();
    const gen = terminalGen;
    intentionalClose = false;
    sessionId = id;
    onExitCallback = options.onExit || null;

    try {
      await createTerminal(container, gen);
    } catch (err) {
      if (gen !== terminalGen) return; // superseded; another chain owns cleanup
      detach();
      throw err;
    }

    if (gen !== terminalGen || sessionId !== id) return;
    connect(id);
  }

  function isAttached(id) {
    return sessionId === id && !!term;
  }

  function getSessionId() {
    return sessionId;
  }

  window.SessionTerminal = {
    attach,
    detach,
    isAttached,
    getSessionId,
    sendResize,
    getEngine,
    setEngine,
    setOnEngineChange,
  };
})();
