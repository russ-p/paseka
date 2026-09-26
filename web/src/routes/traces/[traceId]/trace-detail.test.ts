import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import TraceDetail from './TraceDetail.svelte';
import { createTraceStore, type TraceStore } from '$lib/stores/trace.svelte';
import { createToastStore, type ToastStore } from '$lib/stores/toast.svelte';
import { artifactView, eventFeedItem, traceDetail } from '../../../tests/fixtures';
import type { ArtifactView, EnergyAddResult, TraceDetail as TraceDetailPayload } from '$lib/api/types';

const traceId = 'trace-01a0bd6963faa14f';

function harness(overrides: {
	detail?: () => Promise<TraceDetailPayload>;
	artifacts?: () => Promise<ArtifactView[]>;
	topUpEnergy?: (traceId: string, amount: number) => Promise<EnergyAddResult>;
} = {}) {
	const store = createTraceStore({
		loadTrace: overrides.detail ?? (async () => traceDetail()),
		loadArtifacts: overrides.artifacts ?? (async () => [artifactView()]),
		topUpEnergy: overrides.topUpEnergy,
		pollIntervalMs: 0
	});
	const toasts = createToastStore(0);
	return { store, toasts };
}

/** The trail header: the back link, the title, the badges, and the summary. */
function header(): HTMLElement {
	return screen.getByRole('heading', { level: 1 }).closest('header') as HTMLElement;
}

/** One `Section` by its deep-link id, so a note in the title cannot break a name match. */
async function section(id: string): Promise<HTMLElement> {
	await waitFor(() => expect(document.querySelector(`#${id}`)).toBeInTheDocument());
	return document.querySelector(`#${id}`) as HTMLElement;
}

