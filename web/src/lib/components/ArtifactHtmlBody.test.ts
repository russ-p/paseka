import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { render, waitFor } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import ArtifactHtmlBody from './ArtifactHtmlBody.svelte';
import { MAX_DIAGRAMS, type MermaidHandle } from '$lib/mermaid';

const mountDiagram = vi.hoisted(() => vi.fn());

vi.mock('$lib/mermaid', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/mermaid')>();
	return { ...actual, mountDiagram };
});

function fence(source: string): string {
	return `<pre><code class="language-mermaid">${source}</code></pre>`;
}

function handle(error: string | null = null): MermaidHandle {
	return { source: '', error, destroy: vi.fn() };
}

/** A source file under `web/src`, for the stylesheet and literal contracts. */
function sourceOf(file: string): string {
	return readFileSync(join(process.cwd(), 'src', file), 'utf8');
}

beforeEach(() => {
	mountDiagram.mockReset();
});

afterEach(() => {
	vi.restoreAllMocks();
	delete document.documentElement.dataset.theme;
});

describe('ArtifactHtmlBody', () => {
	it('renders the server HTML unchanged when there is no diagram', async () => {
		render(ArtifactHtmlBody, { html: '<h1 id="seam">Seam</h1><p>Body text</p>' });
		await tick();

		expect(document.querySelector('.artifact-body h1')).toHaveTextContent('Seam');
		expect(document.querySelector('.artifact-body')).toHaveTextContent('Body text');
		expect(mountDiagram).not.toHaveBeenCalled();
	});

	it('mounts a fence into a host beside folded source and destroys it on unmount', async () => {
		const mounted = handle();
		mountDiagram.mockResolvedValue(mounted);
		const { container, unmount } = render(ArtifactHtmlBody, { html: fence('graph TD\n  A --> B') });

		await waitFor(() => expect(mountDiagram).toHaveBeenCalledTimes(1));
		const [host, source] = mountDiagram.mock.calls[0];
		expect(source).toBe('graph TD\n  A --> B');
		expect(host).toBe(container.querySelector('.artifact-diagram-host'));
		expect(host).toBeInstanceOf(HTMLDivElement);

		const details = container.querySelector('details');
		expect(details).not.toBeNull();
		expect(details?.open).toBe(false);
		expect(details?.querySelector('summary')).toHaveTextContent('Mermaid source');
		expect(details?.querySelector('pre > code.language-mermaid')).toHaveTextContent('graph TD');
		expect(container.querySelector('.alert-warning')).not.toBeVisible();

		unmount();
		expect(mounted.destroy).toHaveBeenCalledTimes(1);
	});

	it('opens the source and warns when the engine reports a failure', async () => {
		mountDiagram.mockResolvedValue(handle('Diagram type "flowchart" is not supported yet.'));
		const { container } = render(ArtifactHtmlBody, { html: fence('flowchart LR') });

		await waitFor(() => expect(container.querySelector('.alert-warning')).toBeVisible());
		expect(container.querySelector('details')?.open).toBe(true);
		// Exact, not substring: the notice must not stack its prefix twice.
		expect(container.querySelector('.alert-warning')?.textContent).toBe(
			'Diagram failed to render: Diagram type "flowchart" is not supported yet.'
		);
		expect(container.querySelector('details pre code.language-mermaid')).toHaveTextContent(
			'flowchart LR'
		);
	});

	it('falls back to the visible source when the mount rejects outright', async () => {
		mountDiagram.mockRejectedValue(new Error('failed to load diagram chunk'));
		const { container } = render(ArtifactHtmlBody, { html: fence('graph TD') });

		await waitFor(() => expect(container.querySelector('.alert-warning')).toBeVisible());
		expect(container.querySelector('details')?.open).toBe(true);
		expect(container.querySelector('.alert-warning')?.textContent).toBe(
			'Diagram failed to render: failed to load diagram chunk'
		);
		expect(container.querySelector('details pre code.language-mermaid')).toHaveTextContent('graph TD');
	});

	it('shows the plain sentence once when the failure carries no message', async () => {
		mountDiagram.mockResolvedValue(handle(''));
		const { container } = render(ArtifactHtmlBody, { html: fence('graph TD') });

		await waitFor(() => expect(container.querySelector('.alert-warning')).toBeVisible());
		// `''` means failed-without-detail, so the source still opens and the
		// notice is the sentence once, not the sentence twice.
		expect(container.querySelector('details')?.open).toBe(true);
		expect(container.querySelector('.alert-warning')?.textContent).toBe('Diagram failed to render.');
	});

	it('shows the plain sentence once when the mount rejects with an empty message', async () => {
		mountDiagram.mockRejectedValue(new Error('  \n\t '));
		const { container } = render(ArtifactHtmlBody, { html: fence('graph TD') });

		await waitFor(() => expect(container.querySelector('.alert-warning')).toBeVisible());
		expect(container.querySelector('details')?.open).toBe(true);
		expect(container.querySelector('.alert-warning')?.textContent).toBe('Diagram failed to render.');
		expect(container.querySelector('details pre code.language-mermaid')).toHaveTextContent('graph TD');
	});

	it('does not retry a failed mount in a loop', async () => {
		mountDiagram.mockRejectedValue(new Error('failed to load diagram chunk'));
		const { container } = render(ArtifactHtmlBody, { html: fence('graph TD') });

		await waitFor(() => expect(container.querySelector('.alert-warning')).toBeVisible());
		// A failing chunk or render is a result, not a transient state: the source
		// stays readable and the modal issues no second attempt on its own.
		await new Promise((resolve) => setTimeout(resolve, 25));
		expect(mountDiagram).toHaveBeenCalledTimes(1);
		expect(container.querySelector('details pre code.language-mermaid')).toHaveTextContent('graph TD');
	});

	it('leaves blocks past the cap as source with one explanation', async () => {
		mountDiagram.mockResolvedValue(handle());
		const sources = Array.from(
			{ length: MAX_DIAGRAMS + 1 },
			(_, index) => `graph TD\n  n${index} --> n${index + 1}`
		);
		const { container } = render(ArtifactHtmlBody, {
			html: sources.map((source) => fence(source)).join('')
		});

		await waitFor(() => expect(mountDiagram).toHaveBeenCalledTimes(MAX_DIAGRAMS));

		expect(container.querySelectorAll('.artifact-diagram')).toHaveLength(MAX_DIAGRAMS);
		expect(container.querySelectorAll('.artifact-diagram-limit')).toHaveLength(1);
		const note = container.querySelector('.artifact-diagram-limit');
		expect(note).toHaveTextContent(
			`Only the first ${MAX_DIAGRAMS} diagrams in this artifact are rendered; the rest stay as source.`
		);

		// The skipped fence keeps its source, is not wrapped in a mount host, and
		// was never handed to the engine.
		const skipped = note?.nextElementSibling;
		expect(skipped?.tagName).toBe('PRE');
		expect(skipped).toHaveTextContent(`n${MAX_DIAGRAMS} --> n${MAX_DIAGRAMS + 1}`);
		expect(skipped?.querySelector('.artifact-diagram-host')).toBeNull();
		expect(mountDiagram.mock.calls.map((call) => call[1])).not.toContain(
			sources[MAX_DIAGRAMS]
		);
	});

	it('mounts several fences one after another, in source order', async () => {
		let resolveFirst: (value: MermaidHandle) => void = () => {};
		const first = new Promise<MermaidHandle>((resolve) => {
			resolveFirst = resolve;
		});
		const secondHandle = handle();
		mountDiagram.mockReturnValueOnce(first).mockResolvedValueOnce(secondHandle);
		const { container } = render(ArtifactHtmlBody, { html: fence('graph TD') + fence('pie') });

		await waitFor(() => expect(mountDiagram).toHaveBeenCalledTimes(1));
		expect(mountDiagram.mock.calls[0][1]).toBe('graph TD');
		expect(mountDiagram).toHaveBeenCalledTimes(1);

		resolveFirst(handle());
		await waitFor(() => expect(mountDiagram).toHaveBeenCalledTimes(2));
		expect(mountDiagram.mock.calls[1][1]).toBe('pie');
		expect(container.querySelectorAll('.artifact-diagram')).toHaveLength(2);
	});

	it('destroys the previous mounts when the HTML changes', async () => {
		const firstHandle = handle();
		const secondHandle = handle();
		mountDiagram.mockResolvedValueOnce(firstHandle).mockResolvedValueOnce(secondHandle);
		const { rerender } = render(ArtifactHtmlBody, { html: fence('graph TD') });

		await waitFor(() => expect(mountDiagram).toHaveBeenCalledTimes(1));

		await rerender({ html: fence('pie') });

		await waitFor(() => expect(mountDiagram).toHaveBeenCalledTimes(2));
		expect(firstHandle.destroy).toHaveBeenCalledTimes(1);
	});

	it('tears down a mount that resolves after the body is gone', async () => {
		let resolveMount: (value: MermaidHandle) => void = () => {};
		const late = handle();
		mountDiagram.mockReturnValue(
			new Promise<MermaidHandle>((resolve) => {
				resolveMount = resolve;
			})
		);
		const { unmount } = render(ArtifactHtmlBody, { html: fence('graph TD') });

		await waitFor(() => expect(mountDiagram).toHaveBeenCalledTimes(1));
		unmount();
		resolveMount(late);

		await waitFor(() => expect(late.destroy).toHaveBeenCalledTimes(1));
	});

	it('keeps the mounted diagram when the document theme changes', async () => {
		mountDiagram.mockResolvedValue(handle());
		render(ArtifactHtmlBody, { html: fence('graph TD') });

		await waitFor(() => expect(mountDiagram).toHaveBeenCalledTimes(1));

		document.documentElement.dataset.theme = 'catppuccin-mocha';
		await tick();

		// Pele paints from CSS variables, so a theme switch repaints the live
		// SVG; remounting would lose the operator's zoom and pan.
		expect(mountDiagram).toHaveBeenCalledTimes(1);
	});
});

