const state = {
  tab: 'dashboard',
  bees: [],
  sessions: [],
  runs: [],
  traces: [],
  selectedTraceId: null,
  selectedTraceDetail: null,
  traceDetailError: '',
  traceDetailLoading: false,
  tasks: null,
  reviews: null,
  selectedReviewKey: null,
  selectedReviewDetail: null,
  selectedTaskKey: null,
  selectedTaskDetail: null,
  dashboard: null,
  timelineItems: [],
  timelineCursor: '',
  timelineHasMore: false,
  timelineShowRaw: false,
  timelineFilters: {},
  runtime: null,
  runtimeBusy: false,
  selectedId: null,
  selectedRunKey: null,
  transcriptCursor: 0,
  transcriptLines: [],
  eventsCursor: 0,
  eventLines: [],
  terminalWide: false,
  pollTimer: null,
  runtimePollTimer: null,
  dashboardPollTimer: null,
  tracesPollTimer: null,
  tasksPollTimer: null,
  reviewsPollTimer: null,
};

const el = {
  subtitle: document.getElementById('subtitle'),
  tabDashboard: document.getElementById('tab-dashboard'),
  tabTraces: document.getElementById('tab-traces'),
  tabTimeline: document.getElementById('tab-timeline'),
  tabTasks: document.getElementById('tab-tasks'),
  tabReviews: document.getElementById('tab-reviews'),
  tabSessions: document.getElementById('tab-sessions'),
  tabRuns: document.getElementById('tab-runs'),
  dashboardLayout: document.getElementById('dashboard-layout'),
  tracesLayout: document.getElementById('traces-layout'),
  timelineLayout: document.getElementById('timeline-layout'),
  dashboardStats: document.getElementById('dashboard-stats'),
  dashboardTraces: document.getElementById('dashboard-traces'),
  dashboardFailedRuns: document.getElementById('dashboard-failed-runs'),
  dashboardInsights: document.getElementById('dashboard-insights'),
  dashboardRefreshBtn: document.getElementById('dashboard-refresh-btn'),
  tracesRefreshBtn: document.getElementById('traces-refresh-btn'),
  traceList: document.getElementById('trace-list'),
  traceDetailEmpty: document.getElementById('trace-detail-empty'),
  traceDetailError: document.getElementById('trace-detail-error'),
  traceDetailLoading: document.getElementById('trace-detail-loading'),
  traceDetailBody: document.getElementById('trace-detail-body'),
  traceDetailMeta: document.getElementById('trace-detail-meta'),
  traceEnergyWrap: document.getElementById('trace-energy-wrap'),
  traceEnergy: document.getElementById('trace-energy'),
  traceWorktreeWrap: document.getElementById('trace-worktree-wrap'),
  traceWorktreeMeta: document.getElementById('trace-worktree-meta'),
  traceTasksList: document.getElementById('trace-tasks-list'),
  traceRunsList: document.getElementById('trace-runs-list'),
  traceEventsList: document.getElementById('trace-events-list'),
  traceOpenTimelineBtn: document.getElementById('trace-open-timeline-btn'),
  timelineRefreshBtn: document.getElementById('timeline-refresh-btn'),
  timelineFilters: document.getElementById('timeline-filters'),
  filterTrace: document.getElementById('filter-trace'),
  filterTask: document.getElementById('filter-task'),
  filterBee: document.getElementById('filter-bee'),
  filterType: document.getElementById('filter-type'),
  filterKind: document.getElementById('filter-kind'),
  filterSeverity: document.getElementById('filter-severity'),
  timelineRawToggle: document.getElementById('timeline-raw-toggle'),
  timelineFeed: document.getElementById('timeline-feed'),
  timelineMoreBtn: document.getElementById('timeline-more-btn'),
  tasksLayout: document.getElementById('tasks-layout'),
  taskCreateForm: document.getElementById('task-create-form'),
  taskTitleInput: document.getElementById('task-title-input'),
  taskBodyInput: document.getElementById('task-body-input'),
  taskBeeInput: document.getElementById('task-bee-input'),
  taskTraceInput: document.getElementById('task-trace-input'),
  taskSectorInput: document.getElementById('task-sector-input'),
  taskIntentSelect: document.getElementById('task-intent-select'),
  taskReviewSelect: document.getElementById('task-review-select'),
  taskDependsInput: document.getElementById('task-depends-input'),
  taskAutorunToggle: document.getElementById('task-autorun-toggle'),
  taskCreateBtn: document.getElementById('task-create-btn'),
  taskCreateError: document.getElementById('task-create-error'),
  tasksRefreshBtn: document.getElementById('tasks-refresh-btn'),
  taskBoard: document.getElementById('task-board'),
  taskDetailEmpty: document.getElementById('task-detail-empty'),
  taskDetailMeta: document.getElementById('task-detail-meta'),
  taskBodyWrap: document.getElementById('task-body-wrap'),
  taskBody: document.getElementById('task-body'),
  taskRunsWrap: document.getElementById('task-runs-wrap'),
  taskRunsList: document.getElementById('task-runs-list'),
  taskDetailActions: document.getElementById('task-detail-actions'),
  taskStartBtn: document.getElementById('task-start-btn'),
  taskApproveBtn: document.getElementById('task-approve-btn'),
  taskRejectBtn: document.getElementById('task-reject-btn'),
  taskOpenTimelineBtn: document.getElementById('task-open-timeline-btn'),
  taskReviewActions: document.getElementById('task-review-actions'),
  taskApproveForm: document.getElementById('task-approve-form'),
  taskApproveSummary: document.getElementById('task-approve-summary'),
  taskMergeMessageLabel: document.getElementById('task-merge-message-label'),
  taskMergeMessage: document.getElementById('task-merge-message'),
  taskRejectForm: document.getElementById('task-reject-form'),
  taskRejectFeedback: document.getElementById('task-reject-feedback'),
  taskReviewError: document.getElementById('task-review-error'),
  reviewsLayout: document.getElementById('reviews-layout'),
  reviewsRefreshBtn: document.getElementById('reviews-refresh-btn'),
  reviewQueueList: document.getElementById('review-queue-list'),
  reviewDetailEmpty: document.getElementById('review-detail-empty'),
  reviewDetailMeta: document.getElementById('review-detail-meta'),
  reviewSummaryWrap: document.getElementById('review-summary-wrap'),
  reviewSummary: document.getElementById('review-summary'),
  reviewActionsWrap: document.getElementById('review-actions-wrap'),
  reviewFinalHint: document.getElementById('review-final-hint'),
  reviewApproveForm: document.getElementById('review-approve-form'),
  reviewApproveSummary: document.getElementById('review-approve-summary'),
  reviewMergeMessageLabel: document.getElementById('review-merge-message-label'),
  reviewMergeMessage: document.getElementById('review-merge-message'),
  reviewRejectForm: document.getElementById('review-reject-form'),
  reviewRejectFeedback: document.getElementById('review-reject-feedback'),
  reviewOpenTimelineBtn: document.getElementById('review-open-timeline-btn'),
  reviewOpenRunsBtn: document.getElementById('review-open-runs-btn'),
  reviewActionError: document.getElementById('review-action-error'),
  reviewActionSuccess: document.getElementById('review-action-success'),
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
  terminalWrap: document.getElementById('terminal-wrap'),
  terminalContainer: document.getElementById('terminal-container'),
  terminalStatus: document.getElementById('terminal-status'),
  terminalWideBtn: document.getElementById('terminal-wide-btn'),
  transcriptWrap: document.getElementById('transcript-wrap'),
  transcriptHeading: document.getElementById('transcript-heading'),
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
    return new Date(iso).toLocaleString(undefined, {
      year: 'numeric',
      month: 'numeric',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    });
  } catch {
    return iso;
  }
}

