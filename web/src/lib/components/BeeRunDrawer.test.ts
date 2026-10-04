import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import BeeRunDrawer from './BeeRunDrawer.svelte';
import BeeRunDrawerHarness from './BeeRunDrawerHarness.svelte';
import { bee, beeRoster } from '../../tests/fixtures';
import type { Bee, RunBeeRequest, RunBeeResult } from '$lib/api/types';

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

interface RunAnswer {
	status?: number;
	body?: unknown;
	text?: string;
}

/**
 * The drawer talks to one endpoint, so the stub answers it and records what was
 * sent — the request is the contract, and a form that quietly drops the intent or
 * sends a blank trail id reads the same on screen as one that does not.
 */
function stubRun(
	answer: RunAnswer = {},
	roster: Bee[] | null = null
): { calls: string[]; bodies: RunBeeRequest[] } {
	const calls: string[] = [];
	const bodies: RunBeeRequest[] = [];
	vi.stubGlobal(
		'fetch',
		vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
			calls.push(String(input));
			if (init?.body) bodies.push(JSON.parse(String(init.body)) as RunBeeRequest);
			if (answer.text !== undefined) return new Response(answer.text, { status: answer.status ?? 400 });
			// The roster read is only reached when the page passed none, and it is a
			// plain GET — the run POSTs to the same prefix.
			const url = String(input);
			if (url.startsWith('/api/bees') && !url.endsWith('/run')) {
				if (roster === null) return new Response('not found', { status: 404 });
				return jsonResponse(roster);
			}
			return jsonResponse(answer.body ?? { traceId: 'trace-9f0c1d2e3a4b5c6d', bee: 'builder' }, answer.status ?? 201);
		})
	);
	return { calls, bodies };
}

