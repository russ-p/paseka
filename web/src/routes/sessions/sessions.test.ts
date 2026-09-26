import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import SessionList from './SessionList.svelte';
import SessionDetail from './[sessionId]/SessionDetail.svelte';
import { createSessionStore } from '$lib/stores/session.svelte';
import { createToastStore } from '$lib/stores/toast.svelte';
import { invite, session, transcriptEntry } from '../../tests/fixtures';
import type { Invite, Session, StopSessionResult, TranscriptEntry } from '$lib/api/types';

/**
 * xterm is a terminal emulator: it measures a container, paints a grid, and installs
 * document-level pointer listeners, none of which a test renderer has. Mocked here so
 * these tests stay about the *page's* decisions — which panel a session gets, what a
 * stop reports, whether a resume is offered — and `SessionTerminal.test.ts` covers the
 * relay protocol itself with the emulator mocked out.
 *
 * A **function**, not a class: a Svelte 5 component is called as one, and a class here
 * throws "cannot be invoked without 'new'" from inside the page's own render — which
 * fails the assertion *and* leaves an unhandled error that stops the runner exiting.
 *
 * `fireEvent` rather than `userEvent` for the controls on a *running* session: on that
 * branch `userEvent.click` never settles under this renderer, while `fireEvent` runs the
 * same handler. Every other click here uses `userEvent`, so the difference is recorded
 * rather than hidden — and the running-session page is verified in a real browser
 * regardless, which is the check that settles it.
 */
vi.mock('$lib/components/SessionTerminal.svelte', () => ({
	default: () => {}
}));

/**
 * A resume navigates, and a test renderer has no router: `goto` is recorded rather than
 * followed, so the test can also assert that the resume went to the *new* session — which
 * is the part that matters, since a resume is a new session and staying on the old page
 * would describe a session that no longer runs.
 */
const goto = vi.fn(async () => {});
vi.mock('$app/navigation', () => ({ goto: (...args: unknown[]) => goto(...(args as [])) }));

