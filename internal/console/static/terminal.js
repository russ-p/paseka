/* global Terminal, FitAddon, WebLinksAddon */

(function () {
  // UMD builds expose { FitAddon: class } / { WebLinksAddon: class } on the global.
  const FitAddonCtor = FitAddon.FitAddon || FitAddon;
  const WebLinksAddonCtor = WebLinksAddon.WebLinksAddon || WebLinksAddon;

  const RECONNECT_BASE_MS = 500;
  const RECONNECT_MAX_MS = 5000;

  let term = null;
  let fitAddon = null;
  let ws = null;
  let sessionId = null;
  let reconnectTimer = null;
  let reconnectAttempt = 0;
  let intentionalClose = false;
  let resizeObserver = null;
  let onExitCallback = null;

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
    if (!term) return;
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

  function ensureTerminal(container) {
    if (term) return;
    term = new Terminal({
      cursorBlink: true,
      fontSize: 13,
      fontFamily: "'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
      theme: {
        background: '#111318',
        foreground: '#e8eaed',
        cursor: '#f5c518',
      },
      scrollback: 5000,
    });
    fitAddon = new FitAddonCtor();
    term.loadAddon(fitAddon);
    term.loadAddon(new WebLinksAddonCtor());
    term.open(container);
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
    sessionId = null;
    onExitCallback = null;

    if (resizeObserver) {
      resizeObserver.disconnect();
      resizeObserver = null;
    }

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

    if (term) {
      term.dispose();
      term = null;
      fitAddon = null;
    }
  }

  function attach(id, container, options = {}) {
    if (!id || !container) return;
    if (sessionId === id && term) {
      fitAddon?.fit();
      return;
    }

    detach();
    intentionalClose = false;
    sessionId = id;
    onExitCallback = options.onExit || null;

    ensureTerminal(container);
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
  };
})();
