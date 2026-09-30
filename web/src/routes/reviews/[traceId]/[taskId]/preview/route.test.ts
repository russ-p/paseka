import { render, screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mergeDiff, reviewQueue, taskDetail } from '../../../../../tests/fixtures';
import { setRouteParams } from '../../../../../tests/route-params.svelte';
import * as pageModule from './+page.ts';

vi.mock('$app/state', async () => {
	const { routeParams } = await import('../../../../../tests/route-params.svelte');
	return { page: { get params() { return routeParams; } } };
});

import Page from './+page.svelte';

function json(body: unknown): Response {
	return new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } });
}

/**
 * A diff is addressed by trail alone, so the trail in the URL is the only thing that
 * can pick which patch is on screen — and the task beside it decides whether there
 * is a preview at all. Both come from the path, which is what the route contributes.
 */
function stubApi(overrides: {
	queue?: () => Response;
	task?: (traceId: string, taskId: string) => Response;
	diff?: (traceId: string) => Response;
} = {}): { urls: string[] } {
	const urls: string[] = [];
	vi.stubGlobal(
		'fetch',
		vi.fn(async (input: RequestInfo | URL) => {
			const url = String(input);
			urls.push(url);
			if (url === '/api/review-queue') return (overrides.queue ?? (() => json(reviewQueue())))();
			const task = url.match(/^\/api\/traces\/([^/]+)\/tasks\/([^/]+)$/);
			if (task) {
				return (
					overrides.task?.(decodeURIComponent(task[1]), decodeURIComponent(task[2])) ??
					json(taskDetail({ taskId: '_review', isFinal: true }))
				);
			}
			const diff = url.match(/^\/api\/traces\/([^/]+)\/merge-diff$/);
			if (diff) {
				const traceId = decodeURIComponent(diff[1]);
				return overrides.diff?.(traceId) ?? json(mergeDiff({ traceId, branch: `paseka/${traceId}` }));
			}
			return new Response('not found', { status: 404 });
		})
	);
	return { urls };
}

beforeEach(() => {
	setRouteParams({});
});

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('/reviews/:traceId/:taskId/preview', () => {
	it('is served by the SPA fallback rather than rendered or prerendered', () => {
		expect(pageModule.ssr).toBe(false);
		expect(pageModule.prerender).toBe(false);
	});

	it('reads the diff for the trail the URL names, and anchors the notes to it', async () => {
		const { urls } = stubApi();
		setRouteParams({ traceId: 'trace-preview', taskId: '_review' });

		render(Page);

		expect(await screen.findByText('paseka/trace-preview')).toBeInTheDocument();
		expect(urls).toContain('/api/traces/trace-preview/merge-diff');
		expect(urls).not.toContain('/api/traces/trace-01a0bd6963faa14f/merge-diff');
		// The panel is only mountable once there is a diff to annotate, so its presence
		// is what says the two halves arrived together rather than one after the other.
		expect(await screen.findByLabelText('Review comments')).toBeInTheDocument();
		expect(within(screen.getByLabelText('Merge diff')).getByText('export const third = 3;')).toBeInTheDocument();
	});

	it('links back to the review the URL named, because the two are one path', async () => {
		stubApi();
		setRouteParams({ traceId: 'trace-preview', taskId: '_review' });

		render(Page);
		await screen.findByLabelText('Merge diff');

		expect(screen.getByRole('link', { name: '← Back to review' })).toHaveAttribute(
			'href',
			'/next/reviews/trace-preview/_review'
		);
	});

	it('follows the URL when it changes, because the router reuses the page', async () => {
		// The same reason the detail route needs it, and here it is worse to miss: a
		// frozen trail id leaves one patch on screen under another trail's heading.
		const { urls } = stubApi({
			diff: (traceId) => json(mergeDiff({ traceId, branch: `paseka/${traceId}` }))
		});
		setRouteParams({ traceId: 'trace-first', taskId: '_review' });

		render(Page);
		expect(await screen.findByText('paseka/trace-first')).toBeInTheDocument();

		setRouteParams({ traceId: 'trace-second', taskId: '_review' });

		await waitFor(() => expect(urls).toContain('/api/traces/trace-second/merge-diff'));
		expect(await screen.findByText('paseka/trace-second')).toBeInTheDocument();
		expect(screen.queryByText('paseka/trace-first')).not.toBeInTheDocument();
	});

	it('reports a diff it could not read, rather than a preview with nothing in it', async () => {
		stubApi({
			diff: () => new Response('worktree branch not found', { status: 500 })
		});
		setRouteParams({ traceId: 'trace-gone', taskId: '_review' });

		render(Page);

		expect(await screen.findByRole('alert')).toHaveTextContent('worktree branch not found');
		// And the page keeps its frame: the back link and the title are how an operator
		// gets out, and an alert that replaced them would be a dead end.
		expect(screen.getByRole('heading', { name: 'Merge preview' })).toBeInTheDocument();
		expect(screen.getByRole('link', { name: '← Back to review' })).toBeInTheDocument();
	});
});
