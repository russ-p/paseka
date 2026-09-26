<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import { getTraceArtifactContent } from '$lib/api/client';
	import { artifactLabel } from '$lib/format';
	import type { ArtifactContent, ArtifactView } from '$lib/api/types';

	let {
		open,
		traceId,
		artifact,
		onclose
	}: {
		open: boolean;
		traceId: string;
		/** The comb file to read; `null` while nothing is selected. */
		artifact: ArtifactView | null;
		onclose: () => void;
	} = $props();

	let content = $state<ArtifactContent | null>(null);
	let loading = $state(false);
	let error = $state('');
	/** The ref the in-flight read belongs to, so a late answer for a row the operator already left is dropped. */
	let requested = $state('');

	/**
	 * Markdown arrives as server-rendered HTML; anything else as text. The
	 * server refuses to inline binary or oversized bodies, so `omitted` replaces
	 * both fields rather than truncating them — there is nothing to page.
	 */
	const body = $derived.by(() => {
		if (loading) return 'loading';
		if (error) return 'error';
		if (!content) return 'empty';
		if (content.omitted) return 'omitted';
		if (content.contentHtml) return 'html';
		if (content.content) return 'text';
		return 'empty';
	});

	$effect(() => {
		if (!open || !artifact) return;
		const ref = artifact.ref;
		requested = ref;
		content = null;
		error = '';
		loading = true;
		void getTraceArtifactContent(traceId, ref)
			.then((view) => {
				if (requested !== ref) return;
				content = view;
			})
			.catch((cause: unknown) => {
				if (requested !== ref) return;
				error = cause instanceof Error ? cause.message : String(cause);
			})
			.finally(() => {
				if (requested === ref) loading = false;
			});
	});

	const title = $derived(artifact ? artifactLabel(artifact) : 'Artifact');
	const subtitle = $derived(artifact ? `${artifact.artifactKind} · ${artifact.ref}` : '');
</script>

<Modal {open} {title} description={subtitle} size="lg" {onclose}>
	{#if body === 'loading'}
		<div class="space-y-2" aria-busy="true">
			<span class="skeleton block h-3 w-full"></span>
			<span class="skeleton block h-3 w-5/6"></span>
			<span class="skeleton block h-3 w-2/3"></span>
		</div>
	{:else if body === 'error'}
		<div class="alert alert-error" role="alert"><span>{error}</span></div>
	{:else if body === 'omitted'}
		<p class="text-sm text-base-content/60">{content?.omitted}</p>
	{:else if body === 'empty'}
		<p class="text-sm text-base-content/60">Empty file.</p>
	{:else if body === 'html'}
		<div
			class="artifact-body text-sm [&_a]:link [&_blockquote]:border-l-2 [&_blockquote]:border-base-300 [&_blockquote]:pl-3 [&_code]:font-mono [&_code]:text-xs [&_h1]:mt-2 [&_h1]:text-lg [&_h1]:font-bold [&_h2]:mt-2 [&_h2]:text-base [&_h2]:font-semibold [&_h3]:mt-2 [&_h3]:font-semibold [&_li]:ml-4 [&_li]:list-disc [&_ol>li]:list-decimal [&_p]:my-2 [&_p]:leading-relaxed [&_pre]:overflow-x-auto [&_pre]:rounded-box [&_pre]:bg-base-200 [&_pre]:p-2 [&_table]:mt-2 [&_table]:text-xs [&_td]:border [&_td]:border-base-300 [&_td]:px-2 [&_td]:py-1 [&_th]:border [&_th]:border-base-300 [&_th]:px-2 [&_th]:py-1"
		>
			<!-- `contentHtml` is goldmark output rendered by this binary from the trail's own comb, not operator input. -->
			{@html content?.contentHtml}
		</div>
	{:else}
		<pre class="overflow-x-auto rounded-box bg-base-200 p-2 font-mono text-xs whitespace-pre-wrap">{content?.content}</pre>
	{/if}

	{#snippet footer()}
		<button type="button" class="btn btn-sm" onclick={onclose}>Close</button>
	{/snippet}
</Modal>
