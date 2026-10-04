import type {
	ApproveTaskRequest,
	AcceptInviteResult,
	ApproveTaskResult,
	ArtifactContent,
	ArtifactView,
	Bee,
	BeeScope,
	BuildView,
	CreateSessionRequest,
	CreateTaskRequest,
	CreateTaskResult,
	Cue,
	ColonyConfig,
	DashboardSummary,
	EnergyAddResult,
	EventFeedPage,
	EventFilters,
	GitActionResult,
	GitView,
	Invite,
	MergeDiff,
	RejectTaskRequest,
	RejectTaskResult,
	RetryTaskResult,
	ReviewQueue,
	RunBeeRequest,
	RunBeeResult,
	RunCueResult,
	RunEventsPage,
	RunSummary,
	RuntimeStatus,
	Session,
	StartTaskResult,
	StopSessionResult,
	SystemView,
	TaskBoard,
	TaskDetail,
	Topology,
	TraceDetail,
	TraceSummary,
	TranscriptPage
} from '$lib/api/types';

/**
 * Every console endpoint is root-relative: the `/next` prefix belongs to
 * frontend routes and assets only, never to the API.
 */
const apiRoot = '/api';

export class ApiError extends Error {
	constructor(
		message: string,
		readonly status: number
	) {
		super(message);
		this.name = 'ApiError';
	}
}

/**
 * Go's `http.Error` writes a plain-text reason, and the domain errors
 * (`honey reserve not configured`, `before must start with an RFC3339
 * timestamp`) are the whole point of a 400 or a 503 — reporting only the status
 * would throw that away.
 */
async function failureMessage(response: Response, status: number): Promise<string> {
	try {
		const body = (await response.text()).trim();
		if (body !== '') return body.split('\n')[0];
	} catch {
		// A body we cannot read must not mask the status.
	}
	return `request failed: ${status}`;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	const response = await fetch(`${apiRoot}${path}`, init);
	if (!response.ok) throw new ApiError(await failureMessage(response, response.status), response.status);
	if (response.status === 204) return undefined as T;
	return (await response.json()) as T;
}

export function fetchDashboard(): Promise<DashboardSummary> {
	return request<DashboardSummary>('/dashboard');
}

export function startRuntime(): Promise<RuntimeStatus> {
	return request<RuntimeStatus>('/runtime/start', { method: 'POST' });
}

export function stopRuntime(): Promise<RuntimeStatus> {
	return request<RuntimeStatus>('/runtime/stop', { method: 'POST' });
}

export function listCues(): Promise<Cue[]> {
	return request<Cue[]>('/cues');
}

export function runCue(
	cueId: string,
	body: { text: string; traceId?: string }
): Promise<RunCueResult> {
	return request<RunCueResult>(`/cues/${encodeURIComponent(cueId)}/run`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ text: body.text, traceId: body.traceId ?? '', vars: {} })
	});
}

/**
 * One page of the trail history, newest first. `before` is the cursor the client
 * reads off the last row of the page it holds, so the answer is anchored to a
 * trail rather than to a rank — which is what keeps a page stable while the colony
 * keeps producing new ones. The order is total (activity time, trace id breaking
 * ties), so a boundary walked twice cannot skip or repeat a trail.
 */
export function listTraces(page: { limit?: number; before?: string } = {}): Promise<TraceSummary[]> {
	const query = new URLSearchParams();
	if (page.limit !== undefined) query.set('limit', String(page.limit));
	if (page.before) query.set('before', page.before);
	const suffix = query.size > 0 ? `?${query}` : '';
	return request<TraceSummary[]>(`/traces${suffix}`);
}

/** The cursor that resumes strictly after `trace`; mirrors `hiveview.TraceCursorFor`. */
export function traceCursor(trace: TraceSummary): string {
	return `${trace.lastActivityAt}|${trace.traceId}`;
}

export function getTrace(traceId: string): Promise<TraceDetail> {
	return request<TraceDetail>(`/traces/${encodeURIComponent(traceId)}`);
}

