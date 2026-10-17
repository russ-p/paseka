import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import ProposalPatchList from './ProposalPatchList.svelte';
import { proposalPatch, proposalPatchList } from '../../tests/fixtures';
import type { ProposalPatchContent, ProposalPatchList as ProposalPatchListPayload } from '$lib/api/types';

interface Reads {
	urls: string[];
}

/**
 * The listing is a separate endpoint from the merge-gate preview on purpose, so the
 * stub answers by URL rather than by call order: a component that started asking for the
 * body without being opened has to fail here, not pass.
 */
function stubApi(list: ProposalPatchListPayload, body?: ProposalPatchContent): Reads {
	const reads: Reads = { urls: [] };
	vi.stubGlobal(
		'fetch',
		vi.fn(async (input: RequestInfo | URL) => {
			const url = String(input);
			reads.urls.push(url);
			if (url.includes('agentId=')) {
				return new Response(JSON.stringify(body ?? { ref: 'r', agentId: 'a', bytes: 0, content: '' }));
			}
			return new Response(JSON.stringify(list));
		})
	);
	return reads;
}

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('ProposalPatchList', () => {
	it('says so when a trail archived nothing, rather than showing an empty box', async () => {
		stubApi({ patches: [], diagnostics: [] });
		render(ProposalPatchList, { traceId: 'trace-1' });

		expect(await screen.findByText('No proposal patches archived for this trail.')).toBeInTheDocument();
	});

	it('lists each record with its file count and size, and does not read a body yet', async () => {
		const reads = stubApi(proposalPatchList());
		render(ProposalPatchList, { traceId: 'trace-1' });

		const row = await screen.findByRole('button', { name: /01a0bd6963faa150/ });
		// Size and file count are what tell an operator how big the thing they are about
		// to open is; without them a refused preview would be the whole story.
		expect(row).toHaveTextContent('2 files');
		expect(row).toHaveTextContent('4 KiB');
		expect(row).toHaveTextContent('published');
		expect(reads.urls).not.toContain('/api/traces/trace-1/proposal-patches?agentId=01a0bd6963faa150');
	});

	it('reads a body only when its row is opened', async () => {
		const user = userEvent.setup();
		const reads = stubApi(proposalPatchList(), {
			ref: '.paseka/runs/trace-1/proposals/a.patch',
			agentId: '01a0bd6963faa150',
			bytes: 12,
			content: 'diff --git a/x b/x'
		});
		render(ProposalPatchList, { traceId: 'trace-1' });

		await user.click(await screen.findByRole('button', { name: /01a0bd6963faa150/ }));

		expect(await screen.findByText(/diff --git a\/x b\/x/)).toBeInTheDocument();
		expect(reads.urls).toContain('/api/traces/trace-1/proposal-patches?agentId=01a0bd6963faa150');
	});

	it('never calls a truncated preview the whole patch', async () => {
		const user = userEvent.setup();
		stubApi(proposalPatchList(), {
			ref: 'r',
			agentId: '01a0bd6963faa150',
			bytes: 900_000,
			truncated: true,
			content: 'diff --git a/x b/x'
		});
		render(ProposalPatchList, { traceId: 'trace-1' });

		await user.click(await screen.findByRole('button', { name: /01a0bd6963faa150/ }));

		expect(await screen.findByText(/Preview truncated/)).toBeInTheDocument();
	});

	it('shows the omission reason beside the size the server refuses to read', async () => {
		const user = userEvent.setup();
		stubApi(proposalPatchList(), {
			ref: 'r',
			agentId: '01a0bd6963faa150',
			bytes: 700_000,
			omitted: 'patch too large for inline preview (700000 bytes)'
		});
		render(ProposalPatchList, { traceId: 'trace-1' });

		await user.click(await screen.findByRole('button', { name: /01a0bd6963faa150/ }));

		expect(await screen.findByText(/too large for inline preview/)).toBeInTheDocument();
		expect(screen.getByText(/684 KiB/)).toBeInTheDocument();
	});

	it('reports an uncertain publication as uncertain, beside a retained patch', async () => {
		stubApi(proposalPatchList({ patches: [proposalPatch({ publication: 'unconfirmed' })] }));
		render(ProposalPatchList, { traceId: 'trace-1' });

		expect(await screen.findByText(/outcome is unknown/)).toBeInTheDocument();
	});

	it('keeps an incomplete record from hiding the complete ones', async () => {
		stubApi({
			patches: [proposalPatch()],
			diagnostics: [{ agentId: 'half-written', reason: 'incomplete record: no readable patch body' }]
		});
		render(ProposalPatchList, { traceId: 'trace-1' });

		expect(await screen.findByRole('button', { name: /01a0bd6963faa150/ })).toBeInTheDocument();
		expect(screen.getByText(/incomplete patch record/)).toBeInTheDocument();
		expect(screen.getByText(/half-written/)).toBeInTheDocument();
	});

	it('reads the trail named by the prop, and re-reads when it changes', async () => {
		const reads = stubApi(proposalPatchList());
		const { rerender } = render(ProposalPatchList, { traceId: 'trace-1' });

		await screen.findByRole('button', { name: /01a0bd6963faa150/ });
		await rerender({ traceId: 'trace-2' });

		await waitFor(() => expect(reads.urls).toContain('/api/traces/trace-2/proposal-patches'));
	});
});