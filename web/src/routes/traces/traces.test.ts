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
		expect(screen.getByText('Showing 3 trails, newest activity first.')).toBeInTheDocument();
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

	it('offers the next page only while the server has more history', async () => {
		const user = userEvent.setup();
		const loadTraces = vi
			.fn<(page: { limit?: number; before?: string }) => Promise<TraceSummary[]>>()
			.mockResolvedValueOnce(trails(50))
			.mockResolvedValueOnce(trails(10, 50));
		const store = createTracesStore({ loadTraces, pageSize: 50, pollIntervalMs: 0 });
		render(Traces, harness(store));
		await waitFor(() => expect(screen.getByRole('table')).toBeInTheDocument());

		const more = screen.getByRole('button', { name: 'Load older trails' });
		await user.click(more);

		await waitFor(() =>
			expect(screen.getByText('Showing 60 trails, newest activity first.')).toBeInTheDocument()
		);
		// The table pages at 15, so the second server page lands behind the pager.
		expect(screen.getByText('1–15 of 60')).toBeInTheDocument();
		expect(screen.queryByRole('button', { name: 'Load older trails' })).not.toBeInTheDocument();
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
