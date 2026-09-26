import { afterEach, describe, expect, it, vi } from 'vitest';
import {
	ApiError,
	addTraceEnergy,
	getTrace,
	getTraceArtifactContent,
	listTraceArtifacts,
	listTraces,
	traceCursor
} from './client';
import { traceSummary } from '../../tests/fixtures';

function stubFetch(handler: (url: string, init?: RequestInit) => Response): ReturnType<typeof vi.fn> {
	const mock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) =>
		handler(String(input), init)
	);
	vi.stubGlobal('fetch', mock);
	return mock;
}

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('ApiError', () => {
	it('reports the server reason rather than the bare status', async () => {
		stubFetch(() => new Response('honey reserve not configured\n', { status: 503 }));

		await expect(addTraceEnergy('trace-1', 5)).rejects.toThrow('honey reserve not configured');
	});

	it('keeps the status so a caller can still branch on it', async () => {
		stubFetch(() => new Response('before must start with an RFC3339 timestamp', { status: 400 }));

		const failure = await listTraces({ before: 'nope' }).catch((error: unknown) => error);
		expect(failure).toBeInstanceOf(ApiError);
		expect((failure as ApiError).status).toBe(400);
	});

	it('falls back to the status when the body is empty or unreadable', async () => {
		stubFetch(() => new Response('', { status: 502 }));
		await expect(listTraces()).rejects.toThrow('request failed: 502');

		stubFetch(() => new Response('boom', { status: 500 }));
		const failure = await getTrace('t').catch((error: unknown) => error);
		expect(failure).toBeInstanceOf(ApiError);
		expect((failure as ApiError).message).toBe('boom');
	});
});

describe('trace endpoints', () => {
	it('sends no query at all for the default page', async () => {
		const mock = stubFetch(() => new Response('[]'));
		await listTraces();
		expect(mock).toHaveBeenCalledWith('/api/traces', undefined);
	});

	it('encodes limit and the before cursor', async () => {
		const mock = stubFetch(() => new Response('[]'));
		await listTraces({ limit: 50, before: '2026-09-25T18:04:22.5Z|trace/../evil' });
		expect(mock).toHaveBeenCalledWith(
			'/api/traces?limit=50&before=2026-09-25T18%3A04%3A22.5Z%7Ctrace%2F..%2Fevil',
			undefined
		);
	});

	it('builds the cursor the server hands back through the last row', () => {
		expect(traceCursor(traceSummary())).toBe(
			'2026-09-25T18:04:22Z|trace-01a0bd6963faa14f'
		);
	});

	it('escapes the trace id on the detail and comb paths', async () => {
		const mock = stubFetch(() => new Response('{}'));
		await getTrace('trace/../evil');
		await listTraceArtifacts('trace/../evil');
		await getTraceArtifactContent('trace-1', 'notes file.md');
		expect(mock.mock.calls.map((call) => call[0])).toEqual([
			'/api/traces/trace%2F..%2Fevil',
			'/api/traces/trace%2F..%2Fevil/artifacts',
			'/api/traces/trace-1/artifacts?ref=notes+file.md'
		]);
	});

	it('posts the top-up amount as JSON', async () => {
		const mock = stubFetch(() => new Response('{}'));
		await addTraceEnergy('trace-1', 12);
		expect(mock).toHaveBeenCalledWith('/api/traces/trace-1/energy/add', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ amount: 12 })
		});
	});
});
