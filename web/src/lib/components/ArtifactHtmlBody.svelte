<script lang="ts">
	import { findMermaidBlocks, mountDiagram, type MermaidHandle } from '$lib/mermaid';

	let { html }: { html: string } = $props();

	let body = $state<HTMLElement | null>(null);

	/**
	 * A diagram is mounted into a host beside its source, never in place of it:
	 * a failed render leaves the fenced block readable and the failure is a
	 * notice, not an empty modal.
	 */
	function wrapSource(pre: HTMLElement): {
		host: HTMLElement;
		details: HTMLDetailsElement;
		notice: HTMLElement;
	} {
		const wrapper = document.createElement('div');
		wrapper.className = 'artifact-diagram my-2 space-y-1';
		const host = document.createElement('div');
		host.className = 'artifact-diagram-host flex justify-center overflow-x-auto';
		const notice = document.createElement('div');
		notice.className = 'alert alert-warning py-1 text-xs';
		notice.setAttribute('role', 'alert');
		notice.hidden = true;
		const details = document.createElement('details');
		const summary = document.createElement('summary');
		summary.className = 'cursor-pointer text-xs text-base-content/60';
		summary.textContent = 'Mermaid source';
		pre.replaceWith(wrapper);
		details.append(summary, pre);
		wrapper.append(host, notice, details);
		return { host, details, notice };
	}

	function describe(cause: unknown): string {
		const message = cause instanceof Error ? cause.message : String(cause);
		const firstLine = message.split('\n').find((line) => line.trim() !== '');
		if (!firstLine) return 'Diagram failed to render.';
		const short = firstLine.length > 200 ? `${firstLine.slice(0, 197)}…` : firstLine;
		return `Diagram failed to render: ${short}`;
	}

	$effect(() => {
		// The HTML is read here so a new artifact tears down the previous mounts
		// before this effect hydrates the new body.
		const root = body;
		if (!root || html === '') return;
		const container: HTMLElement = root;

		let disposed = false;
		const handles: MermaidHandle[] = [];

		async function hydrate(): Promise<void> {
			// One chart at a time: a document with several diagrams is read in
			// source order, and a slow mount does not race the one below it.
			for (const code of findMermaidBlocks(container)) {
				if (disposed) return;
				const pre = code.parentElement;
				if (!pre) continue;
				const source = code.textContent ?? '';
				const { host, details, notice } = wrapSource(pre);

				let handle: MermaidHandle;
				try {
					handle = await mountDiagram(host, source);
				} catch (cause) {
					handle = { source, error: describe(cause), destroy: () => {} };
				}
				// A mount that resolves after the modal closed destroys itself
				// rather than writing into detached DOM.
				if (disposed) {
					handle.destroy();
					return;
				}
				handles.push(handle);
				if (handle.error) {
					// A successful mount keeps the source folded; a failure opens it
					// so the operator can read or copy what did not render.
					details.open = true;
					notice.textContent = describe(handle.error);
					notice.hidden = false;
				}
			}
		}

		void hydrate();

		return () => {
			disposed = true;
			for (const handle of handles) handle.destroy();
			handles.length = 0;
		};
	});
</script>

<div
	bind:this={body}
	class="artifact-body text-sm [&_a]:link [&_blockquote]:border-l-2 [&_blockquote]:border-base-300 [&_blockquote]:pl-3 [&_code]:font-mono [&_code]:text-xs [&_h1]:mt-2 [&_h1]:text-lg [&_h1]:font-bold [&_h2]:mt-2 [&_h2]:text-base [&_h2]:font-semibold [&_h3]:mt-2 [&_h3]:font-semibold [&_li]:ml-4 [&_li]:list-disc [&_ol>li]:list-decimal [&_p]:my-2 [&_p]:leading-relaxed [&_pre]:overflow-x-auto [&_pre]:rounded-box [&_pre]:bg-base-200 [&_pre]:p-2 [&_table]:mt-2 [&_table]:text-xs [&_td]:border [&_td]:border-base-300 [&_td]:px-2 [&_td]:py-1 [&_th]:border [&_th]:border-base-300 [&_th]:px-2 [&_th]:py-1"
>
	<!-- `html` is goldmark output rendered by this binary from the trail's own comb, not operator input. -->
	{@html html}
</div>
