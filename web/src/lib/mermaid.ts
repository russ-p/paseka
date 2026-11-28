/**
 * Pele is the console's only Mermaid engine, and this module is the only place
 * that imports it. Goldmark already emits a fenced block as
 * `<pre><code class="language-mermaid">`, so hydrating an artifact is a
 * client-side substitution: the API and the initial bundle stay unaware of it.
 */

/** What the UI may do with a diagram; `pele` types stay on this side. */
export interface MermaidHandle {
	/** The source the diagram was mounted from, kept for a fallback surface. */
	readonly source: string;
	/** A short, operator-readable failure, or `null` when it rendered. */
	readonly error: string | null;
	destroy(): void;
}

type Mounted = {
	readonly error: unknown;
	readonly source: string;
	destroy(): void;
};

/**
 * One artifact is a modal, not a document editor. Pele 1.13.0 defaults are
 * sized for the latter (`limit` 50 000 source chars, `outputLimit` 4 000 000
 * SVG chars, `maxEdges` 5 000); the console lowers all three so a model-written
 * fence cannot spend the modal's time on a pathological diagram. A block over
 * one of these limits reports the engine's limit error and stays readable as
 * source beside it.
 */
export const diagramOptions = {
	// A comb artifact can carry model-generated text, so nothing in a diagram
	// may navigate or fetch. Icons and math are left unset for the same reason.
	links: false,
	images: false,
	limit: 20_000,
	outputLimit: 1_000_000,
	maxEdges: 500
} as const;

/**
 * A document with more fences than this is read as text: the remaining blocks
 * stay source, and the first skipped block explains why. Twenty diagrams is
 * already more than one artifact needs; an unbounded loop over a generated
 * document would hold the modal open for no reading benefit.
 */
export const MAX_DIAGRAMS = 20;

type MountAsync = (
	element: HTMLElement,
	source: string,
	options: typeof diagramOptions & {
		zoom: 'auto';
		fontSize: number;
		responsive: boolean;
		autoDirection: boolean;
	}
) => Promise<Mounted>;

/**
 * The successful engine import is cached so opening a second artifact does not
 * re-request the chunk. A failed import is dropped, because the next artifact
 * is a fresh chance to load it.
 */
let engine: Promise<MountAsync> | null = null;

function loadEngine(): Promise<MountAsync> {
	engine ??= import('pele/lazy')
		.then((pele) => pele.mountAsync)
		.catch((cause: unknown) => {
			engine = null;
			throw cause;
		});
	return engine;
}

/**
 * The fenced blocks goldmark produces for ` ```mermaid `: the `language-mermaid`
 * class is matched as a token, and the code element's text is the diagram.
 */
export function findMermaidBlocks(root: ParentNode): HTMLElement[] {
	return Array.from(root.querySelectorAll<HTMLElement>('pre > code')).filter((code) =>
		code.classList.contains('language-mermaid')
	);
}

/**
 * Mounts one diagram without ever rejecting: a bad diagram, an unknown type,
 * or a failed chunk load is reported through `error`, because Pele's own
 * `mount()` resolves with an error instead of throwing and the modal has no
 * place to catch one.
 */
export async function mountDiagram(host: HTMLElement, source: string): Promise<MermaidHandle> {
	try {
		const mountAsync = await loadEngine();
		const mounted = await mountAsync(host, source, {
			...diagramOptions,
			zoom: 'auto',
			fontSize: 14,
			// The artifact modal is narrow on a phone. Pele fits the SVG to its
			// host and flips a wide flowchart at its 640px breakpoint, so a
			// diagram stays legible without the modal scrolling sideways; the
			// zoom controls cover the detail that fitting shrinks.
			responsive: true,
			autoDirection: true
		});
		if (mounted.error) {
			return { source, error: describeError(mounted.error), destroy: () => mounted.destroy() };
		}
		return { source, error: null, destroy: () => mounted.destroy() };
	} catch (cause) {
		return { source, error: describeError(cause), destroy: () => {} };
	}
}

/** Pele reports through `Error`s and thrown records; neither reaches the UI raw. */
function describeError(cause: unknown): string {
	const message = cause instanceof Error ? cause.message : String(cause);
	const firstLine = message.split('\n').find((line) => line.trim() !== '') ?? 'Diagram failed to render.';
	return firstLine.length > 200 ? `${firstLine.slice(0, 197)}…` : firstLine;
}
