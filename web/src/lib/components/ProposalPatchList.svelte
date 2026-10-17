<script lang="ts">
	import { getTraceProposalPatchContent, listTraceProposalPatches } from '$lib/api/client';
	import type { ProposalPatchContent, ProposalPatchList } from '$lib/api/types';
	import { formatSize, formatTimestamp, proposalPatchLabel, proposalPatchNote } from '$lib/format';

	let { traceId = '' }: { traceId?: string } = $props();

	/** One read in flight; a second would race the first at the same boundary. */
	let loading = $state(true);
	let error = $state('');
	let listed = $state<ProposalPatchList | null>(null);
	/** The agent whose body is open, so a row and its body cannot disagree. */
	let openAgent = $state('');
	let content = $state<ProposalPatchContent | null>(null);
	let contentLoading = $state(false);
	let contentError = $state('');

	const patches = $derived(listed?.patches ?? []);
	const diagnostics = $derived(listed?.diagnostics ?? []);

	$effect(() => {
		const id = traceId;
		openAgent = '';
		content = null;
		contentError = '';
		if (!id) {
			loading = false;
			listed = null;
			return;
		}
		loading = true;
		error = '';
		void listTraceProposalPatches(id)
			.then((view) => {
				if (traceId !== id) return;
				listed = view;
			})
			.catch((cause: unknown) => {
				if (traceId !== id) return;
				error = cause instanceof Error ? cause.message : String(cause);
			})
			.finally(() => {
				if (traceId === id) loading = false;
			});
	});

	function toggle(agentId: string): void {
		if (openAgent === agentId) {
			openAgent = '';
			content = null;
			contentError = '';
			return;
		}
		openAgent = agentId;
		content = null;
		contentError = '';
		contentLoading = true;
		void getTraceProposalPatchContent(traceId, agentId)
			.then((view) => {
				if (openAgent !== agentId) return;
				content = view;
			})
			.catch((cause: unknown) => {
				if (openAgent !== agentId) return;
				contentError = cause instanceof Error ? cause.message : String(cause);
			})
			.finally(() => {
				if (openAgent === agentId) contentLoading = false;
			});
	}
</script>

<div class="space-y-3">
	<p class="text-xs text-base-content/60">
		What the diff collector returned when each run finished. Not a strict per-run delta, not a workspace
		backup, and not proof of the code a Guard approved.
	</p>

	{#if error}
		<div class="alert alert-error" role="alert"><span>{error}</span></div>
	{:else if loading}
		<div class="space-y-2" aria-busy="true">
			<span class="skeleton block h-3 w-1/3"></span>
			<span class="skeleton block h-3 w-1/2"></span>
		</div>
	{:else if patches.length === 0}
		<p class="text-sm text-base-content/70">No proposal patches archived for this trail.</p>
	{:else}
		<ul class="space-y-2">
			{#each patches as patch (patch.agentId)}
				<li class="rounded-box border border-base-300">
					<button
						type="button"
						class="flex w-full flex-wrap items-center justify-between gap-2 px-3 py-2 text-left"
						aria-expanded={openAgent === patch.agentId}
						onclick={() => toggle(patch.agentId)}
					>
						<span class="flex min-w-0 flex-wrap items-baseline gap-x-2">
							<span class="truncate font-mono text-sm">{patch.agentId}</span>
							<span class="text-xs text-base-content/60">{patch.bee}</span>
							<span class="text-xs text-base-content/50">{proposalPatchLabel(patch)}</span>
							<!-- Publication is what the broker confirmed at capture time, and it is
							     a separate fact from approval: nothing here is reviewed state. -->
							<span class="text-xs text-base-content/50">{proposalPatchNote(patch.publication)}</span>
						</span>
						<span class="shrink-0 text-xs text-base-content/50">
							{formatTimestamp(patch.capturedAt)}
							· {patch.fileCount ?? '?'} file{patch.fileCount === 1 ? '' : 's'}
							· {formatSize(patch.bytes)}
						</span>
					</button>

					{#if openAgent === patch.agentId}
						<div class="space-y-2 border-t border-base-300 px-3 py-2">
							{#if patch.summary}
								<p class="text-sm whitespace-pre-wrap">{patch.summary}</p>
							{/if}
							<p class="text-xs text-base-content/50">
								<code>{patch.ref}</code>
								{#if patch.baseSha}· base <code>{patch.baseSha}</code>{/if}
								{#if patch.taskId}· task <code>{patch.taskId}</code>{/if}
								{#if patch.worktreePath}· worktree <code>{patch.worktreePath}</code>{/if}
							</p>
							{#if patch.publishError}
								<div class="alert alert-warning" role="alert">
									<span>Last publish error: {patch.publishError}</span>
								</div>
							{/if}
							{#if contentLoading}
								<div class="space-y-2" aria-busy="true">
									<span class="skeleton block h-3 w-full"></span>
									<span class="skeleton block h-24 w-full"></span>
								</div>
							{:else if contentError}
								<div class="alert alert-error" role="alert"><span>{contentError}</span></div>
							{:else if content?.omitted}
								<!-- The reason and the size together are what make the next
								     decision possible; a truncated prefix is never dressed up as
								     the original patch. -->
								<p class="text-sm text-base-content/60">
									{content.omitted}
									<span class="text-base-content/50">· {formatSize(content.bytes)}</span>
								</p>
							{:else if content}
								{#if content.truncated}
									<div class="alert alert-warning" role="alert">
										<span>Preview truncated at the server's size cap — this is not the whole patch.</span>
									</div>
								{/if}
								<!-- Escaped text, never markup: a patch body is repository content,
								     not something this page interprets. -->
								<pre class="max-h-96 overflow-auto rounded-box bg-base-200/50 p-3 font-mono text-xs whitespace-pre-wrap">{content.content}</pre>
							{/if}
						</div>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}

	{#if diagnostics.length > 0}
		<div class="alert alert-warning" role="alert">
			<div>
				<p class="font-medium">{diagnostics.length} incomplete patch record(s) on disk</p>
				<ul class="list-inside list-disc text-sm">
					{#each diagnostics as diagnostic (diagnostic.agentId)}
						<li><span class="font-mono">{diagnostic.agentId}</span> — {diagnostic.reason}</li>
					{/each}
				</ul>
			</div>
		</div>
	{/if}
</div>