import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import SessionTerminal from './SessionTerminal.svelte';

/**
 * The relay protocol, with the emulator and the browser stubbed out.
 *
 * What is worth testing here is the wiring this component owns — the URL, which frame
 * is output and which is control, what a keystroke sends, and what happens when the
 * browser or the server says no. xterm's own cell grid is the library's to test.
 */

interface FakeTerminal {
	options: Record<string, unknown>;
	written: string[];
	disposed: boolean;
	dataHandler: ((data: string) => void) | null;
	cols: number;
	rows: number;
	loadAddon: (addon: unknown) => void;
	open: (host: Element) => void;
	write: (data: string | Uint8Array) => void;
	dispose: () => void;
	onData: (handler: (data: string) => void) => void;
	focus: () => void;
}

const terminals: FakeTerminal[] = [];

vi.mock('@xterm/xterm', () => ({
	Terminal: class {
		cols = 80;
		rows = 24;
		written: string[] = [];
		disposed = false;
		dataHandler: ((data: string) => void) | null = null;
		constructor(public options: Record<string, unknown> = {}) {
			terminals.push(this as unknown as FakeTerminal);
		}
		loadAddon() {}
		open() {}
		write(data: string | Uint8Array) {
			this.written.push(typeof data === 'string' ? data : new TextDecoder().decode(data));
		}
		dispose() {
			this.disposed = true;
		}
		onData(handler: (data: string) => void) {
			this.dataHandler = handler;
		}
		focus() {}
	}
}));

vi.mock('@xterm/addon-fit', () => ({
	FitAddon: class {
		fit() {}
	}
}));

vi.mock('@xterm/addon-web-links', () => ({
	WebLinksAddon: class {}
}));

vi.mock('@xterm/xterm/css/xterm.css', () => ({}));

class FakeSocket {
	// The component checks `readyState === WebSocket.OPEN`, so the stub needs the same
	// constants the real class exposes; without them every send is silently skipped.
	static readonly CONNECTING = 0;
	static readonly OPEN = 1;
	static readonly CLOSING = 2;
	static readonly CLOSED = 3;
	static last: FakeSocket | null = null;
	static instances: FakeSocket[] = [];
	binaryType = '';
	readyState = 0;
	sent: unknown[] = [];
	closed = false;
	onopen: (() => void) | null = null;
	onmessage: ((event: { data: ArrayBuffer | string }) => void) | null = null;
	onclose: (() => void) | null = null;
	onerror: (() => void) | null = null;
	constructor(public url: string) {
		FakeSocket.last = this;
		FakeSocket.instances.push(this);
	}
	send(data: unknown) {
		this.sent.push(data);
	}
	close() {
		this.closed = true;
	}
	open_() {
		this.readyState = 1;
		this.onopen?.();
	}
	emit(data: ArrayBuffer | string) {
		this.onmessage?.({ data });
	}
}

function renderTerminal(props: Record<string, unknown> = {}) {
	const statuses: { state: string; reason: string }[] = [];
	const result = render(SessionTerminal, {
		sessionId: 'agent 01/a',
		attachable: true,
		enabled: true,
		onstatus: (next: { state: string; reason: string }) => statuses.push(next),
		...props
	});
	return { statuses, rerender: result.rerender };
}

