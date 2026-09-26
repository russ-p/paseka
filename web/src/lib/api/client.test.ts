import { afterEach, describe, expect, it, vi } from 'vitest';
import {
	ApiError,
	addTraceEnergy,
	approveTask,
	createTask,
	getTask,
	listBees,
	listTasks,
	rejectTask,
	retryTask,
	startTask,
	getGit,
	getSystem,
	getRun,
	getTopology,
	listRunEvents,
	listRuns,
	getTrace,
	listEvents,
	getTraceArtifactContent,
	gitDeleteBranches,
	gitFetch,
	gitPruneWorktrees,
	gitPull,
	gitPush,
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

describe('git endpoints', () => {
	it('reads the clone without a query', async () => {
		const mock = stubFetch(() => new Response('{}'));

		await getGit();

		expect(mock).toHaveBeenCalledWith('/api/git', undefined);
	});

	it('posts the three sync actions with no body at all', async () => {
		const mock = stubFetch(() => new Response('{"ok":true}'));

		await gitFetch();
		await gitPull();

		expect(mock.mock.calls).toEqual([
			['/api/git/fetch', { method: 'POST' }],
			['/api/git/pull', { method: 'POST' }]
		]);
	});

	it('sends the push-hooks flag as JSON, since the server reads a body for it', async () => {
		const mock = stubFetch(() => new Response('{"ok":true}'));

		await gitPush(true);

		expect(mock).toHaveBeenCalledWith('/api/git/push', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ runHooks: true })
		});
	});

	it('deletes every branch in one request and prunes worktrees in another', async () => {
		const mock = stubFetch(() => new Response('{"ok":true}'));

		await gitDeleteBranches(['paseka/a', 'paseka/b']);
		await gitPruneWorktrees();

		expect(mock.mock.calls).toEqual([
			[
				'/api/git/branches/delete',
				{
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ names: ['paseka/a', 'paseka/b'] })
				}
			],
			['/api/git/worktrees/prune', { method: 'POST' }]
		]);
	});

	it('carries a server refusal through, so the page can say why the pull was refused', async () => {
		stubFetch(() => new Response('live bee is using the colony root checkout; refuse pull', { status: 409 }));

		const failure = await gitPull().catch((error: unknown) => error);

		expect(failure).toBeInstanceOf(ApiError);
		expect((failure as ApiError).message).toBe('live bee is using the colony root checkout; refuse pull');
		expect((failure as ApiError).status).toBe(409);
	});
});

describe('system endpoints', () => {
	it('reads the host snapshot with a bodiless GET and no query', async () => {
		const mock = stubFetch(() => new Response('{}'));

		await getSystem();

		expect(mock).toHaveBeenCalledWith('/api/system', undefined);
	});
});

describe('event feed endpoint', () => {
	it('sends the colony-wide feed as a bare page request', async () => {
		const mock = stubFetch(() => new Response('{"items":[],"hasMore":false}'));

		await listEvents();

		expect(mock).toHaveBeenCalledWith('/api/events?limit=50', undefined);
	});

	it('sends every filter under the name the server parses', async () => {
		const mock = stubFetch(() => new Response('{"items":[],"hasMore":false}'));

		await listEvents({
			traceId: 'trace-1',
			taskId: 'task-b2',
			bee: 'scout',
			type: 'VERIFICATION',
			kind: 'review.gate',
			severity: 'high'
		});

		const url = String(mock.mock.calls[0][0]);
		expect(url.startsWith('/api/events?')).toBe(true);
		const query = new URLSearchParams(url.split('?')[1]);
		expect(Object.fromEntries(query)).toEqual({
			traceId: 'trace-1',
			taskId: 'task-b2',
			bee: 'scout',
			type: 'VERIFICATION',
			kind: 'review.gate',
			severity: 'high',
			limit: '50'
		});
	});

	it('omits an absent filter rather than sending it blank', async () => {
		const mock = stubFetch(() => new Response('{"items":[],"hasMore":false}'));

		await listEvents({ traceId: '', bee: 'scout' }, 'cursor-1');

		const query = new URLSearchParams(String(mock.mock.calls[0][0]).split('?')[1]);
		// A blank field would ask the server to match emptiness, which says nothing useful.
		expect(query.has('traceId')).toBe(false);
		expect(query.get('bee')).toBe('scout');
		expect(query.get('after')).toBe('cursor-1');
	});
});

