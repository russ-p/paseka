const state = {
  tab: 'sessions',
  bees: [],
  sessions: [],
  runs: [],
  runtime: null,
  runtimeBusy: false,
  selectedId: null,
  selectedRunKey: null,
  transcriptCursor: 0,
  transcriptLines: [],
  eventsCursor: 0,
  eventLines: [],
  pollTimer: null,
  runtimePollTimer: null,
};

const el = {
  subtitle: document.getElementById('subtitle'),
  tabSessions: document.getElementById('tab-sessions'),
  tabRuns: document.getElementById('tab-runs'),
  sessionsLayout: document.getElementById('sessions-layout'),
  runsLayout: document.getElementById('runs-layout'),
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
  runList: document.getElementById('run-list'),
  runsRefreshBtn: document.getElementById('runs-refresh-btn'),
  runDetailEmpty: document.getElementById('run-detail-empty'),
  runDetailMeta: document.getElementById('run-detail-meta'),
  runSummaryWrap: document.getElementById('run-summary-wrap'),
  runSummary: document.getElementById('run-summary'),
  runEventsWrap: document.getElementById('run-events-wrap'),
  runEvents: document.getElementById('run-events'),
  runtimeBadge: document.getElementById('runtime-badge'),
  runtimeMeta: document.getElementById('runtime-meta'),
  runtimeStartBtn: document.getElementById('runtime-start-btn'),
  runtimeStopBtn: document.getElementById('runtime-stop-btn'),
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

function runKey(run) {
  return `${run.traceId}/${run.agentId}`;
}

function badgeClass(itemState) {
  const s = (itemState || '').toLowerCase();
  if (s === 'active' || s === 'running' || s === 'queued') return 'active';
  if (s === 'completed') return 'completed';
  if (s === 'failed' || s === 'cancelled') return 'failed';
  if (s === 'stale' || s === 'stopping') return 'failed';
  return '';
}

function renderRuntime() {
  const rt = state.runtime || { status: 'stopped', alive: false };
  const status = (rt.status || 'stopped').toLowerCase();
  el.runtimeBadge.textContent = status;
  el.runtimeBadge.className = `badge ${status}`;

  const parts = [];
  if (rt.pid) parts.push(`pid ${rt.pid}`);
  if (rt.startedAt) parts.push(`started ${formatTime(rt.startedAt)}`);
  if (rt.lastHeartbeatAt) parts.push(`heartbeat ${formatTime(rt.lastHeartbeatAt)}`);
  el.runtimeMeta.textContent = parts.length ? parts.join(' · ') : (rt.alive ? 'Running' : 'Not running');

  const canStart = !state.runtimeBusy && status !== 'running' && status !== 'stopping' && status !== 'starting';
  const canStop = !state.runtimeBusy && (rt.alive || status === 'running' || status === 'stopping');

  el.runtimeStartBtn.classList.toggle('hidden', !canStart);
  el.runtimeStopBtn.classList.toggle('hidden', !canStop);
  el.runtimeStartBtn.disabled = state.runtimeBusy;
  el.runtimeStopBtn.disabled = state.runtimeBusy;
}

async function loadRuntime() {
  state.runtime = await api('/api/runtime');
  renderRuntime();
}

function startRuntimePolling() {
  if (state.runtimePollTimer) return;
  state.runtimePollTimer = setInterval(() => {
    loadRuntime().catch(console.error);
  }, 3000);
}

function escapeHtml(str) {
  return String(str)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;');
}

function setTab(tab) {
  state.tab = tab;
  const isSessions = tab === 'sessions';
  el.tabSessions.classList.toggle('active', isSessions);
  el.tabRuns.classList.toggle('active', !isSessions);
  el.tabSessions.setAttribute('aria-selected', String(isSessions));
  el.tabRuns.setAttribute('aria-selected', String(!isSessions));
  el.sessionsLayout.classList.toggle('hidden', !isSessions);
  el.runsLayout.classList.toggle('hidden', isSessions);
  el.subtitle.textContent = isSessions
    ? 'Sessions — launch and observe interactive bees'
    : 'Runs — observe headless adapter invocations';
  stopPolling();
  if (isSessions && state.selectedId) {
    startSessionPolling();
  } else if (!isSessions && state.selectedRunKey) {
    startRunPolling();
  }
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

function renderRuns() {
  el.runList.innerHTML = '';
  if (!state.runs.length) {
    const li = document.createElement('li');
    li.className = 'muted';
    li.textContent = 'No runs yet.';
    el.runList.appendChild(li);
    return;
  }
  for (const run of state.runs) {
    const key = runKey(run);
    const li = document.createElement('li');
    li.className = 'session-item' + (key === state.selectedRunKey ? ' selected' : '');
    li.dataset.key = key;
    const sessionNote = run.hasSession ? ' · session' : '';
    li.innerHTML = `
      <div class="top">
        <span class="bee">${escapeHtml(run.bee)}</span>
        <span class="badge ${badgeClass(run.state)}">${escapeHtml(run.state || 'unknown')}</span>
      </div>
      <div class="id">${escapeHtml(run.traceId)} / ${escapeHtml(run.agentId)}</div>
      <div class="muted" style="font-size:0.8rem;margin-top:0.25rem">${formatTime(run.startedAt)}${escapeHtml(sessionNote)}</div>
    `;
    li.addEventListener('click', () => selectRun(run.traceId, run.agentId));
    el.runList.appendChild(li);
  }
}

function renderTranscript() {
  el.transcript.innerHTML = state.transcriptLines
    .map((line) => `<div class="line-${escapeHtml(line.role)}">[${escapeHtml(line.role)}] ${escapeHtml(line.content)}</div>`)
    .join('');
  el.transcript.scrollTop = el.transcript.scrollHeight;
}

function renderEvents() {
  el.runEvents.innerHTML = state.eventLines
    .map((ev) => {
      const payload = ev.payload ? ` ${JSON.stringify(ev.payload)}` : '';
      return `<div class="line-agent">[${escapeHtml(ev.type)} #${ev.seq}]${escapeHtml(payload)}</div>`;
    })
    .join('');
  el.runEvents.scrollTop = el.runEvents.scrollHeight;
}

function renderSessionDetail(session) {
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

function renderRunDetail(run) {
  if (!run) {
    el.runDetailEmpty.classList.remove('hidden');
    el.runDetailMeta.classList.add('hidden');
    el.runSummaryWrap.classList.add('hidden');
    el.runEventsWrap.classList.add('hidden');
    return;
  }
  el.runDetailEmpty.classList.add('hidden');
  el.runDetailMeta.classList.remove('hidden');

  const rows = [
    ['State', run.state],
    ['Trace ID', run.traceId],
    ['Agent ID', run.agentId],
    ['Bee', run.bee],
    ['Adapter', run.adapter],
    ['Task ID', run.taskId],
    ['Intent', run.intent],
    ['Workspace', run.workspace],
    ['Run dir', run.runDir],
    ['Started', formatTime(run.startedAt)],
    ['Finished', formatTime(run.finishedAt)],
    ['Has session', run.hasSession ? 'yes' : 'no'],
    ['Has events', run.hasEvents ? 'yes' : 'no'],
  ];

  el.runDetailMeta.innerHTML = rows
    .map(([k, v]) => `<dt>${escapeHtml(k)}</dt><dd>${escapeHtml(v || '—')}</dd>`)
    .join('');

  if (run.task) {
    el.runDetailMeta.innerHTML += `<dt>Task</dt><dd>${escapeHtml(run.task)}</dd>`;
  }

  if (run.summary) {
    el.runSummaryWrap.classList.remove('hidden');
    el.runSummary.textContent = run.summary;
  } else {
    el.runSummaryWrap.classList.add('hidden');
    el.runSummary.textContent = '';
  }

  if (run.hasEvents) {
    el.runEventsWrap.classList.remove('hidden');
    renderEvents();
  } else {
    el.runEventsWrap.classList.add('hidden');
    el.runEvents.textContent = '';
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
    if (still) renderSessionDetail(still);
  }
}

async function loadRuns() {
  state.runs = await api('/api/runs');
  renderRuns();
  if (state.selectedRunKey) {
    const still = state.runs.find((r) => runKey(r) === state.selectedRunKey);
    if (still) renderRunDetail(still);
  }
}

async function selectSession(id) {
  state.selectedId = id;
  state.transcriptCursor = 0;
  state.transcriptLines = [];
  renderSessions();
  const session = await api(`/api/sessions/${encodeURIComponent(id)}`);
  renderSessionDetail(session);
  renderTranscript();
  if (state.tab === 'sessions') {
    startSessionPolling();
  }
}

async function selectRun(traceId, agentId) {
  state.selectedRunKey = `${traceId}/${agentId}`;
  state.eventsCursor = 0;
  state.eventLines = [];
  renderRuns();
  const run = await api(`/api/runs/${encodeURIComponent(traceId)}/${encodeURIComponent(agentId)}`);
  renderRunDetail(run);
  if (state.tab === 'runs') {
    startRunPolling();
  }
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

async function pollRunEvents() {
  if (!state.selectedRunKey) return;
  const [traceId, agentId] = state.selectedRunKey.split('/');
  try {
    const page = await api(`/api/runs/${encodeURIComponent(traceId)}/${encodeURIComponent(agentId)}/events?after=${state.eventsCursor}`);
    if (page.entries && page.entries.length) {
      state.eventLines.push(...page.entries);
      state.eventsCursor = page.nextCursor;
      renderEvents();
    }
    const run = state.runs.find((r) => runKey(r) === state.selectedRunKey);
    if (run && (run.state === 'running' || run.state === 'queued')) {
      await loadRuns();
      const refreshed = await api(`/api/runs/${encodeURIComponent(traceId)}/${encodeURIComponent(agentId)}`);
      renderRunDetail(refreshed);
    }
  } catch (err) {
    console.error(err);
  }
}

function stopPolling() {
  if (state.pollTimer) {
    clearInterval(state.pollTimer);
    state.pollTimer = null;
  }
}

function startSessionPolling() {
  stopPolling();
  state.pollTimer = setInterval(pollTranscript, 1500);
  pollTranscript();
}

function startRunPolling() {
  stopPolling();
  state.pollTimer = setInterval(pollRunEvents, 1500);
  pollRunEvents();
}

el.tabSessions.addEventListener('click', () => setTab('sessions'));
el.tabRuns.addEventListener('click', () => {
  setTab('runs');
  loadRuns().catch(console.error);
});

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

el.runsRefreshBtn.addEventListener('click', () => {
  loadRuns().catch(console.error);
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

el.runtimeStartBtn.addEventListener('click', async () => {
  state.runtimeBusy = true;
  renderRuntime();
  try {
    state.runtime = await api('/api/runtime/start', { method: 'POST' });
    renderRuntime();
    startRuntimePolling();
  } catch (err) {
    alert(err.message);
    await loadRuntime();
  } finally {
    state.runtimeBusy = false;
    renderRuntime();
  }
});

el.runtimeStopBtn.addEventListener('click', async () => {
  if (!confirm('Stop the hive runtime? In-flight dispatches may be interrupted.')) return;
  state.runtimeBusy = true;
  renderRuntime();
  try {
    state.runtime = await api('/api/runtime/stop', { method: 'POST' });
    renderRuntime();
  } catch (err) {
    alert(err.message);
    await loadRuntime();
  } finally {
    state.runtimeBusy = false;
    renderRuntime();
  }
});

async function init() {
  try {
    await loadRuntime();
    startRuntimePolling();
    await loadBees();
    await loadSessions();
    await loadRuns();
  } catch (err) {
    console.error(err);
  }
}

init();
