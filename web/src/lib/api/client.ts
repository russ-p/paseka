import type {
	ArtifactContent,
	ArtifactView,
	Cue,
	DashboardSummary,
	EnergyAddResult,
	GitActionResult,
	GitView,
	RunCueResult,
	RuntimeStatus,
	TraceDetail,
	TraceSummary
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
 * One page of the trail history, newest first. `before` is the cursor the server
 * hands back through the last row of the previous page, so a page never repeats
 * or skips a trail even when two share an activity instant.
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