beforeEach(() => {
	terminals.length = 0;
	FakeSocket.instances.length = 0;
	FakeSocket.last = null;
	vi.stubGlobal('WebSocket', FakeSocket);
	vi.stubGlobal(
		'ResizeObserver',
		class {
			observe() {}
			disconnect() {}
		}
	);
	vi.stubGlobal('location', { protocol: 'http:', host: 'console.local' });
});

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('SessionTerminal', () => {
	it('opens the relay on the API, escaping the session id', () => {
		renderTerminal();

		// `/next` is a frontend prefix and the API has none, so the socket must not
		// inherit it — and a session id with a slash in it would otherwise read as a
		// different route.
		expect(FakeSocket.last?.url).toBe('ws://console.local/api/sessions/agent%2001%2Fa/pty');
	});

	it('upgrades to wss on a TLS host', () => {
		vi.stubGlobal('location', { protocol: 'https:', host: 'console.local' });
		renderTerminal();

		expect(FakeSocket.last?.url).toBe('wss://console.local/api/sessions/agent%2001%2Fa/pty');
	});

	it('reports connecting until the socket is open, then connected', () => {
		const { statuses } = renderTerminal();

		expect(statuses[0]).toEqual({ state: 'connecting', reason: '' });
		FakeSocket.last?.open_();
		expect(statuses.at(-1)).toEqual({ state: 'connected', reason: '' });
	});

	it('writes binary frames straight into the terminal, as raw PTY bytes', () => {
		renderTerminal();
		FakeSocket.last?.open_();

		// Bytes, not decoded text: a TUI repaints with escape sequences, and a UTF-8
		// decode at the wrong moment would corrupt the cell grid.
		FakeSocket.last?.emit(new TextEncoder().encode('[31mhi[0m').buffer as ArrayBuffer);

		expect(terminals[0].written.join('')).toBe('[31mhi[0m');
	});

	it('reads a status frame as control and keeps the reason the server gave', () => {
		const { statuses } = renderTerminal();
		FakeSocket.last?.open_();

		FakeSocket.last?.emit(
			JSON.stringify({
				type: 'status',
				state: 'exited',
				reason: 'sessions: session "agent 01/a" not active in this process'
			})
		);

		// The reason is the whole diagnosis: it is how a session owned by another
		// process is told apart from one that merely ended.
		expect(statuses.at(-1)).toEqual({
			state: 'exited',
			reason: 'sessions: session "agent 01/a" not active in this process'
		});
	});

	it('writes an unparseable text frame as output, because the server reads it as input', () => {
		renderTerminal();
		FakeSocket.last?.open_();

		FakeSocket.last?.emit('not json at all');

		expect(terminals[0].written.join('')).toBe('not json at all');
	});

	it('sends every keystroke as raw input, unframed', () => {
		renderTerminal();
		FakeSocket.last?.open_();

		terminals[0].dataHandler?.('c');

		expect(FakeSocket.last?.sent).toContain('c');
	});

	it('pushes its size once the socket is open, so an agent TUI draws into the right box', () => {
		terminals.length = 0;
		renderTerminal();
		const socket = FakeSocket.last;
		terminals[0].cols = 120;
		terminals[0].rows = 40;

		// A terminal that believes it is 80x24 while it is 120x40 wraps an agent's
		// interface into unreadable columns — and a size sent before the socket opens is
		// a size the relay never receives.
		const before = (socket?.sent ?? []).filter((entry) => typeof entry === 'string').length;
		socket?.open_();
		const sent = (socket?.sent ?? []).filter(
			(entry): entry is string => typeof entry === 'string' && entry.includes('"resize"')
		);

		expect(before).toBe(0);
		expect(sent).toHaveLength(1);
		expect(JSON.parse(sent[0])).toEqual({ type: 'resize', cols: 120, rows: 40 });
	});

	it('reports a closed socket as closed rather than leaving a stale connected', () => {
		const { statuses } = renderTerminal();
		FakeSocket.last?.open_();

		FakeSocket.last?.onclose?.();

		// The legacy set "connecting" then "connected" synchronously on every poll, so a
		// socket that had already failed still read as connected.
		expect(statuses.at(-1)).toEqual({ state: 'closed', reason: '' });
	});

	it('does not overwrite an exit the server already explained with a plain close', () => {
		const { statuses } = renderTerminal();
		FakeSocket.last?.open_();
		FakeSocket.last?.emit(JSON.stringify({ type: 'status', state: 'exited', reason: 'gone' }));

		FakeSocket.last?.onclose?.();

		expect(statuses.at(-1)?.state).toBe('exited');
	});

	it('opens nothing for a session this process cannot relay', () => {
		renderTerminal({ attachable: false });

		expect(FakeSocket.instances).toHaveLength(0);
		expect(terminals).toHaveLength(0);
	});

	it('opens nothing while detached, which is what the page Detach means', () => {
		renderTerminal({ enabled: false });

		expect(FakeSocket.instances).toHaveLength(0);
	});

	it('says so rather than throwing when the browser has no WebSocket', () => {
		vi.stubGlobal('WebSocket', undefined);
		const { statuses } = renderTerminal();

		// A component that throws here takes the whole page down with it, and a page
		// that throws is worse than a terminal that admits it cannot draw.
		expect(statuses.at(-1)?.state).toBe('closed');
		expect(statuses.at(-1)?.reason).toMatch(/cannot open the terminal relay/);
	});

	it('closes the socket when the session changes, so two are never open at once', async () => {
		// A second `render` would be a second component, not a changed prop — the point is
		// that one terminal follows its session, so the same instance is re-rendered.
		const { rerender } = renderTerminal();
		const first = FakeSocket.last;
		first?.open_();

		await rerender({ sessionId: 'agent-02' });

		expect(first?.closed).toBe(true);
		expect(FakeSocket.last?.url).toContain('agent-02');
	});

	it('does not open a second socket when the page around it re-renders', async () => {
		// The store replaces the selected session with a fresh projection on every poll,
		// which re-runs any effect downstream of it. This one is idempotent instead: a
		// re-run with the same session must find the relay already open and do nothing.
		// It used to return a cleanup that closed the socket, so every poll tore the relay
		// down and opened another one — several times a second, leaving the old sockets
		// open and flashing the "relay closed" notice each time.
		const { rerender } = renderTerminal();
		const first = FakeSocket.last;
		first?.open_();
		expect(FakeSocket.instances).toHaveLength(1);

		for (let poll = 0; poll < 5; poll += 1) {
			await rerender({ sessionId: 'agent 01/a', attachable: true, enabled: true });
		}

		expect(FakeSocket.instances).toHaveLength(1);
		expect(first?.closed).toBe(false);
	});

	it('closes the socket when it is detached, which is what the page Detach means', async () => {
		const { rerender } = renderTerminal();
		const first = FakeSocket.last;
		first?.open_();

		await rerender({ enabled: false });

		expect(first?.closed).toBe(true);
	});
});
