import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import Header from './Header.svelte';
import { createConsoleStatusStore } from '$lib/stores/console-status.svelte';
import { createToastStore } from '$lib/stores/toast.svelte';
import type { ChromeFrame } from '$lib/api/types';
import { formatClock } from '$lib/format';
import { dashboardSummary, traceSummary } from '../../tests/fixtures';

function chromeFrame(overrides: Partial<ChromeFrame> = {}): ChromeFrame {
	return {
		schemaVersion: 1,
		runtime: {
			status: 'running',
			alive: true,
			slug: 'demo',
			pid: 42,
			startedAt: '2026-09-25T10:00:00Z',
			lastHeartbeatAt: '2026-09-25T10:00:00Z'
		},
		agents: { count: 2, afk: 1, sessions: 1 },
		attention: { reviews: 3, sessions: 1 },
		host: {
			os: 'linux',
			arch: 'amd64',
			cpus: 16,
			hostname: 'thinkpad',
			load1: 1.25,
			cpuPercent: 13,
			memUsedBytes: 12_768_492_032,
			memTotalBytes: 33_404_239_232
		},
		git: {
			branch: 'paseka/trace-1',
			headSha: 'ba7b43c0d1',
			headShaShort: 'ba7b43c',
			dirty: false,
			defaultBranch: 'main',
			originUrl: 'git@github.com:example/paseka.git',
			ahead: 0,
			behind: 0,
			lastFetchAgeSeconds: 3600
		},
		...overrides
	};
}

function harness(frame: ChromeFrame = chromeFrame()) {
	const start = vi.fn(async () => ({ status: 'running', alive: true, slug: 'demo', pid: 42 }));
	const stop = vi.fn(async () => ({ status: 'stopped', alive: false, slug: 'demo' }));
	const store = createConsoleStatusStore({ runtime: { start, stop }, pollIntervalMs: 0 });
	store.applyChromeFrame(frame);
	store.applyDashboard(
		dashboardSummary({ recentTraces: [traceSummary({ traceId: 'trace-active', hasActive: true })] })
	);
	const toasts = createToastStore(0);
	return { store, toasts, start, stop };
}

function visibleLine(root: HTMLElement, text: string): HTMLElement | undefined {
	return Array.from(root.querySelectorAll('p')).find((node) => node.textContent?.trim() === text);
}

