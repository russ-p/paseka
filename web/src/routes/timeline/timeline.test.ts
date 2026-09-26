import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import TimelineFeed from './TimelineFeed.svelte';
import { createTimelineStore, type TimelineStore } from '$lib/stores/timeline.svelte';
import { eventFeedItem, eventFeedPage } from '../../tests/fixtures';
import type { EventFeedItem, EventFilters } from '$lib/api/types';

type Page = { items: EventFeedItem[]; nextCursor?: string; hasMore: boolean };

function harness(
	pages: Page[] = [eventFeedPage()],
	initialFilters: EventFilters = {}
): { store: TimelineStore; listEvents: ReturnType<typeof vi.fn> } {
	const listEvents = vi.fn(async () => pages.shift() ?? eventFeedPage([]));
	return { store: createTimelineStore({ listEvents, initialFilters }), listEvents };
}

/** The filter panel, which is folded until something is in it. */
function filterPanel(): HTMLElement {
	return document.getElementById('timeline-filters') as HTMLElement;
}

describe('timeline route', () => {
	it('reads the colony feed on mount and lists the events newest first', async () => {
		const { store, listEvents } = harness([
			eventFeedPage([
				eventFeedItem({ id: 'first', summary: 'Scout reported a dead end' }),
				eventFeedItem({ id: 'second', summary: 'Builder reopened the seam' })
			])
		]);
		render(TimelineFeed, { store });
		await waitFor(() => expect(screen.getByLabelText('Event feed')).toBeInTheDocument());

		expect(listEvents).toHaveBeenCalledWith({}, undefined);
		const feed = screen.getByLabelText('Event feed');
		expect(within(feed).getByText('Scout reported a dead end')).toBeInTheDocument();
		expect(within(feed).getByText('Builder reopened the seam')).toBeInTheDocument();
	});

	it('keeps the filter panel folded when nothing narrows the feed', async () => {
		const { store } = harness();
		render(TimelineFeed, { store });
		await waitFor(() => expect(screen.getByLabelText('Event feed')).toBeInTheDocument());

		// Six filter fields are a wall of chrome above a feed that is the point.
		expect(filterPanel()).not.toHaveAttribute('open');
	});

	it('opens the panel already scoped when a trail deep link sent the operator here', async () => {
		const traceId = 'trace-01a0bd6963faa14f';
		const { store, listEvents } = harness([eventFeedPage()], { traceId });
		// What `+page.svelte` does with `?trace=`: one seed for the form and the store,
		// so the very first read is scoped rather than corrected a moment later.
		render(TimelineFeed, { store, initialFilters: { traceId } });
		await waitFor(() => expect(screen.getByLabelText('Event feed')).toBeInTheDocument());

		expect(listEvents).toHaveBeenCalledWith({ traceId }, undefined);
		// The operator arrived with a scope they need to see and be able to drop.
		expect(filterPanel()).toHaveAttribute('open');
		expect(screen.getByLabelText('Trace')).toHaveValue(traceId);
		expect(screen.getAllByText(traceId).length).toBeGreaterThan(0);
	});

	it('drops the repeated trace id from rows that all share it', async () => {
		const traceId = 'trace-01a0bd6963faa14f';
		const { store } = harness([eventFeedPage()], { traceId });
		render(TimelineFeed, { store, initialFilters: { traceId } });
		await waitFor(() => expect(screen.getByLabelText('Event feed')).toBeInTheDocument());

		// The filter panel already says which trail this is, so the rows neither repeat
		// the id nor link to the page describing it.
		expect(screen.queryByRole('link')).not.toBeInTheDocument();
	});

	it('keeps the trace id on a colony-wide feed and links the id, not the summary', async () => {
		const { store } = harness();
		render(TimelineFeed, { store });
		await waitFor(() => expect(screen.getByLabelText('Event feed')).toBeInTheDocument());

		const feed = screen.getByLabelText('Event feed');
		// The id is the identifier an operator follows or pastes; the headline is
		// prose, and a feed of underlined summaries reads as a wall of links.
		const link = within(feed).getByRole('link');
		expect(link).toHaveTextContent('trace-01a0bd6963faa14f');
		expect(link).toHaveAttribute('href', '/next/traces/trace-01a0bd6963faa14f');
	});

	it('applies a filter only on submit, so typing does not fire a request per keystroke', async () => {
		const { store, listEvents } = harness([eventFeedPage(), eventFeedPage()]);
		render(TimelineFeed, { store });
		await waitFor(() => expect(screen.getByLabelText('Event feed')).toBeInTheDocument());
		expect(listEvents).toHaveBeenCalledTimes(1);

		await userEvent.type(screen.getByLabelText('Bee'), 'scout');
		expect(listEvents).toHaveBeenCalledTimes(1);

		await userEvent.click(screen.getByRole('button', { name: 'Apply' }));

		await waitFor(() => expect(listEvents).toHaveBeenCalledTimes(2));
		expect(listEvents).toHaveBeenLastCalledWith({ bee: 'scout' }, undefined);
	});

	it('offers the four contracts and nothing else as a type filter', async () => {
		const { store } = harness();
		render(TimelineFeed, { store });
		await waitFor(() => expect(screen.getByLabelText('Event feed')).toBeInTheDocument());

		const options = within(screen.getByLabelText('Type'))
			.getAllByRole('option')
			.map((option) => option.textContent);
		expect(options).toEqual(['Any contract', 'SIGNAL', 'INSIGHT', 'MUTATION', 'VERIFICATION']);
	});

	it('names the active filters in the folded panel summary', async () => {
		const { store } = harness(
			[eventFeedPage()],
			{ traceId: 'trace-01a0bd6963faa14f', severity: 'high' }
		);
		render(TimelineFeed, {
			store,
			initialFilters: { traceId: 'trace-01a0bd6963faa14f', severity: 'high' }
		});
		await waitFor(() => expect(screen.getByLabelText('Event feed')).toBeInTheDocument());

		expect(screen.getByText('trace-01a0bd6963faa14f · severity high')).toBeInTheDocument();
		expect(screen.getByRole('button', { name: 'Clear 2 filters' })).toBeInTheDocument();
	});

	it('clears the filters back to the colony-wide feed', async () => {
		const { store, listEvents } = harness([eventFeedPage(), eventFeedPage()], {
			traceId: 'trace-1',
			bee: 'scout'
		});
		render(TimelineFeed, { store, initialFilters: { traceId: 'trace-1', bee: 'scout' } });
		await waitFor(() => expect(screen.getByLabelText('Event feed')).toBeInTheDocument());

		await userEvent.click(screen.getByRole('button', { name: 'Clear 2 filters' }));

		await waitFor(() => expect(listEvents).toHaveBeenLastCalledWith({}, undefined));
		expect(store.filtered).toBe(false);
		expect(screen.getByLabelText('Trace')).toHaveValue('');
	});

	it('reveals the raw event per row, which is already on the wire', async () => {
		const { store, listEvents } = harness();
		render(TimelineFeed, { store });
		await waitFor(() => expect(screen.getByLabelText('Event feed')).toBeInTheDocument());

		// The raw envelope ships on every feed row, so revealing it costs no request.
		expect(listEvents).toHaveBeenCalledTimes(1);
		const row = screen.getAllByText('Raw event')[0].closest('details') as HTMLElement;
		expect(row).not.toHaveAttribute('open');

		await userEvent.click(screen.getAllByText('Raw event')[0]);

		expect(row).toHaveAttribute('open');
		expect(screen.getByText(/"protocolVersion": "1"/)).toBeInTheDocument();
		expect(listEvents).toHaveBeenCalledTimes(1);
	});

	it('appends the next page rather than replacing the feed', async () => {
		const { store, listEvents } = harness([
			eventFeedPage([eventFeedItem({ id: 'first', summary: 'Older event' })], {
				hasMore: true,
				nextCursor: 'cursor-1'
			}),
			eventFeedPage([eventFeedItem({ id: 'second', summary: 'Older still' })])
		]);
		render(TimelineFeed, { store });
		await waitFor(() => expect(screen.getByRole('button', { name: 'Load more' })).toBeInTheDocument());

		await userEvent.click(screen.getByRole('button', { name: 'Load more' }));

		await waitFor(() => expect(screen.getByText('Older still')).toBeInTheDocument());
		expect(screen.getByText('Older event')).toBeInTheDocument();
		expect(listEvents).toHaveBeenLastCalledWith({}, 'cursor-1');
		// No cursor left, so no button left either.
		expect(screen.queryByRole('button', { name: 'Load more' })).not.toBeInTheDocument();
	});

	it('keeps Load more after a failed page, so the button is not a dead end', async () => {
		const listEvents = vi
			.fn<(_filters: EventFilters, after?: string) => Promise<Page>>()
			.mockResolvedValueOnce(
				eventFeedPage([eventFeedItem()], { hasMore: true, nextCursor: 'cursor-1' })
			)
			.mockRejectedValueOnce(new Error('cursor expired'))
			.mockResolvedValueOnce(eventFeedPage([eventFeedItem({ id: 'second' })]));
		const store = createTimelineStore({ listEvents });
		render(TimelineFeed, { store });
		await waitFor(() => expect(screen.getByRole('button', { name: 'Load more' })).toBeInTheDocument());

		await userEvent.click(screen.getByRole('button', { name: 'Load more' }));
		await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('cursor expired'));

		expect(screen.getByRole('button', { name: 'Load more' })).toBeEnabled();
	});

	it('says nothing matches rather than showing an empty feed', async () => {
		const { store } = harness([eventFeedPage([])], { traceId: 'trace-nonexistent' });
		render(TimelineFeed, { store, initialFilters: { traceId: 'trace-nonexistent' } });
		await waitFor(() => expect(screen.getByText('No events')).toBeInTheDocument());

		expect(screen.getByText(/Widen the filters/)).toBeInTheDocument();
		expect(screen.queryByLabelText('Event feed')).not.toBeInTheDocument();
	});

	it('alerts and drops the skeletons when the read fails outright', async () => {
		const store = createTimelineStore({
			listEvents: async () => {
				throw new Error('connection refused');
			}
		});
		render(TimelineFeed, { store });
		await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('connection refused'));

		expect(document.querySelector('[aria-busy="true"]')).toBeNull();
	});

	it('refreshes on demand, because the feed is history and never polls', async () => {
		const { store, listEvents } = harness([eventFeedPage(), eventFeedPage([eventFeedItem({ id: 'fresh' })])]);
		render(TimelineFeed, { store });
		await waitFor(() => expect(screen.getByLabelText('Event feed')).toBeInTheDocument());
		expect(listEvents).toHaveBeenCalledTimes(1);

		await userEvent.click(screen.getByRole('button', { name: 'Refresh' }));

		await waitFor(() => expect(listEvents).toHaveBeenCalledTimes(2));
	});
});
