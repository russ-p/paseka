<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import type { Snippet } from 'svelte';

	let {
		open = false,
		title = 'Delete merged leftover branches?',
		description,
		size = 'md',
		placement = 'center',
		/** A body with no focusable in it, which is what a read-only preview is. */
		bare = false,
		withFooter = true,
		children
	}: {
		open?: boolean;
		title?: string;
		description?: string;
		size?: 'md' | 'lg';
		placement?: 'center' | 'right';
		bare?: boolean;
		withFooter?: boolean;
		children?: Snippet;
	} = $props();

	function close(): void {
		open = false;
	}
</script>

<!-- The trigger lives in the harness rather than in the test because focus return is a
     contract between the dialog and whatever opened it, and a test cannot type at the
     dialog's caller. -->
<button type="button" onclick={() => (open = true)}>Open dialog</button>
<p>page behind the dialog</p>

{#snippet body()}
	{#if bare}
		<p>modal body</p>
	{:else}
		<label class="fieldset-legend" for="modal-branch">Branch</label>
		<input id="modal-branch" class="input input-bordered w-full" />
	{/if}
	{#if children}{@render children()}{/if}
{/snippet}

<!-- Two blocks rather than one with a conditional snippet: a `footer` snippet declared
     inside an `{#if}` is scoped to that block, so gating it there would pass `Modal` no
     footer prop at all rather than an empty one — the opposite of what is being tested. -->
{#if withFooter}
	<Modal {open} {title} {description} {size} {placement} onclose={close}>
		{@render body()}
		{#snippet footer()}
			<button type="button" class="btn btn-ghost btn-sm" onclick={close}>Cancel</button>
			<button type="button" class="btn btn-error btn-sm">Delete</button>
		{/snippet}
	</Modal>
{:else}
	<Modal {open} {title} {description} {size} {placement} onclose={close}>
		{@render body()}
	</Modal>
{/if}
