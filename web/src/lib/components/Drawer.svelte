<script lang="ts">
	import type { Snippet } from 'svelte';
	import Modal from '$lib/components/Modal.svelte';

	let {
		open,
		title,
		description,
		onclose,
		children,
		footer
	}: {
		open: boolean;
		title: string;
		description?: string;
		onclose: () => void;
		children: Snippet;
		footer?: Snippet;
	} = $props();
</script>

<!-- A panel rather than a dialog: a form with more than a couple of fields wants
     the full height, and an operator checking it against the board behind it
     should not lose the board. `Modal` owns the focus trap, so this is a
     placement and a width — never a second focus implementation. -->
<Modal {open} {title} {description} placement="right" {onclose} {footer}>
	{@render children()}
</Modal>
