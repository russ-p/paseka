import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { findMermaidBlocks, mountDiagram } from './mermaid';

const pele = vi.hoisted(() => ({ mountAsync: vi.fn() }));

vi.mock('pele/lazy', () => pele);

function mounted(error: unknown = undefined) {
	return { error, source: '', destroy: vi.fn() };
}

beforeEach(() => {
	pele.mountAsync.mockReset();
});

afterEach(() => {
	document.body.innerHTML = '';
});

describe('findMermaidBlocks', () => {
	it('reads the source from the fenced markup goldmark emits', () => {
		document.body.innerHTML =
			'<pre><code class="language-mermaid">flowchart LR\r\n  A --&gt; B</code></pre>' +
			'<pre><code class="language-json">{"a":1}</code></pre>';

		const blocks = findMermaidBlocks(document.body);

		expect(blocks).toHaveLength(1);
		expect(blocks[0].textContent).toBe('flowchart LR\n  A --> B');
	});

	it('matches the class as a token rather than a substring', () => {
		document.body.innerHTML =
			'<pre><code class="language-mermaid-extra">flowchart LR</code></pre>' +
			'<pre><code class="code language-mermaid"></code></pre>';

		expect(findMermaidBlocks(document.body)).toHaveLength(1);
	});

	it('ignores a language-mermaid code element outside a pre', () => {
		document.body.innerHTML = '<code class="language-mermaid">flowchart LR</code>';

		expect(findMermaidBlocks(document.body)).toEqual([]);
	});
});

describe('mountDiagram', () => {
	it('mounts with links and images off, at the artifact preview size', async () => {
		const handle = mounted();
		pele.mountAsync.mockResolvedValue(handle);
		const host = document.createElement('pre');
		const source = 'graph TD\n  A --> B';

		const diagram = await mountDiagram(host, source);

		expect(pele.mountAsync).toHaveBeenCalledWith(host, source, {
			links: false,
			images: false,
			zoom: 'auto',
			fontSize: 14
		});
		expect(diagram).toMatchObject({ source, error: null });
		diagram.destroy();
		expect(handle.destroy).toHaveBeenCalledTimes(1);
	});

	it('reports the mounted error instead of throwing, because mount() does not throw', async () => {
		pele.mountAsync.mockResolvedValue(mounted(new Error('Diagram type "flowchart" is not supported yet.')));
		const diagram = await mountDiagram(document.createElement('pre'), 'flowchart LR');

		expect(diagram.error).toBe('Diagram type "flowchart" is not supported yet.');
	});

	it('keeps the first line of a multiline error short enough for the fallback text', async () => {
		pele.mountAsync.mockResolvedValue(mounted(new Error(`syntax error\n${'x'.repeat(300)}`)));

		const diagram = await mountDiagram(document.createElement('pre'), 'graph TD');

		expect(diagram.error).toBe('syntax error');
	});

	it('turns a rejected engine call into an error handle', async () => {
		pele.mountAsync.mockRejectedValue(new Error('failed to load diagram chunk'));

		const diagram = await mountDiagram(document.createElement('pre'), 'graph TD');

		expect(diagram.error).toBe('failed to load diagram chunk');
		expect(diagram.destroy).not.toThrow();
	});
});