describe('topology endpoint', () => {
	it('reads the projection with a bodiless GET and no query', async () => {
		const mock = stubFetch(() => new Response('{}'));

		await getTopology();

		expect(mock).toHaveBeenCalledWith('/api/colony/topology', undefined);
	});
});

describe('run endpoints', () => {
	it('reads the recent runs with a bodiless GET', async () => {
		const mock = stubFetch(() => new Response('[]'));

		await listRuns();

		expect(mock).toHaveBeenCalledWith('/api/runs', undefined);
	});

	it('escapes both ids on the detail and the events path', async () => {
		const mock = stubFetch(() => new Response('{}'));

		await getRun('trace 1/a', 'run 1/b');

		expect(mock).toHaveBeenCalledWith('/api/runs/trace%201%2Fa/run%201%2Fb', undefined);
	});

	it('sends no cursor on the first events read, and an index on the next', async () => {
		// The events cursor is an index into the run's own sequence, not a trace
		// cursor, so paging is append-only.
		const mock = stubFetch(() => new Response('{"entries":[],"nextCursor":0}'));

		await listRunEvents('trace-1', 'run-1');
		await listRunEvents('trace-1', 'run-1', 7);

		expect(mock.mock.calls[0][0]).toBe('/api/runs/trace-1/run-1/events');
		expect(mock.mock.calls[1][0]).toBe('/api/runs/trace-1/run-1/events?after=7');
	});

	it('keeps a zero cursor rather than dropping it, because zero is the first page', async () => {
		const mock = stubFetch(() => new Response('{"entries":[],"nextCursor":0}'));

		await listRunEvents('trace-1', 'run-1', 0);

		expect(String(mock.mock.calls[0][0])).toContain('after=0');
	});
});

describe('task endpoints', () => {
	it('reads the board with a bodiless GET', async () => {
		const mock = stubFetch(() => new Response('{"groups":[],"taskCounts":{}}'));

		await listTasks();

		expect(mock).toHaveBeenCalledWith('/api/tasks', undefined);
	});

	it('escapes both ids, because a task is addressed by trail and task together', async () => {
		const mock = stubFetch(() => new Response('{}'));

		await getTask('trace 1/a', 'task 1/b');

		expect(mock).toHaveBeenCalledWith('/api/traces/trace%201%2Fa/tasks/task%201%2Fb', undefined);
	});

	it('sends the create body verbatim, so an unset field stays unset', async () => {
		// The server decides startability from what is absent as much as from what
		// is present, so the form must not pre-fill `dependsOn` with an empty array.
		const mock = stubFetch(() => new Response('{"traceId":"t","taskId":"1","bee":"builder","autorun":true}'));

		await createTask({ title: 'Do the thing', bee: 'builder', autorun: true });

		const init = mock.mock.calls[0][1] as RequestInit;
		expect(String(mock.mock.calls[0][0])).toBe('/api/tasks');
		expect(init.method).toBe('POST');
		expect(JSON.parse(String(init.body))).toEqual({
			title: 'Do the thing',
			bee: 'builder',
			autorun: true
		});
	});

	it('posts an action to the task\'s own path, escaping both ids each time', async () => {
		const mock = stubFetch(() => new Response('{}'));

		await startTask('trace 1', 'task 1');
		await retryTask('trace 1', 'task 1');

		expect(mock.mock.calls[0][0]).toBe('/api/traces/trace%201/tasks/task%201/start');
		expect(mock.mock.calls[1][0]).toBe('/api/traces/trace%201/tasks/task%201/retry');
	});

	it('carries the review fields, including the ones left off', async () => {
		const mock = stubFetch(() => new Response('{}'));

		await approveTask('trace-1', 'task-1', { summary: 'looks right' });
		await rejectTask('trace-1', 'task-1', { feedback: 'not yet' });

		const approve = JSON.parse(String((mock.mock.calls[0][1] as RequestInit).body));
		expect(approve).toEqual({ summary: 'looks right' });
		const reject = JSON.parse(String((mock.mock.calls[1][1] as RequestInit).body));
		expect(reject).toEqual({ feedback: 'not yet' });
		expect(String(mock.mock.calls[1][0])).toBe('/api/traces/trace-1/tasks/task-1/reject');
	});

	it('reads the bee list that the create and launch forms build their intents from', async () => {
		const mock = stubFetch(() => new Response('[]'));

		await listBees();

		expect(mock).toHaveBeenCalledWith('/api/bees', undefined);
	});
});