function renderDetail(
	overrides: Parameters<typeof harness>[0] = {}
): { store: TraceStore; toasts: ToastStore } {
	const props = harness(overrides);
	render(TraceDetail, { traceId, ...props });
	return props;
}

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('trace detail', () => {
	it('heads the page with the trail title, a way back, and a way into the timeline', async () => {
		renderDetail();

		expect(await screen.findByRole('heading', { name: 'Refactor the adapter seam', level: 1 })).toBeInTheDocument();
		expect(screen.getByRole('link', { name: '← Traces' })).toHaveAttribute('href', '/next/traces');
		expect(screen.getByRole('link', { name: 'Open timeline' })).toHaveAttribute(
			'href',
			`/next/timeline?trace=${traceId}`
		);
		expect(screen.getByText(/Move adapter construction behind one interface/)).toBeInTheDocument();
	});

	it('shares one desktop row between the identity block and the honey reserve', async () => {
		renderDetail();

		const trail = await section('trace-trail');
		const honey = await section('trace-honey');
		// jsdom does not lay out, so the contract is the grid wrapper, not the geometry.
		const row = trail.parentElement as HTMLElement;
		expect(row).toBe(honey.parentElement);
		expect(row.className).toContain('grid-cols-1');
		expect(row.className).toContain('lg:grid-cols-3');
		// Two thirds to identity, one third to the reserve.
		expect(trail.className).toContain('lg:col-span-2');
		expect(honey.className).not.toContain('col-span');
	});

	it('summarises the trail in the Trail block rather than a header panel', async () => {
		renderDetail();

		const block = await section('trace-trail');
		expect(within(block).getByText(traceId)).toBeInTheDocument();
		expect(within(block).getByText('builder')).toBeInTheDocument();
		expect(within(block).getByText('4')).toBeInTheDocument();
		expect(within(block).getByText('2')).toBeInTheDocument();
	});

	it('offers the trail id on the clipboard and nothing else does', async () => {
		const user = userEvent.setup();
		const writeText = vi.fn(async () => {});
		Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
		renderDetail();
		await screen.findByRole('heading', { name: 'Refactor the adapter seam', level: 1 });

		const trail = await section('trace-trail');
		await user.click(within(trail).getByRole('button', { name: 'Copy trace' }));

		expect(writeText).toHaveBeenCalledWith(traceId);
		expect(within(trail).getByRole('button', { name: 'Trace copied' })).toBeInTheDocument();
		expect(
			within(await section('trace-honey')).queryByRole('button', { name: /^Copy/ })
		).not.toBeInTheDocument();
	});

	it('badges the standing flag and speaks up only when the trail has news', async () => {
		const { unmount } = render(TraceDetail, {
			traceId,
			...harness({ detail: async () => traceDetail({ standing: true, hasActive: false, hasFailures: false }) })
		});
		await screen.findByRole('heading', { name: 'Refactor the adapter seam', level: 1 });
		expect(within(header()).getByText('standing')).toBeInTheDocument();
		expect(within(header()).queryByText('active')).not.toBeInTheDocument();
		unmount();

		renderDetail();
		await screen.findByRole('heading', { name: 'Refactor the adapter seam', level: 1 });
		expect(within(header()).getByText('active')).toBeInTheDocument();
	});

	it('offers the honey top-ups and reports the outcome through a toast', async () => {
		const user = userEvent.setup();
		const topUpEnergy = vi.fn(async () => ({
			traceId,
			amount: 5,
			energyBudget: 12,
			energyRemaining: 12,
			energyAdded: 5,
			energyAllocated: 12,
			lowEnergy: false
		}));
		const { toasts } = renderDetail({ topUpEnergy });

		expect(await screen.findByText('8 / 12')).toBeInTheDocument();
		await user.click(screen.getByRole('button', { name: '+5' }));

		await waitFor(() => expect(topUpEnergy).toHaveBeenCalledWith(traceId, 5));
		expect(toasts.items[0]).toMatchObject({ tone: 'success', message: 'Honey reserve topped up by +5' });
		expect(await screen.findByText('12 / 12')).toBeInTheDocument();
	});

	it('keeps the reserve on screen and toasts the reason when a top-up fails', async () => {
		const user = userEvent.setup();
		const { toasts } = renderDetail({
			topUpEnergy: async () => {
				throw new Error('honey reserve not configured');
			}
		});

		await screen.findByText('8 / 12');
		await user.click(screen.getByRole('button', { name: '+1' }));

		await waitFor(() => expect(toasts.items).toHaveLength(1));
		expect(toasts.items[0]).toMatchObject({ tone: 'error', message: 'honey reserve not configured' });
		expect(screen.getByText('8 / 12')).toBeInTheDocument();
	});

	it('opens a comb file in a modal and reads its body on demand', async () => {
		const user = userEvent.setup();
		const fetchMock = vi.fn(
			async () => new Response(JSON.stringify({ ref: 'notes.md', contentHtml: '<p>Seam body</p>' }))
		);
		vi.stubGlobal('fetch', fetchMock);
		renderDetail();

		await user.click(await screen.findByRole('button', { name: 'View' }));

		expect(await screen.findByRole('dialog', { name: 'Adapter seam notes' })).toBeInTheDocument();
		await waitFor(() => expect(screen.getByText('Seam body')).toBeInTheDocument());
		expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining('ref='), undefined);
	});

	it('separates an announced comb file from a staged one', async () => {
		renderDetail({
			artifacts: async () => [
				artifactView({ ref: 'notes.md', title: 'Notes' }),
				artifactView({ ref: 'raw.bin', title: 'Raw', announced: false, staged: true })
			]
		});

		expect(await screen.findByText('announced')).toBeInTheDocument();
		expect(screen.getByText('staged')).toBeInTheDocument();
	});

	it('reports an unreadable comb apart from the trail', async () => {
		renderDetail({
			artifacts: async () => {
				throw new Error('comb is gone');
			}
		});

		const block = await section('trace-artifacts');
		expect(within(block).getByText('comb is gone')).toBeInTheDocument();
		expect(screen.getByRole('heading', { name: 'Refactor the adapter seam', level: 1 })).toBeInTheDocument();
	});

	it('says so for an empty comb', async () => {
		renderDetail({ artifacts: async () => [] });

		expect(await screen.findByText('No trail artifacts in the comb.')).toBeInTheDocument();
	});

	it('lists the tasks and runs with their state badges', async () => {
		renderDetail();

		const tasks = await section('trace-tasks');
		expect(within(tasks).getByText('Introduce the Adapter interface')).toBeInTheDocument();
		expect(within(tasks).getByText('task-a1 · builder')).toBeInTheDocument();
		expect(within(tasks).getByText('completed')).toBeInTheDocument();
		expect(within(tasks).getByText('waiting_review')).toBeInTheDocument();

		const runs = await section('trace-runs');
		expect(within(runs).getByText('run-04 · task-01')).toBeInTheDocument();
		expect(within(runs).getByText(/1\.2k in · 340 out · 800 cache/)).toBeInTheDocument();
	});

	it('links every task and run row to its own page', async () => {
		// Both blocks used to be inert, which was the right call while their routes
		// were placeholders: a row that promises a destination and dead-ends is worse
		// than one that only shows state. Now that both exist, every row leads
		// somewhere.
		renderDetail();
		await screen.findByText('Introduce the Adapter interface');

		const tasks = await section('trace-tasks');
		const runs = await section('trace-runs');
		expect(within(tasks).getAllByRole('link')[0]).toHaveAttribute(
			'href',
			expect.stringContaining('/next/tasks/trace-01a0bd6963faa14f/') as unknown as string
		);
		expect(within(runs).getAllByRole('link')[0]).toHaveAttribute(
			'href',
			expect.stringContaining('/next/runs/trace-01a0bd6963faa14f/') as unknown as string
		);
	});

	it('says so for a trail with no tasks, runs, or events', async () => {
		renderDetail({
			detail: async () => traceDetail({ tasks: null, runs: null, recentEvents: null, worktree: undefined })
		});

		expect(await screen.findByText('No tasks in this trail.')).toBeInTheDocument();
		expect(screen.getByText('No runs in this trail.')).toBeInTheDocument();
		expect(screen.getByText('No recent events.')).toBeInTheDocument();
	});

	it('hides the worktree and usage blocks when the trail has neither', async () => {
		renderDetail({
			detail: async () => traceDetail({ worktree: undefined, usage: undefined })
		});

		await screen.findByText('8 / 12');
		expect(screen.queryByRole('heading', { name: 'Worktree' })).not.toBeInTheDocument();
		expect(screen.queryByRole('heading', { name: 'LLM usage' })).not.toBeInTheDocument();
	});

	it('collapses the worktree and the event feed by default', async () => {
		renderDetail();

		await screen.findByText('8 / 12');
		const worktree = screen.getByText('Worktree').closest('details');
		const events = screen.getByText(/Recent events/).closest('details');
		expect(worktree).not.toHaveAttribute('open');
		expect(events).not.toHaveAttribute('open');
	});

	it('previews the event feed and points the rest at the timeline', async () => {
		const user = userEvent.setup();
		const many = Array.from({ length: 12 }, (_, index) =>
			eventFeedItem({ id: `event-${index}`, summary: `Event number ${index}` })
		);
		renderDetail({ detail: async () => traceDetail({ recentEvents: many }) });

		await user.click(await screen.findByText(/Recent events/));

		expect(screen.getByText('Event number 0')).toBeInTheDocument();
		expect(screen.getByText('Event number 7')).toBeInTheDocument();
		expect(screen.queryByText('Event number 8')).not.toBeInTheDocument();
		expect(screen.getByText('4 older events on the timeline.')).toBeInTheDocument();
	});

	it('renders skeletons before the trail lands and an alert when it cannot', async () => {
		const { container, unmount } = render(TraceDetail, {
			traceId,
			...harness({
				detail: async () => {
					throw new Error('request failed: 404');
				}
			})
		});
		await waitFor(() => expect(container.ownerDocument.body).toHaveTextContent('request failed: 404'));
		expect(screen.getByRole('alert')).toBeInTheDocument();
		unmount();

		let release: ((value: TraceDetailPayload) => void) | undefined;
		const gate = new Promise<TraceDetailPayload>((resolve) => (release = resolve));
		const pending = render(TraceDetail, { traceId, ...harness({ detail: () => gate }) });
		await waitFor(() => expect(pending.container.querySelectorAll('.skeleton').length).toBeGreaterThan(0));
		release?.(traceDetail());
	});
});
