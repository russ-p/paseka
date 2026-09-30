<script lang="ts">
	import { base } from '$app/paths';
	import { goto } from '$app/navigation';
	import { Plus, RefreshCw } from 'lucide-svelte';
	import DataTable, { type DataColumn } from '$lib/components/DataTable.svelte';
	import Section from '$lib/components/Section.svelte';
	import SessionLaunchDrawer from './SessionLaunchDrawer.svelte';
	import { createSessionStore, type SessionStore } from '$lib/stores/session.svelte';
	import { sessionDetailPath, traceDetailPath } from '$lib/navigation';
	import { sessionDuration, sessionLabel, sessionRowMeta, sessionStateLabel } from '$lib/format';
	import { formatTimestamp } from '$lib/format';
	import type { Invite, Session } from '$lib/api/types';

	let { store = createSessionStore() }: { store?: SessionStore } = $props();

	let launching = $state(false);
	let acting = $state('');
	let actionError = $state('');

	/**
	 * The store owns every timer, including the list poll that puts a session started
	 * from `paseka bee chat` on screen without anyone reloading — the legacy never
	 * polled this list, so that session stayed invisible until Refresh was pressed.
	 */
	$effect(() => {
		store.start();
		return () => store.stop();
	});

	/**
	 * The session id is the row's identity, so the id is the link and the bee is the
	 * label. The legacy made each row a `<li>` with a click handler: no `href`, no
	 * `tabindex`, no role, so a session could not be selected with a keyboard at all and
	 * could not be linked to.
	 */
	const columns: DataColumn<Session>[] = [
		{
			key: 'session',
			label: 'Session',
			text: sessionLabel,
			searchText,
			href: (row) => sessionDetailPath(base, row.sessionId),
			grow: true
		},
		{
			key: 'state',
			label: 'State',
			text: (row) => sessionStateLabel(row),
			badge: (row) => ({ status: row.active ? 'active' : row.state === 'completed' ? 'completed' : 'failed', label: sessionStateLabel(row) })
		},
		{ key: 'started', label: 'Started', text: (row) => formatTimestamp(row.startedAt), secondary: true },
		{ key: 'duration', label: 'Duration', text: (row) => sessionDuration(row), align: 'right' },
		{ key: 'trace', label: 'Trail', text: (row) => row.traceId, mono: true, href: (row) => traceDetailPath(base, row.traceId), secondary: true }
	];

	/** The filter matches the id and the adapter too, which are searchable but not shown. */
	function searchText(row: Session): string {
		return `${row.sessionId} ${sessionRowMeta(row)} ${row.adapter ?? ''} ${row.runDir}`;
	}

	async function onLaunched(session: Session): Promise<void> {
		launching = false;
		await goto(sessionDetailPath(base, session.sessionId));
	}

	async function accept(invite: Invite): Promise<void> {
		acting = invite.inviteId;
		actionError = '';
		try {
			const sessionId = await store.acceptInvite(invite.inviteId);
			if (sessionId !== '') await goto(sessionDetailPath(base, sessionId));
		} catch (cause) {
			actionError = cause instanceof Error ? cause.message : String(cause);
		} finally {
			acting = '';
		}
	}

	async function reject(invite: Invite): Promise<void> {
		acting = invite.inviteId;
		actionError = '';
		try {
			await store.rejectInvite(invite.inviteId);
		} catch (cause) {
			actionError = cause instanceof Error ? cause.message : String(cause);
		} finally {
			acting = '';
		}
	}
</script>

<svelte:head>
	<title>Sessions · Queen Console Next</title>
</svelte:head>

<div class="space-y-4">
	<header class="flex flex-wrap items-end justify-between gap-3">
		<div class="min-w-0">
			<h1 class="text-2xl font-bold">Sessions</h1>
			<p class="text-base-content/70">
				Interactive bees you can talk to, and the transcript of the ones that have finished.
			</p>
		</div>
		<div class="flex items-center gap-2">
			<button type="button" class="btn btn-ghost btn-sm" onclick={() => void store.reload()}>
				<RefreshCw class="h-4 w-4" strokeWidth={2.5} />
				Refresh
			</button>
			<button type="button" class="btn btn-primary btn-sm" onclick={() => (launching = true)}>
				<Plus class="h-4 w-4" strokeWidth={2.5} />
				Launch session
			</button>
		</div>
	</header>

	{#if store.lastError}
		<div class="alert alert-error" role="alert"><span>{store.lastError}</span></div>
	{/if}

	{#if store.inviteError}
		<div class="alert alert-warning" role="alert"><span>{store.inviteError}</span></div>
	{/if}

	<Section
		title="Pending invites"
		note={store.pending.length === 0 ? 'none' : String(store.pending.length)}
		collapsible
		open={store.pending.length > 0}
	>
		{#if actionError}
			<div class="alert alert-error mb-2" role="alert"><span>{actionError}</span></div>
		{/if}
		{#if store.pending.length === 0}
			<p class="text-sm text-base-content/60">
				Nothing is waiting. An invite appears when a task needs a human to start it — from
				<code class="font-mono">paseka task create</code> with a bee that cannot run headless, or
				from a chat gateway.
			</p>
		{:else}
			<ul class="space-y-2">
				{#each store.pending as invite (invite.inviteId)}
					<li class="rounded-box border border-base-300 p-3">
						<div class="flex flex-wrap items-baseline gap-2">
							<span class="font-semibold">{invite.bee}</span>
							{#if invite.intent}
								<span class="badge badge-sm badge-ghost">{invite.intent}</span>
							{/if}
							<span class="font-mono text-xs text-base-content/60">{invite.inviteId}</span>
						</div>
						<p class="mt-1 text-sm">{invite.task}</p>
						<p class="mt-1 text-xs text-base-content/60">
							Trail <span class="font-mono">{invite.traceId}</span>
							{#if invite.artifactRef}
								· artifact <span class="font-mono">{invite.artifactRef}</span>
							{/if}
						</p>
						<div class="mt-2 flex items-center gap-2">
							<button
								type="button"
								class="btn btn-primary btn-xs"
								disabled={acting === invite.inviteId}
								onclick={() => void accept(invite)}
							>
								{acting === invite.inviteId ? 'Accepting…' : invite.intent === 'breakdown' ? 'Start breakdown' : 'Accept'}
							</button>
							<button
								type="button"
								class="btn btn-ghost btn-xs"
								disabled={acting === invite.inviteId}
								onclick={() => void reject(invite)}
							>
								Reject
							</button>
						</div>
					</li>
				{/each}
			</ul>
		{/if}
	</Section>

	<DataTable
		{columns}
		rows={store.sessions}
		rowKey={(row) => row.sessionId}
		label="Sessions"
		filterLabel="Filter sessions"
		filterPlaceholder="Bee, id, adapter, trail"
		emptyMessage="No sessions yet. Launch one, or start a bee with paseka bee chat."
		pageSize={15}
		loading={store.showSkeletons}
	/>
</div>

<!-- The page's own store, so the session the drawer starts is already in the list
     behind it rather than appearing only after the next poll. -->
<SessionLaunchDrawer
	open={launching}
	{store}
	onclose={() => (launching = false)}
	onlaunched={(session) => void onLaunched(session)}
/>
