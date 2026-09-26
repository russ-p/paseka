import { describe, expect, it, vi } from 'vitest';
import { createTraceStore } from './trace.svelte';
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
				throw new Error('request failed: 404');
			},
			loadArtifacts: async () => [],
			pollIntervalMs: 0
		});

		await store.load('trace-missing');

		expect(store.lastError).toBe('request failed: 404');
		expect(store.detail).toBeNull();
		expect(store.showSkeletons).toBe(false);
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
