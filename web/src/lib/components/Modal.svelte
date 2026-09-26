<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		open,
		title,
		description,
		size = 'md',
		onclose,
		children,
		footer
	}: {
		open: boolean;
		title: string;
		description?: string;
		/** `md` fits a short form; `lg` a document preview. The body scrolls either way. */
		size?: 'md' | 'lg';
		onclose: () => void;
		children: Snippet;
		footer?: Snippet;
	} = $props();

	const sizeClass = $derived(size === 'lg' ? 'max-w-3xl' : 'max-w-md');

	let dialog = $state<HTMLElement | null>(null);
	let trigger = $state<HTMLElement | null>(null);

	$effect(() => {
		if (!open) return;
		trigger = document.activeElement as HTMLElement | null;
		const focusable = dialog?.querySelectorAll<HTMLElement>(
			'button:not([disabled]), a[href], input:not([disabled]), select:not([disabled]), textarea:not([disabled])'
		);
		(focusable?.[0] ?? dialog)?.focus();
		return () => trigger?.focus();
	});

	function focusableElements(): HTMLElement[] {
		if (!dialog) return [];
		return Array.from(
			dialog.querySelectorAll<HTMLElement>(
				'button:not([disabled]), a[href], input:not([disabled]), select:not([disabled]), textarea:not([disabled])'
			)
		);
	}

	function handleKeydown(event: KeyboardEvent): void {
		if (!open) return;
		if (event.key === 'Escape') {
			event.preventDefault();
			onclose();
			return;
		}
		if (event.key !== 'Tab') return;

		const elements = focusableElements();
		if (elements.length === 0) return;
		const first = elements[0];
		const last = elements[elements.length - 1];
		if (event.shiftKey && document.activeElement === first) {
			event.preventDefault();
			last.focus();
		} else if (!event.shiftKey && document.activeElement === last) {
			event.preventDefault();
			first.focus();
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
	<div class="fixed inset-0 z-50 flex items-center justify-center p-4">
		<button
			type="button"
			class="absolute inset-0 cursor-default bg-neutral/40"
			aria-label="Close dialog"
			onclick={onclose}
		></button>
		<div
			bind:this={dialog}
			class="card relative flex max-h-[90vh] w-full flex-col {sizeClass} bg-base-100 shadow-xl"
			role="dialog"
			aria-modal="true"
			aria-label={title}
			tabindex="-1"
		>
			<div class="card-body min-h-0 grow gap-4 overflow-y-auto">
				<div class="space-y-1">
					<h2 class="card-title">{title}</h2>
					{#if description}
						<p class="text-sm text-base-content/70">{description}</p>
					{/if}
				</div>
				{@render children()}
			</div>
			{#if footer}
				<div class="card-actions shrink-0 justify-end border-t border-base-300 px-6 py-3">
					{@render footer()}
				</div>
			{/if}
		</div>
	</div>
{/if}
