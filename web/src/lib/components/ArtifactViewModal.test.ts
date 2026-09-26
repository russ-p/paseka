import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import ArtifactViewModal from './ArtifactViewModal.svelte';
import ArtifactViewModalHarness from './ArtifactViewModalHarness.svelte';
import { artifactView } from '../../tests/fixtures';
import type { ArtifactContent } from '$lib/api/types';

function stubContent(content: ArtifactContent | Response): ReturnType<typeof vi.fn> {
	const mock = vi.fn(async () =>
		content instanceof Response ? content : new Response(JSON.stringify(content))
	);
	vi.stubGlobal('fetch', mock);
	return mock;
}

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('ArtifactViewModal', () => {
	it('names the artifact and its comb path in the dialog', () => {
		stubContent({ ref: 'notes.md', content: 'body' });
		render(ArtifactViewModal, {
			open: true,
			traceId: 'trace-1',
			artifact: artifactView(),
			onclose: vi.fn()
		});

		const dialog = screen.getByRole('dialog', { name: 'Adapter seam notes' });
		expect(dialog).toBeInTheDocument();
		expect(
			screen.getByText('note · .paseka/runs/trace-01a0bd6963faa14f/artifacts/notes.md')
		).toBeInTheDocument();
	});

	it('requests the content for the selected ref', async () => {
		const mock = stubContent({ ref: 'notes.md', content: 'body' });
		render(ArtifactViewModal, {
			open: true,
			traceId: 'trace-1',
			artifact: artifactView(),
			onclose: vi.fn()
		});

		await waitFor(() =>
			expect(mock).toHaveBeenCalledWith(
				'/api/traces/trace-1/artifacts?ref=.paseka%2Fruns%2Ftrace-01a0bd6963faa14f%2Fartifacts%2Fnotes.md',
				undefined
			)
		);
	});

	it('renders server-rendered markdown as HTML', async () => {
		stubContent({
			ref: 'notes.md',
			content: '# Seam',
			contentHtml: '<h1 id="seam">Seam</h1><p>Body text</p>'
		});
		const { container } = render(ArtifactViewModal, {
			open: true,
			traceId: 'trace-1',
			artifact: artifactView(),
			onclose: vi.fn()
		});

		await waitFor(() => expect(container.querySelector('.artifact-body h1')).toBeInTheDocument());
		expect(container.querySelector('.artifact-body h1')).toHaveTextContent('Seam');
		expect(container.querySelector('.artifact-body')).toHaveTextContent('Body text');
	});

	it('falls back to a preformatted block for plain text', async () => {
		stubContent({ ref: 'patch.diff', content: 'diff --git a/x b/x' });
		const { container } = render(ArtifactViewModal, {
			open: true,
			traceId: 'trace-1',
			artifact: artifactView({ ref: 'patch.diff', title: 'Patch' }),
			onclose: vi.fn()
		});

		await waitFor(() => expect(container.querySelector('pre')).toBeInTheDocument());
		expect(container.querySelector('pre')).toHaveTextContent('diff --git a/x b/x');
		expect(container.querySelector('.artifact-body')).toBeNull();
	});

	it('discards a body for an artifact the operator already moved off', async () => {
		const user = userEvent.setup();
		/** The first read is slow, so its answer lands after the operator moved on. */
		const slowRead = new Promise<void>((resolve) => setTimeout(resolve, 40));
		vi.stubGlobal(
			'fetch',
			vi.fn(async (url: RequestInfo | URL) => {
				const ref = new URL(String(url), 'http://x').searchParams.get('ref') ?? '';
				if (ref === 'notes.md') await slowRead;
				return new Response(JSON.stringify({ ref, content: `body of ${ref}` }));
			})
		);

		render(ArtifactViewModalHarness, {
			first: artifactView({ ref: 'notes.md' }),
			second: artifactView({ ref: 'other.md' })
		});
		await user.click(screen.getByRole('button', { name: 'Open second' }));

		await waitFor(() => expect(document.body.textContent).toContain('body of other.md'));
		expect(document.body.textContent).not.toContain('body of notes.md');
	});

	it('shows the server reason instead of an empty body when it omits one', async () => {
		stubContent({ ref: 'report.bin', omitted: 'binary or invalid UTF-8' });
		render(ArtifactViewModal, {
			open: true,
			traceId: 'trace-1',
			artifact: artifactView({ ref: 'report.bin', title: 'Report' }),
			onclose: vi.fn()
		});

		expect(await screen.findByText('binary or invalid UTF-8')).toBeInTheDocument();
		expect(screen.queryByText('Empty file.')).not.toBeInTheDocument();
	});

	it('says so for a genuinely empty file', async () => {
		stubContent({ ref: 'empty.md' });
		render(ArtifactViewModal, {
			open: true,
			traceId: 'trace-1',
			artifact: artifactView({ ref: 'empty.md' }),
			onclose: vi.fn()
		});

		expect(await screen.findByText('Empty file.')).toBeInTheDocument();
	});

	it('reports a failed read as an alert and keeps the dialog open', async () => {
		stubContent(new Response('artifact is gone', { status: 404 }));
		const onclose = vi.fn();
		render(ArtifactViewModal, {
			open: true,
			traceId: 'trace-1',
			artifact: artifactView(),
			onclose
		});

		expect(await screen.findByRole('alert')).toHaveTextContent('artifact is gone');
		expect(screen.getByRole('dialog')).toBeInTheDocument();
		expect(onclose).not.toHaveBeenCalled();
	});

	it('closes on the footer button and on Escape', async () => {
		const user = userEvent.setup();
		stubContent({ ref: 'notes.md', content: 'body' });
		const onclose = vi.fn();
		const { unmount } = render(ArtifactViewModal, {
			open: true,
			traceId: 'trace-1',
			artifact: artifactView(),
			onclose
		});

		await user.click(screen.getByRole('button', { name: 'Close' }));
		expect(onclose).toHaveBeenCalledTimes(1);
		unmount();

		render(ArtifactViewModal, { open: true, traceId: 'trace-1', artifact: artifactView(), onclose });
		await user.keyboard('{Escape}');
		expect(onclose).toHaveBeenCalledTimes(2);
	});

	it('fetches nothing while it is closed', () => {
		const mock = stubContent({ ref: 'notes.md', content: 'body' });
		render(ArtifactViewModal, {
			open: false,
			traceId: 'trace-1',
			artifact: artifactView(),
			onclose: vi.fn()
		});

		expect(mock).not.toHaveBeenCalled();
		expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
	});
});
