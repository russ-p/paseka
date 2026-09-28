import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Traces from './+page.svelte';
import { createTracesStore, type TracesStore } from '$lib/stores/traces.svelte';
import { traceSummary } from '../../tests/fixtures';
import type { TraceSummary } from '$lib/api/types';

function trails(count: number, offset = 0): TraceSummary[] {
	return Array.from({ length: count }, (_, index) =>
		traceSummary({
			traceId: `trace-${String(offset + index).padStart(2, '0')}`,
			title: `Trail ${offset + index}`,
			hasActive: false,
			hasFailures: false
		})
	);
}

function harness(store: TracesStore) {
	return { store };
}

afterEach(() => {
	vi.unstubAllGlobals();
	// The table publishes `?q=&page=` into the address bar, so one test's page would
	// otherwise seed the next table that mounts.
	window.history.replaceState(null, '', '/');
});

describe('traces route', () => {
	it('lists the trails newest first with a link into each detail page', async () => {
		const store = createTracesStore({
			loadTraces: async () => trails(3),
			pageSize: 50,
			pollIntervalMs: 0
		});
		render(Traces, harness(store));
		await waitFor(() => expect(screen.getByRole('table')).toBeInTheDocument());

		expect(screen.getByRole('heading', { name: 'Traces', level: 1 })).toBeInTheDocument();
		expect(screen.getByText('Trail 0')).toBeInTheDocument();
		expect(screen.getByRole('link', { name: 'Trail 1' })).toHaveAttribute(
			'href',
			'/next/traces/trace-01'
		);
	});

	it('badges a standing trail and speaks up only when a trail has news', async () => {
		const store = createTracesStore({
			loadTraces: async () => [
				traceSummary({ traceId: 'trace-standing', title: 'Standing', standing: true, hasActive: false }),
				traceSummary({ traceId: 'trace-busy', title: 'Busy', hasActive: true }),
				traceSummary({ traceId: 'trace-quiet', title: 'Quiet', hasActive: false })
			],
			pageSize: 50,
			pollIntervalMs: 0
		});
		render(Traces, harness(store));
		await waitFor(() => expect(screen.getByRole('table')).toBeInTheDocument());

		expect(screen.getByText('standing')).toBeInTheDocument();
		expect(screen.getByText('active')).toBeInTheDocument();
		const quiet = screen.getByRole('link', { name: 'Quiet' }).closest('tr');
		const cells = within(quiet as HTMLElement).getAllByRole('cell');
		expect(cells[1]).toBeEmptyDOMElement();
	});

	it('filters the table by title, id, or bee', async () => {
		const user = userEvent.setup();
		const store = createTracesStore({
			loadTraces: async () => trails(3),
			pageSize: 50,
			pollIntervalMs: 0
		});
		render(Traces, harness(store));
		await waitFor(() => expect(screen.getByRole('table')).toBeInTheDocument());

		await user.type(screen.getByLabelText('Filter traces'), 'trace-02');
		expect(screen.getByRole('link', { name: 'Trail 2' })).toBeInTheDocument();
		expect(screen.queryByRole('link', { name: 'Trail 0' })).not.toBeInTheDocument();
	});

	it('appends to the list on one control, and drops it once the server runs dry', async () => {
		const user = userEvent.setup();
		const loadTraces = vi
			.fn<(page: { limit?: number; before?: string }) => Promise<TraceSummary[]>>()
			.mockResolvedValueOnce(trails(51))
			.mockResolvedValueOnce(trails(50, 50));
		const store = createTracesStore({ loadTraces, pageSize: 50, pollIntervalMs: 0 });
		render(Traces, harness(store));
		await waitFor(() => expect(screen.getByRole('link', { name: 'Trail 49' })).toBeInTheDocument());

		// The server is asked for 51 and the table holds 50: the row past the page is the
		// answer to "is there more", and it is not a row the operator sees.
		expect(loadTraces).toHaveBeenLastCalledWith({ limit: 51 });
		expect(screen.queryByRole('link', { name: 'Trail 50' })).not.toBeInTheDocument();

		await user.click(screen.getByRole('button', { name: 'Load older trails' }));

		// The list grew, which is the whole point: a client page under this button put the
		// new rows behind a pager and left the press looking like nothing had happened.
		await waitFor(() => expect(screen.getByRole('link', { name: 'Trail 50' })).toBeInTheDocument());
		expect(screen.getByRole('link', { name: 'Trail 99' })).toBeInTheDocument();
		expect(screen.queryByRole('button', { name: 'Load older trails' })).not.toBeInTheDocument();
	});

	it('pages nothing itself, and so writes no page and counts nothing', async () => {
		const store = createTracesStore({
			loadTraces: async () => trails(50),
			pageSize: 50,
			pollIntervalMs: 0
		});
		render(Traces, harness(store));
		await waitFor(() => expect(screen.getByText('Trail 49')).toBeInTheDocument());

		// The depth of this list belongs to the store, so the table shows all of it: no
		// pager to press, and no "of N" that would read as a total the server cannot
		// honestly produce for a history this list never counts.
		expect(screen.getAllByRole('row')).toHaveLength(51);
		expect(screen.queryByRole('button', { name: 'Next' })).not.toBeInTheDocument();
		expect(screen.queryByRole('button', { name: 'Previous' })).not.toBeInTheDocument();
		expect(screen.queryByText(/of 50/)).not.toBeInTheDocument();
		expect(window.location.search).toBe('');
	});

	it('leaves the filter as the whole of this route’s shareable view', async () => {
		const user = userEvent.setup();
		const store = createTracesStore({
			loadTraces: async () => trails(50),
			pageSize: 50,
			pollIntervalMs: 0
		});
		render(Traces, harness(store));
		await waitFor(() => expect(screen.getByText('Trail 49')).toBeInTheDocument());

		await user.type(screen.getByLabelText('Filter traces'), 'trace-07');

		// A narrowed list is still a link somebody can be sent, which is what `?q=` was
		 // for. What is gone is `?page=`: there is no page of an unpaged list to name.
		expect(window.location.search).toBe('?q=trace-07');
		expect(screen.getByRole('link', { name: 'Trail 7' })).toBeInTheDocument();
		expect(screen.queryByRole('link', { name: 'Trail 8' })).not.toBeInTheDocument();
	});

	it('renders skeletons before the first payload and an empty message after it', async () => {
		let release: ((rows: TraceSummary[]) => void) | undefined;
		const gate = new Promise<TraceSummary[]>((resolve) => (release = resolve));
		const store = createTracesStore({ loadTraces: () => gate, pageSize: 50, pollIntervalMs: 0 });
		const { container, unmount } = render(Traces, harness(store));

		await waitFor(() => expect(container.querySelectorAll('.skeleton').length).toBeGreaterThan(0));
		unmount();

		const empty = createTracesStore({ loadTraces: async () => [], pageSize: 50, pollIntervalMs: 0 });
		render(Traces, harness(empty));
		await waitFor(() => expect(screen.getByText('No trails yet.')).toBeInTheDocument());
		release?.([]);
	});

	it('surfaces a list failure as an alert without dropping the rows', async () => {
		const store = createTracesStore({
			loadTraces: async () => {
				throw new Error('nats url not configured');
			},
			pageSize: 50,
			pollIntervalMs: 0
		});
		render(Traces, harness(store));

		await waitFor(() =>
			expect(screen.getByRole('alert')).toHaveTextContent('nats url not configured')
		);
	});
});
