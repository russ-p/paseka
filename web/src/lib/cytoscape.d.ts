/**
 * cytoscape 3.31 ships its own types at `index.d.ts`, but its package `exports`
 * map has no `types` condition — only `import` and `require` — so TypeScript
 * under `moduleResolution: "bundler"` resolves the `.mjs` and never sees them.
 *
 * The two real fixes are a `paths` override in tsconfig or a patched `exports`
 * map, and both are worse than this: a local `paths` entry replaces SvelteKit's
 * generated `$lib` aliases rather than merging with them, and patching a
 * dependency's manifest is not ours to do. So this declares the surface the
 * topology graph actually calls, and nothing more — the library's own
 * provisional spec is not needed to draw a bipartite graph.
 *
 * The shape matches the runtime module: its ESM bundle's default export is the
 * `cytoscape` factory, which is what `import cytoscape from 'cytoscape'` resolves
 * to at build time.
 */
declare module 'cytoscape' {
	/** A node or edge, positioned in model coordinates. */
	export interface Position {
		x: number;
		y: number;
	}

	export interface Element {
		/** The `data.id` the element was created with. */
		id(): string;
		nonempty(): boolean;
		/** Reads the position with no argument, sets it with one. */
		position(point?: Position): Position;
	}

	export interface Collection {
		forEach(callback: (element: Element) => void): void;
	}

	export interface StyleBlock {
		selector: string;
		style: Record<string, string | number>;
	}

	export interface Core {
		destroy(): void;
		/** Fits the viewport; the padding is in rendered pixels. */
		fit(padding?: number, paddingUnits?: number): void;
		getElementById(id: string): Element;
		batch(callback: () => void): void;
		nodes(): Collection;
		on(event: string, selector: string, handler: () => void): void;
		style(sheet: StyleBlock[]): unknown;
	}

	export interface Options {
		container: HTMLElement;
		/** Each element's `data` carries its own id plus the fields the stylesheet reads. */
		elements: { data: object }[];
		style: StyleBlock[];
		layout: { name: string };
		minZoom?: number;
		maxZoom?: number;
		wheelSensitivity?: number;
	}

	export default function cytoscape(options: Options): Core;
}
