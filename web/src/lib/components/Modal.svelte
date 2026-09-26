<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		open,
		title,
		description,
		size = 'md',
		placement = 'center',
		onclose,
		children,
		footer
	}: {
		open: boolean;
		title: string;
		description?: string;
		/** `md` fits a short form; `lg` a document preview. The body scrolls either way. */
		size?: 'md' | 'lg';
		/**
		 * `right` fills the height and slides in from the edge, which is what a form
		 * long enough to need its own scroll wants. It is a placement, not a second
		 * dialog: the focus trap, the ESC key, and the focus return below are the
		 * same ones a centered dialog uses, which is the whole point of `Drawer`
		 * being this component rather than a new one.
		 */
		placement?: 'center' | 'right';
		onclose: () => void;
		children: Snippet;
		footer?: Snippet;
	} = $props();

	const sizeClass = $derived(size === 'lg' ? 'max-w-3xl' : 'max-w-md');
	const side = $derived(placement === 'right');
	/**
	 * The panel is mounted off-screen first and stepped in on the next frame, since
	 * a transform that is already at its final value on the first paint cannot
	 * transition. There is no exit animation: the dialog is removed outright, and a
	 * delayed close would leave a keyboard trap live after ESC.
	 */
	let slid = $state(false);
	$effect(() => {
		if (!open) {
			slid = false;
			return;
		}
		const frame = requestAnimationFrame(() => {
			slid = true;
		});
		return () => cancelAnimationFrame(frame);
	});

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
	<div
		class="fixed inset-0 z-50 flex {side
			? 'justify-end'
			: 'items-center justify-center p-4'}"
	>
		<button
			type="button"
			class="absolute inset-0 cursor-default bg-neutral/40"
			aria-label="Close dialog"
			onclick={onclose}
		></button>
		<div
			bind:this={dialog}
			class="card relative flex w-full flex-col bg-base-100 shadow-xl {side
				? `h-full max-h-none max-w-2xl transition-transform duration-200 ${slid ? 'translate-x-0' : 'translate-x-full'}`
				: `max-h-[90vh] ${sizeClass}`}"
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
