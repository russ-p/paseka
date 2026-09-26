<script lang="ts">
	import { base } from '$app/paths';
	import { goto } from '$app/navigation';
	import { Maximize2, Minimize2, Play, Plug, Square } from 'lucide-svelte';
	import MetaList from '$lib/components/MetaList.svelte';
	import Section from '$lib/components/Section.svelte';
	import SessionTerminal from '$lib/components/SessionTerminal.svelte';
	import SessionTranscript from '$lib/components/SessionTranscript.svelte';
	import { createSessionStore, type SessionStore } from '$lib/stores/session.svelte';
	import { toastStore, type ToastStore } from '$lib/stores/toast.svelte';
	import { consolePath, runDetailPath, sessionDetailPath, traceDetailPath } from '$lib/navigation';
	import {
		sessionIdentityRows,
		sessionRelayBlocker,
		sessionResumeBlocker,
		sessionStateLabel
	} from '$lib/format';
	import type { TerminalStatus } from '$lib/components/SessionTerminal.svelte';

	let {
		sessionId = '',
		store = createSessionStore(),
		toasts = toastStore
	}: {
		sessionId?: string;
		store?: SessionStore;
		toasts?: ToastStore;
	} = $props();

	let link = $state<{ state: 'connecting' | 'connected' | 'closed' | 'exited'; reason: string }>({
		state: 'connecting',
		reason: ''
	});
	let focused = $state(false);
	/** The page's Detach, as a prop rather than a call into the child. */
	let attached = $state(true);
	let continuation = $state('');
	let acting = $state('');
	let actionError = $state('');
	/** The stop a human is being asked to confirm, so the dialog is about this session. */
	let confirmingStop = $state(false);

	const session = $derived(store.current);
	/**
	 * The relay is always tried for a live session, because the client cannot know in
	 * advance whether this process owns the PTY: a session started by `paseka bee chat` in
	 * another terminal is `active` with a live PID and looks identical here. The server
	 * says so on connect, and `sessionRelayBlocker` turns that into something an operator
	 * can act on — which is the whole difference from the legacy, where the same failure
	 * left a dead terminal under a raw Go error string with Stop and Resume hidden.
	 */
	const attachable = $derived(session?.active === true);
	const relayBlocker = $derived(sessionRelayBlocker(link.reason));
	/** Resume is a server rule, and the reason is shown rather than the control vanishing. */
	const resumeBlocker = $derived(session ? sessionResumeBlocker(session) : '');
	const rows = $derived(
		sessionIdentityRows(
			session,
			(traceId) => traceDetailPath(base, traceId),
			(traceId, agentId) => runDetailPath(base, traceId, agentId)
		)
	);

	$effect(() => {
		store.start();
		return () => store.stop();
	});

	$effect(() => {
		if (sessionId !== '') void store.select(sessionId);
	});

	$effect(() => {
		// Focus mode is a mode, not a route: a live terminal is nobody else's to link to,
		// and the URL must keep saying which session this is.
		if (!focused) return;
		const onKey = (event: KeyboardEvent) => {
			if (event.key === 'Escape') focused = false;
		};
		document.addEventListener('keydown', onKey);
		return () => document.removeEventListener('keydown', onKey);
	});

	async function stop(): Promise<void> {
		if (!session) return;
		acting = 'stop';
		actionError = '';
		try {
			const result = await store.stopSession(session.sessionId);
			confirmingStop = false;
			// Which of the two things happened is the whole point: a session in this
			// process is killed outright, one in another gets a SIGTERM it can act on.
			// The legacy threw this away, so both looked identical.
			attached = false;
			toasts.push(
				result.status === 'stopped' ? 'warning' : 'success',
				result.status === 'stopped'
					? 'Killed. The process was stopped in place, with no chance to clean up.'
					: 'Signalled. The session was asked to exit, and can finish what it was doing.'
			);
		} catch (cause) {
			actionError = cause instanceof Error ? cause.message : String(cause);
		} finally {
			acting = '';
		}
	}

	async function resume(): Promise<void> {
		if (!session || resumeBlocker !== '') return;
		acting = 'resume';
		actionError = '';
		try {
			const created = await store.resumeSession(session.sessionId, continuation.trim());
			continuation = '';
			toasts.push('success', 'Resumed as a new session.');
			// A resume is a *new* session with its own id, so this is a navigation rather
			// than a re-read: the old page would describe a session that no longer runs.
			await goto(sessionDetailPath(base, created.sessionId));
		} catch (cause) {
			actionError = cause instanceof Error ? cause.message : String(cause);
		} finally {
			acting = '';
		}
	}

	function onTerminalStatus(next: TerminalStatus): void {
		link = next;
	}
