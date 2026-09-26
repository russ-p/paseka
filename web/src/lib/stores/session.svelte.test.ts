import { describe, expect, it, vi } from 'vitest';
import { createSessionStore } from './session.svelte';
import { invite, session, transcriptEntry } from '../../tests/fixtures';
import type { Invite, Session, StopSessionResult, TranscriptEntry } from '$lib/api/types';

function harness(
	sessions: Session[] = [session()],
	invites: Invite[] = [],
	pages: { entries: TranscriptEntry[]; nextCursor: number }[] = []
) {
	const listSessions = vi.fn(async () => sessions);
	const listInvites = vi.fn(async () => invites);
	const getSession = vi.fn(async (sessionId: string) => {
		const found = sessions.find((entry) => entry.sessionId === sessionId);
		if (!found) throw new Error('session not found');
		return found;
	});
	let page = 0;
	const getTranscript = vi.fn(async (_sessionId: string, after: number) => {
		const next = pages[Math.min(page, pages.length - 1)] ?? { entries: [], nextCursor: after };
		page += 1;
		return next;
	});
	const createSession = vi.fn(async () => session({ sessionId: 'agent-new', active: true }));
	const stopSession = vi.fn(async (): Promise<StopSessionResult> => ({ status: 'stopped' }));
	const resumeSession = vi.fn(async () => session({ sessionId: 'agent-02', active: true, state: 'active' }));
	const acceptInvite = vi.fn(async () => ({ sessionId: 'agent-03' }));
	const rejectInvite = vi.fn(async (inviteId: string) => invite({ inviteId, status: 'rejected' }));
	const store = createSessionStore({
		listSessions,
		listInvites,
		getSession,
		getTranscript,
		createSession,
		stopSession,
		resumeSession,
		acceptInvite,
		rejectInvite,
		activeIntervalMs: 0,
		transcriptIntervalMs: 0,
		listIntervalMs: 0
	});
	return {
		store,
		listSessions,
		listInvites,
		getSession,
		getTranscript,
		createSession,
		stopSession,
		resumeSession,
		acceptInvite,
		rejectInvite
	};
}

