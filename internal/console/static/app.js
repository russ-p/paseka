const state = {
  bees: [],
  sessions: [],
  selectedId: null,
  transcriptCursor: 0,
  transcriptLines: [],
  pollTimer: null,
};

const el = {
  beeSelect: document.getElementById('bee-select'),
  taskInput: document.getElementById('task-input'),
  rawToggle: document.getElementById('raw-prompt-toggle'),
  rawLabel: document.getElementById('raw-label'),
  taskLabel: document.getElementById('task-label'),
  rawInput: document.getElementById('raw-prompt-input'),
  traceInput: document.getElementById('trace-input'),
  intentSelect: document.getElementById('intent-select'),
  launchForm: document.getElementById('launch-form'),
  launchError: document.getElementById('launch-error'),
  launchBtn: document.getElementById('launch-btn'),
  sessionList: document.getElementById('session-list'),
  refreshBtn: document.getElementById('refresh-btn'),
  detailEmpty: document.getElementById('detail-empty'),
  detailMeta: document.getElementById('detail-meta'),
  transcriptWrap: document.getElementById('transcript-wrap'),
  transcript: document.getElementById('transcript'),
  stopBtn: document.getElementById('stop-btn'),
};

async function api(path, options = {}) {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
    ...options,
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || res.statusText);
  }
  if (res.status === 204) return null;
  return res.json();
}

