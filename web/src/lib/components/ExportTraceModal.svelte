<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import { exportTrace } from '$lib/api/client';
	import { downloadBlob } from '$lib/download';
	import { traceExportFormats, traceExportIncludes } from '$lib/api/types';
	import type { TraceExportFormat, TraceExportInclude } from '$lib/api/types';

	let {
		open,
		traceId,
		onclose,
		onexported
	}: {
		open: boolean;
		traceId: string;
		onclose: () => void;
		/** Fired with the saved file's name so the caller can toast the receipt. */
		onexported?: (filename: string) => void;
	} = $props();

	let format = $state<TraceExportFormat>('html');
	let include = $state<TraceExportInclude[]>([]);
	let exporting = $state(false);
	let error = $state('');

	/**
	 * Every open starts from the defaults: the report is the trail itself, and the
	 * optional slices are choices the operator makes for this one download rather
	 * than a setting that outlives it.
	 */
	$effect(() => {
		if (!open) return;
		format = 'html';
		include = [];
		exporting = false;
		error = '';
	});

	function toggleInclude(value: TraceExportInclude): void {
		include = include.includes(value)
			? include.filter((item) => item !== value)
			: [...include, value];
	}

	async function submit(): Promise<void> {
		if (exporting) return;
		exporting = true;
		error = '';
		try {
			const { blob, filename } = await exportTrace(traceId, { format, include });
			downloadBlob(blob, filename);
			onexported?.(filename);
			onclose();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : String(cause);
		} finally {
			exporting = false;
		}
	}
</script>

<Modal
	{open}
	title="Export trail"
	description="Download a self-contained report of this trail — the same one `paseka export` writes."
	onclose={onclose}
>
	{#if error}
		<div class="alert alert-error" role="alert"><span>{error}</span></div>
	{/if}

	<fieldset class="fieldset">
		<legend class="fieldset-legend">Format</legend>
		<div class="flex flex-wrap gap-4">
			{#each traceExportFormats as option (option.value)}
				<label class="flex items-center gap-2">
					<input
						type="radio"
						name="trace-export-format"
						class="radio radio-primary radio-sm"
						value={option.value}
						checked={format === option.value}
						onchange={() => (format = option.value)}
					/>
					<span>{option.label}</span>
				</label>
			{/each}
		</div>
	</fieldset>

	<fieldset class="fieldset">
		<legend class="fieldset-legend">Include</legend>
		<p class="fieldset-label">
			The report always carries the trail itself; add the slices an operator asked for.
		</p>
		<div class="space-y-2">
			{#each traceExportIncludes as option (option.value)}
				<label class="flex items-start gap-2">
					<input
						type="checkbox"
						class="checkbox checkbox-primary checkbox-sm mt-0.5"
						checked={include.includes(option.value)}
						onchange={() => toggleInclude(option.value)}
					/>
					<span>
						<span class="block text-sm">{option.label}</span>
						<span class="block text-xs text-base-content/60">{option.description}</span>
					</span>
				</label>
			{/each}
		</div>
	</fieldset>

	{#snippet footer()}
		<button type="button" class="btn btn-ghost btn-sm" onclick={onclose}>Cancel</button>
		<button
			type="button"
			class="btn btn-primary btn-sm"
			disabled={exporting}
			onclick={() => void submit()}
		>
			{exporting ? 'Exporting…' : 'Export'}
		</button>
	{/snippet}
</Modal>