function runKey(run) {
  return `${run.traceId}/${run.agentId}`;
}

function taskKey(task) {
  return `${task.traceId}/${task.taskId}`;
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
  const tabs = ['dashboard', 'traces', 'timeline', 'tasks', 'reviews', 'sessions', 'runs'];
  const layouts = {
    dashboard: el.dashboardLayout,
    traces: el.tracesLayout,
    timeline: el.timelineLayout,
    tasks: el.tasksLayout,
    reviews: el.reviewsLayout,
    sessions: el.sessionsLayout,
    runs: el.runsLayout,
  };
  for (const name of tabs) {
    const active = tab === name;
    const btn = el[`tab${name.charAt(0).toUpperCase()}${name.slice(1)}`];
    if (btn) {
      btn.classList.toggle('active', active);
      btn.setAttribute('aria-selected', String(active));
    }
    if (layouts[name]) {
      layouts[name].classList.toggle('hidden', !active);
    }
  }

  const subtitles = {
    dashboard: 'Dashboard — colony-wide snapshot and recent activity',
    traces: 'Traces — inspect flight trails, tasks, runs, and worktrees',
    timeline: 'Timeline — filterable event feed across the colony',
    tasks: 'Tasks — create, start, and inspect trace tasks',
    reviews: 'Reviews — approve or reject proposals awaiting human review',
    sessions: 'Sessions — launch, attach, and observe interactive bees',
    runs: 'Runs — observe headless adapter invocations',
  };
  el.subtitle.textContent = subtitles[tab] || '';

  stopPolling();
  stopDashboardPolling();
  stopTracesPolling();
  stopTasksPolling();
  stopReviewsPolling();
  if (tab !== 'sessions') {
    detachSessionTerminal();
    setTerminalWide(false);
  }
  if (tab === 'sessions' && state.selectedId) {
    startSessionPolling();
  } else if (tab === 'runs' && state.selectedRunKey) {
    startRunPolling();
  } else if (tab === 'dashboard') {
    startDashboardPolling();
  } else if (tab === 'traces') {
    startTracesPolling();
  } else if (tab === 'timeline') {
    loadTimeline(true).catch(console.error);
  } else if (tab === 'tasks') {
    startTasksPolling();
  } else if (tab === 'reviews') {
    startReviewsPolling();
  }
}

function findBee(role) {
  return state.bees.find((b) => b.role === role);
}

function renderIntentSelect(selectEl, bee) {
  selectEl.innerHTML = '';
  const blank = document.createElement('option');
  blank.value = '';
  blank.textContent = '—';
  selectEl.appendChild(blank);
  if (!bee?.intents?.length) {
    return;
  }
  for (const intent of bee.intents) {
    const opt = document.createElement('option');
    opt.value = intent;
    opt.textContent = intent;
    selectEl.appendChild(opt);
  }
  if (bee.defaultIntent) {
    selectEl.value = bee.defaultIntent;
  }
}

function syncSessionIntents() {
  const bee = findBee(el.beeSelect.value);
  renderIntentSelect(el.intentSelect, bee);
}

function syncTaskIntents() {
  const bee = findBee(el.taskBeeInput.value);
  renderIntentSelect(el.taskIntentSelect, bee);
}

