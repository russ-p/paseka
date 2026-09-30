<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import { listCues, runCue } from '$lib/api/client';
	import type { Cue, RunCueResult } from '$lib/api/types';

	let {
		open,
		onclose,
		onran
	}: {
		open: boolean;
		onclose: () => void;
		/** Fired with the published cue so the caller can refresh and toast. */
		onran?: (result: RunCueResult) => void;
	} = $props();

	let cues = $state<Cue[]>([]);
	let selectedId = $state('');
	let text = $state('');
	let traceId = $state('');
	let textTouched = $state(false);
	let loading = $state(false);
	let submitting = $state(false);
	let error = $state('');

	/** The hint is neutral until the operator has actually touched the field. */
	const textMissing = $derived(text.trim() === '');
	const textInvalid = $derived(textMissing && textTouched);
	const ready = $derived(selectedId !== '' && !textMissing && !submitting);

	async function loadCueList(): Promise<void> {
		loading = true;
		error = '';
		try {
			cues = await listCues();
			if (cues.length === 1) selectedId = cues[0].id;
		} catch (cause) {
			error = cause instanceof Error ? cause.message : String(cause);
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		if (!open) return;
		cues = [];
		selectedId = '';
		text = '';
		traceId = '';
		textTouched = false;
		error = '';
		void loadCueList();
	});

	async function submit(): Promise<void> {
		if (!ready) return;
		submitting = true;
		error = '';
		try {
			const result = await runCue(selectedId, { text: text.trim(), traceId: traceId.trim() });
			onran?.(result);
			onclose();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : String(cause);
		} finally {
			submitting = false;
		}
	}

	const selectedCue = $derived(cues.find((cue) => cue.id === selectedId));
</script>

<Modal
	{open}
	title="Run cue"
	description="Publish a SIGNAL to the colony. The cue's bee picks it up and a trace opens."
	onclose={onclose}
>
	{#if error}
		<div class="alert alert-error" role="alert"><span>{error}</span></div>
	{/if}

	{#if loading}
		<div class="space-y-2" aria-busy="true">
			<span class="skeleton block h-3 w-full"></span>
			<span class="skeleton block h-3 w-2/3"></span>
		</div>
	{:else}
		<div class="fieldset">
			<label class="fieldset-legend" for="cue-run-cue">Cue</label>
			<select id="cue-run-cue" class="select select-bordered w-full" bind:value={selectedId}>
				<option value="" disabled>Select a cue</option>
				{#each cues as cue (cue.id)}
					<option value={cue.id}>{cue.id}{cue.description ? ` — ${cue.description}` : ''}</option>
				{/each}
			</select>
			{#if selectedCue?.standingTrace}
				<span class="fieldset-label">Standing trace {selectedCue.standingTrace}</span>
			{/if}
		</div>

		<div class="fieldset">
			<label class="fieldset-legend" for="cue-run-text">Text</label>
			<textarea
				id="cue-run-text"
				class="textarea textarea-bordered min-h-24 w-full"
				placeholder="What should the colony work on?"
				aria-invalid={textInvalid}
				aria-describedby="cue-run-text-hint"
				oninput={() => (textTouched = true)}
				bind:value={text}
			></textarea>
			<span id="cue-run-text-hint" class="fieldset-label" class:text-error={textInvalid}>
				{textInvalid
					? 'Text is required'
					: textMissing
						? 'Published to the cue bee as a SIGNAL.'
						: 'Says what the colony should work on.'}
			</span>
		</div>

		<div class="fieldset">
			<label class="fieldset-legend" for="cue-run-trace">Trace ID</label>
			<input
				id="cue-run-trace"
				type="text"
				class="input input-bordered w-full"
				placeholder="Optional — a standing trail continues it"
				bind:value={traceId}
			/>
		</div>
	{/if}

	{#snippet footer()}
		<button type="button" class="btn btn-ghost btn-sm" onclick={onclose}>Cancel</button>
		<button
			type="button"
			class="btn btn-primary btn-sm"
			disabled={!ready}
			onclick={() => void submit()}
		>
			{submitting ? 'Publishing…' : 'Publish'}
		</button>
	{/snippet}
</Modal>
