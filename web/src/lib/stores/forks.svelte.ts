import {
	decideForkRequest,
	listTrails as requestTrails
} from '$lib/api/client';
import type { ForkDecision, ForkRequest, TrailCollection, TraceSummary } from '$lib/api/types';

interface ForksStoreOptions {
	loadTrails?: () => Promise<TrailCollection>;
	decideFork?: (
		requestId: string,
		action: 'approve' | 'reject' | 'cancel',
		reason: string
	) => Promise<ForkDecision>;
	pollIntervalMs?: number;
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

/**
 * The colony-wide experiment queue. It reads the shared trail collection, so a
 * runnable fork and a pending request are never the same row: the collection
 * separates them by construction.
 */
export function createForksStore(options: ForksStoreOptions = {}) {
	const readTrails = options.loadTrails ?? requestTrails;
	const sendDecision = options.decideFork ?? decideForkRequest;
	const pollIntervalMs = options.pollIntervalMs ?? 10000;

	let collection = $state<TrailCollection>({ trails: [], pendingForkRequests: [] });
	let loading = $state(true);
	let error = $state('');
	let actionError = $state('');
	let decisionPending = $state('');
	let timer: ReturnType<typeof setInterval> | undefined;
	let started = false;

	function forks(): TraceSummary[] {
		return (collection.trails ?? []).filter((trail) => Boolean(trail.forkOf || trail.forkId));
	}

	function requests(): ForkRequest[] {
		return collection.pendingForkRequests ?? [];
	}

	async function refresh(): Promise<void> {
		try {
			const loaded = await readTrails();
			collection = {
				trails: loaded.trails ?? [],
				pendingForkRequests: loaded.pendingForkRequests ?? []
			};
			error = '';
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			loading = false;
		}
	}

	async function decide(
		requestId: string,
		action: 'approve' | 'reject' | 'cancel',
		reason = ''
	): Promise<ForkDecision | null> {
		if (decisionPending !== '') return null;
		decisionPending = requestId;
		actionError = '';
		try {
			const decision = await sendDecision(requestId, action, reason);
			await refresh();
			return decision;
		} catch (cause) {
			actionError = errorMessage(cause);
			return null;
		} finally {
			decisionPending = '';
		}
	}

	function start(): void {
		if (started) return;
		started = true;
		void refresh();
		if (pollIntervalMs > 0) {
			timer = setInterval(() => void refresh(), pollIntervalMs);
		}
	}

	function stop(): void {
		started = false;
		if (timer !== undefined) clearInterval(timer);
		timer = undefined;
	}

	return {
		get forks(): TraceSummary[] {
			return forks();
		},
		get requests(): ForkRequest[] {
			return requests();
		},
		get loading(): boolean {
			return loading;
		},
		get lastError(): string {
			return error;
		},
		get actionError(): string {
			return actionError;
		},
		get decisionPending(): string {
			return decisionPending;
		},
		refresh,
		decide,
		start,
		stop
	};
}

export type ForksStore = ReturnType<typeof createForksStore>;
