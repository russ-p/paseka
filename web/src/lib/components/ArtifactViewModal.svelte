<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import ArtifactHtmlBody from '$lib/components/ArtifactHtmlBody.svelte';
	import { getTraceArtifactContent } from '$lib/api/client';
	import { artifactLabel, formatSize } from '$lib/format';
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
		<!-- The reason alone leaves "too large" as the whole of what the operator knows;
		     the size is what tells them whether to raise the cap, page it, or look elsewhere. -->
		<p class="text-sm text-base-content/60">
			{content?.omitted}
			{#if content?.bytes}
				<span class="text-base-content/50">· {formatSize(content.bytes)}</span>
			{/if}
		</p>
	{:else if body === 'empty'}
		<p class="text-sm text-base-content/60">Empty file.</p>
	{:else if body === 'html'}
		<!-- Keyed by ref so a second artifact gets a fresh body element and the
		     previous one's diagrams are torn down before the new body hydrates. -->
		{#key content?.ref}
			<ArtifactHtmlBody html={content?.contentHtml ?? ''} />
		{/key}
	{:else}
		<pre class="overflow-x-auto rounded-box bg-base-200 p-2 font-mono text-xs whitespace-pre-wrap">{content?.content}</pre>
	{/if}

	{#snippet footer()}
		<button type="button" class="btn btn-sm" onclick={onclose}>Close</button>
	{/snippet}
</Modal>