const roster = (): Bee[] => beeRoster();

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('BeeRunDrawer', () => {
	it('offers every bee in the roster, script ones included', async () => {
		stubRun();
		render(BeeRunDrawer, { open: true, bees: roster(), onclose: vi.fn() });

		const select = await screen.findByLabelText<HTMLSelectElement>('Bee');
		await waitFor(() => expect(select.options).toHaveLength(4));
		// `sweeper` is the row a launch session cannot offer, and this is the one
		// control that can start it — so the picker behind this form must not be the
		// launchable one.
		expect(screen.getByRole('option', { name: 'sweeper — script' })).toBeInTheDocument();
		expect(select.value).toBe('');
	});

	it('preselects the only bee, so a one-bee colony is one click', async () => {
		stubRun();
		render(BeeRunDrawer, { open: true, bees: [bee()], onclose: vi.fn() });

		const select = await screen.findByLabelText<HTMLSelectElement>('Bee');
		await waitFor(() => expect(select.value).toBe('builder'));
	});

	it('defaults the intent to the bee’s own, so the common case is one field', async () => {
		const user = userEvent.setup();
		const { bodies } = stubRun();
		const roster = beeRoster();
		render(BeeRunDrawer, {
			open: true,
			bees: [...roster, bee({ role: 'drone', defaultIntent: 'general' })],
			onclose: vi.fn()
		});

		await user.selectOptions(await screen.findByLabelText('Bee'), 'drone');
		const intent = screen.getByLabelText<HTMLSelectElement>('Intent');
		await waitFor(() => expect(intent.value).toBe('general'));

		await user.type(screen.getByLabelText('Task'), 'ship it');
		await user.click(screen.getByRole('button', { name: 'Run' }));

		await waitFor(() => expect(bodies).toHaveLength(1));
		expect(bodies[0]?.intent).toBe('general');
	});

	it('rebuilds the intent list around the bee that was chosen', async () => {
		const user = userEvent.setup();
		stubRun();
		render(BeeRunDrawer, { open: true, bees: roster(), onclose: vi.fn() });

		await user.selectOptions(await screen.findByLabelText('Bee'), 'builder');
		const intent = screen.getByLabelText<HTMLSelectElement>('Intent');
		await waitFor(() => expect(screen.getByRole('option', { name: 'feature' })).toBeInTheDocument());
		// `builder` declares no default, so the choice is the operator's — but the
		// vocabulary is the bee's own, which is what makes a bee that adds an intent
		// need no console change.
		expect(intent.value).toBe('');

		await user.selectOptions(screen.getByLabelText('Bee'), 'hivewright');
		// `hivewright` declares no intents at all, so the vocabulary empties rather
		// than keeping the previous bee's words on offer.
		expect(screen.queryByRole('option', { name: 'feature' })).not.toBeInTheDocument();
		expect(screen.getByLabelText<HTMLSelectElement>('Intent')).toBeDisabled();
	});

	it('runs the bee that was chosen, not the first one in the roster', async () => {
		const user = userEvent.setup();
		const { calls, bodies } = stubRun();
		const onran = vi.fn<(result: RunBeeResult) => void>();
		const onclose = vi.fn();
		render(BeeRunDrawer, { open: true, bees: roster(), onclose, onran });

		await user.selectOptions(await screen.findByLabelText('Bee'), 'hivewright');
		await user.type(screen.getByLabelText('Task'), 'add the retry backoff');
		await user.click(screen.getByRole('button', { name: 'Run' }));

		await waitFor(() => expect(onran).toHaveBeenCalledTimes(1));
		expect(calls).toEqual(['/api/bees/hivewright/run']);
		// Only what the operator filled: `hivewright` declares no intents and no
		// default, so an unset one is absent rather than sent as `""`.
		expect(bodies[0]).toEqual({ body: 'add the retry backoff' });
		expect(onclose).toHaveBeenCalledTimes(1);
	});

	it('names the trail the operator typed', async () => {
		const user = userEvent.setup();
		const { bodies } = stubRun();
		render(BeeRunDrawer, { open: true, bees: roster(), onclose: vi.fn() });

		await user.selectOptions(await screen.findByLabelText('Bee'), 'builder');
		await user.type(screen.getByLabelText('Task'), 'go');
		await user.type(screen.getByLabelText('Trail ID'), 'trail-daily');
		await user.click(screen.getByRole('button', { name: 'Run' }));

		await waitFor(() => expect(bodies).toHaveLength(1));
		expect(bodies[0]?.traceId).toBe('trail-daily');
	});

	it('leaves a blank trail id out rather than sending an empty one', async () => {
		const user = userEvent.setup();
		const { bodies } = stubRun();
		render(BeeRunDrawer, { open: true, bees: roster(), onclose: vi.fn() });

		await user.selectOptions(await screen.findByLabelText('Bee'), 'builder');
		await user.type(screen.getByLabelText('Task'), 'go');
		await user.click(screen.getByRole('button', { name: 'Run' }));

		await waitFor(() => expect(bodies).toHaveLength(1));
		// The server reads a present-but-blank trace id differently from an absent
		// one, where absent means "generate one".
		expect(bodies[0]).not.toHaveProperty('traceId');
	});

	it('needs a bee and a task before it will run anything', async () => {
		const user = userEvent.setup();
		stubRun();
		render(BeeRunDrawer, { open: true, bees: roster(), onclose: vi.fn() });

		const run = await screen.findByRole('button', { name: 'Run' });
		expect(run).toBeDisabled();
		await user.selectOptions(screen.getByLabelText('Bee'), 'builder');
		expect(run).toBeDisabled();
		await user.type(screen.getByLabelText('Task'), 'x');
		await waitFor(() => expect(run).toBeEnabled());
	});

	it('runs a script bee with nothing to do, because it runs its own command', async () => {
		const user = userEvent.setup();
		const { calls, bodies } = stubRun();
		render(BeeRunDrawer, { open: true, bees: roster(), onclose: vi.fn() });

		await user.selectOptions(await screen.findByLabelText('Bee'), 'sweeper');
		const run = screen.getByRole('button', { name: 'Run' });
		await waitFor(() => expect(run).toBeEnabled());
		await user.click(run);

		await waitFor(() => expect(bodies).toHaveLength(1));
		expect(calls).toEqual(['/api/bees/sweeper/run']);
		expect(bodies[0]).toEqual({ body: '' });
	});

	it('sends the raw prompt instead of the task when the operator writes one', async () => {
		const user = userEvent.setup();
		const { bodies } = stubRun();
		render(BeeRunDrawer, { open: true, bees: [bee()], onclose: vi.fn() });

		await user.click(await screen.findByLabelText('Advanced: write the prompt myself'));
		await user.type(screen.getByLabelText('Raw prompt'), 'you are a build bot');
		await user.click(screen.getByRole('button', { name: 'Run' }));

		await waitFor(() => expect(bodies).toHaveLength(1));
		expect(bodies[0]).toEqual({ inlinePrompt: 'you are a build bot' });
	});

	it('reports the server’s refusal in the form instead of closing it', async () => {
		const user = userEvent.setup();
		stubRun({ text: 'bee "builder": task body or inline prompt is required' });
		const onclose = vi.fn();
		render(BeeRunDrawer, { open: true, bees: [bee()], onclose });

		await user.type(await screen.findByLabelText('Task'), 'go');
		await user.click(screen.getByRole('button', { name: 'Run' }));

		expect(await screen.findByRole('alert')).toHaveTextContent('task body or inline prompt is required');
		expect(onclose).not.toHaveBeenCalled();
	});

	it('says a colony with no bees has none, instead of showing an empty form', async () => {
		stubRun();
		render(BeeRunDrawer, { open: true, bees: [], onclose: vi.fn() });

		expect(await screen.findByRole('alert')).toHaveTextContent('This colony has no bees');
		expect(screen.queryByLabelText('Task')).not.toBeInTheDocument();
		expect(screen.getByRole('button', { name: 'Run' })).toBeDisabled();
	});

	it('starts empty every time it opens, so a previous task is not resubmitted', async () => {
		const user = userEvent.setup();
		stubRun();
		const { rerender } = render(BeeRunDrawer, { open: true, bees: roster(), onclose: vi.fn() });

		await user.selectOptions(await screen.findByLabelText('Bee'), 'builder');
		await user.type(screen.getByLabelText('Task'), 'first run');
		await rerender({ open: false, bees: roster(), onclose: vi.fn() });
		await rerender({ open: true, bees: roster(), onclose: vi.fn() });

		// Reopened with the operator's last task still in it, Run would re-send work
		// they had already decided about — and the bee choice resets with it.
		expect(await screen.findByLabelText('Task')).toHaveValue('');
		expect(screen.getByLabelText<HTMLSelectElement>('Bee').value).toBe('');
		expect(screen.getByRole('button', { name: 'Run' })).toBeDisabled();
	});

	it('reads the colony roster itself when the page has none', async () => {
		const user = userEvent.setup();
		// The trail page holds no roster of its own, and the picker it needs is the
		// colony one: a script bee is the row this control exists for.
		const { calls, bodies } = stubRun({}, beeRoster());
		render(BeeRunDrawer, { open: true, onclose: vi.fn() });

		const select = await screen.findByLabelText<HTMLSelectElement>('Bee');
		await waitFor(() => expect(select.options).toHaveLength(4));
		expect(calls).toEqual(['/api/bees?scope=colony']);

		await user.selectOptions(select, 'sweeper');
		await user.click(screen.getByRole('button', { name: 'Run' }));

		await waitFor(() => expect(bodies).toHaveLength(1));
		expect(calls).toContain('/api/bees/sweeper/run');
	});

	it('preselects a lone bee it read itself', async () => {
		stubRun({}, [bee()]);
		render(BeeRunDrawer, { open: true, onclose: vi.fn() });

		await waitFor(() =>
			expect(screen.getByLabelText<HTMLSelectElement>('Bee').value).toBe('builder')
		);
	});

	it('says a failed roster read instead of showing an empty picker', async () => {
		stubRun({});
		render(BeeRunDrawer, { open: true, onclose: vi.fn() });

		expect(await screen.findByRole('alert')).toHaveTextContent('not found');
	});

	it('joins the trail it was opened from, and says so', async () => {
		const user = userEvent.setup();
		const { bodies } = stubRun();
		render(BeeRunDrawer, {
			open: true,
			bees: roster(),
			traceId: 'trail-daily',
			onclose: vi.fn()
		});

		const trail = await screen.findByLabelText<HTMLTextAreaElement | HTMLInputElement>('Trail ID');
		expect(trail).toHaveValue('trail-daily');
		expect(
			screen.getByText(/joins it rather than opening a second one/)
		).toBeInTheDocument();

		await user.selectOptions(screen.getByLabelText('Bee'), 'builder');
		await user.type(screen.getByLabelText('Task'), 'go');
		await user.click(screen.getByRole('button', { name: 'Run' }));

		await waitFor(() => expect(bodies).toHaveLength(1));
		expect(bodies[0]?.traceId).toBe('trail-daily');
	});

	it('keeps a half-written task when the page re-reads its own bees', async () => {
		const user = userEvent.setup();
		stubRun();
		render(BeeRunDrawerHarness, { roster: beeRoster() });

		await user.click(screen.getByRole('button', { name: 'Open' }));
		await user.selectOptions(await screen.findByLabelText('Bee'), 'builder');
		await user.type(screen.getByLabelText('Task'), 'half-written');

		// The page behind the drawer re-reads its roster — a poll, a Refresh, a commit
		// landing. A form that reset here would throw away what the operator is in the
		// middle of typing, and the trail it was launched from with it.
		await user.click(screen.getByRole('button', { name: 'Re-read bees' }));

		expect(screen.getByLabelText('Task')).toHaveValue('half-written');
		expect(screen.getByLabelText<HTMLSelectElement>('Bee').value).toBe('builder');
		expect(screen.getByLabelText('Trail ID')).toHaveValue('trail-daily');
	});
});
