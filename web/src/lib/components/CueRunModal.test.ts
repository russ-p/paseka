import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import CueRunModal from './CueRunModal.svelte';
import type { RunCueResult } from '$lib/api/types';

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

const cues = [
	{ id: 'standup', description: 'Daily standup', standingTrace: 'trace-standing' },
	{ id: 'ship', description: 'Ship the release' }
];

function stubFetch(
	overrides: { cues?: unknown[]; run?: () => Response } = {}
): ReturnType<typeof vi.fn> {
	const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
		const url = String(input);
		if (url === '/api/cues') return jsonResponse(overrides.cues ?? cues);
		if (url.startsWith('/api/cues/') && url.endsWith('/run')) {
			return overrides.run?.() ?? jsonResponse({ traceId: 'trace-new', taskId: 'task-01' });
		}
		return new Response('not found', { status: 404 });
	});
	vi.stubGlobal('fetch', fetchMock);
	return fetchMock;
}

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('CueRunModal', () => {
	it('preselects the only cue and shows its standing trace', async () => {
		stubFetch({ cues: [cues[0]] });
		render(CueRunModal, { open: true, onclose: vi.fn() });

		const select = await screen.findByLabelText<HTMLSelectElement>('Cue');
		await waitFor(() => expect(select.value).toBe('standup'));
		expect(screen.getByText('Standing trace trace-standing')).toBeInTheDocument();
	});

	it('offers every cue and leaves the choice to the operator', async () => {
		stubFetch();
		render(CueRunModal, { open: true, onclose: vi.fn() });

		const select = await screen.findByLabelText<HTMLSelectElement>('Cue');
		await waitFor(() => expect(select.options).toHaveLength(3));
		expect(select.value).toBe('');
		expect(screen.queryByText(/Standing trace/)).not.toBeInTheDocument();
	});

	it('requires a cue and text before publishing', async () => {
		const user = userEvent.setup();
		stubFetch();
		render(CueRunModal, { open: true, onclose: vi.fn() });

		const text = await screen.findByLabelText('Text');
		const publish = screen.getByRole('button', { name: 'Publish' });
		expect(text).toHaveAttribute('aria-invalid', 'false');
		expect(screen.getByText('Published to the cue bee as a SIGNAL.')).toBeInTheDocument();
		expect(publish).toBeDisabled();

		await user.type(text, 'Ship it');
		expect(publish).toBeDisabled();
		expect(screen.getByText('Says what the colony should work on.')).toBeInTheDocument();

		await user.selectOptions(screen.getByLabelText('Cue'), 'ship');
		expect(publish).toBeEnabled();

		await user.clear(text);
		expect(text).toHaveAttribute('aria-invalid', 'true');
		expect(screen.getByText('Text is required')).toBeInTheDocument();
		expect(publish).toBeDisabled();
	});

	it('publishes the cue, hands the trace back, and closes', async () => {
		const user = userEvent.setup();
		const fetchMock = stubFetch();
		const onclose = vi.fn();
		const onran = vi.fn<(result: RunCueResult) => void>();
		render(CueRunModal, { open: true, onclose, onran });

		await user.selectOptions(await screen.findByLabelText('Cue'), 'ship');
		await user.type(screen.getByLabelText('Text'), 'Ship the release');
		await user.type(screen.getByLabelText('Trace ID'), 'trace-01');
		await user.click(screen.getByRole('button', { name: 'Publish' }));

		await waitFor(() => expect(onran).toHaveBeenCalledWith({ traceId: 'trace-new', taskId: 'task-01' }));
		expect(onclose).toHaveBeenCalledTimes(1);
		expect(fetchMock).toHaveBeenCalledWith('/api/cues/ship/run', expect.objectContaining({ method: 'POST' }));
	});

	it('keeps the dialog open and shows the reason when publishing fails', async () => {
		const user = userEvent.setup();
		stubFetch({ run: () => new Response('nats url not configured', { status: 500 }) });
		const onclose = vi.fn();
		render(CueRunModal, { open: true, onclose });

		await user.selectOptions(await screen.findByLabelText('Cue'), 'ship');
		await user.type(screen.getByLabelText('Text'), 'Ship the release');
		await user.click(screen.getByRole('button', { name: 'Publish' }));

		expect(await screen.findByRole('alert')).toHaveTextContent('nats url not configured');
		expect(onclose).not.toHaveBeenCalled();
	});

	it('reports a failed cue list without offering a submit', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn(async () => new Response('boom', { status: 503 }))
		);
		render(CueRunModal, { open: true, onclose: vi.fn() });

		expect(await screen.findByRole('alert')).toHaveTextContent('boom');
		expect(screen.getByRole('button', { name: 'Publish' })).toBeDisabled();
	});

	it('closes on Escape without publishing', async () => {
		const user = userEvent.setup();
		const fetchMock = stubFetch();
		const onclose = vi.fn();
		render(CueRunModal, { open: true, onclose });

		await screen.findByLabelText('Cue');
		await user.keyboard('{Escape}');

		expect(onclose).toHaveBeenCalledTimes(1);
		expect(fetchMock).toHaveBeenCalledTimes(1);
	});

	it('renders nothing while closed', () => {
		stubFetch();
		render(CueRunModal, { open: false, onclose: vi.fn() });
		expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
	});
});
