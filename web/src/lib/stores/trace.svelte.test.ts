import { describe, expect, it, vi } from 'vitest';
import { createTraceStore } from './trace.svelte';
import { ApiError } from '$lib/api/client';
import { artifactView, traceDetail } from '../../tests/fixtures';
import type { ArtifactView, EnergyAddResult, TraceDetail } from '$lib/api/types';

const artifacts: ArtifactView[] = [artifactView(), artifactView({ ref: 'patch.diff', title: 'Patch' })];

function topUp(overrides: Partial<EnergyAddResult> = {}): EnergyAddResult {
	return {
		traceId: 'trace-01a0bd6963faa14f',
		amount: 5,
		energyBudget: 12,
		energyRemaining: 12,
		energyAdded: 5,
		energyAllocated: 12,
		lowEnergy: false,
		...overrides
	};
}

describe('createTraceStore', () => {
	it('reads the trail and its comb together on the first load', async () => {
		const loadTrace = vi.fn(async () => traceDetail());
		const loadArtifacts = vi.fn(async () => artifacts);
		const store = createTraceStore({ loadTrace, loadArtifacts, pollIntervalMs: 0 });

		expect(store.showSkeletons).toBe(true);
		await store.load('trace-01a0bd6963faa14f');

		expect(loadTrace).toHaveBeenCalledWith('trace-01a0bd6963faa14f');
		expect(loadArtifacts).toHaveBeenCalledWith('trace-01a0bd6963faa14f');
		expect(store.detail?.runCount).toBe(4);
		expect(store.artifacts).toHaveLength(2);
		expect(store.showSkeletons).toBe(false);
	});

	it('keeps an unreadable comb from hiding a readable trail', async () => {
		const store = createTraceStore({
			loadTrace: async () => traceDetail(),
			loadArtifacts: async () => {
				throw new Error('comb is gone');
			},
			pollIntervalMs: 0
		});

		await store.load('trace-01a0bd6963faa14f');

		expect(store.lastError).toBe('');
		expect(store.artifactsError).toBe('comb is gone');
		expect(store.artifacts).toEqual([]);
		expect(store.detail).not.toBeNull();
	});

	it('reports a trail that could not be read', async () => {
		const store = createTraceStore({
			loadTrace: async () => {
				throw new ApiError('nats url not configured', 503);
			},
			loadArtifacts: async () => [],
			pollIntervalMs: 0
		});

		await store.load('trace-missing');

		// Go's own reason, not just the status: `503` alone would throw away the sentence
		// that says what to fix.
		expect(store.lastError).toBe('nats url not configured');
		expect(store.awaitingTrail).toBe(false);
		expect(store.detail).toBeNull();
		expect(store.showSkeletons).toBe(false);
	});

	it('reports a failure that is not a status at all, rather than guessing it is a 404', async () => {
		const store = createTraceStore({
			loadTrace: async () => {
				throw new Error('request failed: 404');
			},
			loadArtifacts: async () => [],
			pollIntervalMs: 0
		});

		await store.load('trace-missing');

		// The message mentions 404 and the request never got far enough for one to mean
		// anything, so only the status may decide this.
		expect(store.lastError).toBe('request failed: 404');
		expect(store.awaitingTrail).toBe(false);
	});

	it('waits for a trail that has been cued but not picked up, because a 404 is not "no such trail"', async () => {
		const store = createTraceStore({
			loadTrace: async () => {
				throw new ApiError('trace not found', 404);
			},
			loadArtifacts: async () => [],
			pollIntervalMs: 0
		});

		await store.load('trace-cued-just-now');

		// Publishing a cue mints an id and writes nothing, so a trail the operator just
		// created is a 404 until a bee answers. Calling that a failure would be reporting
		// their own action as broken.
		expect(store.awaitingTrail).toBe(true);
		expect(store.lastError).toBe('');
		expect(store.detail).toBeNull();
		// Skeletons are for a payload on its way; nothing failed and there is nothing to
		// stand in for.
		expect(store.showSkeletons).toBe(false);
	});

	it('picks the trail up when a bee starts, and stops waiting without being asked', async () => {
		let answered = false;
		const store = createTraceStore({
			loadTrace: async () => {
				if (!answered) throw new ApiError('trace not found', 404);
				return traceDetail({ traceId: 'trace-cued-just-now' });
			},
			loadArtifacts: async () => artifacts,
			pollIntervalMs: 0
		});
		await store.load('trace-cued-just-now');
		expect(store.awaitingTrail).toBe(true);

		answered = true;
		await store.refresh();

		expect(store.awaitingTrail).toBe(false);
		expect(store.detail?.traceId).toBe('trace-cued-just-now');
		expect(store.lastError).toBe('');
	});

	it('treats a 404 after the trail is on screen as a real failure, because a deleted trail is gone', async () => {
		let deleted = false;
		const store = createTraceStore({
			loadTrace: async () => {
				if (deleted) throw new ApiError('trace not found', 404);
				return traceDetail();
			},
			loadArtifacts: async () => artifacts,
			pollIntervalMs: 0
		});
		await store.load('trace-01a0bd6963faa14f');

		deleted = true;
		await store.refresh();

		// "Not started yet" would be a lie about a trail that is already on screen, and
		// the operator would watch it wait forever.
		expect(store.awaitingTrail).toBe(false);
		expect(store.lastError).toBe('trace not found');
	});

	it('quiesces the comb failure while the trail is still only a cue', async () => {
		const store = createTraceStore({
			loadTrace: async () => {
				throw new ApiError('trace not found', 404);
			},
			// The comb 404s for the same reason, and the two reads race.
			loadArtifacts: async () => {
				throw new ApiError('trace not found', 404);
			},
			pollIntervalMs: 0
		});

		await store.load('trace-cued-just-now');

		// Left alone it would still be on screen the moment the trail landed and the
		// artifacts section became renderable.
		expect(store.awaitingTrail).toBe(true);
		expect(store.artifactsError).toBe('');
		expect(store.artifacts).toEqual([]);
	});

	it('stops waiting eventually, because a mistyped id is not a cue coming', async () => {
		vi.useFakeTimers();
		try {
			const store = createTraceStore({
				loadTrace: async () => {
					throw new ApiError('trace not found', 404);
				},
				loadArtifacts: async () => [],
				pollIntervalMs: 0,
				awaitingTrailMs: 30000
			});

			await store.load('trace-typo');
			expect(store.awaitingTrail).toBe(true);
			expect(store.lastError).toBe('');

			// The clock only moves when the console asks again, so the wait is judged on
			// reads rather than on a wall clock nothing is watching.
			await vi.advanceTimersByTimeAsync(20000);
			await store.refresh();
			expect(store.awaitingTrail).toBe(true);

			// A wait with no end is how a wrong id becomes a spinner that never resolves,
			// and how a cue nobody picked up hides behind the same silence. Both dead ends
			// converge here, so the message names both.
			await vi.advanceTimersByTimeAsync(15000);
			await store.refresh();
			expect(store.awaitingTrail).toBe(false);
			expect(store.lastError).toContain('No trail with this id');
			expect(store.lastError).toContain('none appeared in the last 30 seconds');
			expect(store.lastError).toContain('whether anything picked it up');
		} finally {
			vi.useRealTimers();
		}
	});

	it('starts the wait over for a trail the operator switches to', async () => {
		vi.useFakeTimers();
		try {
			const store = createTraceStore({
				loadTrace: async () => {
					throw new ApiError('trace not found', 404);
				},
				loadArtifacts: async () => [],
				pollIntervalMs: 0,
				awaitingTrailMs: 30000
			});

			await store.load('trace-one');
			await vi.advanceTimersByTimeAsync(40000);
			await store.refresh();
			expect(store.awaitingTrail).toBe(false);

			await store.load('trace-two');

			// The first trail's spent clock must not hand the second one a wait that is
			// already over, or a freshly cued trail would report itself dead on arrival.
			expect(store.awaitingTrail).toBe(true);
			expect(store.lastError).toBe('');
		} finally {
			vi.useRealTimers();
		}
	});

	it('drops the previous trail instead of showing a stale one', async () => {
		const store = createTraceStore({
			loadTrace: async (traceId) => traceDetail({ traceId, title: `Trail ${traceId}` }),
			loadArtifacts: async () => artifacts,
			pollIntervalMs: 0
		});

		await store.load('trail-a');
		await store.load('trail-b');

		expect(store.detail?.title).toBe('Trail trail-b');
		expect(store.traceId).toBe('trail-b');
	});

	it('ignores a repeat load of the trail already on screen', async () => {
		const loadTrace = vi.fn(async () => traceDetail());
		const store = createTraceStore({ loadTrace, loadArtifacts: async () => artifacts, pollIntervalMs: 0 });

		await store.load('trace-01a0bd6963faa14f');
		await store.load('trace-01a0bd6963faa14f');

		expect(loadTrace).toHaveBeenCalledTimes(1);
	});

	it('patches the reserve from the top-up answer instead of refetching', async () => {
		const loadTrace = vi.fn(async () => traceDetail());
		const topUpEnergy = vi.fn(async () => topUp());
		const store = createTraceStore({
			loadTrace,
			loadArtifacts: async () => artifacts,
			topUpEnergy,
			pollIntervalMs: 0
		});
		await store.load('trace-01a0bd6963faa14f');

		expect(await store.topUp(5)).toBe(true);

		expect(topUpEnergy).toHaveBeenCalledWith('trace-01a0bd6963faa14f', 5);
		expect(loadTrace).toHaveBeenCalledTimes(1);
		expect(store.detail?.energyRemaining).toBe(12);
		expect(store.detail?.energyAdded).toBe(5);
		expect(store.detail?.lowEnergy).toBe(false);
	});

	it('keeps a failed top-up on screen with the reason', async () => {
		const store = createTraceStore({
			loadTrace: async () => traceDetail(),
			loadArtifacts: async () => artifacts,
			topUpEnergy: async () => {
				throw new Error('honey reserve not configured');
			},
			pollIntervalMs: 0
		});
		await store.load('trace-01a0bd6963faa14f');

		expect(await store.topUp(5)).toBe(false);

		expect(store.energyError).toBe('honey reserve not configured');
		expect(store.detail?.energyRemaining).toBe(8);
	});

	it('refuses to fire two top-ups at once', async () => {
		let release: (() => void) | undefined;
		const gate = new Promise<void>((resolve) => (release = resolve));
		const topUpEnergy = vi.fn(async () => {
			await gate;
			return topUp();
		});
		const store = createTraceStore({
			loadTrace: async () => traceDetail(),
			loadArtifacts: async () => artifacts,
			topUpEnergy,
			pollIntervalMs: 0
		});
		await store.load('trace-01a0bd6963faa14f');

		const first = store.topUp(1);
		const second = await store.topUp(5);
		expect(second).toBe(false);
		release?.();
		await first;

		expect(topUpEnergy).toHaveBeenCalledTimes(1);
		expect(store.topUpPending).toBe(false);
	});

	it('does nothing when asked to top up before a trail is loaded', async () => {
		const topUpEnergy = vi.fn(async () => topUp());
		const store = createTraceStore({ topUpEnergy, pollIntervalMs: 0 });

		expect(await store.topUp(5)).toBe(false);
		expect(topUpEnergy).not.toHaveBeenCalled();
	});

	it('ignores a slow read for a trail the operator already moved off', async () => {
		let release: (() => void) | undefined;
		const gate = new Promise<void>((resolve) => (release = resolve));
		const loadTrace = vi.fn(async (traceId: string): Promise<TraceDetail> => {
			if (traceId === 'trail-slow') await gate;
			return traceDetail({ traceId, title: `Trail ${traceId}` });
		});
		const store = createTraceStore({ loadTrace, loadArtifacts: async () => artifacts, pollIntervalMs: 0 });

		const slow = store.load('trail-slow');
		await store.load('trail-fast');
		release?.();
		await slow;

		expect(store.detail?.traceId).toBe('trail-fast');
		expect(store.traceId).toBe('trail-fast');
	});
});