</script>

<svelte:head>
	<title>Session · Queen Console Next</title>
</svelte:head>

<div class="space-y-4">
	<header class="flex flex-wrap items-start justify-between gap-3">
		<div class="min-w-0 space-y-1">
			<a class="link text-xs text-base-content/60" href={consolePath(base, '/sessions')}>← All sessions</a>
			<h1 class="text-2xl font-bold">
				{session?.bee ?? 'Session'}
				{#if session}
					<span class="badge badge-lg align-middle {session.active ? 'badge-success' : 'badge-ghost'}">
						{sessionStateLabel(session)}
					</span>
				{/if}
			</h1>
			{#if session}
				<p class="font-mono text-sm break-all text-base-content/60">{session.sessionId}</p>
				{#if store.detached}
					<p class="text-xs text-base-content/60">
						Not in the recent window — this session was read on its own.
					</p>
				{/if}
			{/if}
		</div>

		<div class="flex flex-wrap items-center gap-2">
			{#if session?.active}
				<!--
				     Stop is up here because it applies to a running session and nothing
				     else does. Resume is *not*: it needs a continuation typed next to it,
				     so it lives with that box at the foot of the page rather than as a
				     second button that does the same thing from the other end of the screen.
				-->
				<button
					type="button"
					class="btn btn-error btn-sm"
					disabled={acting !== ''}
					onclick={() => (confirmingStop = true)}
				>
					<Square class="h-4 w-4" strokeWidth={2.5} />
					Stop
				</button>
			{/if}
		</div>
	</header>

	{#if store.detailError}
		<div class="alert alert-error" role="alert"><span>{store.detailError}</span></div>
	{/if}

	{#if actionError}
		<div class="alert alert-error" role="alert"><span>{actionError}</span></div>
	{/if}

	{#if confirmingStop && session}
		<!-- Stopping is one click on the legacy, with nothing said and no way back: a
		     local stop is `Process.Kill`, so a live agent is gone before the operator has
		     decided. A confirmation is the difference between a control and a trap. -->
		<div class="alert alert-warning" role="alert">
			<div class="space-y-2">
				<span>
					Stop <span class="font-mono">{session.sessionId}</span>? The agent gets no chance to
					finish what it is doing.
				</span>
				<div class="flex items-center gap-2">
					<button
						type="button"
						class="btn btn-error btn-sm"
						disabled={acting === 'stop'}
						onclick={() => void stop()}
					>
						{acting === 'stop' ? 'Stopping…' : 'Stop it'}
					</button>
					<button type="button" class="btn btn-ghost btn-sm" onclick={() => (confirmingStop = false)}>
						Keep it running
					</button>
				</div>
			</div>
		</div>
	{/if}

	{#if store.detailLoading && !session}
		<div class="space-y-2" aria-busy="true">
			<span class="skeleton block h-3 w-1/3"></span>
			<span class="skeleton block h-96 w-full"></span>
		</div>
	{:else if !session}
		<p class="text-base-content/70">
			This session is not in the colony's recent window and could not be read.
		</p>
	{:else}
		<Section title="Identity">
			<MetaList {rows} label="Session identity" />
		</Section>

		{#if session.active}
			<!--
			     The legacy's "Widen" button collapsed the whole page to one column, hid the
			     launch form and the session list, and left that same button as the only way
			     back. Focus mode here changes the terminal's *box* and nothing else: the list
			     is never gone, the URL still says which session this is, and Escape leaves.

			     One `SessionTerminal` in both states, deliberately. A second one inside an
			     overlay would open a second relay subscription to the same PTY, so the agent
			     would have two terminals answering for one and every keystroke would arrive
			     twice.
			-->
			<Section
				title="Terminal"
				note={link.state === 'connected' ? 'connected' : link.state === 'exited' ? 'ended' : link.state}
				class={focused ? 'fixed inset-0 z-50 flex flex-col rounded-none border-0 bg-base-100 p-3' : ''}
			>
				{#snippet actions()}
					<button
						type="button"
						class="btn btn-ghost btn-xs"
						onclick={() => (attached = !attached)}
					>
						<Plug class="h-3.5 w-3.5" strokeWidth={2.5} />
						{attached ? 'Detach' : 'Reconnect'}
					</button>
					<button
						type="button"
						class="btn btn-ghost btn-xs"
						aria-pressed={focused}
						onclick={() => (focused = !focused)}
					>
						{#if focused}
							<Minimize2 class="h-3.5 w-3.5" strokeWidth={2.5} />
							Leave full screen
						{:else}
							<Maximize2 class="h-3.5 w-3.5" strokeWidth={2.5} />
							Full screen
						{/if}
					</button>
				{/snippet}

				{#if !attachable}
					<div class="alert alert-warning" role="alert">
						<span>This session is no longer running, so there is no terminal to attach to.</span>
					</div>
				{:else if relayBlocker !== ''}
					<div class="alert alert-warning" role="alert">
						<span>{relayBlocker}</span>
					</div>
				{:else if link.state === 'exited'}
					<div class="alert alert-error" role="alert">
						<span>
							The terminal relay ended{link.reason === '' ? '' : `: ${link.reason}`}
						</span>
					</div>
				{:else if link.state === 'closed'}
					<div class="alert alert-warning" role="alert">
						<span>
							The relay closed. <button
									type="button"
									class="link"
									onclick={() => void store.reload()}>Reconnect</button
								> — the agent is still running, the console just stopped listening.
						</span>
					</div>
				{/if}

				{#if focused}
					<p class="mb-2 shrink-0 text-xs text-base-content/50">
						Full screen · Escape or the button leaves
					</p>
				{/if}
				<div class={focused ? 'min-h-0 grow' : ''}>
					<SessionTerminal
						sessionId={session.sessionId}
						{attachable}
						enabled={attached}
						onstatus={onTerminalStatus}
					/>
				</div>
			</Section>
		{:else}
			<Section title="Transcript" note={`${store.lines.length} lines`}>
				<SessionTranscript
					lines={store.lines}
					loading={store.transcriptLoading}
					error={store.transcriptError}
					dropped={store.droppedLines}
				/>
			</Section>

			<Section title="Continue this session">
				{#if resumeBlocker !== ''}
					<p class="text-sm text-base-content/60">{resumeBlocker}</p>
				{:else}
					<form
						class="flex flex-wrap items-end gap-2"
						onsubmit={(event) => {
							event.preventDefault();
							void resume();
						}}
					>
						<label class="fieldset grow">
							<span class="fieldset-legend text-xs">Continue with (optional)</span>
							<input
								type="text"
								class="input input-bordered w-full"
								placeholder="What it should do next"
								bind:value={continuation}
							/>
						</label>
						<button type="submit" class="btn btn-primary btn-sm" disabled={acting !== ''}>
							<Play class="h-4 w-4" strokeWidth={2.5} />
							{acting === 'resume' ? 'Resuming…' : 'Resume'}
						</button>
					</form>
					<p class="mt-2 text-xs text-base-content/60">
						A resume starts a <span class="font-mono">new</span> session continuing the provider's
						conversation, so this one stays on record and the new one links back to it. Needs
						NATS and a running <code class="font-mono">paseka run</code>.
					</p>
				{/if}
			</Section>
		{/if}
	{/if}
</div>
