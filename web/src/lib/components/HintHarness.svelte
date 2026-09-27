<script lang="ts">
	import Hint from '$lib/components/Hint.svelte';
	import type { Snippet } from 'svelte';

	let {
		lines = ['Loaded from the home config.'],
		children
	}: {
		lines?: string[];
		children?: Snippet;
	} = $props();
</script>

<!-- `Hint` wraps whatever it is given rather than owning the content, so a test has to
     supply some — and the anchor is a plain span, which is what the topbar and
     `MetaList` both pass it. -->
<span class="inline-flex">
	<Hint {lines}>
		<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
		<span tabindex="0">main</span>
		{#if children}{@render children()}{/if}
	</Hint>
</span>
<!-- A second control so a keyboard-driven test can move focus *off* the hint, which
     is the only way to see the blur half of the contract without calling it directly. -->
<button type="button">next control</button>
