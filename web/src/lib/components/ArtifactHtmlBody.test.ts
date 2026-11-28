import { render, waitFor } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import ArtifactHtmlBody from './ArtifactHtmlBody.svelte';
import type { MermaidHandle } from '$lib/mermaid';

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

beforeEach(() => {
	mountDiagram.mockReset();
});

afterEach(() => {
	vi.restoreAllMocks();
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
		expect(container.querySelector('.alert-warning')).toHaveTextContent(
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
		expect(container.querySelector('.alert-warning')).toHaveTextContent(
			'Diagram failed to render: failed to load diagram chunk'
		);
		expect(container.querySelector('details pre code.language-mermaid')).toHaveTextContent('graph TD');
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
});
