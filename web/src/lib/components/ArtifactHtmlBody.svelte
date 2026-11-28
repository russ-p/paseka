<script lang="ts">
	import {
		findMermaidBlocks,
		MAX_DIAGRAMS,
		mountDiagram,
		type MermaidHandle
	} from '$lib/mermaid';

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
		// `relative` anchors Pele's absolutely positioned zoom controls to this
		// box. The host must not itself scroll: a scroll container carries those
		// controls away with the diagram. Pele fits the SVG to the host and zooms
		// from there, so a wide diagram stays readable without horizontal scroll.
		host.className = 'artifact-diagram-host relative flex w-full justify-center';
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

	/** The raw first line of a failure; the notice adds the prefix exactly once. */
	function firstLine(cause: unknown): string {
		const message = cause instanceof Error ? cause.message : String(cause);
		const line = message.split('\n').find((candidate) => candidate.trim() !== '');
		if (!line) return '';
		return line.length > 200 ? `${line.slice(0, 197)}…` : line;
	}

	function noticeFor(error: string): string {
		return error === '' ? 'Diagram failed to render.' : `Diagram failed to render: ${error}`;
	}

	/**
	 * Blocks past the cap stay in place as source. The note is beside the first
	 * skipped block rather than repeated for every one, so a generated document
	 * does not turn into a wall of identical warnings.
	 */
	function explainCap(skipped: HTMLElement): void {
		const note = document.createElement('p');
		note.className = 'artifact-diagram-limit my-1 text-xs text-base-content/60';
		note.textContent = `Only the first ${MAX_DIAGRAMS} diagrams in this artifact are rendered; the rest stay as source.`;
		skipped.before(note);
	}

	$effect(() => {
		// The HTML is read here so a new artifact tears down the previous mounts
		// before this effect hydrates the new body.
		const root = body;
		if (!root || html === '') return;
		const container: HTMLElement = root;

		let disposed = false;
		const handles: MermaidHandle[] = [];
		const blocks = findMermaidBlocks(container);
		const renderable = blocks.slice(0, MAX_DIAGRAMS);
		const skipped = blocks.slice(MAX_DIAGRAMS);
		const firstSkippedPre = skipped[0]?.parentElement;
		if (firstSkippedPre) explainCap(firstSkippedPre);

		async function hydrate(): Promise<void> {
			// One chart at a time: a document with several diagrams is read in
			// source order, and a slow mount does not race the one below it.
			for (const code of renderable) {
				if (disposed) return;
				const pre = code.parentElement;
				if (!pre) continue;
				const source = code.textContent ?? '';
				const { host, details, notice } = wrapSource(pre);

				let handle: MermaidHandle;
				try {
					handle = await mountDiagram(host, source);
				} catch (cause) {
					handle = { source, error: firstLine(cause), destroy: () => {} };
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
					notice.textContent = noticeFor(handle.error);
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