export function listTraceArtifacts(traceId: string): Promise<ArtifactView[]> {
	return request<ArtifactView[]>(`/traces/${encodeURIComponent(traceId)}/artifacts`);
}

/** `ref` selects the content mode of the artifacts endpoint; the list is the no-`ref` form. */
export function getTraceArtifactContent(traceId: string, ref: string): Promise<ArtifactContent> {
	const query = new URLSearchParams({ ref });
	return request<ArtifactContent>(`/traces/${encodeURIComponent(traceId)}/artifacts?${query}`);
}

export function addTraceEnergy(traceId: string, amount: number): Promise<EnergyAddResult> {
	return request<EnergyAddResult>(`/traces/${encodeURIComponent(traceId)}/energy/add`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ amount })
	});
}

/**
 * The colony clone against origin, without contacting the remote: the checked
 * out branch, its divergence from the last fetch, the unpublished commits, the
 * colony-managed worktrees, and the local branches.
 */
export function getGit(): Promise<GitView> {
	return request<GitView>('/git');
}

/** Updates remote-tracking refs only; the working tree is untouched. */
export function gitFetch(): Promise<GitActionResult> {
	return request<GitActionResult>('/git/fetch', { method: 'POST' });
}

/** Publishes the default branch. The server never passes `--force`. */
export function gitPush(runHooks: boolean): Promise<GitActionResult> {
	return request<GitActionResult>('/git/push', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ runHooks })
	});
}

/**
 * Fast-forward only, and a backup for when the inbound webhook sidecar did not
 * update this clone. The server refuses it while live bees sit on the colony
 * root, so a refusal is a 409 carrying the reason.
 */
export function gitPull(): Promise<GitActionResult> {
	return request<GitActionResult>('/git/pull', { method: 'POST' });
}

/** One request for every name; the answer reports each name separately. */
export function gitDeleteBranches(names: string[]): Promise<GitActionResult> {
	return request<GitActionResult>('/git/branches/delete', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ names })
	});
}

/** Drops checkouts under `.paseka/worktrees` that no trail claims any more. */
export function gitPruneWorktrees(): Promise<GitActionResult> {
	return request<GitActionResult>('/git/worktrees/prune', { method: 'POST' });
}

/**
 * Which Paseka is answering. It is a build fact rather than a colony one, so it
 * changes only when the binary does — the console fetches it once and reads the
 * store, and a report that has to name a build takes it from here.
 */
export function getVersion(): Promise<BuildView> {
	return request<BuildView>('/version');
}

/**
 * The observe-only host snapshot behind `GET /api/system`: the same identity
 * and metrics the topbar's Host plaque carries, plus the capped process list
 * the plaque leaves out. The server answers 200 with an `error` string when
 * part of the snapshot failed, so the page shows what it got and says what it
 * did not get.
 */
export function getSystem(): Promise<SystemView> {
	return request<SystemView>('/system');
}

/** One page of the event feed. The server defaults to 50 rows and caps at 200. */
const eventFeedPageLimit = 50;

/**
 * The colony-wide event feed, or one trail's when `filters.traceId` is set. An
 * `after` cursor pages strictly older, so `loadMore` appends without repeating
 * or skipping an event.
 */
export function listEvents(filters: EventFilters = {}, after?: string): Promise<EventFeedPage> {
	const query = new URLSearchParams();
	// An empty filter is not a filter: the server matches a blank field against
	// everything, but sending it would make the request say something it does not mean.
	if (filters.traceId) query.set('traceId', filters.traceId);
	if (filters.taskId) query.set('taskId', filters.taskId);
	if (filters.bee) query.set('bee', filters.bee);
	if (filters.type) query.set('type', filters.type);
	if (filters.kind) query.set('kind', filters.kind);
	if (filters.severity) query.set('severity', filters.severity);
	query.set('limit', String(eventFeedPageLimit));
	if (after) query.set('after', after);
	return request<EventFeedPage>(`/events?${query.toString()}`);
}

