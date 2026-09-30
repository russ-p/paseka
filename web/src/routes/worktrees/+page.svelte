<script lang="ts">
	import { base } from '$app/paths';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { DataColumn } from '$lib/components/DataTable.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { gitActionLabel, gitActionMessage, gitWorktreeState, worktreesHeadline } from '$lib/format';
	import { traceDetailPath } from '$lib/navigation';
	import { createGitStore, type GitStore } from '$lib/stores/git.svelte';
	import { toastStore, type ToastStore } from '$lib/stores/toast.svelte';
	import type { GitWorktree } from '$lib/api/types';

	/**
	 * The worktree list is part of `GET /api/git`, and Git is the other route that
	 * reads it, so both use the same store: the two are never mounted at once, which
	 * is what keeps one endpoint on one poller.
	 */
	let { store = createGitStore(), toasts = toastStore }: { store?: GitStore; toasts?: ToastStore } =
		$props();

	$effect(() => {
		store.start();
		return () => store.stop();
	});

	/** Prune destroys local state, so it confirms first; the sync actions never do. */
	let confirming = $state(false);

	const worktrees = $derived(store.git?.worktrees ?? []);

	const worktreeColumns: DataColumn<GitWorktree>[] = [
		{
			key: 'branch',
			label: 'Branch',
			text: (worktree) => worktree.branch || '—',
			// The path is `<colony root>/.paseka/worktrees/<trace id>`, so it is derivable
			// from the row itself; searching it beats a column wide enough to crowd out
			// the columns an operator scans.
			searchText: (worktree) => worktree.path,
			mono: true,
			grow: true
		},
		{
			key: 'trail',
			label: 'Trail',
			text: (worktree) => worktree.traceId || '—',
			// An unregistered worktree has no trail to open, so the cell stays plain text
			// rather than a link to nothing.
			href: (worktree) => (worktree.traceId ? traceDetailPath(base, worktree.traceId) : null)
		},
		{
			key: 'state',
			label: 'State',
			text: (worktree) => gitWorktreeState(worktree).label,
			badge: (worktree) => gitWorktreeState(worktree)
		},
		{
			key: 'pr',
			label: 'Pull request',
			text: (worktree) => (worktree.prUrl ? 'open' : ''),
			href: (worktree) => worktree.prUrl ?? null,
			// The declared-but-null badge is what keeps a worktree with no PR an empty
			// cell; without it the link's fallback text would read as a dead `open`.
			badge: () => null,
			secondary: true
		},
		{
			key: 'base',
			label: 'Base SHA',
			text: (worktree) => worktree.baseSha?.slice(0, 8) || '—',
			mono: true,
			secondary: true
		}
	];

	async function prune(): Promise<void> {
		const outcome = await store.run('prune');
		// A refused overlap is not an error; the action that was already running owns the report.
		if (outcome.status === 'busy') return;
		// The dialog stays open on failure and says why in place, so there is no toast for it.
		if (outcome.status === 'failed') return;
		confirming = false;
		toasts.push(
			outcome.result.ok ? 'success' : 'warning',
			gitActionMessage(outcome.result, 'No orphan worktrees')
		);
	}
</script>

<svelte:head>
	<title>Worktrees · Queen Console Next</title>
</svelte:head>

<div class="space-y-6">
	<header class="flex flex-wrap items-start justify-between gap-3">
		<!-- The measure is capped so the page action stays on the header row instead of being
		     pushed under a paragraph three screens wide. -->
		<div class="min-w-0 max-w-3xl space-y-1">
			<h1 class="text-3xl font-bold">Worktrees</h1>
			<p class="text-base-content/70">{worktreesHeadline(worktrees.length)}</p>
			<!-- What a worktree is, and what pruning does not touch, said once on the page that
			     owns the verb rather than left to a dialog the operator has to open to learn. -->
			<p class="text-xs text-base-content/50">
				A worktree is a checkout under <span class="font-mono">.paseka/worktrees</span>, so a trail
				mutates code without touching the colony root. Pruning drops the checkouts no trail claims
				any more and unregisters the ones whose directory is already gone; branches are kept.
			</p>
		</div>
		<button
			id="worktrees-prune"
			type="button"
			class="btn btn-sm"
			disabled={store.busy}
			onclick={() => (confirming = true)}
		>
			{gitActionLabel('prune', store.pending === 'prune')}
		</button>
	</header>

	{#if store.lastError}
		<div class="alert alert-error" role="alert"><span>{store.lastError}</span></div>
	{/if}

	{#if store.showSkeletons}
		<div class="space-y-3" aria-busy="true">
			<span class="skeleton block h-3 w-1/3"></span>
			<span class="skeleton block h-64 w-full"></span>
		</div>
	{:else if store.git}
		<DataTable
			label="Worktrees"
			columns={worktreeColumns}
			rows={worktrees}
			rowKey={(worktree) => worktree.path}
			emptyMessage="No colony-managed worktrees."
			filterLabel="Filter worktrees"
			filterPlaceholder="branch, trail, path"
			pageSize={10}
		/>
	{/if}
</div>

<Modal
	open={confirming}
	title="Prune orphan worktrees?"
	description="Drops checkouts under .paseka/worktrees that no trail claims any more, and unregisters the ones whose directory is already gone. Branches are kept."
	onclose={() => (confirming = false)}
>
	{#if store.actionError}
		<p class="text-sm text-error" role="alert">{store.actionError}</p>
	{/if}
	{#snippet footer()}
		<button
			id="worktrees-prune-cancel"
			type="button"
			class="btn btn-ghost btn-sm"
			onclick={() => (confirming = false)}
		>
			Cancel
		</button>
		<button
			id="worktrees-prune-confirm"
			type="button"
			class="btn btn-error btn-sm"
			disabled={store.busy}
			onclick={() => void prune()}
		>
			{gitActionLabel('prune', store.pending === 'prune')}
		</button>
	{/snippet}
</Modal>
