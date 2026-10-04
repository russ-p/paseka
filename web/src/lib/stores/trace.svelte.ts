import {
	addTraceEnergy as requestEnergy,
	ApiError,
	getTrace as requestTrace,
	listTraceArtifacts as requestArtifacts
} from '$lib/api/client';
import type { ArtifactView, EnergyAddResult, TraceDetail } from '$lib/api/types';

interface TraceStoreOptions {
	loadTrace?: (traceId: string) => Promise<TraceDetail>;
	loadArtifacts?: (traceId: string) => Promise<ArtifactView[]>;
	topUpEnergy?: (traceId: string, amount: number) => Promise<EnergyAddResult>;
	pollIntervalMs?: number;
	/** How long a 404 may read as "cued, not yet picked up" before it is a failure. */
	awaitingTrailMs?: number;
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

/**
 * A 404 here is not always "no such trail". Publishing a cue mints a trace id and puts a
 * SIGNAL on the bus, and **nothing is written to disk** until a bee picks that signal up
 * and starts writing — so a trail the operator just created by cueing is a 404 for as
 * long as the colony takes to answer. Saying "not found" would be telling the operator
 * their own action failed, and refusing to keep looking would be correct-looking and
 * wrong. So the page waits, and says so.
 *
 * Not forever, though. A wait with no end is how a mistyped id becomes a spinner that
 * never resolves, and how a cue nobody picked up — a real problem — hides behind the same
 * silence. Both converge on the same dead end, so the wait is bounded and the message
 * then names both: the id is wrong, or no bee took the cue.
 */
function isNotYetStarted(error: unknown): boolean {
	return error instanceof ApiError && error.status === 404;
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
	const awaitingTrailMs = options.awaitingTrailMs ?? 30000;

	let traceId = $state('');
	let detail = $state<TraceDetail | null>(null);
	let artifacts = $state<ArtifactView[]>([]);
	let loading = $state(true);
	let error = $state('');
	let artifactsError = $state('');
	let energyError = $state('');
	let topUpPending = $state(false);
	let pendingTrail = $state(false);
	/** When the first 404 of this wait arrived; 0 while nothing is being waited on. */
	let awaitingSince = 0;
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
			if (!detail && isNotYetStarted(cause)) {
				// A trail the operator is already looking at never becomes "not started
				// again" because a poll landed between a mutation and its write.
				const now = Date.now();
				if (awaitingSince === 0) awaitingSince = now;
				pendingTrail = now - awaitingSince < awaitingTrailMs;
				error = pendingTrail
					? ''
					: `No trail with this id, and none appeared in the last ${Math.round(awaitingTrailMs / 1000)} seconds. Check the id, or whether anything picked it up.`;
				loading = false;
				return;
			}
			pendingTrail = false;
			awaitingSince = 0;
			error = errorMessage(cause);
			loading = false;
			return;
		}
		// A slow read for a trail the operator already left must not overwrite the current one.
		if (traceId !== id) return;
		detail = view;
		pendingTrail = false;
		awaitingSince = 0;
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
		pendingTrail = false;
		awaitingSince = 0;
		loading = true;
		await Promise.all([refresh(), loadArtifacts(id)]);
		// The comb 404s for the same reason the trail does, and the two reads race, so the
		// trail's verdict is what settles it. Left alone, the artifact failure would still be
		// on screen the moment the trail lands and the section becomes renderable.
		if (pendingTrail) {
			artifacts = [];
			artifactsError = '';
		}
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
		/**
		 * The trail exists on the bus but not yet on disk, so the page is waiting rather
		 * than reporting a failure. It clears the moment a poll brings the projection.
		 */
		get awaitingTrail(): boolean {
			return pendingTrail;
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
