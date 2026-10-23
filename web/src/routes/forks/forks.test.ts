import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Forks from './+page.svelte';
import { createForksStore } from '$lib/stores/forks.svelte';
import { createToastStore } from '$lib/stores/toast.svelte';
import { forkRequest, trailCollection, traceSummary } from '../../tests/fixtures';
import type { ForkDecision, TrailCollection } from '$lib/api/types';

afterEach(() => {
	vi.restoreAllMocks();
});

describe('forks route', () => {
	it('lists pending requests with evidence and experimental trails', async () => {
		const store = createForksStore({
			loadTrails: async () => trailCollection(),
			pollIntervalMs: 0
		});
		render(Forks, { store, toasts: createToastStore(0) });

		expect(await screen.findByRole('heading', { name: 'Forks', level: 1 })).toBeInTheDocument();
		expect(screen.getAllByText('Try the stricter guard').length).toBeGreaterThan(0);
		expect(screen.getByText('task-b2')).toBeInTheDocument();
		expect(screen.getByRole('link', { name: 'trace-01a0bd6963faa14f' })).toHaveAttribute(
			'href',
			'/next/traces/trace-01a0bd6963faa14f'
		);
		expect(screen.getByText('pinned')).toBeInTheDocument();
	});

	it('decides a request and reloads both queues', async () => {
		const user = userEvent.setup();
		const request = forkRequest();
		const loadTrails = vi
			.fn<() => Promise<TrailCollection>>()
			.mockResolvedValueOnce(trailCollection())
			.mockResolvedValueOnce(
				trailCollection({
					trails: [traceSummary({ traceId: 'trace-fork', title: 'Forked', forkOf: 'trace-source' })],
					pendingForkRequests: []
				})
			);
		const decideFork = vi.fn(
			async (): Promise<ForkDecision> => ({
				request: { ...request, state: 'rejected' }
			})
		);
		const store = createForksStore({ loadTrails, decideFork, pollIntervalMs: 0 });
		render(Forks, { store, toasts: createToastStore(0) });
		await waitFor(() => expect(screen.getAllByText('Try the stricter guard').length).toBeGreaterThan(0));

		await user.type(screen.getByPlaceholderText('Decision reason (recorded)'), 'not now');
		await user.click(screen.getByRole('button', { name: 'Reject' }));

		expect(decideFork).toHaveBeenCalledWith(request.requestId, 'reject', 'not now');
		await waitFor(() => expect(screen.queryByText(request.requestId)).not.toBeInTheDocument());
		expect(screen.getByText('No fork request is waiting on an operator.')).toBeInTheDocument();
		expect(screen.getByRole('link', { name: 'Forked' })).toHaveAttribute(
			'href',
			'/next/traces/trace-fork'
		);
	});
});
