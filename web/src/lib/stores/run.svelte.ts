import { getRun as requestRun, listRunEvents, listRuns } from '$lib/api/client';
import type { ProtocolEvent, RunSummary } from '$lib/api/types';

interface RunStoreOptions {
	listRuns?: () => Promise<RunSummary[]>;
	getRun?: (traceId: string, agentId: string) => Promise<RunSummary>;
	listRunEvents?: (traceId: string, agentId: string, after?: number) => Promise<{
		entries: ProtocolEvent[];
		nextCursor: number;
	}>;
	/** A run's state moves while an operator watches it, so this page does poll. */
	pollIntervalMs?: number;
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

export interface RunPosition {
	/** Where this run sits among its trail's, newest first; `-1` when unknown. */
	index: number;
	total: number;
	/** The run that started before this one, or `null` at the start of the trail. */
	previous: RunSummary | null;
	/** The run that started after this one, or `null` at the newest. */
	next: RunSummary | null;
}

export function createRunStore(options: RunStoreOptions = {}) {
	const readRuns = options.listRuns ?? listRuns;
	const readRun = options.getRun ?? requestRun;
	const readEvents = options.listRunEvents ?? listRunEvents;
	const pollIntervalMs = options.pollIntervalMs ?? 5000;

	let runs = $state<RunSummary[]>([]);
	let selectedTraceId = $state('');
	let selectedAgentId = $state('');
	let loading = $state(true);
	let error = $state('');
	/** Set when a deep link names a run too old to be in the recent list. */
	let detached = $state<RunSummary | null>(null);
	let events = $state<ProtocolEvent[]>([]);
	let eventsLoading = $state(false);
	let eventsError = $state('');
	let timer: ReturnType<typeof setInterval> | undefined;
	let started = false;

	async function refresh(): Promise<void> {
		try {
			runs = await readRuns();
			error = '';
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			loading = false;
		}
	}

	/** The newest first, so the trail's own order is start time descending. */
	function siblings(traceId: string): RunSummary[] {
		return runs
			.filter((run) => run.traceId === traceId)
			.sort((a, b) => Date.parse(b.startedAt) - Date.parse(a.startedAt));
	}

	/**
	 * Plain functions rather than `$derived`, matching every other store here: a
	 * derived outside an effect can hand back a value computed before the last write,
	 * and a run page that showed the previous run's events would be worse than one
	 * that recomputes.
	 */
	function current(): RunSummary | null {
		if (
			detached &&
			detached.agentId === selectedAgentId &&
			detached.traceId === selectedTraceId
		) {
			return detached;
		}
		return (
			runs.find((run) => run.traceId === selectedTraceId && run.agentId === selectedAgentId) ??
			null
		);
	}

	/**
	 * Where this run sits among its trail's runs. Neighbours are by start time, so
	 * "the previous run" means the one that started before this one — which is how an
	 * operator reads a trail, rather than a list position that a fresh run shifts.
	 */
	function position(): RunPosition {
		const run = current();
		if (!run) return { index: -1, total: 0, previous: null, next: null };
		const list = siblings(run.traceId);
		const index = list.findIndex((entry) => entry.agentId === run.agentId);
		if (index < 0) return { index: -1, total: list.length, previous: null, next: null };
		return {
			index,
			total: list.length,
			// The list is newest first, so the older run is the one after it.
			previous: list[index + 1] ?? null,
			next: list[index - 1] ?? null
		};
	}

	async function loadEvents(traceId: string, agentId: string): Promise<void> {
		eventsLoading = true;
		eventsError = '';
		try {
			const page = await readEvents(traceId, agentId);
			// The endpoint returns everything from its cursor with no server-side
			// cap, so one read is the whole log. `nextCursor` is an index for a
			// future capped read, not a promise that more exist, so there is no
			// paging control here to press.
			events = page.entries;
		} catch (cause) {
			eventsError = errorMessage(cause);
		} finally {
			eventsLoading = false;
		}
	}

	/** Point the store at one run, reading its events once. */
	async function select(traceId: string, agentId: string): Promise<void> {
		if (!traceId || !agentId) return;
		selectedTraceId = traceId;
		selectedAgentId = agentId;
		events = [];
		await loadEvents(traceId, agentId);
		// A deep link can name a run older than the recent window, so the detail is
		// fetched directly rather than left blank.
		if (!runs.some((run) => run.traceId === traceId && run.agentId === agentId)) {
			try {
				detached = await readRun(traceId, agentId);
			} catch (cause) {
				detached = null;
				error = errorMessage(cause);
			}
		} else {
			detached = null;
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
		get runs(): RunSummary[] {
			return runs;
		},
		get current(): RunSummary | null {
			return current();
		},
		get selected(): boolean {
			return selectedTraceId !== '' && selectedAgentId !== '';
		},
		get position(): RunPosition {
			return position();
		},
		get events(): ProtocolEvent[] {
			return events;
		},
		get showSkeletons(): boolean {
			return loading && runs.length === 0;
		},
		get lastError(): string {
			return error;
		},
		get eventsLoading(): boolean {
			return eventsLoading;
		},
		get eventsError(): string {
			return eventsError;
		},
		refresh,
		select,
		start,
		stop
	};
}

export type RunStore = ReturnType<typeof createRunStore>;