describe('Pele theme bridge', () => {
	it('maps the core Pele palette onto DaisyUI theme tokens', () => {
		const css = sourceOf('lib/pele-theme.css');
		const tokens: Record<string, string> = {
			'--pele-bg': 'var(--color-base-100)',
			'--pele-fg': 'var(--color-base-content)',
			'--pele-surface': 'var(--color-base-200)',
			'--pele-surface-alt': 'var(--color-base-300)',
			'--pele-accent': 'var(--color-primary)',
			'--pele-critical': 'var(--color-error)',
			'--pele-error': 'var(--color-error)',
			'--pele-radius': 'var(--radius-field)'
		};
		for (const [variable, value] of Object.entries(tokens)) {
			expect(css).toContain(`${variable}: ${value};`);
		}
		for (const variable of ['--pele-muted', '--pele-border', '--pele-line']) {
			expect(css).toMatch(
				new RegExp(`${variable}:\\s*color-mix\\([^;]*var\\(--color-base-content\\)`)
			);
		}
	});

	it('is scoped to the artifact body and imported once from the global stylesheet', () => {
		const css = sourceOf('lib/pele-theme.css');
		expect(css).toMatch(/^\.artifact-body\s*\{/m);
		expect(sourceOf('app.css').match(/@import\s+["']\.\/lib\/pele-theme\.css["']/g)).toHaveLength(1);
	});

	it('adds no raw colour literals to the theme file or the Svelte body', () => {
		const literals = [/#[0-9a-fA-F]{3,8}\b/, /\brgba?\(/, /\bhsla?\(/, /\boklch\(/];
		for (const file of ['lib/pele-theme.css', 'lib/components/ArtifactHtmlBody.svelte']) {
			for (const pattern of literals) {
				expect(sourceOf(file)).not.toMatch(pattern);
			}
		}
	});
});