/**
 * The colony's effective configuration: NATS, the profile, every adapter's
 * machine-local settings, the Telegram gate's push modes, and the attach
 * terminal — each value carrying the source that decides it.
 *
 * Credentials are references, not values: an adapter arrives as the name of the
 * variable it reads plus whether that variable resolves, and the gate as
 * `botTokenSet`. The key itself is never sent, because the credential model
 * stores a reference and reads the environment at call time.
 *
 * This is configuration, not liveness. Whether NATS is connected belongs to the
 * topbar's chrome stream and the one dashboard poll, so this page reads that
 * store rather than asking the server a second time.
 */
export function getConfig(): Promise<ColonyConfig> {
	return request<ColonyConfig>('/config');
}

/**
 * The config-derived EDA topology: bees, the event kinds their rules reference,
 * the edges between them, and the same graph as Mermaid. Built from bee YAML and
 * colony `auto_invites` on the filesystem, so it answers with NATS and the hive
 * runtime down.
 */
export function getTopology(): Promise<Topology> {
	return request<Topology>('/colony/topology');
}

/** Recent headless adapter runs, newest first. Each row carries the whole run view. */
export function listRuns(): Promise<RunSummary[]> {
	return request<RunSummary[]>('/runs');
}

/** One run by its trail and agent ids, for a run too old to be in the recent list. */
export function getRun(traceId: string, agentId: string): Promise<RunSummary> {
	return request<RunSummary>(`/runs/${encodeURIComponent(traceId)}/${encodeURIComponent(agentId)}`);
}

/**
 * The events a run recorded, oldest first. `after` is an index into that sequence
 * rather than a trace cursor, so paging is append-only and cannot skip a repeat.
 */
export function listRunEvents(
	traceId: string,
	agentId: string,
	after?: number
): Promise<RunEventsPage> {
	const suffix = after === undefined ? '' : `?after=${after}`;
	return request<RunEventsPage>(
		`/runs/${encodeURIComponent(traceId)}/${encodeURIComponent(agentId)}/events${suffix}`
	);
}

/**
 * The colony-wide task board. One bodiless read, grouped by status in lifecycle
 * order by the server: the order encodes the pipeline, so the client does not
 * re-sort it.
 */
export function listTasks(): Promise<TaskBoard> {
	return request<TaskBoard>('/tasks');
}

/** A task is addressed by both ids, so a task id alone is not a URL. */
export function getTask(traceId: string, taskId: string): Promise<TaskDetail> {
	return request<TaskDetail>(`/traces/${encodeURIComponent(traceId)}/tasks/${encodeURIComponent(taskId)}`);
}

function post<T>(path: string, body?: unknown): Promise<T> {
	return request<T>(path, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(body ?? {})
	});
}

export function createTask(task: CreateTaskRequest): Promise<CreateTaskResult> {
	return post<CreateTaskResult>('/tasks', task);
}

export function startTask(traceId: string, taskId: string): Promise<StartTaskResult> {
	return post<StartTaskResult>(
		`/traces/${encodeURIComponent(traceId)}/tasks/${encodeURIComponent(taskId)}/start`
	);
}

export function retryTask(traceId: string, taskId: string): Promise<RetryTaskResult> {
	return post<RetryTaskResult>(
		`/traces/${encodeURIComponent(traceId)}/tasks/${encodeURIComponent(taskId)}/retry`
	);
}

export function approveTask(
	traceId: string,
	taskId: string,
	review: ApproveTaskRequest
): Promise<ApproveTaskResult> {
	return post<ApproveTaskResult>(
		`/traces/${encodeURIComponent(traceId)}/tasks/${encodeURIComponent(taskId)}/approve`,
		review
	);
}

export function rejectTask(
	traceId: string,
	taskId: string,
	review: RejectTaskRequest
): Promise<RejectTaskResult> {
	return post<RejectTaskResult>(
		`/traces/${encodeURIComponent(traceId)}/tasks/${encodeURIComponent(taskId)}/reject`,
		review
	);
}