function formatTime(iso) {
  if (!iso) return '—';
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

function badgeClass(sessionState) {
  const s = (sessionState || '').toLowerCase();
  if (s === 'active') return 'active';
  if (s === 'completed') return 'completed';
  if (s === 'failed' || s === 'cancelled') return 'failed';
  return '';
}

function renderBees() {
  el.beeSelect.innerHTML = '';
  if (!state.bees.length) {
    const opt = document.createElement('option');
    opt.value = '';
    opt.textContent = 'No interactive bees found';
    el.beeSelect.appendChild(opt);
    el.launchBtn.disabled = true;
    return;
  }
  el.launchBtn.disabled = false;
  for (const bee of state.bees) {
    const opt = document.createElement('option');
    opt.value = bee.role;
    opt.textContent = `${bee.role} (${bee.adapter})`;
    el.beeSelect.appendChild(opt);
  }
}

function renderSessions() {
  el.sessionList.innerHTML = '';
  if (!state.sessions.length) {
    const li = document.createElement('li');
    li.className = 'muted';
    li.textContent = 'No sessions yet.';
    el.sessionList.appendChild(li);
    return;
  }
  for (const s of state.sessions) {
    const li = document.createElement('li');
    li.className = 'session-item' + (s.sessionId === state.selectedId ? ' selected' : '');
    li.dataset.id = s.sessionId;
    li.innerHTML = `
      <div class="top">
        <span class="bee">${escapeHtml(s.bee)}</span>
        <span class="badge ${badgeClass(s.state)}">${escapeHtml(s.state || 'unknown')}</span>
      </div>
      <div class="id">${escapeHtml(s.sessionId)}</div>
      <div class="muted" style="font-size:0.8rem;margin-top:0.25rem">${formatTime(s.startedAt)}</div>
    `;
    li.addEventListener('click', () => selectSession(s.sessionId));
    el.sessionList.appendChild(li);
  }
}

function escapeHtml(str) {
  return String(str)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;');
}

function renderTranscript() {
  el.transcript.innerHTML = state.transcriptLines
    .map((line) => `<div class="line-${escapeHtml(line.role)}">[${escapeHtml(line.role)}] ${escapeHtml(line.content)}</div>`)
    .join('');
  el.transcript.scrollTop = el.transcript.scrollHeight;
}

function renderDetail(session) {
  if (!session) {
    el.detailEmpty.classList.remove('hidden');
    el.detailMeta.classList.add('hidden');
    el.transcriptWrap.classList.add('hidden');
    el.stopBtn.classList.add('hidden');
    return;
  }
  el.detailEmpty.classList.add('hidden');
  el.detailMeta.classList.remove('hidden');
  el.transcriptWrap.classList.remove('hidden');

  const rows = [
    ['State', session.state],
    ['Session ID', session.sessionId],
    ['Trace ID', session.traceId],
    ['Agent ID', session.agentId],
    ['Bee', session.bee],
    ['Workspace', session.workspace],
    ['Run dir', session.runDir],
    ['Started', formatTime(session.startedAt)],
    ['Finished', formatTime(session.finishedAt)],
  ];
  if (session.pid) rows.push(['PID', String(session.pid)]);

  el.detailMeta.innerHTML = rows
    .map(([k, v]) => `<dt>${escapeHtml(k)}</dt><dd>${escapeHtml(v || '—')}</dd>`)
    .join('');

  if (session.active) {
    el.stopBtn.classList.remove('hidden');
  } else {
    el.stopBtn.classList.add('hidden');
  }
}

async function loadBees() {
  state.bees = await api('/api/bees');
  renderBees();
}

async function loadSessions() {
  state.sessions = await api('/api/sessions');
  renderSessions();
  if (state.selectedId) {
    const still = state.sessions.find((s) => s.sessionId === state.selectedId);
    if (still) renderDetail(still);
  }
}

async function selectSession(id) {
  state.selectedId = id;
  state.transcriptCursor = 0;
  state.transcriptLines = [];
  renderSessions();
  const session = await api(`/api/sessions/${encodeURIComponent(id)}`);
  renderDetail(session);
  renderTranscript();
  startPolling();
}

async function pollTranscript() {
  if (!state.selectedId) return;
  try {
    const page = await api(`/api/sessions/${encodeURIComponent(state.selectedId)}/transcript?after=${state.transcriptCursor}`);
    if (page.entries && page.entries.length) {
      state.transcriptLines.push(...page.entries);
      state.transcriptCursor = page.nextCursor;
      renderTranscript();
    }
    const session = state.sessions.find((s) => s.sessionId === state.selectedId);
    if (session && session.active) {
      await loadSessions();
    }
  } catch (err) {
    console.error(err);
  }
}

function startPolling() {
  if (state.pollTimer) clearInterval(state.pollTimer);
  state.pollTimer = setInterval(pollTranscript, 1500);
  pollTranscript();
}

el.rawToggle.addEventListener('change', () => {
  const on = el.rawToggle.checked;
  el.rawLabel.classList.toggle('hidden', !on);
  el.taskLabel.classList.toggle('hidden', on);
});

el.launchForm.addEventListener('submit', async (ev) => {
  ev.preventDefault();
  el.launchError.classList.add('hidden');
  el.launchBtn.disabled = true;
  try {
    const body = {
      bee: el.beeSelect.value,
      traceId: el.traceInput.value.trim(),
      intent: el.intentSelect.value,
      useRawPrompt: el.rawToggle.checked,
      task: el.taskInput.value.trim(),
      rawPrompt: el.rawInput.value.trim(),
    };
    const created = await api('/api/sessions', { method: 'POST', body: JSON.stringify(body) });
    await loadSessions();
    await selectSession(created.sessionId);
    el.taskInput.value = '';
    el.rawInput.value = '';
    el.traceInput.value = '';
  } catch (err) {
    el.launchError.textContent = err.message;
    el.launchError.classList.remove('hidden');
  } finally {
    el.launchBtn.disabled = false;
  }
});

el.refreshBtn.addEventListener('click', () => {
  loadSessions().catch(console.error);
});

el.stopBtn.addEventListener('click', async () => {
  if (!state.selectedId) return;
  el.stopBtn.disabled = true;
  try {
    await api(`/api/sessions/${encodeURIComponent(state.selectedId)}/stop`, { method: 'POST' });
    await loadSessions();
    await selectSession(state.selectedId);
  } catch (err) {
    alert(err.message);
  } finally {
    el.stopBtn.disabled = false;
  }
});

async function init() {
  try {
    await loadBees();
    await loadSessions();
  } catch (err) {
    console.error(err);
  }
}

init();
