<script lang="ts">
	import { base } from '$app/paths';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { DataColumn } from '$lib/components/DataTable.svelte';
	import DetailRow from '$lib/components/DetailRow.svelte';
	import MetaList from '$lib/components/MetaList.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import Section from '$lib/components/Section.svelte';
	import { createGitStore, type GitStore } from '$lib/stores/git.svelte';
	import { toastStore, type ToastStore } from '$lib/stores/toast.svelte';
	import { traceDetailPath } from '$lib/navigation';
	import {
		gitActionLabel,
		gitActionMessage,
		gitBranchFlags,
		gitBranchState,
		gitCloneRows,
		gitOriginRows,
		gitWorktreeState,
		type GitAction
	} from '$lib/format';
	import type { GitBranch, GitWorktree } from '$lib/api/types';

	let { store = createGitStore(), toasts = toastStore }: { store?: GitStore; toasts?: ToastStore } = $props();

	$effect(() => {
		store.start();
		return () => store.stop();
	});

	/** The two destructive actions confirm first; the three sync ones do not. */
	let confirming = $state<GitAction | null>(null);
	let runHooks = $state(false);

	const git = $derived(store.git);
	const cloneRows = $derived(gitCloneRows(git));
	const originRows = $derived(gitOriginRows(git));
	const leftovers = $derived(git?.branches.filter((branch) => branch.leftover) ?? []);
	const worktrees = $derived(git?.worktrees ?? []);
	const branches = $derived(git?.branches ?? []);
	/** The three that move the clone; the other two destroy local state and confirm first. */
	const syncActions: GitAction[] = ['fetch', 'push', 'pull'];
	/** A button has to be worth pressing, so Push carries the primary tone only with work to publish. */
	const pushIsPrimary = $derived(store.unpublished.length > 0);

	const worktreeColumns: DataColumn<GitWorktree>[] = [
		{
			key: 'branch',
			label: 'Branch',
			text: (worktree) => worktree.branch || '—',
			// The path is `<colony root>/.paseka/worktrees/<trace id>`, so it is derivable from the
			// row itself; searching it beats a column wide enough to crowd out the others.
			searchText: (worktree) => worktree.path,
			mono: true,
			grow: true
		},
		{
			key: 'trace',
			label: 'Trace',
			text: (worktree) => worktree.traceId || '—',
			// An unregistered worktree has no trail to open, so the cell stays plain text.
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

	const branchColumns: DataColumn<GitBranch>[] = [
		{
			key: 'name',
			label: 'Branch',
			text: (branch) => branch.name,
			// The worktree path and the trail behind a branch are searchable, not on screen:
			// the Worktrees table above already shows the branch that holds the checkout.
			searchText: (branch) => [branch.worktreePath, branch.traceId].filter(Boolean).join(' '),
			mono: true
		},
		{
			key: 'state',
			label: 'State',
			text: (branch) => gitBranchState(branch)?.label ?? '',
			badge: (branch) => gitBranchState(branch)
		},
		{ key: 'flags', label: 'Flags', text: (branch) => gitBranchFlags(branch), secondary: true },
		// The subject is prose, so it is the column that takes the leftover width; giving `grow`
		// to the branch name instead would starve the subject down to its longest word.
		{ key: 'subject', label: 'Subject', text: (branch) => branch.subject || '—', grow: true }
	];

	/** What the toast says when git printed nothing of its own. */
	const doneFallback: Record<GitAction, string> = {
		fetch: 'Fetch complete',
		push: 'Push complete',
		pull: 'Pull complete',
		prune: 'No orphan worktrees',
		delete: 'Merged leftovers deleted'
	};

	async function perform(action: GitAction): Promise<void> {
		const inDialog = confirming === action;
		const outcome = await store.run(action, runHooks);
		// A refused overlap is not an error; the button that was already running owns the report.
		if (outcome.status === 'busy') return;
		// A dialog stays open on failure and shows the reason in place; a header action has no
		// such surface, so its failure goes to a toast.
		if (outcome.status === 'failed') {
			if (!inDialog) toasts.push('error', outcome.message);
			return;
		}
		confirming = null;
		// A batch delete is only as good as its worst name, so a partial success reads as a warning.
		toasts.push(outcome.result.ok ? 'success' : 'warning', gitActionMessage(outcome.result, doneFallback[action]));
	}
</script>

<svelte:head>
	<title>Git · Queen Console Next</title>
</svelte:head>

<div class="space-y-6">
	<header class="flex flex-wrap items-start justify-between gap-3">
		<div class="min-w-0 space-y-1">
			<h1 class="text-3xl font-bold">Git</h1>
			<!-- A disabled control cannot carry a tooltip, so the reason it is disabled is said here. -->
			<p class="text-base-content/70">
				{#if git && !store.hasOrigin}
					No origin remote, so there is nothing to fetch, push, or pull. Set one on the colony
					clone to enable the sync actions.
				{:else}
					Colony clone against origin, the commits it has not published, its worktrees, and its
					local branches.
				{/if}
			</p>
		</div>
		<div class="flex flex-wrap items-center gap-2">
			{#each syncActions as action (action)}
				<button
					id={`git-${action}`}
					type="button"
					class="btn btn-sm {action === 'push' && pushIsPrimary ? 'btn-primary' : ''}"
					disabled={store.busy || !store.hasOrigin}
					onclick={() => void perform(action)}
				>
					{gitActionLabel(action, store.pending === action)}
				</button>
			{/each}
			<label class="flex items-center gap-2 text-xs text-base-content/60">
				<input type="checkbox" class="toggle toggle-xs" bind:checked={runHooks} />
				Run git hooks on push
			</label>
		</div>
	</header>

	{#if store.lastError}
		<div class="alert alert-error" role="alert"><span>{store.lastError}</span></div>
	{/if}

	{#if store.showSkeletons}
		<div class="space-y-3" aria-busy="true">
			<span class="skeleton block h-3 w-full"></span>
			<span class="skeleton block h-3 w-2/3"></span>
			<span class="skeleton block h-24 w-full"></span>
		</div>
	{:else if git}
		<Section id="git-origin" title="Colony root vs origin">
			<!-- The checkout and the remote answer different questions, so each keeps its own list
			     rather than sharing a two-column grid that paired unrelated rows. -->
			<div class="grid grid-cols-1 gap-x-8 lg:grid-cols-2">
				<MetaList rows={cloneRows} label="Colony root" columns={1} />
				<MetaList rows={originRows} label="Origin" columns={1} />
			</div>
			<p class="mt-3 text-xs text-base-content/50">
				Fetch updates remote-tracking refs only. Push publishes the default branch and never
				forces. Pull is fast-forward only — a backup for when the inbound webhook sidecar did
				not update this clone. Isolated worktrees do not block Pull; a dirty root or live bees
				on the colony root do.
			</p>
		</Section>

		{#if store.unpublished.length > 0}
			<Section id="git-unpublished" title="Unpublished commits" note={`${store.unpublished.length} to push`}>
				<ul class="space-y-2">
					{#each store.unpublished as commit (commit.sha)}
						<DetailRow title={commit.subject || commit.sha} meta={commit.sha.slice(0, 8)} />
					{/each}
				</ul>
			</Section>
		{/if}

		<!-- The DataTable carries its own frame, so these blocks stay plain sections with a heading row. -->
		<section id="git-worktrees" class="space-y-3">
			<div class="flex flex-wrap items-center justify-between gap-3">
				<h2 class="text-xl font-semibold">Worktrees</h2>
				<button
					id="git-prune"
					type="button"
					class="btn btn-sm"
					disabled={store.busy}
					onclick={() => (confirming = 'prune')}
				>
					{gitActionLabel('prune', store.pending === 'prune')}
				</button>
			</div>
			<DataTable
				label="Worktrees"
				columns={worktreeColumns}
				rows={worktrees}
				rowKey={(worktree) => worktree.path}
				emptyMessage="No colony-managed worktrees."
				filterLabel="Filter worktrees"
				filterPlaceholder="branch, trace, path"
				pageSize={10}
			/>
		</section>

		<section id="git-branches" class="space-y-3">
			<div class="flex flex-wrap items-center justify-between gap-3">
				<h2 class="text-xl font-semibold">Branches</h2>
				<button
					id="git-delete-leftovers"
					type="button"
					class="btn btn-sm"
					disabled={store.busy || leftovers.length === 0}
					onclick={() => (confirming = 'delete')}
				>
					{leftovers.length > 0
						? `Delete ${leftovers.length} leftover${leftovers.length === 1 ? '' : 's'}`
						: 'No merged leftovers'}
				</button>
			</div>
			<DataTable
				label="Branches"
				columns={branchColumns}
				rows={branches}
				rowKey={(branch) => branch.name}
				emptyMessage="No local branches."
				filterLabel="Filter branches"
				filterPlaceholder="name, subject, trace"
				pageSize={15}
			/>
		</section>
	{/if}
</div>

<Modal
	open={confirming === 'prune'}
	title="Prune orphan worktrees?"
	description="Drops checkouts under .paseka/worktrees that no trail claims any more, and unregisters the ones whose directory is already gone. Branches are kept."
	onclose={() => (confirming = null)}
>
	{#if store.actionError}
		<p class="text-sm text-error" role="alert">{store.actionError}</p>
	{/if}
	{#snippet footer()}
		<button
			id="git-prune-cancel"
			type="button"
			class="btn btn-ghost btn-sm"
			onclick={() => (confirming = null)}
		>
			Cancel
		</button>
		<button
			id="git-prune-confirm"
			type="button"
			class="btn btn-error btn-sm"
			disabled={store.busy}
			onclick={() => void perform('prune')}
		>
			{gitActionLabel('prune', store.pending === 'prune')}
		</button>
	{/snippet}
</Modal>

<Modal
	open={confirming === 'delete'}
	title="Delete merged leftover branches?"
	description="Local branches only — nothing is deleted on origin, and a branch a live worktree holds is refused."
	onclose={() => (confirming = null)}
>
	<ul class="max-h-48 space-y-1 overflow-y-auto font-mono text-xs">
		{#each store.leftoverNames as name (name)}
			<li>{name}</li>
		{/each}
	</ul>
	{#if store.actionError}
		<p class="text-sm text-error" role="alert">{store.actionError}</p>
	{/if}
	{#snippet footer()}
		<button
			id="git-delete-cancel"
			type="button"
			class="btn btn-ghost btn-sm"
			onclick={() => (confirming = null)}
		>
			Cancel
		</button>
		<button
			id="git-delete-confirm"
			type="button"
			class="btn btn-error btn-sm"
			disabled={store.busy}
			onclick={() => void perform('delete')}
		>
			{gitActionLabel('delete', store.pending === 'delete')}
		</button>
	{/snippet}
</Modal>