/**
 * The tasks waiting on a human, colony-wide. `count` is the server's own figure
 * and can exceed the items it chose to send, so it is what a tab badge shows.
 */
export function listReviews(): Promise<ReviewQueue> {
	return request<ReviewQueue>('/review-queue');
}

/**
 * The accumulated worktree diff for a trail's merge gate. Trace-scoped rather than
 * task-scoped: the branch is the trail's, and several final-gate tasks can look at
 * the same one.
 */
export function getMergeDiff(traceId: string): Promise<MergeDiff> {
	return request<MergeDiff>(`/traces/${encodeURIComponent(traceId)}/merge-diff`);
}

/**
 * The interactive bees: role, adapter, and the intents its prompt templates
 * declare. The launch and create forms build their intent list from this, so a
 * bee that adds an intent needs no console change.
 *
 * The default scope is the launch picker, which is why a script bee never
 * appears here: it cannot be started as a session, so offering it would be a
 * dead end in a form. `colony` is the whole roster, script bees included, and is
 * what the Bees route reads.
 */
export function listBees(scope?: BeeScope): Promise<Bee[]> {
	if (scope === undefined) return request<Bee[]>('/bees');
	return request<Bee[]>(`/bees?scope=${encodeURIComponent(scope)}`);
}

/**
 * One headless run of one bee, the way `paseka bee run` does it: the console
 * dispatches the adapter itself rather than publishing a signal for a runtime
 * that may not be running. The answer is the trail the run joins, not its output
 * — a run lasts as long as the agent takes, and the Runs list is where it is
 * watched. Only the fields the form filled are sent, because a present-but-empty
 * trace id is not the same as an absent one, where absent means "generate one".
 */
export function runBee(role: string, body: RunBeeRequest): Promise<RunBeeResult> {
	return post<RunBeeResult>(`/bees/${encodeURIComponent(role)}/run`, body);
}

/**
 * Every session the colony knows about, newest activity first: the live registry,
 * this process's own sessions, and a scan of `.paseka/runs`. One request rather
 * than a list plus a detail per row, because a run's identity is already on it.
 */
export function listSessions(): Promise<Session[]> {
	return request<Session[]>('/sessions');
}

export function getSession(sessionId: string): Promise<Session> {
	return request<Session>(`/sessions/${encodeURIComponent(sessionId)}`);
}

/** A slice of the transcript from `after`, which is an entry index. */
export function getSessionTranscript(sessionId: string, after: number): Promise<TranscriptPage> {
	return request<TranscriptPage>(
		`/sessions/${encodeURIComponent(sessionId)}/transcript?after=${after}`
	);
}

/**
 * Start a session. The form sends only the fields the operator filled: the server
 * treats a present-but-blank trace id differently from an absent one, where absent
 * means "generate one".
 */
export function createSession(session: CreateSessionRequest): Promise<Session> {
	return post<Session>('/sessions', session);
}

export function stopSession(sessionId: string): Promise<StopSessionResult> {
	return post<StopSessionResult>(`/sessions/${encodeURIComponent(sessionId)}/stop`, {});
}

/** Continue a finished session in a *new* one; the result is the new session. */
export function resumeSession(sessionId: string, body: string): Promise<Session> {
	return post<Session>(`/sessions/${encodeURIComponent(sessionId)}/resume`, { body });
}

/** Pending invites, which are tasks waiting for a human to start them. */
export function listInvites(): Promise<Invite[]> {
	return request<Invite[]>('/invites');
}

/** Starts a session when the invite names one, so the caller can go straight to it. */
export function acceptInvite(inviteId: string): Promise<AcceptInviteResult> {
	return post<AcceptInviteResult>(`/invites/${encodeURIComponent(inviteId)}/accept`, {});
}

export function rejectInvite(inviteId: string): Promise<Invite> {
	return post<Invite>(`/invites/${encodeURIComponent(inviteId)}/reject`, {});
}