describe('Header', () => {
	it('renders a loading shell before status arrives', () => {
		const { container } = render(Header, {
			store: createConsoleStatusStore({ pollIntervalMs: 0 }),
			toasts: createToastStore(0)
		});

		expect(container.querySelectorAll('.skeleton').length).toBeGreaterThan(0);
		expect(screen.getByRole('status')).toHaveTextContent('Reconnecting to the console event stream.');
	});

	it('renders live stream and dashboard status', () => {
		const { store, toasts } = harness();

		const { container } = render(Header, { store, toasts });
		expect(visibleLine(container, 'demo')).toBeInTheDocument();
		expect(screen.getByRole('img', { name: 'NATS connected' })).toBeInTheDocument();
		expect(screen.getByText('Reviews 3')).toBeInTheDocument();
		expect(screen.getByText('Invites 1')).toBeInTheDocument();
	});

	it('renders the legacy topbar panels from the chrome frame', () => {
		const { store, toasts } = harness();
		render(Header, { store, toasts });

		expect(screen.getByRole('button', { name: 'Stop hive runtime' })).toBeInTheDocument();
		expect(screen.getByLabelText('Hive runtime')).toHaveTextContent('running');
		expect(screen.getByLabelText('Hive runtime')).toHaveTextContent('pid 42');
		expect(screen.getByLabelText('Live bees')).toHaveTextContent('2 live · 1 afk · 1 session');
		expect(screen.getByLabelText('Host')).toHaveTextContent('13%');
		expect(screen.getByLabelText('Host')).toHaveTextContent('11.9 / 31.1 GiB');
		expect(screen.getByLabelText('Git')).toHaveTextContent('in sync');
		expect(screen.getByLabelText('Git')).toHaveTextContent('main');

		expect(screen.getByLabelText('Host')).toHaveTextContent('load 1.25');
		expect(screen.getByLabelText('Git')).toHaveTextContent('ba7b43c');
	});

	it('links the Host and Git plaques to their own routes', () => {
		// Legacy parity: the plaque is the way into the page it summarizes. The label
		// is the link rather than the whole panel, because a stretched overlay would
		// swallow the hover popover underneath it.
		const { store, toasts } = harness();
		render(Header, { store, toasts });

		expect(screen.getByRole('link', { name: 'Host' })).toHaveAttribute('href', '/next/system');
		expect(screen.getByRole('link', { name: 'Git' })).toHaveAttribute('href', '/next/git');
		// Live bees points at Runs and Sessions, which are still placeheld.
		expect(screen.queryByRole('link', { name: 'Live bees' })).not.toBeInTheDocument();
	});

	it('keeps the full text of every clipped line in a multi-line hint', () => {
		const { store, toasts } = harness();
		render(Header, { store, toasts });

		// hints are portaled to body, so read them in document order
		const hints = Array.from(document.querySelectorAll('[id^="paseka-hint-"]')).map((node) =>
			Array.from(node.querySelectorAll('span')).map((line) => line.textContent?.trim())
		);
		const at = formatClock('2026-09-25T10:00:00Z');

		expect(hints).toEqual([
			['demo'],
			['running'],
			[`heartbeat ${at}`, 'pid 42', `started ${at}`],
			['2 live · 1 afk · 1 session'],
			['thinkpad', 'linux amd64 · 16 cpu', '11.9 / 31.1 GiB', 'load 1.25'],
			['thinkpad', 'load 1.25'],
			['main', 'origin git@github.com:example/paseka'],
			['ba7b43c', 'fetch 1h ago']
		]);

		// the tooltip is a visual duplicate, so it stays out of the a11y tree
		expect(document.querySelectorAll('[id^="paseka-hint-"][aria-hidden="true"]').length).toBe(8);
		expect(visibleLine(document.body, `heartbeat ${at} · pid 42 · started ${at}`)).toBeInTheDocument();
	});

	it('shows and hides a hint on hover', async () => {
		const user = userEvent.setup();
		const { store, toasts } = harness();
		render(Header, { store, toasts });

		const line = visibleLine(document.body, 'running');
		const wrapper = line?.closest<HTMLElement>('[data-hint]');
		const tip = wrapper ? document.getElementById(wrapper.dataset.hint ?? '') : null;
		if (!line || !tip) throw new Error('hint anchor or content missing');

		expect(tip).toHaveClass('opacity-0');
		await user.hover(line);
		expect(tip).toHaveClass('opacity-100');
		await user.unhover(line);
		expect(tip).toHaveClass('opacity-0');
	});

	it('shows one action per runtime state', () => {
		const running = harness();
		const { unmount } = render(Header, { store: running.store, toasts: running.toasts });
		expect(screen.getByRole('button', { name: 'Stop hive runtime' })).toBeEnabled();
		unmount();

		const stopped = harness(chromeFrame({ runtime: { status: 'stopped', alive: false, slug: 'demo' } }));
		const { unmount: unmountStopped } = render(Header, { store: stopped.store, toasts: stopped.toasts });
		expect(screen.getByRole('button', { name: 'Start hive runtime' })).toBeEnabled();
		unmountStopped();

		const stale = harness(chromeFrame({ runtime: { status: 'stale', alive: false, slug: 'demo' } }));
		const { container } = render(Header, { store: stale.store, toasts: stale.toasts });
		expect(screen.getByRole('button', { name: 'Start hive runtime' })).toBeEnabled();
		expect(visibleLine(container, 'stale · registry entry, start respawns')).toBeInTheDocument();
	});

	it('blocks the action while the runtime is shutting down', () => {
		const stopping = harness(chromeFrame({ runtime: { status: 'stopping', alive: true, slug: 'demo' } }));
		const { container } = render(Header, { store: stopping.store, toasts: stopping.toasts });

		expect(screen.getByRole('button', { name: 'Stopping hive runtime' })).toBeDisabled();
		expect(visibleLine(container, 'stopping · shutting down')).toBeInTheDocument();
	});

	it('asks which action to take when the status is unclear', async () => {
		const user = userEvent.setup();
		const { store, toasts, start, stop } = harness(
			chromeFrame({ runtime: { status: 'degraded', alive: true, slug: 'demo' } })
		);
		render(Header, { store, toasts });

		await user.click(screen.getByRole('button', { name: /choose an action/ }));

		const dialog = screen.getByRole('dialog');
		expect(dialog).toHaveTextContent('Hive runtime state is unclear');
		expect(start).not.toHaveBeenCalled();
		expect(stop).not.toHaveBeenCalled();

		await user.click(screen.getByRole('button', { name: 'Start' }));
		await waitFor(() => expect(start).toHaveBeenCalledTimes(1));
	});

	it('routes the unclear state to the stop confirmation', async () => {
		const user = userEvent.setup();
		const { store, toasts, stop } = harness(
			chromeFrame({ runtime: { status: 'degraded', alive: true, slug: 'demo' } })
		);
		render(Header, { store, toasts });

		await user.click(screen.getByRole('button', { name: /choose an action/ }));
		await user.click(screen.getByRole('button', { name: 'Stop…' }));

		expect(screen.getByRole('dialog')).toHaveTextContent('Stop the hive runtime?');
		expect(stop).not.toHaveBeenCalled();

		await user.click(screen.getByRole('button', { name: 'Stop runtime' }));
		await waitFor(() => expect(stop).toHaveBeenCalledTimes(1));
	});

	it('starts the runtime and reports the result', async () => {
		const user = userEvent.setup();
		const { store, toasts, start } = harness(chromeFrame({ runtime: { status: 'stopped', alive: false } }));
		render(Header, { store, toasts });

		await user.click(screen.getByRole('button', { name: 'Start hive runtime' }));

		await waitFor(() => expect(start).toHaveBeenCalledTimes(1));
		expect(toasts.items).toHaveLength(1);
		expect(toasts.items[0]).toMatchObject({ tone: 'success', message: 'Hive runtime started' });
	});

	it('confirms before stopping and cancels on Escape', async () => {
		const user = userEvent.setup();
		const { store, toasts, stop } = harness();
		render(Header, { store, toasts });

		await user.click(screen.getByRole('button', { name: 'Stop hive runtime' }));

		const dialog = screen.getByRole('dialog');
		expect(dialog).toHaveTextContent('Stop the hive runtime?');
		expect(stop).not.toHaveBeenCalled();

		await user.keyboard('{Escape}');
		await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
		expect(stop).not.toHaveBeenCalled();
		expect(toasts.items).toHaveLength(0);
	});

	it('stops the runtime after confirmation', async () => {
		const user = userEvent.setup();
		const { store, toasts, stop } = harness();
		render(Header, { store, toasts });

		await user.click(screen.getByRole('button', { name: 'Stop hive runtime' }));
		await user.click(screen.getByRole('button', { name: 'Stop runtime' }));

		await waitFor(() => expect(stop).toHaveBeenCalledTimes(1));
		await waitFor(() => expect(toasts.items[0]).toMatchObject({ message: 'Hive runtime stopped' }));
		expect(store.runtimeStatus).toBe('stopped');
	});

	it('surfaces runtime failures as an error alert and toast', async () => {
		const user = userEvent.setup();
		const store = createConsoleStatusStore({
			runtime: {
				start: async () => {
					throw new Error('runtime start failed: 500');
				}
			},
			pollIntervalMs: 0
		});
		store.applyChromeFrame(chromeFrame({ runtime: { status: 'stopped', alive: false } }));
		const toasts = createToastStore(0);
		render(Header, { store, toasts });

		await user.click(screen.getByRole('button', { name: 'Start hive runtime' }));

		await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('runtime start failed: 500'));
		expect(toasts.items[0]).toMatchObject({ tone: 'error' });
	});
});