function harness(
	sessions: Session[] = [session()],
	invites: Invite[] = [],
	lines: TranscriptEntry[] = []
) {
	const listSessions = vi.fn(async () => sessions);
	const listInvites = vi.fn(async () => invites);
	const getSession = vi.fn(async (sessionId: string) => {
		const found = sessions.find((entry) => entry.sessionId === sessionId);
		if (!found) throw new Error(`session ${sessionId} not found`);
		return found;
	});
	const getTranscript = vi.fn(async () => ({ entries: lines, nextCursor: lines.length }));
	const stopSession = vi.fn(async (): Promise<StopSessionResult> => ({ status: 'stopped' }));
	const resumeSession = vi.fn(async () => session({ sessionId: 'agent-02' }));
	const createSession = vi.fn(async () => session({ sessionId: 'agent-new' }));
	const acceptInvite = vi.fn(async () => ({ sessionId: 'agent-03' }));
	const rejectInvite = vi.fn(async () => invite({ status: 'rejected' }));
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
		toasts: createToastStore(0),
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

function stubFetch(handler: (url: string) => Response): void {
	vi.stubGlobal(
		'fetch',
		vi.fn(async (input: RequestInfo | URL) => handler(String(input)))
	);
}

describe('sessions list', () => {
	it('lists what is running and links each row to its own page', async () => {
		const h = harness();
		render(SessionList, { store: h.store });
		await waitFor(() => expect(screen.getByLabelText('Sessions')).toBeInTheDocument());

		expect(screen.getByRole('heading', { name: 'Sessions', level: 1 })).toBeInTheDocument();
		const table = screen.getByLabelText('Sessions');
		// The id is the link, so a session is reachable by URL. The legacy made each row
		// a `<li>` with a click handler: no href, no tabindex, no role, so a session could
		// not be selected with a keyboard and could not be linked to at all.
		expect(within(table).getByRole('link', { name: 'builder · cursor' })).toHaveAttribute(
			'href',
			'/next/sessions/agent-01a0bd743c33d82c'
		);
	});

	it('badges a running session apart from a finished one', async () => {
		const h = harness([
			session(),
			session({ sessionId: 'agent-02', active: false, state: 'completed', pid: undefined })
		]);
		render(SessionList, { store: h.store });
		await waitFor(() => expect(screen.getByLabelText('Sessions')).toBeInTheDocument());

		expect(within(screen.getByLabelText('Sessions')).getByText('running')).toBeInTheDocument();
		expect(within(screen.getByLabelText('Sessions')).getByText('completed')).toBeInTheDocument();
	});

	it('calls a stopped session "stopped" rather than failing it, because the operator stopped it', async () => {
		// The legacy badged `cancelled` with the `failed` class, so stopping a session
		// and watching one crash were indistinguishable.
		const h = harness([session({ sessionId: 'agent-02', active: false, state: 'cancelled' })]);
		render(SessionList, { store: h.store });
		await waitFor(() => expect(screen.getByLabelText('Sessions')).toBeInTheDocument());

		expect(within(screen.getByLabelText('Sessions')).getByText('stopped')).toBeInTheDocument();
	});

	it('says a colony with no sessions what to do, rather than showing an empty table', async () => {
		const h = harness([]);
		render(SessionList, { store: h.store });
		await waitFor(() => expect(screen.getByLabelText('Sessions')).toBeInTheDocument());

		expect(screen.getByText(/No sessions yet/)).toBeInTheDocument();
	});

	it('reports a list it could not read, where the legacy only logged it', async () => {
		const h = harness();
		h.listSessions.mockRejectedValueOnce(new Error('scan failed'));
		render(SessionList, { store: h.store });

		// `loadSessions` had no try/catch at all: the failure became an unhandled
		// rejection and the list silently kept whatever it had.
		expect(await screen.findByRole('alert')).toHaveTextContent('scan failed');
	});

	it('opens the launch form on a bee whose intents are null, rather than crashing on them', async () => {
		// A bee with no prompt templates answers `intents: null`, because `BeeView.Intents`
		// has no `omitempty` and Go marshals a nil slice as null. Reading `.length` on it
		// took the whole page down — and the identical expression was already sitting in
		// the task create form, waiting for the same bee.
		stubFetch(() =>
			new Response(
				JSON.stringify([
					{ role: 'builder', adapter: 'cursor', promptTemplate: '', worktree: false, intents: null }
				])
			)
		);
		const h = harness();
		render(SessionList, { store: h.store });
		await waitFor(() => expect(screen.getByLabelText('Sessions')).toBeInTheDocument());

		await userEvent.click(screen.getByRole('button', { name: 'Launch session' }));

		const drawer = await screen.findByRole('dialog');
		expect(within(drawer).getByLabelText('Bee')).toBeInTheDocument();
		expect(within(drawer).getByText('The selected bee declares no intents.')).toBeInTheDocument();
		// Scoped to the drawer, because the button that opens it carries the same name —
		// and the launch itself needs a bee and a prompt, so a bee with no intents is a
		// narrower form rather than a dead end.
		expect(within(drawer).getByRole('button', { name: 'Launch session' })).toBeDisabled();
	});

	it('folds invites under a header that is closed when there are none', async () => {
		const h = harness([], []);
		render(SessionList, { store: h.store });
		await waitFor(() => expect(screen.getByLabelText('Sessions')).toBeInTheDocument());

		// The spec's "group invitations under a collapsible header": closed and empty is
		// the state a colony spends most of its time in, so it should not cost a screen.
		const invites = document.querySelector('details');
		expect(invites).not.toHaveAttribute('open');
		expect(screen.getByText('none')).toBeInTheDocument();

		await userEvent.click(screen.getByText('Pending invites'));

		expect(document.querySelector('details')).toHaveAttribute('open');
		expect(screen.getByText(/Nothing is waiting/)).toBeInTheDocument();
	});

	it('offers an invite a name that says which button starts what', async () => {
		const h = harness([], [invite(), invite({ inviteId: 'invite-02', intent: 'breakdown' })]);
		render(SessionList, { store: h.store });
		await waitFor(() => expect(screen.getByText('Start breakdown')).toBeInTheDocument());

		// A breakdown is not the same act as accepting a task, and the legacy used the
		// same button with the label swapped by string comparison.
		expect(screen.getByRole('button', { name: 'Accept' })).toBeInTheDocument();
		expect(screen.getByRole('button', { name: 'Start breakdown' })).toBeInTheDocument();
		expect(screen.getAllByRole('button', { name: 'Reject' })).toHaveLength(2);
	});

	it('shows an invite failure in place, not as the page', async () => {
		const h = harness([], [invite()]);
		h.acceptInvite.mockRejectedValueOnce(new Error('Top up from the Trace view Energy section'));
		render(SessionList, { store: h.store });
		await waitFor(() => expect(screen.getByRole('button', { name: 'Accept' })).toBeInTheDocument());

		await userEvent.click(screen.getByRole('button', { name: 'Accept' }));

		// The legacy answered this with `alert()`; the energy hint is a real message an
		// operator needs to read next to the button that caused it.
		expect(await screen.findByRole('alert')).toHaveTextContent('Top up from the Trace view');
	});

	it('filters by the adapter and the id, which are searchable but not shown', async () => {
		const h = harness([session(), session({ sessionId: 'agent-02', bee: 'scout', adapter: 'claude' })]);
		render(SessionList, { store: h.store });
		await waitFor(() => expect(screen.getByLabelText('Sessions')).toBeInTheDocument());

		await userEvent.type(screen.getByLabelText('Filter sessions'), 'claude');

		const table = screen.getByLabelText('Sessions');
		expect(within(table).getByText('scout · claude')).toBeInTheDocument();
		expect(within(table).queryByText('builder · cursor')).not.toBeInTheDocument();
	});
});

describe('session detail', () => {
	async function openDetail(sessions: Session[] = [session()], lines: TranscriptEntry[] = []) {
		const h = harness(sessions, [], lines);
		render(SessionDetail, {
			store: h.store,
			sessionId: 'agent-01a0bd743c33d82c',
			toasts: h.toasts
		});
		// Waited on the store rather than on a request: a session that is on the list is
		// selected from the list, so `getSession` is never called and waiting for it
		// would wait for something that is not supposed to happen.
		await waitFor(() => expect(h.store.current).not.toBeNull());
		return h;
	}

	it('names the session and the identity a run directory is not enough to guess', async () => {
		await openDetail();

		expect(screen.getByRole('heading', { name: /builder/ })).toBeInTheDocument();
		const identity = screen.getByLabelText('Session identity');
		expect(within(identity).getByText('Session')).toBeInTheDocument();
		expect(within(identity).getByText('.paseka/runs/trace-01a0bd6963faa14f/01a0bd743c33d82c')).toBeInTheDocument();
		expect(within(identity).getByText('PID')).toBeInTheDocument();
	});

	it('shows the adapter, which the legacy read on every poll and never displayed', async () => {
		await openDetail();

		// Resume eligibility is decided by the adapter, so a session that cannot be
		// resumed used to give no clue why.
		expect(within(screen.getByLabelText('Session identity')).getByText('cursor')).toBeInTheDocument();
	});

	it('shows how long a session ran, which the legacy never computed here', async () => {
		await openDetail([
			session({ active: false, state: 'completed', startedAt: '2026-09-25T18:01:00Z', finishedAt: '2026-09-25T18:11:30Z' })
		]);

		expect(within(screen.getByLabelText('Session identity')).getByText('10m 30s')).toBeInTheDocument();
	});

	it('gives a running session a terminal and withholds the transcript', async () => {
		await openDetail([session({ active: true })]);

		expect(screen.getByText('Terminal')).toBeInTheDocument();
		expect(screen.queryByText('Transcript')).not.toBeInTheDocument();
	});

	it('gives a finished session a transcript and no terminal', async () => {
		await openDetail([session({ active: false, state: 'completed' })], [transcriptEntry()]);

		expect(screen.getByText('Transcript')).toBeInTheDocument();
		expect(screen.getByText('Looking at the gate now.')).toBeInTheDocument();
		expect(screen.queryByText('Terminal')).not.toBeInTheDocument();
	});

	it('says when a session is not in the recent window, instead of an empty pane', async () => {
		const h = harness();
		render(SessionDetail, { store: h.store, sessionId: 'agent-gone', toasts: h.toasts });

		// `selectSession` had no try/catch: the detail pane kept rendering the *previous*
		// session while the list highlighted the new one.
		expect(await screen.findByRole('alert')).toHaveTextContent('session agent-gone not found');
	});

	it('asks before stopping, because a local stop is a kill with no chance to clean up', async () => {
		const h = await openDetail([session({ active: true })]);
		await screen.findByRole('button', { name: 'Stop' });

		await fireEvent.click(screen.getByRole('button', { name: 'Stop' }));

		// One click on the legacy killed a live agent, with nothing said and no way back.
		expect(h.stopSession).not.toHaveBeenCalled();
		expect(screen.getByRole('button', { name: 'Stop it' })).toBeInTheDocument();

		await fireEvent.click(screen.getByRole('button', { name: 'Stop it' }));
		await waitFor(() => expect(h.stopSession).toHaveBeenCalledWith('agent-01a0bd743c33d82c'));
	});

	it('reports which stop happened, because a kill and a SIGTERM are not the same', async () => {
		const h = harness();
		h.stopSession.mockResolvedValueOnce({ status: 'signalled' });
		const store = createSessionStore({
			listSessions: h.listSessions,
			listInvites: h.listInvites,
			getSession: h.getSession,
			getTranscript: h.getTranscript,
			stopSession: h.stopSession,
			resumeSession: h.resumeSession,
			activeIntervalMs: 0,
			transcriptIntervalMs: 0,
			listIntervalMs: 0
		});
		render(SessionDetail, { store, sessionId: 'agent-01a0bd743c33d82c', toasts: h.toasts });
		await waitFor(() => expect(store.current).not.toBeNull());
		await fireEvent.click(await screen.findByRole('button', { name: 'Stop' }));
		await fireEvent.click(screen.getByRole('button', { name: 'Stop it' }));

		// The legacy discarded this body entirely, so both looked like the same event.
		// The toast is asserted on the store, not the DOM: the shell renders toasts, not
		// the page, so a page test that looks for the message in the document is looking
		// for something this component never renders.
		await waitFor(() => expect(h.toasts.items).toHaveLength(1));
		expect(h.toasts.items[0].message).toMatch(/asked to exit/);
		expect(h.toasts.items[0].tone).toBe('success');
	});

	it('says a killed session was killed, which is not the same as asked', async () => {
		const h = await openDetail([session({ active: true })]);
		await fireEvent.click(await screen.findByRole('button', { name: 'Stop' }));
		await fireEvent.click(screen.getByRole('button', { name: 'Stop it' }));

		await waitFor(() => expect(h.toasts.items).toHaveLength(1));
		expect(h.toasts.items[0].message).toMatch(/Killed/);
		expect(h.toasts.items[0].tone).toBe('warning');
	});

	it('lets an operator keep a session running when they change their mind', async () => {
		const h = await openDetail([session({ active: true })]);
		await screen.findByRole('button', { name: 'Stop' });

		await fireEvent.click(screen.getByRole('button', { name: 'Stop' }));
		await fireEvent.click(screen.getByRole('button', { name: 'Keep it running' }));

		expect(h.stopSession).not.toHaveBeenCalled();
		expect(screen.queryByRole('button', { name: 'Stop it' })).not.toBeInTheDocument();
	});

	it('offers a continuation that submits on Enter, which a bare input did not', async () => {
		const h = await openDetail([session({ active: false, state: 'completed', providerSessionId: 'prov-9f2c' })]);
		await screen.findByText('Continue this session');

		const field = screen.getByPlaceholderText('What it should do next');
		await fireEvent.input(field, { target: { value: 'now the tests' } });
		await fireEvent.submit(field.closest('form') as HTMLFormElement);

		// The legacy's resume input sat outside any form, so the one thing an operator
		// does with a one-line box — type and press Enter — did nothing.
		await waitFor(() =>
			expect(h.resumeSession).toHaveBeenCalledWith('agent-01a0bd743c33d82c', 'now the tests')
		);
		// And it goes to the session the resume *created*, not the one it continued.
		await waitFor(() => expect(goto).toHaveBeenCalledWith('/next/sessions/agent-02'));
	});

	it('explains an adapter that cannot be resumed, rather than hiding the control', async () => {
		await openDetail([
			session({ active: false, state: 'completed', adapter: 'pi', providerSessionId: undefined })
		]);

		// The legacy hid Resume entirely here, so there was nothing to click and nothing
		// to read about why.
		expect(screen.getByText(/adapter does not support resuming/)).toBeInTheDocument();
		expect(screen.queryByRole('button', { name: 'Resume' })).not.toBeInTheDocument();
	});

	it('explains a resumable session with no provider conversation to continue', async () => {
		await openDetail([
			session({ active: false, state: 'completed', providerSessionId: undefined })
		]);

		expect(screen.getByText(/never reported a session id/)).toBeInTheDocument();
	});

	it('enters and leaves a full-screen terminal that keeps the session in the URL', async () => {
		await openDetail([session({ active: true })]);
		const full = await screen.findByRole('button', { name: 'Full screen' });

		await fireEvent.click(full);

		// The legacy's Widen button collapsed the page to one column and hid the launch
		// form and the whole session list, with that button as the only way back.
		expect(screen.getByText(/Escape or the button leaves/)).toBeInTheDocument();
		expect(screen.getAllByRole('button', { name: 'Leave full screen' }).length).toBeGreaterThan(0);

		await fireEvent.keyDown(document, { key: 'Escape' });

		expect(screen.queryByText(/Escape or the button leaves/)).not.toBeInTheDocument();
		expect(screen.getByRole('button', { name: 'Full screen' })).toBeInTheDocument();
	});

	it('does not let a refresh re-arm the polling that issued it', async () => {
		// The page starts the store from inside an effect, so anything the start path
		// *reads* becomes a dependency of that effect. It used to read the selected
		// session, which a list poll then writes back — so every poll invalidated the
		// effect that started polling, that effect stopped and restarted the store, and
		// the restart issued another poll. It ran at request latency: hundreds of reads a
		// second, with the terminal socket torn down and reopened on every turn.
		//
		// Counted on `setInterval` rather than by advancing time, because the cascade is
		// self-sustaining: waiting for it to settle is waiting for the bug to be slow.
		const intervals = vi.spyOn(globalThis, 'setInterval');
		try {
			const h = harness();
			const store = createSessionStore({
				listSessions: h.listSessions,
				listInvites: h.listInvites,
				getSession: h.getSession,
				getTranscript: h.getTranscript,
				listIntervalMs: 1000,
				activeIntervalMs: 0,
				transcriptIntervalMs: 0
			});
			render(SessionDetail, { store, sessionId: 'agent-01a0bd743c33d82c', toasts: h.toasts });
			await waitFor(() => expect(store.current).not.toBeNull());
			const armed = intervals.mock.calls.length;

			// A list poll rewrites the selected session, which is the write the loop turned
			// on. Each of these must leave the timers exactly as they were.
			await store.refresh();
			await store.refresh();
			await store.refresh();

			expect(intervals.mock.calls.length).toBe(armed);
			store.stop();
		} finally {
			intervals.mockRestore();
		}
	});

	it('offers a way out of a live terminal, which the legacy had none of', async () => {
		await openDetail([session({ active: true })]);

		// Every keystroke in a terminal belongs to the agent, and the only ways to release
		// the socket were stopping the session or leaving the tab.
		expect(await screen.findByRole('button', { name: 'Detach' })).toBeInTheDocument();
	});
});
