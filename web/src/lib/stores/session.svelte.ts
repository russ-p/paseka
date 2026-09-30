import {
	acceptInvite as acceptInviteRequest,
	createSession as createSessionRequest,
	getSession as readSession,
	getSessionTranscript as readTranscript,
	listInvites as readInvites,
	listSessions as readSessions,
	rejectInvite as rejectInviteRequest,
	resumeSession as resumeSessionRequest,
	stopSession as stopSessionRequest
} from '$lib/api/client';
import type {
	CreateSessionRequest,
	Invite,
	Session,
	StopSessionResult,
	TranscriptEntry
} from '$lib/api/types';

interface SessionStoreOptions {
	listSessions?: () => Promise<Session[]>;
	listInvites?: () => Promise<Invite[]>;
	getSession?: (sessionId: string) => Promise<Session>;
	getTranscript?: (sessionId: string, after: number) => Promise<{ entries: TranscriptEntry[]; nextCursor: number }>;
	createSession?: (request: CreateSessionRequest) => Promise<Session>;
	stopSession?: (sessionId: string) => Promise<StopSessionResult>;
	resumeSession?: (sessionId: string, body: string) => Promise<Session>;
	acceptInvite?: (inviteId: string) => Promise<{ sessionId?: string }>;
	rejectInvite?: (inviteId: string) => Promise<Invite>;
	/**
	 * A running session is watched; a finished one has its transcript read. The legacy
	 * used 2s and 1.5s, and issued two requests per tick while a session was running.
	 */
	activeIntervalMs?: number;
	transcriptIntervalMs?: number;
	/**
	 * The list poll, which is what puts a session started from `paseka bee chat` on
	 * screen without anyone reloading. The legacy never polled this list at all.
	 */
	listIntervalMs?: number;
	/**
	 * How many transcript lines stay in the DOM. The legacy kept every line of a long
	 * HITL session and re-rendered all of them on every tick.
	 */
	transcriptWindow?: number;
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

export function createSessionStore(options: SessionStoreOptions = {}) {
	const readList = options.listSessions ?? readSessions;
	const readInviteList = options.listInvites ?? readInvites;
	const readOne = options.getSession ?? readSession;
	const readPage = options.getTranscript ?? readTranscript;
	const launch = options.createSession ?? createSessionRequest;
	// Not named `stop`: that is the lifecycle teardown below, and a store whose
	// dependency and its own disposal share a name is a bug waiting to happen.
	const endSession = options.stopSession ?? stopSessionRequest;
	const resume = options.resumeSession ?? resumeSessionRequest;
	const accept = options.acceptInvite ?? acceptInviteRequest;
	const decline = options.rejectInvite ?? rejectInviteRequest;

	const activeIntervalMs = options.activeIntervalMs ?? 2000;
	const transcriptIntervalMs = options.transcriptIntervalMs ?? 1500;
	const listIntervalMs = options.listIntervalMs ?? 5000;
	const transcriptWindow = options.transcriptWindow ?? 500;

	let sessions = $state<Session[]>([]);
	let invites = $state<Invite[]>([]);
	let selectedId = $state('');
	let current = $state<Session | null>(null);
	/** Set when the selected id is not on the list and was fetched directly. */
	let detached = $state(false);

	let loading = $state(true);
	let error = $state('');
	let detailLoading = $state(false);
	let detailError = $state('');

	let lines = $state<TranscriptEntry[]>([]);
	/**
	 * How many lines the window has dropped from the front, so the view can say the
	 * history it is not showing rather than presenting a truncated log as a whole one.
	 */
	let dropped = $state(0);
	let cursor = $state(0);
	let transcriptLoading = $state(false);
	let transcriptError = $state('');
	let inviteError = $state('');

	/**
	 * Two timers, and they are the *only* two: the list, and whichever thing the
	 * selected session needs watched. The legacy started a timer per tab and shared one
	 * slot between Sessions and Runs, so the cadence was whichever tab was mounted.
	 */
	let timer: ReturnType<typeof setInterval> | undefined;
	let listTimer: ReturnType<typeof setInterval> | undefined;
	let started = false;
	let ready: Promise<void> | undefined;
	/**
	 * Guards a read against the selection moving on. Two sessions polled in quick
	 * succession would otherwise let the slower response land under the wrong id.
	 */
	let token = 0;

	async function refresh(): Promise<void> {
		try {
			// `null`, not `[]`: Go marshals a nil slice that way, and a colony with no
			// sessions at all is a real answer rather than a failure.
			sessions = (await readList()) ?? [];
			error = '';
			if (selectedId !== '') {
				// A poll must not blank the detail pane: the operator is still reading it.
				// The list is only a summary of what the detail already knows.
				const found = find(selectedId);
				if (found) current = found;
			}
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			loading = false;
		}
	}

	async function refreshInvites(): Promise<void> {
		try {
			invites = (await readInviteList()) ?? [];
		} catch (cause) {
			// Invites are a side panel on the sessions page, so a failure here is
			// reported in place rather than as the page's error.
			inviteError = errorMessage(cause);
		}
	}

	/** Plain functions rather than `$derived`, like every other store here. */
	function find(sessionId: string): Session | null {
		return sessions.find((entry) => entry.sessionId === sessionId) ?? null;
	}

	function pendingInvites(): Invite[] {
		return invites.filter((invite) => invite.status === 'pending');
	}

	/**
	 * The transcript is read from its cursor and the *whole* history is not: a session
	 * that has been running for hours has tens of thousands of lines, and the legacy
	 * re-rendered every one of them every 1.5s. Only the tail is kept, and a page the
	 * store has not seen before is truncated from the front so the newest line is the
	 * one on screen.
	 */
	function appendPage(sessionId: string, page: { entries: TranscriptEntry[]; nextCursor: number }): boolean {
		if (sessionId !== selectedId) return false;
		if (page.entries.length > 0) {
			lines = [...lines, ...page.entries];
			if (lines.length > transcriptWindow) {
				const keep = lines.slice(-transcriptWindow);
				// The *lines* given up, not the pages that overflowed: a first page of a
				// long session can drop hundreds in one go, and the view says how much
				// history it is not showing.
				dropped += lines.length - keep.length;
				lines = keep;
			}
		}
		cursor = page.nextCursor;
		return true;
	}

	async function pollTranscript(): Promise<void> {
		const sessionId = selectedId;
		if (sessionId === '') return;
		const mine = token;
		transcriptLoading = true;
		try {
			const page = await readPage(sessionId, cursor);
			if (mine !== token) return;
			appendPage(sessionId, page);
			transcriptError = '';
		} catch (cause) {
			if (mine !== token) return;
			transcriptError = errorMessage(cause);
		} finally {
			if (mine === token) transcriptLoading = false;
		}
	}

	/**
	 * A running session is re-read, because its state and PID move. The legacy fetched
	 * the detail *and* the whole list on every tick, and the list is a filesystem scan
	 * of up to 50 run directories.
	 */
	async function pollActive(): Promise<void> {
		const sessionId = selectedId;
		if (sessionId === '') return;
		const mine = token;
		try {
			const session = await readOne(sessionId);
			if (mine !== token) return;
			const wasActive = current?.active ?? false;
			current = session;
			detailError = '';
			// The moment a session stops, its transcript is the only record of what it
			// did, and the terminal it was streaming into is about to go away. Reset the
			// cursor and read it from the start: the entries written while it ran are
			// exactly the ones an operator is about to want.
			if (wasActive && !session.active) {
				// The entries an agent wrote while it ran are exactly what is wanted the
				// moment it stops, and the terminal it was streaming into is going away —
				// so the cursor goes back to the start and the transcript is read now,
				// rather than on the next tick of a poller that has not noticed yet.
				lines = [];
				dropped = 0;
				cursor = 0;
				await pollTranscript();
			}
		} catch (cause) {
			if (mine !== token) return;
			detailError = errorMessage(cause);
		}
	}

	/**
	 * The poll the selected session actually wants, and the switch between them.
	 *
	 * The cadence is re-armed here and only when the mode actually changes, because this
	 * is a timer callback: reading `current` from a callback is not tracked, while
	 * reading it from the effect that *starts* the store is. That distinction was the
	 * whole bug — see `start`.
	 */
	async function tick(): Promise<void> {
		if (selectedId === '') return;
		const wasActive = current?.active === true;
		if (wasActive) await pollActive();
		else await pollTranscript();
		if ((current?.active === true) !== wasActive) {
			restartTimer(current?.active === true ? activeIntervalMs : transcriptIntervalMs);
		}
	}

	async function select(sessionId: string): Promise<void> {
		if (!sessionId) return;
		if (ready) await ready;
		selectedId = sessionId;
		detailError = '';
		transcriptError = '';
		token += 1;
		lines = [];
		dropped = 0;
		cursor = 0;
		const queued = find(sessionId);
		detached = queued === null;
		if (queued) {
			// From the list: no second request, because the list already carries the
			// identity, the state, and the adapter that resume eligibility depends on.
			current = queued;
			detailLoading = false;
		} else {
			// A deep link to a session older than the recent window, or one started
			// since the list was read.
			detailLoading = true;
			try {
				current = await readOne(sessionId);
				detailError = '';
			} catch (cause) {
				current = null;
				detailError = errorMessage(cause);
			} finally {
				detailLoading = false;
			}
		}
		if (current?.active) {
			// A running session streams into the terminal, so its transcript is not read
			// here; the terminal has the scrollback and the transcript view is withheld
			// until it stops.
			lines = [];
			cursor = 0;
		} else {
			await pollTranscript();
		}
		// Past the `await` above, so reading `current` here is not something the calling
		// effect subscribes to.
		restartTimer(current?.active === true ? activeIntervalMs : transcriptIntervalMs);
	}

	/** Re-reads the list, the invites, and the selected session, on demand. */
	async function reload(): Promise<void> {
		await Promise.all([refresh(), refreshInvites()]);
		if (selectedId !== '') await tick();
	}

	async function startSession(request: CreateSessionRequest): Promise<Session> {
		const created = await launch(request);
		// The new session leads the list, so the page does not have to wait for a poll
		// to show what the operator just started.
		sessions = [created, ...sessions.filter((entry) => entry.sessionId !== created.sessionId)];
		await refreshInvites();
		return created;
	}

	/**
	 * Stop, and report which of the two things happened: this process kills the child,
	 * another process gets a `SIGTERM` from the registry. The legacy threw the answer
	 * away, so a hard kill and a request to exit looked identical.
	 */
	async function stopSession(sessionId: string): Promise<StopSessionResult> {
		const result = await endSession(sessionId);
		if (selectedId === sessionId) {
			// Re-read rather than guess: a stopped session is `cancelled` server-side, and
			// the transcript is what is left to read.
			await pollActive();
			await pollTranscript();
		} else {
			await refresh();
		}
		return result;
	}

	/** A resume is a *new* session, so the caller navigates to what it returns. */
	async function resumeSession(sessionId: string, body: string): Promise<Session> {
		const created = await resume(sessionId, body);
		sessions = [created, ...sessions];
		return created;
	}

	async function acceptInvite(inviteId: string): Promise<string> {
		const result = await accept(inviteId);
		await Promise.all([refreshInvites(), refresh()]);
		return result.sessionId ?? '';
	}

	async function rejectInvite(inviteId: string): Promise<void> {
		await decline(inviteId);
		await refreshInvites();
	}

	/**
	 * One timer for the selected session, at the period it is given.
	 *
	 * The period is a *parameter* rather than something read from `current` here, and
	 * that is not a style choice. This is called from `start`, which a page calls from
	 * inside an effect; an effect that reads reactive state depends on it, so reading
	 * `current` here would make the effect that starts the polling depend on the very
	 * value the polling writes. The first list poll then refreshes the selected session,
	 * that write invalidates the effect, the effect stops and restarts the store, and the
	 * restart fires another poll — a loop that runs at request latency and reopens the
	 * terminal socket on every turn.
	 */
	function restartTimer(periodMs: number): void {
		if (timer !== undefined) clearInterval(timer);
		timer = undefined;
		if (!started || selectedId === '') return;
		// `0` means off, the way it means off for the list timer. A zero interval is not
		// "very often": it is a self-rescheduling loop that never lets the process idle.
		if (periodMs <= 0) return;
		timer = setInterval(() => void tick(), periodMs);
	}

	function stop(): void {
		started = false;
		if (timer !== undefined) clearInterval(timer);
		if (listTimer !== undefined) clearInterval(listTimer);
		timer = undefined;
		listTimer = undefined;
	}

	/**
	 * Begins polling. Deliberately reads **no** reactive state: a page calls this from
	 * inside an effect, and anything read here becomes a dependency of that effect — so
	 * the list poll, whose whole job is to write the selected session back, would
	 * invalidate the effect that started it and restart itself. The session timer is armed
	 * by `select` and re-armed by `tick` instead.
	 */
	function start(): void {
		if (started) return;
		started = true;
		ready = Promise.all([refresh(), refreshInvites()]).then(() => undefined);
		if (listIntervalMs > 0) {
			listTimer = setInterval(() => void refresh(), listIntervalMs);
		}
	}

	return {
		get sessions(): Session[] {
			return sessions;
		},
		get invites(): Invite[] {
			return invites;
		},
		get pending(): Invite[] {
			return pendingInvites();
		},
		get current(): Session | null {
			return current;
		},
		/** The selected session is not on the list, so it was fetched on its own. */
		get detached(): boolean {
			return detached;
		},
		get selectedId(): string {
			return selectedId;
		},
		get lines(): TranscriptEntry[] {
			return lines;
		},
		get droppedLines(): number {
			return dropped;
		},
		get showSkeletons(): boolean {
			return loading;
		},
		get lastError(): string {
			return error;
		},
		get detailLoading(): boolean {
			return detailLoading;
		},
		get detailError(): string {
			return detailError;
		},
		get transcriptLoading(): boolean {
			return transcriptLoading;
		},
		get transcriptError(): string {
			return transcriptError;
		},
		get inviteError(): string {
			return inviteError;
		},
		find,
		refresh,
		refreshInvites,
		select,
		reload,
		startSession,
		stopSession,
		resumeSession,
		acceptInvite,
		rejectInvite,
		start,
		stop
	};
}

export type SessionStore = ReturnType<typeof createSessionStore>;
