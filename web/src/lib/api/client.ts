import type { Cue, DashboardSummary, RunCueResult, RuntimeStatus } from '$lib/api/types';

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

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	const response = await fetch(`${apiRoot}${path}`, init);
	if (!response.ok) throw new ApiError(`request failed: ${response.status}`, response.status);
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
