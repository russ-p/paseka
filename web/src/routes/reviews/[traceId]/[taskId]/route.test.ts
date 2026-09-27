import { render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mergeDiff, reviewQueue, taskDetail } from '../../../../tests/fixtures';
import { setRouteParams } from '../../../../tests/route-params.svelte';
import * as pageModule from './+page.ts';

/**
 * The route module is two booleans, and they are the reason these two paths need a
 * test of their own at all: a deep link under the `/next` base is served by the
 * static adapter's SPA fallback, which only resolves it if the route is neither
 * server-rendered nor prerendered. A `+page.svelte` that renders correctly in jsdom
 * says nothing about either, and a regression here breaks the preview build rather
 * than a screen.
 */
vi.mock('$app/state', async () => {
	const { routeParams } = await import('../../../../tests/route-params.svelte');
	return { page: { get params() { return routeParams; } } };
});

import Page from './+page.svelte';

interface Reads {
	urls: string[];
}

function stubApi(overrides: {
	queue?: () => Response;
	task?: (traceId: string, taskId: string) => Response;
	diff?: (traceId: string) => Response;
} = {}): Reads {
	const reads: Reads = { urls: [] };
	vi.stubGlobal(
		'fetch',
		vi.fn(async (input: RequestInfo | URL) => {
			const url = String(input);
			reads.urls.push(url);
			const queue = overrides.queue ?? (() => json(reviewQueue()));
			const task = overrides.task ?? (() => json(taskDetail({ taskId: '_review', isFinal: true })));
			const diff = overrides.diff ?? ((traceId: string) => json(mergeDiff({ traceId })));
			if (url === '/api/review-queue') return queue();
			const taskMatch = url.match(/^\/api\/traces\/([^/]+)\/tasks\/([^/]+)$/);
			if (taskMatch) return task(decodeURIComponent(taskMatch[1]), decodeURIComponent(taskMatch[2]));
			const diffMatch = url.match(/^\/api\/traces\/([^/]+)\/merge-diff$/);
			if (diffMatch) return diff(decodeURIComponent(diffMatch[1]));
			return new Response('not found', { status: 404 });
		})
	);
	return reads;
}

function json(body: unknown): Response {
	return new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } });
}

beforeEach(() => {
	setRouteParams({});
});

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('/reviews/:traceId/:taskId', () => {
	it('is served by the SPA fallback rather than rendered or prerendered', () => {
		// The other half of the preview: every other deep-link route in the console
		// declares the same pair, so a route that lost one would be the odd one out
		// in a way nothing on screen shows.
		expect(pageModule.ssr).toBe(false);
		expect(pageModule.prerender).toBe(false);
	});

	it('asks for the proposal the URL names, and nothing else', async () => {
		// An empty queue, so the body has to read the task itself: a proposal found in
		// the queue would prove the trail id reached it and leave the task id untested.
		const reads = stubApi({ queue: () => json({ items: [], count: 0 }) });
		setRouteParams({ traceId: 'trace-deep-link', taskId: 'task-42' });

		render(Page);

		expect(
			await screen.findByRole('heading', { name: 'Wire the export format flag' })
		).toBeInTheDocument();
		expect(reads.urls).toContain('/api/traces/trace-deep-link/tasks/task-42');
		// And the trail id on its own, which the merge diff is addressed by.
		expect(reads.urls).toContain('/api/traces/trace-deep-link/merge-diff');
		expect(reads.urls).not.toContain('/api/traces/trace-01a0bd6963faa14f/tasks/task-42');
	});

	it('puts the ids from the URL on the links out, so a copied link is the one that works', async () => {
		stubApi({ queue: () => json({ items: [], count: 0 }) });
		setRouteParams({ traceId: 'trace-deep-link', taskId: 'task-42' });

		render(Page);
		await screen.findByRole('heading', { name: 'Wire the export format flag' });

		expect(screen.getByRole('link', { name: 'Task' })).toHaveAttribute(
			'href',
			'/next/tasks/trace-deep-link/task-42'
		);
		expect(screen.getByRole('link', { name: 'Timeline' })).toHaveAttribute(
			'href',
			'/next/timeline?trace=trace-deep-link'
		);
		expect(await screen.findByRole('link', { name: 'Open merge preview' })).toHaveAttribute(
			'href',
			'/next/reviews/trace-deep-link/task-42/preview'
		);
	});

	it('follows the URL when it changes, because the router reuses the page', async () => {
		// SvelteKit does not remount on a sibling link: a `const traceId = page.params
		// .traceId` renders this proposal correctly and then shows it forever, so the
		// second proposal here is read on one mounted page rather than in a remount.
		const reads = stubApi({
			task: (traceId, taskId) =>
				json(
					taskDetail({
						traceId,
						taskId,
						isFinal: true,
						title: `Proposal on ${traceId}`
					})
				)
		});
		setRouteParams({ traceId: 'trace-first', taskId: '_review' });

		render(Page);
		expect(await screen.findByRole('heading', { name: 'Proposal on trace-first' })).toBeInTheDocument();

		setRouteParams({ traceId: 'trace-second', taskId: '_review' });

		expect(await screen.findByRole('heading', { name: 'Proposal on trace-second' })).toBeInTheDocument();
		expect(reads.urls).toContain('/api/traces/trace-second/tasks/_review');
		expect(reads.urls).toContain('/api/traces/trace-second/merge-diff');
		expect(screen.queryByRole('heading', { name: 'Proposal on trace-first' })).not.toBeInTheDocument();
	});

	it('names both ids from the URL when the proposal is not there', async () => {
		// The two ids only ever come from the path, so the sentence that tells an
		// operator what they mistyped has to be built from the path too.
		stubApi({
			queue: () => json({ items: [], count: 0 }),
			task: () => new Response('task not found', { status: 404 })
		});
		setRouteParams({ traceId: 'trace-gone', taskId: 'task-gone' });

		render(Page);

		expect(await screen.findByText('Proposal not found')).toBeInTheDocument();
		const empty = screen.getByText('Proposal not found').closest('.card');
		expect(empty).toHaveTextContent('task-gone');
		expect(empty).toHaveTextContent('trace-gone');
		expect(screen.getByRole('link', { name: 'All reviews' })).toHaveAttribute('href', '/next/reviews');
	});
});
