import {
	addTraceEnergy as requestEnergy,
	getTrace as requestTrace,
	listTraceArtifacts as requestArtifacts
} from '$lib/api/client';
import type { ArtifactView, EnergyAddResult, TraceDetail } from '$lib/api/types';

interface TraceStoreOptions {
	loadTrace?: (traceId: string) => Promise<TraceDetail>;
	loadArtifacts?: (traceId: string) => Promise<ArtifactView[]>;
	topUpEnergy?: (traceId: string, amount: number) => Promise<EnergyAddResult>;
	pollIntervalMs?: number;
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

/**
 * One trace behind `/next/traces/:id`. The projection carries the summary, the
 * tasks, the runs, the worktree, and the last 20 events in one payload, so the
 * route makes a single request; the comb is a second endpoint and fails
 * independently — a trace with an unreadable comb still inspects its work.
 */
export function createTraceStore(options: TraceStoreOptions = {}) {
	const readTrace = options.loadTrace ?? requestTrace;
	const readArtifacts = options.loadArtifacts ?? requestArtifacts;
	const addEnergy = options.topUpEnergy ?? requestEnergy;
	const pollIntervalMs = options.pollIntervalMs ?? 10000;

	let traceId = $state('');
	let detail = $state<TraceDetail | null>(null);
	let artifacts = $state<ArtifactView[]>([]);
	let loading = $state(true);
	let error = $state('');
	let artifactsError = $state('');
	let energyError = $state('');
	let topUpPending = $state(false);
	let timer: ReturnType<typeof setInterval> | undefined;
	let started = false;

	/** A top-up answers with the recomputed reserve, so patch it in place rather than refetching. */
	function applyEnergy(result: EnergyAddResult): void {
		if (!detail || detail.traceId !== result.traceId) return;
		detail = {
			...detail,
			energyBudget: result.energyBudget,
			energyRemaining: result.energyRemaining,
			energyAdded: result.energyAdded,
			energyAllocated: result.energyAllocated,
			lowEnergy: result.lowEnergy
		};
	}

	async function refresh(): Promise<void> {
		const id = traceId;
		if (id === '') return;
		let view: TraceDetail;
		try {
			view = await readTrace(id);
		} catch (cause) {
			if (traceId !== id) return;
			error = errorMessage(cause);
			loading = false;
			return;
		}
		// A slow read for a trail the operator already left must not overwrite the current one.
		if (traceId !== id) return;
		detail = view;
		error = '';
		loading = false;
	}

	async function loadArtifacts(id: string): Promise<void> {
		let view: ArtifactView[];
		try {
			view = await readArtifacts(id);
		} catch (cause) {
			if (traceId !== id) return;
			artifacts = [];
			artifactsError = errorMessage(cause);
			return;
		}
		if (traceId !== id) return;
		artifacts = view;
		artifactsError = '';
	}

	/** Switching trails drops the previous payload so a stale one is never shown. */
	async function load(id: string): Promise<void> {
		if (id === traceId && detail !== null) return;
		traceId = id;
		detail = null;
		artifacts = [];
		error = '';
		artifactsError = '';
		energyError = '';
		loading = true;
		await Promise.all([refresh(), loadArtifacts(id)]);
	}

	async function topUp(amount: number): Promise<boolean> {
		if (topUpPending || traceId === '') return false;
		topUpPending = true;
		energyError = '';
		try {
			applyEnergy(await addEnergy(traceId, amount));
			return true;
		} catch (cause) {
			energyError = errorMessage(cause);
			return false;
		} finally {
			topUpPending = false;
		}
	}

	function start(): void {
		if (started) return;
		started = true;
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
		get traceId(): string {
			return traceId;
		},
		get detail(): TraceDetail | null {
			return detail;
		},
		get artifacts(): ArtifactView[] {
			return artifacts;
		},
		get loading(): boolean {
			return loading;
		},
		/** Skeletons only before the first payload; a later failure keeps the detail and shows the alert. */
		get showSkeletons(): boolean {
			return loading && detail === null;
		},
		get lastError(): string {
			return error;
		},
		/** The comb is a separate endpoint, so its failure is reported apart from the trail's. */
		get artifactsError(): string {
			return artifactsError;
		},
		get energyError(): string {
			return energyError;
		},
		get topUpPending(): boolean {
			return topUpPending;
		},
		load,
		refresh,
		topUp,
		start,
		stop
	};
}

export type TraceStore = ReturnType<typeof createTraceStore>;
