<script module lang="ts">
	let hintSeq = 0;
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';

	let { lines, children }: { lines: string[]; children: Snippet } = $props();

	const id = `paseka-hint-${hintSeq++}`;

	let open = $state(false);
	let top = $state(0);
	let left = $state(0);
	let tip = $state<HTMLElement | null>(null);

	/**
	 * The tooltip is portaled to `body` so a scrolling ancestor (the topbar row)
	 * cannot clip it, then placed below the anchor and flipped above when the
	 * viewport has no room. The two numbers are geometry, not styling.
	 */
	function place(anchor: HTMLElement, node: HTMLElement): void {
		const box = anchor.getBoundingClientRect();
		const size = node.getBoundingClientRect();
		const gap = 6;
		const below = box.bottom + gap;
		top = below + size.height <= window.innerHeight ? below : Math.max(8, box.top - gap - size.height);
		left = Math.min(Math.max(8, box.left), Math.max(8, window.innerWidth - size.width - 8));
	}

	function portal(node: HTMLElement): { destroy: () => void } {
		document.body.appendChild(node);
		return { destroy: () => node.remove() };
	}

	function show(event: Event): void {
		const anchor = event.currentTarget as HTMLElement | null;
		if (!anchor) return;
		open = true;
		if (tip) place(anchor, tip);
	}

	function hide(): void {
		open = false;
	}
</script>

<span
	class="block w-full"
	role="presentation"
	data-hint={id}
	onmouseenter={show}
	onmouseleave={hide}
	onfocusin={show}
	onfocusout={hide}
>
	{@render children()}
</span>

<div
	bind:this={tip}
	{id}
	use:portal
	class="pointer-events-none fixed z-50 rounded-box border border-base-300 bg-base-100 px-2 py-1 text-xs whitespace-nowrap shadow-md transition-opacity duration-75"
	class:opacity-100={open}
	class:opacity-0={!open}
	style="top: {top}px; left: {left}px"
	aria-hidden="true"
>
	{#each lines as line}
		<span class="block">{line}</span>
	{/each}
</div>
