// @vitest-environment node
/**
 * Pele's escaping contract is checked against the real engine, not a mock: the
 * modal inserts the SVG it gets back with `innerHTML`, so this suite is the
 * proof that a label cannot become markup and that a diagram cannot navigate.
 * `render()` runs without a DOM, which keeps the test about the string Pele
 * produces rather than about a mounted artifact.
 */
import { beforeAll, describe, expect, it } from 'vitest';
import { load, render } from 'pele/lazy';
import { diagramOptions } from './mermaid';

beforeAll(async () => {
	await load('flowchart');
});

describe('Pele escaping under the console diagram options', () => {
	it('renders a script label as text, never as a script element', () => {
		const { svg } = render('graph TD\n  A["<script>alert(1)</script>"] --> B', diagramOptions);

		expect(svg).not.toMatch(/<script/i);
		expect(svg).not.toMatch(/\son\w+\s*=/i);
		// The words survive as label text, which is the readable fallback.
		expect(svg).toContain('alert(1)');
	});

	it('keeps a javascript: click target inert', () => {
		const { svg, links } = render(
			'graph TD\n  A[Hello] --> B\n  click A "javascript:alert(1)"',
			diagramOptions
		);

		expect(svg).not.toMatch(/javascript:/i);
		expect(svg).not.toMatch(/<a\b/i);
		expect(links).toEqual([]);
	});

	it('does not reference an external image', () => {
		const { svg } = render(
			'graph TD\n  A["Node"]@{ img: "https://evil.example/pixel.png" } --> B',
			diagramOptions
		);

		expect(svg).not.toMatch(/<image\b/i);
		expect(svg).not.toContain('evil.example');
	});

	it('rejects a source over the character limit', () => {
		const source = `graph TD\n  A[${'x'.repeat(diagramOptions.limit)}]`;

		expect(() => render(source, diagramOptions)).toThrow(/longer than the limit of 20000/);
	});

	it('rejects an edge-heavy diagram at the edge limit', () => {
		const edges = Array.from(
			{ length: diagramOptions.maxEdges + 1 },
			(_, index) => `  n${index} --> n${index + 1}`
		).join('\n');

		expect(() => render(`graph TD\n${edges}`, diagramOptions)).toThrow(
			/Edge limit exceeded\. The limit is 500\./
		);
	});
});