describe('createSessionStore', () => {
	it('reads the list and the invites on mount, once each', async () => {
		const h = harness();
		expect(h.store.showSkeletons).toBe(true);

		await h.store.start();
		await h.store.start();

		expect(h.listSessions).toHaveBeenCalledTimes(1);
		expect(h.listInvites).toHaveBeenCalledTimes(1);
		expect(h.store.showSkeletons).toBe(false);
		expect(h.store.sessions).toHaveLength(1);
	});

	it('answers a null list the way the other pages do', async () => {
		// Go marshals a nil slice as `null`, and this list can legitimately be empty.
		const h = harness();
		h.listSessions.mockResolvedValueOnce(null as unknown as Session[]);
		await h.store.start();

		expect(h.store.sessions).toEqual([]);
		expect(h.store.lastError).toBe('');
	});

	it('selects from the list without a second read, because the list carries the session', async () => {
		const h = harness();
		await h.store.start();
		await h.store.select('agent-01a0bd743c33d82c');

		expect(h.getSession).not.toHaveBeenCalled();
		expect(h.store.current?.bee).toBe('builder');
		expect(h.store.detached).toBe(false);
	});

	it('fetches a session the list does not hold, and says it was read on its own', async () => {
		// A deep link to a session older than the recent window, or one started since
		// the list was read: the legacy answered with an empty detail pane and no word.
		const h = harness([session()]);
		await h.store.start();
		await h.store.select('agent-gone');

		expect(h.getSession).toHaveBeenCalledWith('agent-gone');
		expect(h.store.detailError).toBe('session not found');
		expect(h.store.current).toBeNull();
	});

	it('reports a list it could not read, rather than showing an empty table', async () => {
		const h = harness();
		h.listSessions.mockRejectedValueOnce(new Error('scan failed'));
		await h.store.start();

		expect(h.store.lastError).toBe('scan failed');
		expect(h.store.showSkeletons).toBe(false);
	});

	it('does not read the transcript of a running session, because the terminal has it', async () => {
		const h = harness([session({ active: true })], [], [{ entries: [], nextCursor: 0 }]);
		await h.store.start();
		await h.store.select('agent-01a0bd743c33d82c');

		// The legacy fetched the transcript for a live session too, and the view showed
		// one or the other — the transcript was read and then hidden.
		expect(h.getTranscript).not.toHaveBeenCalled();
		expect(h.store.lines).toEqual([]);
	});

	it('reads a finished session transcript from its cursor and appends the pages', async () => {
		const done = session({ active: false, state: 'completed', finishedAt: '2026-09-25T18:10:00Z' });
		const h = harness([done], [], [
			{ entries: [transcriptEntry({ content: 'first' })], nextCursor: 1 },
			{ entries: [transcriptEntry({ content: 'second' })], nextCursor: 2 }
		]);
		await h.store.start();
		await h.store.select('agent-01a0bd743c33d82c');

		expect(h.getTranscript).toHaveBeenCalledWith('agent-01a0bd743c33d82c', 0);
		expect(h.store.lines.map((line) => line.content)).toEqual(['first']);

		await h.store.select('agent-01a0bd743c33d82c');
		expect(h.getTranscript).toHaveBeenLastCalledWith('agent-01a0bd743c33d82c', 0);
	});

	it('keeps only the tail of a long transcript, and says how much it dropped', async () => {
		// The legacy re-rendered every line of every session on every tick, forever.
		const done = session({ active: false, state: 'completed' });
		const many = Array.from({ length: 8 }, (_, index) =>
			transcriptEntry({ content: `line ${index}` })
		);
		const h = harness([done], [], [{ entries: many, nextCursor: 8 }]);
		const store = createSessionStore({
			listSessions: async () => [done],
			listInvites: async () => [],
			getSession: async () => done,
			getTranscript: async () => ({ entries: many, nextCursor: 8 }),
			transcriptWindow: 5
		});
		await store.start();
		await store.select('agent-01a0bd743c33d82c');

		expect(store.lines).toHaveLength(5);
		expect(store.lines[0].content).toBe('line 3');
		expect(store.droppedLines).toBe(3);
		expect(h.getTranscript).not.toHaveBeenCalled();
	});

	it('re-reads the transcript cursor from zero when a session stops, because that is when it is read', async () => {
		let live = true;
		const getSession = vi.fn(async () => (live ? session({ active: true }) : session({ active: false, state: 'cancelled' })));
		const getTranscript = vi.fn(async (_id: string, after: number) => ({
			entries: [transcriptEntry({ content: `after ${after}` })],
			nextCursor: after + 1
		}));
		const store = createSessionStore({
			// The list is a scan of the run directories, so it lags: it still says the
			// session is running after the detail says it stopped. The tick therefore
			// *starts* as an active poll, and the transition is discovered mid-poll.
			listSessions: async () => [session({ active: true })],
			listInvites: async () => [],
			getSession,
			getTranscript,
				activeIntervalMs: 0,
			transcriptIntervalMs: 0,
			listIntervalMs: 0
		});
		await store.start();
		await store.select('agent-01a0bd743c33d82c');
		expect(getTranscript).not.toHaveBeenCalled();

		live = false;
		await store.reload();

		// The entries an agent wrote while it ran are exactly what is wanted once it
		// has stopped, so the cursor goes back rather than forward.
		expect(getTranscript).toHaveBeenCalledWith('agent-01a0bd743c33d82c', 0);
		expect(store.current?.state).toBe('cancelled');
	});

	it('reports which of the two stops happened, because they are not the same', async () => {
		const h = harness([session({ active: true })]);
		await h.store.start();
		await h.store.select('agent-01a0bd743c33d82c');

		const result = await h.store.stopSession('agent-01a0bd743c33d82c');

		// A session in this process is killed; one in another gets a SIGTERM. The legacy
		// discarded this body, so a hard kill and a polite request looked identical.
		expect(result).toEqual({ status: 'stopped' });
		expect(h.stopSession).toHaveBeenCalledWith('agent-01a0bd743c33d82c');
	});

	it('leads the list with a session it just started, and with one it just resumed', async () => {
		const h = harness();
		await h.store.start();

		const started = await h.store.startSession({ bee: 'builder', body: 'Do the thing.' });
		expect(h.store.sessions[0].sessionId).toBe(started.sessionId);

		const resumed = await h.store.resumeSession('agent-01a0bd743c33d82c', 'and then this');
		expect(h.store.sessions[0].sessionId).toBe(resumed.sessionId);
		// A resume is a new session, so the continuation goes to the one it creates.
		expect(h.resumeSession).toHaveBeenCalledWith('agent-01a0bd743c33d82c', 'and then this');
	});

	it('re-reads both lists after an invite is accepted, and hands back the session', async () => {
		const h = harness([session()], [invite()]);
		await h.store.start();

		const sessionId = await h.store.acceptInvite('invite-01');

		expect(sessionId).toBe('agent-03');
		expect(h.listInvites).toHaveBeenCalledTimes(2);
		expect(h.listSessions).toHaveBeenCalledTimes(2);
	});

	it('offers nothing for an invite that was rejected, and refreshes anyway', async () => {
		const h = harness([], [invite()]);
		await h.store.start();

		await h.store.rejectInvite('invite-01');

		expect(h.rejectInvite).toHaveBeenCalledWith('invite-01');
		expect(h.listInvites).toHaveBeenCalledTimes(2);
	});

	it('reports an invite failure in place, not as the page', async () => {
		const h = harness([], [invite()]);
		h.listInvites.mockRejectedValueOnce(new Error('invites unavailable'));
		await h.store.start();

		// Invites are a panel beside the list, so their failure is not the page's error.
		expect(h.store.inviteError).toBe('invites unavailable');
		expect(h.store.lastError).toBe('');
	});

	it('keeps a selected session readable across a poll that no longer lists it', async () => {
		const h = harness([session(), session({ sessionId: 'agent-02' })]);
		await h.store.start();
		await h.store.select('agent-02');
		h.listSessions.mockResolvedValue([session()]);
		await h.store.refresh();

		// A poll must not blank the detail pane: the operator is still reading it.
		expect(h.store.current?.sessionId).toBe('agent-02');
	});

	it('polls the list, which is what puts a session started elsewhere on screen', async () => {
		vi.useFakeTimers();
		try {
			const h = harness();
			const store = createSessionStore({
				listSessions: h.listSessions,
				listInvites: h.listInvites,
				getSession: h.getSession,
				getTranscript: h.getTranscript,
				listIntervalMs: 5000,
				activeIntervalMs: 0,
				transcriptIntervalMs: 0,
			});
			await store.start();
			expect(h.listSessions).toHaveBeenCalledTimes(1);

			await vi.advanceTimersByTimeAsync(5000);

			// The legacy never polled this list, so a session started by `paseka bee chat`
			// stayed invisible until someone pressed Refresh.
			expect(h.listSessions).toHaveBeenCalledTimes(2);
			store.stop();
		} finally {
			vi.useRealTimers();
		}
	});
});