function renderBees() {
  el.beeSelect.innerHTML = '';
  el.taskBeeInput.innerHTML = '';
  if (!state.bees.length) {
    const sessionOpt = document.createElement('option');
    sessionOpt.value = '';
    sessionOpt.textContent = 'No interactive bees found';
    el.beeSelect.appendChild(sessionOpt);
    el.launchBtn.disabled = true;
    el.taskCreateBtn.disabled = true;
    renderIntentSelect(el.intentSelect, null);
    renderIntentSelect(el.taskIntentSelect, null);
    return;
  }
  el.launchBtn.disabled = false;
  el.taskCreateBtn.disabled = false;
  for (const bee of state.bees) {
    const sessionOpt = document.createElement('option');
    sessionOpt.value = bee.role;
    sessionOpt.textContent = `${bee.role} (${bee.adapter})`;
    el.beeSelect.appendChild(sessionOpt);

    const taskOpt = document.createElement('option');
    taskOpt.value = bee.role;
    taskOpt.textContent = `${bee.role} (${bee.adapter})`;
    el.taskBeeInput.appendChild(taskOpt);
  }
  const builder = findBee('builder');
  if (builder) {
    el.beeSelect.value = builder.role;
    el.taskBeeInput.value = builder.role;
  }
  syncSessionIntents();
  syncTaskIntents();
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

function setTerminalStatus(text) {
  if (el.terminalStatus) {
    el.terminalStatus.textContent = text;
  }
}

function setTerminalWide(wide) {
  state.terminalWide = !!wide;
  if (el.sessionsLayout) {
    el.sessionsLayout.classList.toggle('terminal-wide', state.terminalWide);
  }
  if (el.terminalWideBtn) {
    el.terminalWideBtn.setAttribute('aria-pressed', String(state.terminalWide));
    el.terminalWideBtn.textContent = state.terminalWide ? 'Restore' : 'Widen';
    el.terminalWideBtn.title = state.terminalWide
      ? 'Restore normal session layout'
      : 'Widen terminal to full page width';
  }
  requestAnimationFrame(() => {
    window.SessionTerminal?.sendResize?.();
  });
}

function detachSessionTerminal() {
  if (window.SessionTerminal) {
    window.SessionTerminal.detach();
  }
  setTerminalStatus('');
}

function attachSessionTerminal(session) {
  if (!session || !session.active || !window.SessionTerminal) {
    detachSessionTerminal();
    return;
  }
  setTerminalStatus('connecting…');
  window.SessionTerminal.attach(session.sessionId, el.terminalContainer, {
    onExit: async (reason) => {
      setTerminalStatus(reason ? `exited — ${reason}` : 'exited');
      await loadSessions();
      if (state.selectedId === session.sessionId) {
        const refreshed = await api(`/api/sessions/${encodeURIComponent(session.sessionId)}`);
        renderSessionDetail(refreshed);
        state.transcriptCursor = 0;
        state.transcriptLines = [];
        await pollTranscript();
        startSessionPolling();
      }
    },
  });
  setTerminalStatus('connected');
  requestAnimationFrame(() => {
    window.SessionTerminal.sendResize?.();
  });
}

function renderSessionDetail(session) {
  if (!session) {
    el.detailEmpty.classList.remove('hidden');
    el.detailMeta.classList.add('hidden');
    el.terminalWrap.classList.add('hidden');
    el.transcriptWrap.classList.add('hidden');
    el.stopBtn.classList.add('hidden');
    detachSessionTerminal();
    setTerminalWide(false);
    return;
  }
  el.detailEmpty.classList.add('hidden');
  el.detailMeta.classList.remove('hidden');

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
    el.terminalWrap.classList.remove('hidden');
    el.transcriptWrap.classList.add('hidden');
    el.transcriptWrap.classList.remove('inactive-only');
    if (state.tab === 'sessions') {
      attachSessionTerminal(session);
    }
  } else {
    el.stopBtn.classList.add('hidden');
    el.terminalWrap.classList.add('hidden');
    el.transcriptWrap.classList.remove('hidden');
    el.transcriptWrap.classList.add('inactive-only');
    if (el.transcriptHeading) {
      el.transcriptHeading.textContent = 'Transcript';
    }
    detachSessionTerminal();
    setTerminalWide(false);
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

function renderDashboard() {
  const d = state.dashboard;
  if (!d) {
    el.dashboardStats.innerHTML = '<p class="muted">Loading…</p>';
    return;
  }

  const taskCounts = d.taskCounts || {};
  const taskParts = Object.entries(taskCounts)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([status, count]) => `<span class="task-count"><strong>${escapeHtml(status)}</strong> ${count}</span>`)
    .join('');

  const natsClass = d.nats?.ok ? 'ok' : (d.nats?.configured ? 'warn' : 'muted');
  el.dashboardStats.innerHTML = `
    <div class="stat-card">
      <span class="stat-label">Runtime</span>
      <span class="stat-value badge ${badgeClass(d.runtime?.status)}">${escapeHtml(d.runtime?.status || 'unknown')}</span>
    </div>
    <div class="stat-card">
      <span class="stat-label">NATS</span>
      <span class="stat-value badge ${natsClass}">${d.nats?.ok ? 'ok' : (d.nats?.configured ? 'degraded' : 'not configured')}</span>
    </div>
    <div class="stat-card">
      <span class="stat-label">Active sessions</span>
      <span class="stat-value">${d.activeSessions ?? 0}</span>
    </div>
    <div class="stat-card">
      <span class="stat-label">Active worktrees</span>
      <span class="stat-value">${d.activeWorktrees ?? 0}</span>
    </div>
    <div class="stat-card stat-wide">
      <span class="stat-label">Task counts</span>
      <div class="task-counts">${taskParts || '<span class="muted">none</span>'}</div>
    </div>
  `;

  renderDashboardList(el.dashboardTraces, d.recentTraces, (trace) => `
    <div class="top">
      <span class="bee">${escapeHtml(trace.traceId)}</span>
      <span class="badge ${trace.hasActive ? 'active' : (trace.hasFailures ? 'failed' : '')}">${trace.runCount} runs</span>
    </div>
    <div class="muted" style="font-size:0.8rem;margin-top:0.25rem">${formatTime(trace.lastActivityAt)} · ${trace.taskCount} tasks</div>
  `, 'No recent traces.', (trace) => {
    navigateToTrace(trace.traceId).catch(console.error);
  });

  renderDashboardList(el.dashboardFailedRuns, d.failedRuns, (run) => `
    <div class="top">
      <span class="bee">${escapeHtml(run.bee)}</span>
      <span class="badge failed">${escapeHtml(run.state)}</span>
    </div>
    <div class="id">${escapeHtml(run.traceId)} / ${escapeHtml(run.agentId)}</div>
  `, 'No failed runs.', (run) => {
    navigateToRun(run.traceId, run.agentId).catch(console.error);
  });

  renderDashboardList(el.dashboardInsights, d.recentInsights, (insight) => `
    <div class="top">
      <span class="bee">${escapeHtml(insight.payloadKind)}</span>
      <span class="muted" style="font-size:0.78rem">${formatTime(insight.createdAt)}</span>
    </div>
    <div>${escapeHtml(insight.summary)}</div>
    <div class="id">${escapeHtml(insight.traceId)}</div>
  `, 'No recent insights.', (insight) => {
    if (insight.agentId) {
      navigateToRun(insight.traceId, insight.agentId).catch(console.error);
    } else {
      navigateToTrace(insight.traceId).catch(console.error);
    }
  });
}

function renderDashboardList(container, items, renderItem, emptyText, onClick) {
  container.innerHTML = '';
  if (!items || !items.length) {
    const li = document.createElement('li');
    li.className = 'muted';
    li.textContent = emptyText;
    container.appendChild(li);
    return;
  }
  for (const item of items) {
    const li = document.createElement('li');
    li.className = 'session-item compact-item';
    li.innerHTML = renderItem(item);
    if (onClick) {
      li.addEventListener('click', () => onClick(item));
    }
    container.appendChild(li);
  }
}

async function navigateToRun(traceId, agentId) {
  if (!traceId || !agentId) return;
  setTab('runs');
  await loadRuns();
  await selectRun(traceId, agentId);
}

async function navigateToTrace(traceId) {
  if (!traceId) return;
  setTab('traces');
  await loadTraces();
  await selectTrace(traceId);
}

function renderTraces() {
  el.traceList.innerHTML = '';
  if (!state.traces.length) {
    const li = document.createElement('li');
    li.className = 'muted';
    li.textContent = 'No traces yet.';
    el.traceList.appendChild(li);
    return;
  }
  for (const trace of state.traces) {
    const li = document.createElement('li');
    li.className = 'session-item';
    if (trace.traceId === state.selectedTraceId) {
      li.classList.add('selected');
    }
    const badge = trace.hasActive ? 'active' : (trace.hasFailures ? 'failed' : '');
    const badgeLabel = trace.hasActive ? 'active' : (trace.hasFailures ? 'failures' : `${trace.runCount} runs`);
    const bees = (trace.bees || []).join(', ') || '—';
    li.innerHTML = `
      <div class="top">
        <span class="bee">${escapeHtml(trace.traceId)}</span>
        <span class="badge ${badge}">${escapeHtml(badgeLabel)}</span>
      </div>
      <div class="muted" style="font-size:0.8rem;margin-top:0.25rem">
        ${formatTime(trace.lastActivityAt)} · ${trace.taskCount} tasks · ${escapeHtml(bees)}
      </div>
    `;
    li.addEventListener('click', () => {
      selectTrace(trace.traceId).catch(console.error);
    });
    el.traceList.appendChild(li);
  }
}

function renderTraceDetail(detail) {
  const hasSelection = !!state.selectedTraceId;
  el.traceDetailEmpty.classList.toggle('hidden', hasSelection || state.traceDetailLoading);
  el.traceDetailLoading.classList.toggle('hidden', !state.traceDetailLoading);
  el.traceOpenTimelineBtn.classList.toggle('hidden', !detail);

  if (state.traceDetailError) {
    el.traceDetailError.textContent = state.traceDetailError;
    el.traceDetailError.classList.remove('hidden');
  } else {
    el.traceDetailError.classList.add('hidden');
    el.traceDetailError.textContent = '';
  }

  if (!detail) {
    el.traceDetailBody.classList.add('hidden');
    return;
  }

  el.traceDetailBody.classList.remove('hidden');
  const bees = (detail.bees || []).join(', ') || '—';
  const flags = [];
  if (detail.hasActive) flags.push('active');
  if (detail.hasFailures) flags.push('failures');
  el.traceDetailMeta.innerHTML = `
    <dt>Trace</dt><dd>${escapeHtml(detail.traceId)}</dd>
    <dt>Last activity</dt><dd>${formatTime(detail.lastActivityAt)}</dd>
    <dt>Runs</dt><dd>${detail.runCount ?? (detail.runs || []).length}</dd>
    <dt>Tasks</dt><dd>${detail.taskCount ?? (detail.tasks || []).length}</dd>
    <dt>Bees</dt><dd>${escapeHtml(bees)}</dd>
    <dt>Flags</dt><dd>${flags.length ? escapeHtml(flags.join(', ')) : '—'}</dd>
  `;

  const hasEnergy = detail.energyBudget > 0 || detail.energyRemaining > 0;
  el.traceEnergyWrap.classList.toggle('hidden', !hasEnergy);
  if (hasEnergy) {
    const low = detail.lowEnergy ? ' <span class="badge warn">low</span>' : '';
    el.traceEnergy.innerHTML = `
      <span>${detail.energyRemaining} / ${detail.energyBudget} remaining</span>${low}
    `;
  }

  const wt = detail.worktree;
  el.traceWorktreeWrap.classList.toggle('hidden', !wt);
  if (wt) {
    el.traceWorktreeMeta.innerHTML = `
      <dt>Path</dt><dd><code>${escapeHtml(wt.path)}</code></dd>
      <dt>Branch</dt><dd>${escapeHtml(wt.branch || '—')}</dd>
      <dt>Base SHA</dt><dd><code>${escapeHtml(wt.baseSha || '—')}</code></dd>
      <dt>Created</dt><dd>${formatTime(wt.createdAt)}</dd>
    `;
  }

  renderTraceTasks(detail.tasks || []);
  renderTraceRuns(detail.runs || []);
  renderTraceEvents(detail.recentEvents || []);
}

function renderTraceTasks(tasks) {
  el.traceTasksList.innerHTML = '';
  if (!tasks.length) {
    const li = document.createElement('li');
    li.className = 'muted';
    li.textContent = 'No tasks in this trace.';
    el.traceTasksList.appendChild(li);
    return;
  }
  for (const task of tasks) {
    const li = document.createElement('li');
    li.className = 'session-item compact-item';
    li.innerHTML = `
      <div class="top">
        <span class="bee">${escapeHtml(task.title || task.taskId)}</span>
        <span class="badge ${badgeClass(task.status)}">${escapeHtml(task.status || '—')}</span>
      </div>
      <div class="id">${escapeHtml(task.taskId)}${task.bee ? ` · ${escapeHtml(task.bee)}` : ''}</div>
    `;
    li.addEventListener('click', () => {
      navigateToTask(state.selectedTraceId, task.taskId).catch(console.error);
    });
    el.traceTasksList.appendChild(li);
  }
}

function renderTraceRuns(runs) {
  el.traceRunsList.innerHTML = '';
  if (!runs.length) {
    const li = document.createElement('li');
    li.className = 'muted';
    li.textContent = 'No runs in this trace.';
    el.traceRunsList.appendChild(li);
    return;
  }
  for (const run of runs) {
    const li = document.createElement('li');
    li.className = 'session-item compact-item';
    li.innerHTML = `
      <div class="top">
        <span class="bee">${escapeHtml(run.bee || run.agentId)}</span>
        <span class="badge ${badgeClass(run.state)}">${escapeHtml(run.state || '—')}</span>
      </div>
      <div class="id">${escapeHtml(run.agentId)}${run.taskId ? ` · ${escapeHtml(run.taskId)}` : ''}</div>
      <div class="muted" style="font-size:0.78rem;margin-top:0.2rem">${formatTime(run.startedAt)}</div>
    `;
    li.addEventListener('click', () => {
      navigateToRun(run.traceId || state.selectedTraceId, run.agentId).catch(console.error);
    });
    el.traceRunsList.appendChild(li);
  }
}

function renderTraceEvents(events) {
  el.traceEventsList.innerHTML = '';
  if (!events.length) {
    const li = document.createElement('li');
    li.className = 'muted';
    li.textContent = 'No recent events.';
    el.traceEventsList.appendChild(li);
    return;
  }
  for (const item of events) {
    const li = document.createElement('li');
    li.className = 'timeline-item';
    const kind = item.payloadKind ? ` · ${item.payloadKind}` : '';
    li.innerHTML = `
      <div class="timeline-top">
        <span class="timeline-type">${escapeHtml(item.type)}${escapeHtml(kind)}</span>
        <span class="muted timeline-time">${formatTime(item.createdAt)}</span>
      </div>
      <div class="timeline-summary">${escapeHtml(item.summary || '')}</div>
      <div class="timeline-meta muted">${escapeHtml(item.agentId || '—')}${item.bee ? ` · ${escapeHtml(item.bee)}` : ''}</div>
    `;
    el.traceEventsList.appendChild(li);
  }
}

async function loadTraces() {
  state.traces = await api('/api/traces');
  renderTraces();
  if (state.selectedTraceId) {
    const still = state.traces.find((t) => t.traceId === state.selectedTraceId);
    if (still) {
      await selectTrace(state.selectedTraceId, { quiet: true });
    } else if (!state.selectedTraceDetail) {
      renderTraceDetail(null);
    }
  }
}

async function selectTrace(traceId, opts = {}) {
  if (!traceId) return;
  const switching = state.selectedTraceId !== traceId;
  state.selectedTraceId = traceId;
  state.traceDetailError = '';
  if (!opts.quiet) {
    state.traceDetailLoading = true;
    if (switching) {
      state.selectedTraceDetail = null;
    }
    renderTraces();
    renderTraceDetail(state.selectedTraceDetail);
  } else {
    renderTraces();
  }
  try {
    const detail = await api(`/api/traces/${encodeURIComponent(traceId)}`);
    if (state.selectedTraceId !== traceId) return;
    state.selectedTraceDetail = detail;
    state.traceDetailLoading = false;
    state.traceDetailError = '';
    renderTraceDetail(detail);
  } catch (err) {
    if (state.selectedTraceId !== traceId) return;
    state.traceDetailLoading = false;
    state.traceDetailError = err.message || String(err);
    state.selectedTraceDetail = null;
    renderTraceDetail(null);
  }
}

function stopTracesPolling() {
  if (state.tracesPollTimer) {
    clearInterval(state.tracesPollTimer);
    state.tracesPollTimer = null;
  }
}

function startTracesPolling() {
  stopTracesPolling();
  state.tracesPollTimer = setInterval(() => {
    loadTraces().catch(console.error);
  }, 5000);
  loadTraces().catch(console.error);
}

function renderTaskBoard() {
  const board = state.tasks;
  el.taskBoard.innerHTML = '';
  if (!board || !board.groups || !board.groups.length) {
    const empty = document.createElement('p');
    empty.className = 'muted';
    empty.textContent = 'No tasks yet. Create one to get started.';
    el.taskBoard.appendChild(empty);
    return;
  }

  for (const group of board.groups) {
    const section = document.createElement('section');
    section.className = 'task-group';
    section.innerHTML = `
      <div class="task-group-header">
        <span>${escapeHtml(group.status)}</span>
        <span class="muted">${group.tasks.length}</span>
      </div>
    `;
    const items = document.createElement('div');
    items.className = 'task-group-items';
    for (const task of group.tasks) {
      const card = document.createElement('div');
      const key = taskKey(task);
      card.className = 'task-card' + (key === state.selectedTaskKey ? ' selected' : '');
      const deps = task.dependsOn?.length ? `<span class="task-tag">deps: ${escapeHtml(task.dependsOn.join(', '))}</span>` : '';
      const sector = task.sector ? `<span class="task-tag">${escapeHtml(task.sector)}</span>` : '';
      const runs = task.runCount ? `<span class="task-tag">${task.runCount} runs</span>` : '';
      const startable = task.canStart ? '<span class="task-tag" style="color:var(--ok)">startable</span>' : '';
      card.innerHTML = `
        <div class="top">
          <span class="title">${escapeHtml(task.title)}</span>
          <span class="badge ${badgeClass(task.status)}">${escapeHtml(task.status)}</span>
        </div>
        <div class="meta-line">${escapeHtml(task.traceId)} / ${escapeHtml(task.taskId)}</div>
        <div class="badges">
          ${task.bee ? `<span class="task-tag">${escapeHtml(task.bee)}</span>` : ''}
          ${sector}${deps}${runs}${startable}
        </div>
      `;
      card.addEventListener('click', () => selectTask(task.traceId, task.taskId));
      items.appendChild(card);
    }
    section.appendChild(items);
    el.taskBoard.appendChild(section);
  }
}

function reviewKey(item) {
  return `${item.traceId}/${item.taskId}`;
}

function renderReviewQueue() {
  el.reviewQueueList.innerHTML = '';
  const items = state.reviews?.items || [];
  if (!items.length) {
    const li = document.createElement('li');
    li.className = 'muted';
    li.textContent = 'No proposals awaiting review.';
    el.reviewQueueList.appendChild(li);
    return;
  }
  for (const item of items) {
    const key = reviewKey(item);
    const li = document.createElement('li');
    li.className = 'session-item' + (key === state.selectedReviewKey ? ' selected' : '');
    const finalTag = item.isFinal ? '<span class="task-tag" style="color:var(--warn)">final gate</span>' : '';
    const reviewTag = item.review ? `<span class="task-tag">${escapeHtml(item.review)}</span>` : '';
    li.innerHTML = `
      <div class="top">
        <span class="bee">${escapeHtml(item.title)}</span>
        <span class="badge waiting_review">waiting_review</span>
      </div>
      <div class="id">${escapeHtml(item.traceId)} / ${escapeHtml(item.taskId)}</div>
      <div class="badges" style="margin-top:0.35rem">${reviewTag}${finalTag}</div>
      ${item.summary ? `<div class="muted" style="font-size:0.8rem;margin-top:0.35rem">${escapeHtml(item.summary)}</div>` : ''}
    `;
    li.addEventListener('click', () => selectReview(item.traceId, item.taskId));
    el.reviewQueueList.appendChild(li);
  }
}

function renderReviewDetail(item) {
  if (!item) {
    el.reviewDetailEmpty.classList.remove('hidden');
    el.reviewDetailMeta.classList.add('hidden');
    el.reviewSummaryWrap.classList.add('hidden');
    el.reviewActionsWrap.classList.add('hidden');
    return;
  }

  el.reviewDetailEmpty.classList.add('hidden');
  el.reviewDetailMeta.classList.remove('hidden');
  el.reviewActionError.classList.add('hidden');
  el.reviewActionSuccess.classList.add('hidden');

  const rows = [
    ['Review policy', item.review || '—'],
    ['Trace ID', item.traceId],
    ['Task ID', item.taskId],
    ['Bee', item.bee],
    ['Sector', item.sector],
    ['Runs', String(item.runCount ?? 0)],
    ['Updated', formatTime(item.updatedAt)],
  ];
  if (item.isFinal) {
    rows.unshift(['Gate', 'Final merge gate']);
  }
  el.reviewDetailMeta.innerHTML = rows
    .map(([k, v]) => `<dt>${escapeHtml(k)}</dt><dd>${escapeHtml(v || '—')}</dd>`)
    .join('');

  if (item.summary) {
    el.reviewSummaryWrap.classList.remove('hidden');
    el.reviewSummary.textContent = item.summary;
  } else {
    el.reviewSummaryWrap.classList.add('hidden');
    el.reviewSummary.textContent = '';
  }

  const canAct = item.canApprove && item.canReject;
  if (canAct) {
    el.reviewActionsWrap.classList.remove('hidden');
    el.reviewFinalHint.classList.toggle('hidden', !item.isFinal);
    el.reviewMergeMessageLabel.classList.toggle('hidden', !item.isFinal);
  } else {
    el.reviewActionsWrap.classList.add('hidden');
  }
}

function updateTaskReviewUI(task) {
  const canReview = task && task.canApprove && task.canReject;
  el.taskApproveBtn.classList.toggle('hidden', !canReview);
  el.taskRejectBtn.classList.toggle('hidden', !canReview);
  el.taskReviewActions.classList.toggle('hidden', !canReview);
  el.taskMergeMessageLabel.classList.toggle('hidden', !(canReview && task.isFinal));
  el.taskReviewError.classList.add('hidden');
}

function renderTaskDetail(task) {
  if (!task) {
    el.taskDetailEmpty.classList.remove('hidden');
    el.taskDetailMeta.classList.add('hidden');
    el.taskBodyWrap.classList.add('hidden');
    el.taskRunsWrap.classList.add('hidden');
    el.taskDetailActions.classList.add('hidden');
    el.taskStartBtn.classList.add('hidden');
    updateTaskReviewUI(null);
    return;
  }

  el.taskDetailEmpty.classList.add('hidden');
  el.taskDetailMeta.classList.remove('hidden');
  el.taskDetailActions.classList.remove('hidden');

  const rows = [
    ['Status', task.status],
    ['Review', task.review || 'none'],
    ['Trace ID', task.traceId],
    ['Task ID', task.taskId],
    ['Bee', task.bee],
    ['Sector', task.sector],
    ['Intent', task.intent],
    ['Source', task.source],
    ['Updated', formatTime(task.updatedAt)],
  ];
  if (task.dependsOn?.length) {
    rows.push(['Depends on', task.dependsOn.join(', ')]);
  }
  if (task.summary) {
    rows.push(['Summary', task.summary]);
  }
  if (task.commit) {
    rows.push(['Commit', task.commit]);
  }

  el.taskDetailMeta.innerHTML = rows
    .map(([k, v]) => `<dt>${escapeHtml(k)}</dt><dd>${escapeHtml(v || '—')}</dd>`)
    .join('');

  if (task.body) {
    el.taskBodyWrap.classList.remove('hidden');
    el.taskBody.textContent = task.body;
  } else {
    el.taskBodyWrap.classList.add('hidden');
    el.taskBody.textContent = '';
  }

  el.taskRunsList.innerHTML = '';
  if (task.runs?.length) {
    el.taskRunsWrap.classList.remove('hidden');
    for (const run of task.runs) {
      const li = document.createElement('li');
      li.className = 'session-item compact-item';
      li.innerHTML = `
        <div class="top">
          <span class="bee">${escapeHtml(run.agentId)}</span>
          <span class="badge ${badgeClass(run.runStatus)}">${escapeHtml(run.runStatus || 'unknown')}</span>
        </div>
        <div class="muted" style="font-size:0.8rem;margin-top:0.25rem">${formatTime(run.startedAt)}</div>
      `;
      li.addEventListener('click', () => navigateToRun(task.traceId, run.agentId));
      el.taskRunsList.appendChild(li);
    }
  } else {
    el.taskRunsWrap.classList.add('hidden');
  }

  if (task.canStart) {
    el.taskStartBtn.classList.remove('hidden');
  } else {
    el.taskStartBtn.classList.add('hidden');
  }
  updateTaskReviewUI(task);
}

async function loadReviews() {
  state.reviews = await api('/api/review-queue');
  renderReviewQueue();
  if (state.selectedReviewKey) {
    const [traceId, taskId] = state.selectedReviewKey.split('/');
    const item = state.reviews.items?.find((i) => i.traceId === traceId && i.taskId === taskId);
    state.selectedReviewDetail = item || null;
    renderReviewDetail(state.selectedReviewDetail);
  }
}

async function selectReview(traceId, taskId) {
  state.selectedReviewKey = `${traceId}/${taskId}`;
  renderReviewQueue();
  const item = state.reviews?.items?.find((i) => i.traceId === traceId && i.taskId === taskId);
  if (item) {
    state.selectedReviewDetail = item;
    renderReviewDetail(item);
    return;
  }
  const detail = await api(`/api/traces/${encodeURIComponent(traceId)}/tasks/${encodeURIComponent(taskId)}`);
  state.selectedReviewDetail = {
    traceId: detail.traceId,
    taskId: detail.taskId,
    title: detail.title,
    review: detail.review,
    summary: detail.summary,
    bee: detail.bee,
    sector: detail.sector,
    runCount: detail.runCount,
    updatedAt: detail.updatedAt,
    isFinal: detail.isFinal,
    canApprove: detail.canApprove,
    canReject: detail.canReject,
  };
  renderReviewDetail(state.selectedReviewDetail);
}

async function approveReview(traceId, taskId, { summary, mergeMessage }) {
  const body = {};
  if (summary) body.summary = summary;
  if (mergeMessage) body.mergeMessage = mergeMessage;
  return api(`/api/traces/${encodeURIComponent(traceId)}/tasks/${encodeURIComponent(taskId)}/approve`, {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

async function rejectReview(traceId, taskId, feedback) {
  const body = {};
  if (feedback) body.feedback = feedback;
  return api(`/api/traces/${encodeURIComponent(traceId)}/tasks/${encodeURIComponent(taskId)}/reject`, {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

function stopReviewsPolling() {
  if (state.reviewsPollTimer) {
    clearInterval(state.reviewsPollTimer);
    state.reviewsPollTimer = null;
  }
}

function startReviewsPolling() {
  stopReviewsPolling();
  state.reviewsPollTimer = setInterval(() => {
    loadReviews().catch(console.error);
  }, 5000);
  loadReviews().catch(console.error);
}

async function loadTasks() {
  state.tasks = await api('/api/tasks');
  renderTaskBoard();
  if (state.selectedTaskKey) {
    const [traceId, taskId] = state.selectedTaskKey.split('/');
    try {
      const detail = await api(`/api/traces/${encodeURIComponent(traceId)}/tasks/${encodeURIComponent(taskId)}`);
      state.selectedTaskDetail = detail;
      renderTaskDetail(detail);
    } catch (err) {
      console.error(err);
    }
  }
}

async function selectTask(traceId, taskId) {
  state.selectedTaskKey = `${traceId}/${taskId}`;
  renderTaskBoard();
  const detail = await api(`/api/traces/${encodeURIComponent(traceId)}/tasks/${encodeURIComponent(taskId)}`);
  state.selectedTaskDetail = detail;
  renderTaskDetail(detail);
}

async function createTaskFromForm() {
  el.taskCreateError.classList.add('hidden');
  el.taskCreateBtn.disabled = true;
  try {
    const dependsRaw = el.taskDependsInput.value.trim();
    const body = {
      title: el.taskTitleInput.value.trim(),
      body: el.taskBodyInput.value.trim(),
      bee: el.taskBeeInput.value,
      traceId: el.taskTraceInput.value.trim(),
      sector: el.taskSectorInput.value.trim(),
      intent: el.taskIntentSelect.value,
      review: el.taskReviewSelect.value,
      dependsOn: dependsRaw ? dependsRaw.split(',').map((s) => s.trim()).filter(Boolean) : [],
      autorun: el.taskAutorunToggle.checked,
    };
    const created = await api('/api/tasks', { method: 'POST', body: JSON.stringify(body) });
    await loadTasks();
    await selectTask(created.traceId, created.taskId);
    el.taskTitleInput.value = '';
    el.taskBodyInput.value = '';
    el.taskTraceInput.value = '';
    el.taskDependsInput.value = '';
    el.taskAutorunToggle.checked = false;
  } catch (err) {
    el.taskCreateError.textContent = err.message;
    el.taskCreateError.classList.remove('hidden');
  } finally {
    el.taskCreateBtn.disabled = false;
  }
}

async function startSelectedTask() {
  if (!state.selectedTaskDetail) return;
  const { traceId, taskId } = state.selectedTaskDetail;
  el.taskStartBtn.disabled = true;
  try {
    await api(`/api/traces/${encodeURIComponent(traceId)}/tasks/${encodeURIComponent(taskId)}/start`, { method: 'POST' });
    await loadTasks();
    await selectTask(traceId, taskId);
  } catch (err) {
    alert(err.message);
  } finally {
    el.taskStartBtn.disabled = false;
  }
}

function stopTasksPolling() {
  if (state.tasksPollTimer) {
    clearInterval(state.tasksPollTimer);
    state.tasksPollTimer = null;
  }
}

function startTasksPolling() {
  stopTasksPolling();
  state.tasksPollTimer = setInterval(() => {
    loadTasks().catch(console.error);
  }, 5000);
  loadTasks().catch(console.error);
}

async function navigateToTask(traceId, taskId) {
  if (!traceId || !taskId) return;
  setTab('tasks');
  await loadTasks();
  await selectTask(traceId, taskId);
}

function navigateToTaskTimeline(traceId, taskId) {
  if (!traceId) return;
  // Apply filters before setTab so its automatic loadTimeline uses them.
  el.filterTrace.value = traceId;
  el.filterTask.value = taskId || '';
  readTimelineFiltersFromForm();
  setTab('timeline');
}

function timelineQueryParams(appendCursor) {
  const f = state.timelineFilters;
  const params = new URLSearchParams();
  if (f.traceId) params.set('traceId', f.traceId);
  if (f.taskId) params.set('taskId', f.taskId);
  if (f.bee) params.set('bee', f.bee);
  if (f.type) params.set('type', f.type);
  if (f.kind) params.set('kind', f.kind);
  if (f.severity) params.set('severity', f.severity);
  params.set('limit', '50');
  if (appendCursor && state.timelineCursor) {
    params.set('after', state.timelineCursor);
  }
  return params.toString();
}

function renderTimeline() {
  el.timelineFeed.innerHTML = '';
  if (!state.timelineItems.length) {
    const li = document.createElement('li');
    li.className = 'muted';
    li.textContent = 'No events match the current filters.';
    el.timelineFeed.appendChild(li);
    return;
  }
  for (const item of state.timelineItems) {
    const li = document.createElement('li');
    li.className = 'timeline-item';
    const kind = item.payloadKind ? ` · ${item.payloadKind}` : '';
    const severity = item.severity ? ` · ${item.severity}` : '';
    const bee = item.bee ? ` · ${item.bee}` : '';
    li.innerHTML = `
      <div class="timeline-top">
        <span class="timeline-type">${escapeHtml(item.type)}${escapeHtml(kind)}</span>
        <span class="muted timeline-time">${formatTime(item.createdAt)}</span>
      </div>
      <div class="timeline-summary">${escapeHtml(item.summary)}</div>
      <div class="timeline-meta muted">${escapeHtml(item.traceId)} / ${escapeHtml(item.agentId)}${escapeHtml(bee)}${escapeHtml(severity)}</div>
      ${state.timelineShowRaw ? `<pre class="timeline-raw">${escapeHtml(JSON.stringify(item.raw, null, 2))}</pre>` : ''}
    `;
    el.timelineFeed.appendChild(li);
  }
  el.timelineMoreBtn.classList.toggle('hidden', !state.timelineHasMore);
}

async function loadDashboard() {
  state.dashboard = await api('/api/dashboard');
  renderDashboard();
}

async function loadTimeline(reset) {
  if (reset) {
    state.timelineItems = [];
    state.timelineCursor = '';
    state.timelineHasMore = false;
  }
  const qs = timelineQueryParams(!reset);
  const page = await api(`/api/events?${qs}`);
  if (reset) {
    state.timelineItems = page.items || [];
  } else {
    state.timelineItems.push(...(page.items || []));
  }
  state.timelineCursor = page.nextCursor || '';
  state.timelineHasMore = !!page.hasMore;
  renderTimeline();
}

function readTimelineFiltersFromForm() {
  state.timelineFilters = {
    traceId: el.filterTrace.value.trim(),
    taskId: el.filterTask.value.trim(),
    bee: el.filterBee.value.trim(),
    type: el.filterType.value,
    kind: el.filterKind.value.trim(),
    severity: el.filterSeverity.value.trim(),
  };
}

function stopDashboardPolling() {
  if (state.dashboardPollTimer) {
    clearInterval(state.dashboardPollTimer);
    state.dashboardPollTimer = null;
  }
}

function startDashboardPolling() {
  stopDashboardPolling();
  state.dashboardPollTimer = setInterval(() => {
    loadDashboard().catch(console.error);
  }, 5000);
  loadDashboard().catch(console.error);
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

async function pollSessionState() {
  if (!state.selectedId) return;
  try {
    const session = await api(`/api/sessions/${encodeURIComponent(state.selectedId)}`);
    const prevActive = state.sessions.find((s) => s.sessionId === state.selectedId)?.active;
    await loadSessions();
    renderSessionDetail(session);
    if (prevActive && !session.active) {
      state.transcriptCursor = 0;
      state.transcriptLines = [];
      await pollTranscript();
    }
  } catch (err) {
    console.error(err);
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
  const session = state.sessions.find((s) => s.sessionId === state.selectedId);
  if (session && session.active) {
    state.pollTimer = setInterval(pollSessionState, 2000);
    pollSessionState();
  } else {
    state.pollTimer = setInterval(pollTranscript, 1500);
    pollTranscript();
  }
}

function startRunPolling() {
  stopPolling();
  state.pollTimer = setInterval(pollRunEvents, 1500);
  pollRunEvents();
}

el.tabDashboard.addEventListener('click', () => setTab('dashboard'));
el.tabTraces.addEventListener('click', () => setTab('traces'));
el.tabTimeline.addEventListener('click', () => setTab('timeline'));
el.tabTasks.addEventListener('click', () => setTab('tasks'));
el.tabReviews.addEventListener('click', () => setTab('reviews'));
el.tabSessions.addEventListener('click', () => setTab('sessions'));
el.tabRuns.addEventListener('click', () => {
  setTab('runs');
  loadRuns().catch(console.error);
});

el.dashboardRefreshBtn.addEventListener('click', () => {
  loadDashboard().catch(console.error);
});

el.tracesRefreshBtn.addEventListener('click', () => {
  loadTraces().catch(console.error);
});

el.traceOpenTimelineBtn.addEventListener('click', () => {
  if (!state.selectedTraceId) return;
  navigateToTaskTimeline(state.selectedTraceId, null);
});

el.timelineRefreshBtn.addEventListener('click', () => {
  loadTimeline(true).catch(console.error);
});

el.timelineFilters.addEventListener('submit', (ev) => {
  ev.preventDefault();
  readTimelineFiltersFromForm();
  loadTimeline(true).catch(console.error);
});

el.timelineRawToggle.addEventListener('change', () => {
  state.timelineShowRaw = el.timelineRawToggle.checked;
  renderTimeline();
});

el.timelineMoreBtn.addEventListener('click', () => {
  loadTimeline(false).catch(console.error);
});

el.taskCreateForm.addEventListener('submit', (ev) => {
  ev.preventDefault();
  createTaskFromForm().catch(console.error);
});

el.tasksRefreshBtn.addEventListener('click', () => {
  loadTasks().catch(console.error);
});

el.taskStartBtn.addEventListener('click', () => {
  startSelectedTask().catch(console.error);
});

el.taskOpenTimelineBtn.addEventListener('click', () => {
  if (!state.selectedTaskDetail) return;
  navigateToTaskTimeline(state.selectedTaskDetail.traceId, state.selectedTaskDetail.taskId);
});

el.taskApproveBtn.addEventListener('click', () => {
  if (!state.selectedTaskDetail) return;
  el.taskReviewActions.classList.remove('hidden');
  el.taskReviewActions.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
});

el.taskRejectBtn.addEventListener('click', () => {
  if (!state.selectedTaskDetail) return;
  el.taskReviewActions.classList.remove('hidden');
  el.taskRejectFeedback.focus();
  el.taskReviewActions.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
});

el.taskApproveForm.addEventListener('submit', async (ev) => {
  ev.preventDefault();
  if (!state.selectedTaskDetail) return;
  const { traceId, taskId } = state.selectedTaskDetail;
  el.taskReviewError.classList.add('hidden');
  try {
    const res = await approveReview(traceId, taskId, {
      summary: el.taskApproveSummary.value.trim(),
      mergeMessage: el.taskMergeMessage.value.trim(),
    });
    await loadTasks();
    await loadReviews();
    alert(res.message || 'Task approved.');
    state.selectedTaskKey = `${traceId}/${taskId}`;
    await selectTask(traceId, taskId);
  } catch (err) {
    el.taskReviewError.textContent = err.message;
    el.taskReviewError.classList.remove('hidden');
  }
});

el.taskRejectForm.addEventListener('submit', async (ev) => {
  ev.preventDefault();
  if (!state.selectedTaskDetail) return;
  const { traceId, taskId } = state.selectedTaskDetail;
  el.taskReviewError.classList.add('hidden');
  try {
    const res = await rejectReview(traceId, taskId, el.taskRejectFeedback.value.trim());
    await loadTasks();
    await loadReviews();
    alert(res.message || 'Feedback published.');
    state.selectedTaskKey = `${traceId}/${taskId}`;
    await selectTask(traceId, taskId);
  } catch (err) {
    el.taskReviewError.textContent = err.message;
    el.taskReviewError.classList.remove('hidden');
  }
});

el.reviewsRefreshBtn.addEventListener('click', () => {
  loadReviews().catch(console.error);
});

function showReviewActionSuccess(message) {
  el.reviewActionError.classList.add('hidden');
  el.reviewActionSuccess.textContent = message;
  el.reviewActionSuccess.classList.remove('hidden');
}

el.reviewApproveForm.addEventListener('submit', async (ev) => {
  ev.preventDefault();
  if (!state.selectedReviewDetail) return;
  const { traceId, taskId } = state.selectedReviewDetail;
  el.reviewActionError.classList.add('hidden');
  el.reviewActionSuccess.classList.add('hidden');
  try {
    const res = await approveReview(traceId, taskId, {
      summary: el.reviewApproveSummary.value.trim(),
      mergeMessage: el.reviewMergeMessage.value.trim(),
    });
    const message = res.commitSha
      ? `${res.message} Commit: ${res.commitSha}`
      : (res.message || 'Task approved.');
    state.selectedReviewKey = null;
    state.selectedReviewDetail = null;
    renderReviewDetail(null);
    showReviewActionSuccess(message);
    await loadReviews();
    await loadTasks();
  } catch (err) {
    el.reviewActionError.textContent = err.message;
    el.reviewActionError.classList.remove('hidden');
  }
});

el.reviewRejectForm.addEventListener('submit', async (ev) => {
  ev.preventDefault();
  if (!state.selectedReviewDetail) return;
  const { traceId, taskId } = state.selectedReviewDetail;
  el.reviewActionError.classList.add('hidden');
  el.reviewActionSuccess.classList.add('hidden');
  try {
    const res = await rejectReview(traceId, taskId, el.reviewRejectFeedback.value.trim());
    state.selectedReviewKey = null;
    state.selectedReviewDetail = null;
    renderReviewDetail(null);
    showReviewActionSuccess(res.message || 'Feedback published.');
    await loadReviews();
    await loadTasks();
  } catch (err) {
    el.reviewActionError.textContent = err.message;
    el.reviewActionError.classList.remove('hidden');
  }
});

el.reviewOpenTimelineBtn.addEventListener('click', () => {
  if (!state.selectedReviewDetail) return;
  navigateToTaskTimeline(state.selectedReviewDetail.traceId, state.selectedReviewDetail.taskId);
});

el.reviewOpenRunsBtn.addEventListener('click', () => {
  if (!state.selectedReviewDetail) return;
  setTab('tasks');
  selectTask(state.selectedReviewDetail.traceId, state.selectedReviewDetail.taskId).catch(console.error);
});

el.beeSelect.addEventListener('change', syncSessionIntents);
el.taskBeeInput.addEventListener('change', syncTaskIntents);

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

el.terminalWideBtn?.addEventListener('click', () => {
  setTerminalWide(!state.terminalWide);
});

el.runsRefreshBtn.addEventListener('click', () => {
  loadRuns().catch(console.error);
});

el.stopBtn.addEventListener('click', async () => {
  if (!state.selectedId) return;
  el.stopBtn.disabled = true;
  try {
    detachSessionTerminal();
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
    setTab('dashboard');
  } catch (err) {
    console.error(err);
  }
}

init();
