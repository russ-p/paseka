import { describe, expect, it, vi } from 'vitest';
import { createSettingsStore } from './settings.svelte';
import { colonyConfig } from '../../tests/fixtures';

vi.mock('$lib/api/client', () => ({ getConfig: vi.fn() }));

describe('createSettingsStore', () => {
	it('reads once on mount and never polls, because config changes on a file edit', async () => {
		const loadConfig = vi.fn(async () => colonyConfig());
		const store = createSettingsStore({ loadConfig });

		expect(store.showSkeletons).toBe(true);
		await store.start();
		await store.start();

		// A timer here would re-read identical values forever: the URL, the profile,
		// and the adapter binaries all change when someone edits a file. The one
		// value that does move under a running process — whether NATS is connected —
		// arrives on the chrome stream the topbar already keeps.
		expect(loadConfig).toHaveBeenCalledTimes(1);
		expect(store.showSkeletons).toBe(false);
		expect(store.config?.slug).toBe('paseka');
	});

	it('keeps the snapshot on screen through a later failure', async () => {
		const loadConfig = vi
			.fn<() => Promise<ReturnType<typeof colonyConfig>>>()
			.mockResolvedValueOnce(colonyConfig())
			.mockRejectedValueOnce(new Error('config: permission denied'));
		const store = createSettingsStore({ loadConfig });

		await store.start();
		await store.refresh();

		expect(store.lastError).toBe('config: permission denied');
		// The values an operator is reading are still the last real ones, and the
		// alert says they stopped moving — blanking the page would be a worse lie.
		expect(store.config?.slug).toBe('paseka');
	});

	it('clears a failure on the next success', async () => {
		const loadConfig = vi
			.fn<() => Promise<ReturnType<typeof colonyConfig>>>()
			.mockRejectedValueOnce(new Error('config: read failed'))
			.mockResolvedValueOnce(colonyConfig());
		const store = createSettingsStore({ loadConfig });

		await store.start();
		expect(store.lastError).not.toBe('');

		await store.refresh();
		expect(store.lastError).toBe('');
		expect(store.config).not.toBeNull();
	});

	it('re-reads on demand and not while idle', async () => {
		const loadConfig = vi.fn(async () => colonyConfig());
		const store = createSettingsStore({ loadConfig });

		await store.start();
		await store.refresh();
		store.stop();
		await store.refresh();

		// Refresh is the honest control here, so it has to work; nothing else does.
		expect(loadConfig).toHaveBeenCalledTimes(3);
	});

	it('answers an absent adapter list as empty rather than undefined', async () => {
		const store = createSettingsStore({ loadConfig: vi.fn(async () => colonyConfig({ adapters: undefined as never })) });

		await store.start();

		expect(store.adapters).toEqual([]);
	});

	it('distinguishes an unread colony from one that is simply not configured', async () => {
		const failing = createSettingsStore({ loadConfig: vi.fn(async () => Promise.reject(new Error('boom'))) });
		// start() returns void and fires the read, so the second call is what awaits
		// the first one's rejection.
		await failing.start();
		await failing.refresh();
		expect(failing.config).toBeNull();
		expect(failing.lastError).toBe('boom');

		const unset = createSettingsStore({
			loadConfig: vi.fn(async () => colonyConfig({ nats: { url: { value: '', source: 'unset' }, subjectPrefix: { value: 'paseka.paseka', source: 'default' } } }))
		});
		await unset.start();
		expect(unset.lastError).toBe('');
		expect(unset.config?.nats.url.source).toBe('unset');
	});
});
