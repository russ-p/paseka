<script lang="ts">
	import { onMount } from 'svelte';
	import { FitAddon } from '@xterm/addon-fit';
	import { WebLinksAddon } from '@xterm/addon-web-links';
	import { Terminal } from '@xterm/xterm';
	import '@xterm/xterm/css/xterm.css';

	let {
		sessionId,
		/** Whether this process can relay this session's PTY, decided by the caller. */
		attachable = true,
		/**
		 * Whether the relay should be open at all. The page's Detach is this flag rather
		 * than a call into the child: a parent reaching through `bind:this` to close a
		 * socket is the same coupling as the legacy's global `SessionTerminal`, and a
		 * prop is testable without standing up an emulator.
		 */
		enabled = true,
		onstatus
	}: {
		sessionId: string;
		/**
		 * False for a session this console does not own. The relay only carries a PTY
		 * the process holds, so asking anyway opens a socket that dies immediately with
		 * a Go error string — which is what the legacy showed, with Stop and Resume
		 * hidden because the session still read as active.
		 */
		attachable?: boolean;
		enabled?: boolean;
		onstatus?: (status: TerminalStatus) => void;
	} = $props();

	export type TerminalStatus = {
		state: 'connecting' | 'connected' | 'closed' | 'exited';
		reason: string;
	};

	interface Attachment {
		terminal: Terminal | null;
		socket: WebSocket | null;
		fit: FitAddon | null;
		container: HTMLDivElement | null;
		observer: ResizeObserver | null;
		/** The session the open socket belongs to, so a prop change can tell. */
		id: string;
	}

	/**
	 * xterm is the one dependency this console takes for a surface, and unlike the diff
	 * parser there is no version of this problem to write instead: a PTY is a terminal
	 * emulator, and the bytes an agent's TUI draws are the bytes xterm was built to
	 * interpret. What it costs is the console's second third-party styling exception,
	 * which is the right trade for a live terminal and the wrong one for a patch.
	 */
	function empty(): Attachment {
		return { terminal: null, socket: null, fit: null, container: null, observer: null, id: '' };
	}

	/**
	 * Deliberately *not* `$state`. Nothing in the markup renders from it, and an effect
	 * that reads reactive state it also writes is a dependency loop: opening a terminal
	 * would retrigger the effect that opened it.
	 */
	let attach: Attachment = empty();
	let host = $state<HTMLDivElement | null>(null);
	let status = $state<TerminalStatus>({ state: 'connecting', reason: '' });

	function report(next: TerminalStatus): void {
		status = next;
		onstatus?.(next);
	}

	/**
	 * The relay is `/api/sessions/:id/pty` and nothing under the `/next` prefix: the
	 * base path belongs to frontend routes, so deriving this from `location` rather than
	 * from the router keeps the socket on the API and picks `wss` on a TLS host.
	 */
	function socketUrl(id: string): string {
		const scheme = globalThis.location?.protocol === 'https:' ? 'wss' : 'ws';
		return `${scheme}://${globalThis.location.host}/api/sessions/${encodeURIComponent(id)}/pty`;
	}

	function sendResize(): void {
		if (attach.socket?.readyState !== WebSocket.OPEN) return;
		try {
			attach.fit?.fit();
		} catch {
			// A container with no layout yet throws, and the observer will call again.
			return;
		}
		attach.socket.send(
			JSON.stringify({
				type: 'resize',
				cols: attach.terminal?.cols ?? 80,
				rows: attach.terminal?.rows ?? 24
			})
		);
	}

	function open(id: string): void {
		if (attach.id === id) return;
		close();

		// A relay needs a WebSocket and a layout to measure, and a component that throws
		// on an environment without them takes the whole page down with it. Both are
		// missing in a test renderer, and both are worth saying out loud rather than
		// crashing: an operator on an old browser deserves a sentence, not a stack.
		if (typeof WebSocket === 'undefined') {
			report({ state: 'closed', reason: 'This browser cannot open the terminal relay.' });
			return;
		}
		if (typeof ResizeObserver === 'undefined') {
			report({ state: 'closed', reason: 'This browser cannot measure the terminal.' });
			return;
		}

		const terminal = new Terminal({
			cursorBlink: true,
			fontSize: 13,
			fontFamily: "'JetBrains Mono', ui-monospace, SFMono-Regular, monospace",
			scrollback: 5000,
			theme: { background: '#111318', foreground: '#e8eaed', cursor: '#f5c518' }
		});
		const fit = new FitAddon();
		terminal.loadAddon(fit);
		terminal.loadAddon(new WebLinksAddon());
		if (!host) return;
		try {
			terminal.open(host);
		} catch {
			report({ state: 'closed', reason: 'The terminal could not be drawn here.' });
			return;
		}

		attach = { terminal, socket: null, fit, container: host, observer: null, id };
		report({ state: 'connecting', reason: '' });

		const socket = new WebSocket(socketUrl(id));
		socket.binaryType = 'arraybuffer';
		attach.socket = socket;

		socket.onopen = () => {
			report({ state: 'connected', reason: '' });
			// The size has to go out *after* the socket opens. Sending it at attach time
			// is sending it into a socket that is not open yet, so it is dropped, and the
			// PTY keeps whatever default size the relay started with — which is how an
			// agent's TUI ends up wrapped into unreadable columns on a wide window.
			sendResize();
		};
		socket.onmessage = (event: MessageEvent<ArrayBuffer | string>) => {
			// Binary is raw PTY output. A text frame is a control message, or raw input
			// from a client that framed it as text: the server treats unparseable text as
			// input, so this does too.
			if (typeof event.data === 'string') {
				let control: { type?: string; state?: string; reason?: string } | undefined;
				try {
					control = JSON.parse(event.data);
				} catch {
					terminal.write(event.data);
					return;
				}
				if (control?.type === 'status') {
					report({ state: 'exited', reason: control.reason ?? control.state ?? '' });
				}
				return;
			}
			terminal.write(new Uint8Array(event.data));
		};
		socket.onclose = () => {
			// A close the server already explained reads as `exited`; anything else is the
			// socket going away underneath us, which is worth saying rather than leaving
			// a frozen terminal under a stale "connected".
			if (status.state !== 'exited') report({ state: 'closed', reason: '' });
		};
		socket.onerror = () => {
			if (status.state === 'connected') report({ state: 'closed', reason: '' });
		};

		// Every keystroke is the agent's, which is the point of an interactive session
		// and also why the terminal needs a way out: the page offers focus mode and a
		// detach, so an operator is never trapped in a live TUI.
		terminal.onData((data: string) => {
			if (socket.readyState === WebSocket.OPEN) socket.send(data);
		});

		// A terminal sized to an unlaid-out element measures zero, and an agent's TUI
		// keeps redrawing into whatever size it last believed in. Observed rather than
		// fitted on demand, because the grid reflows on its own schedule.
		const observer = new ResizeObserver(() => sendResize());
		observer.observe(host);
		attach.observer = observer;
		sendResize();
	}

	function close(): void {
		const current = attach;
		attach = empty();
		current.observer?.disconnect();
		if (current.socket && current.socket.readyState <= WebSocket.OPEN) current.socket.close();
		current.terminal?.dispose();
	}

	$effect(() => {
		const id = sessionId;
		const canAttach = attachable && enabled && host !== null;
		if (!canAttach || id === '') {
			close();
			return;
		}
		open(id);
		return () => close();
	});

	onMount(() => () => close());
</script>

{#if attachable && sessionId !== ''}
	<div bind:this={host} class="h-full min-h-80 w-full" data-testid="session-terminal"></div>
{/if}
