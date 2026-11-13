import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Skills from './+page.svelte';
import { createSkillsStore } from '$lib/stores/skills.svelte';
import { skillProposal, skillRow, skillsInventory, skillApprovalPlan, skillDecisionResult } from '../../tests/fixtures';
import type { SkillsInventory } from '$lib/api/types';

type StoreOptions = NonNullable<Parameters<typeof createSkillsStore>[0]>;

afterEach(() => {
	vi.unstubAllGlobals();
});

function harness(inventory: SkillsInventory = skillsInventory(), extra: StoreOptions = {}) {
	const loadSkills = vi.fn(async () => inventory);
	return createSkillsStore({ loadSkills, ...extra });
}

function storeTable(): HTMLElement {
	const region = screen.getByRole('region', { name: 'Store inventory' });
	return within(region).getByRole('table');
}

function proposalsTable(): HTMLElement {
	const region = screen.getByRole('region', { name: 'Proposal inventory' });
	return within(region).getByRole('table');
}

/** The column headers of a table, so a cell index is named rather than counted. */
function headers(table: HTMLElement): string[] {
	return within(table)
		.getAllByRole('columnheader')
		.map((cell) => cell.textContent ?? '');
}

function cellOf(table: HTMLElement, row: string, column: string): HTMLElement {
	const index = headers(table).indexOf(column);
	if (index < 0) throw new Error(`no ${column} column in ${headers(table).join(', ')}`);
	const rowElement = within(table).getByText(row).closest('tr');
	return within(rowElement as HTMLElement).getAllByRole('cell')[index] as HTMLElement;
}

describe('skills route', () => {
	it('lists the store with state, expanded roles, description, and provenance', async () => {
		render(Skills, { store: harness() });
		await waitFor(() => expect(screen.getByRole('region', { name: 'Store inventory' })).toBeInTheDocument());

		const table = storeTable();
		// The server expands the wildcard, so a role is a name rather than `*`.
		expect(cellOf(table, 'paseka-release', 'Roles')).toHaveTextContent('builder, docs');
		expect(cellOf(table, 'paseka-release', 'State')).toHaveTextContent('ok');
		expect(cellOf(table, 'paseka-release', 'Description')).toHaveTextContent('Run the release procedure.');
		// The store's `proposal` reads as what it means to an operator.
		expect(cellOf(table, 'paseka-release', 'Provenance')).toHaveTextContent('proposed');
		expect(cellOf(table, 'paseka-ship', 'Provenance')).toHaveTextContent('hand-written');
	});

	it('keeps an invalid or disabled skill visible with its reason', async () => {
		render(Skills, {
			store: harness(
				skillsInventory({
					skills: [
						skillRow({
							name: 'broken',
							state: 'invalid',
							reason: 'SKILL.md is missing',
							roles: [],
							provenance: 'hand'
						}),
						skillRow({ name: 'paseka-ship', state: 'disabled', reason: 'wrong step' })
					]
				})
			)
		});
		await waitFor(() => expect(screen.getByText('broken')).toBeInTheDocument());

		const table = storeTable();
		expect(cellOf(table, 'broken', 'State')).toHaveTextContent('invalid: SKILL.md is missing');
		expect(cellOf(table, 'paseka-ship', 'State')).toHaveTextContent('disabled: wrong step');
		// A skill no role carries says so rather than leaving a blank cell.
		expect(cellOf(table, 'broken', 'Roles')).toHaveTextContent('—');
	});

	it('lists proposals with their source trail, skill, time, and state', async () => {
		const pending = skillProposal();
		const trialled = skillProposal({
			trail: 'trace-02b1ce7074bb25ef',
			name: 'paseka-ship',
			state: 'trialled',
			destination: 'trace-03c2df8185cc36f0',
			description: 'Ship a version.'
		});
		const missing = skillProposal({
			trail: 'trace-04d3e09296dd47a1',
			name: 'paseka-gone',
			state: 'missing',
			bundleMissing: true,
			valid: false
		});
		render(Skills, {
			store: harness(skillsInventory({ proposals: [pending, trialled, missing] }))
		});
		await waitFor(() =>
			expect(screen.getByRole('region', { name: 'Proposal inventory' })).toBeInTheDocument()
		);

		const table = proposalsTable();
		const trailLink = within(cellOf(table, pending.trail, 'Trail')).getByRole('link');
		expect(trailLink).toHaveAttribute('href', `/traces/${pending.trail}`);
		expect(cellOf(table, pending.trail, 'Proposed')).toHaveTextContent(
			/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/
		);

		// A trial names the trail it went to, because the state word alone does not
		// say where the procedure now runs.
		expect(cellOf(table, trialled.trail, 'State')).toHaveTextContent(
			'trialled: trace-03c2df8185cc36f0'
		);
		// A missing bundle is a state, not a hidden row.
		expect(cellOf(table, missing.trail, 'State')).toHaveTextContent('missing');
	});

	it('opens a proposal read-only with its body and the reason it was proposed', async () => {
		const user = userEvent.setup();
		const proposal = skillProposal();
		vi.stubGlobal(
			'fetch',
			vi.fn(async () => {
				return new Response(
					JSON.stringify({
						...proposal,
						body: '# paseka-release\n\n1. Do the thing.\n'
					}),
					{ status: 200, headers: { 'Content-Type': 'application/json' } }
				);
			})
		);
		render(Skills, { store: harness() });
		await waitFor(() =>
			expect(screen.getByRole('region', { name: 'Proposal inventory' })).toBeInTheDocument()
		);

		await user.click(
			within(proposalsTable()).getByRole('button', {
				name: `View ${proposal.trail}/${proposal.name}`
			})
		);

		const dialog = await screen.findByRole('dialog');
		expect(within(dialog).getByText(/Two runs repeated this by hand/)).toBeInTheDocument();
		expect(within(dialog).getByText(/1\. Do the thing\./)).toBeInTheDocument();
		// Read-only: the body is a preview, not an editor.
		expect(within(dialog).queryByRole('textbox')).not.toBeInTheDocument();
	});

	it('reports a proposal whose bundle has disappeared where the body would be', async () => {
		const user = userEvent.setup();
		const gone = skillProposal({
			trail: 'trace-gone',
			name: 'paseka-gone',
			state: 'missing',
			bundleMissing: true,
			valid: false
		});
		vi.stubGlobal(
			'fetch',
			vi.fn(async () => {
				return new Response(
					JSON.stringify({
						...gone,
						body: '',
						bodyError: 'skills: proposal trace-gone/paseka-gone has no bundle at "skill-proposals/paseka-gone"'
					}),
					{ status: 200, headers: { 'Content-Type': 'application/json' } }
				);
			})
		);
		render(Skills, { store: harness(skillsInventory({ proposals: [gone] })) });
		await waitFor(() =>
			expect(screen.getByRole('region', { name: 'Proposal inventory' })).toBeInTheDocument()
		);

		await user.click(
			within(proposalsTable()).getByRole('button', { name: `View ${gone.trail}/${gone.name}` })
		);

		const dialog = await screen.findByRole('dialog');
		expect(within(dialog).getByRole('alert')).toHaveTextContent('has no bundle');
	});

	it('explains what an empty store and an empty proposal pane would hold', async () => {
		render(Skills, {
			store: harness({
				store: '/colony/.paseka/skills',
				skills: [],
				issues: [],
				proposals: [],
				proposalIssues: []
			})
		});

		await waitFor(() => expect(screen.getByText('No skill is in the store yet.')).toBeInTheDocument());
		expect(
			screen.getByText(/No skill is in the store yet\. A procedure written into/)
		).toBeInTheDocument();
		expect(screen.getByText('No proposal is on record.')).toBeInTheDocument();
		expect(
			screen.getByText(/No proposal is on record\. A trail's Archivist run/)
		).toBeInTheDocument();
	});

	it('reports store-wide and unreadable-trail findings in place', async () => {
		render(Skills, {
			store: harness(
				skillsInventory({
					issues: ['entry "README" is not a directory'],
					proposalIssues: ['trail trace-bad: read trail: permission denied']
				})
			)
		});

		await waitFor(() => expect(screen.getByText('entry "README" is not a directory')).toBeInTheDocument());
		expect(screen.getByText('trail trace-bad: read trail: permission denied')).toBeInTheDocument();
	});

	it('skeletons while the first read is in flight, and alerts when it fails', async () => {
		let release: ((inventory: SkillsInventory) => void) | undefined;
		const gate = new Promise<SkillsInventory>((resolve) => (release = resolve));
		const slow = createSkillsStore({ loadSkills: () => gate });
		const { container, unmount } = render(Skills, { store: slow });

		await waitFor(() => expect(container.querySelectorAll('.skeleton').length).toBeGreaterThan(0));
		unmount();
		release?.(skillsInventory());

		const failed = createSkillsStore({
			loadSkills: async () => {
				throw new Error('skills: read store: permission denied');
			}
		});
		render(Skills, { store: failed });
		await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('permission denied'));
		expect(screen.queryByRole('region', { name: 'Store inventory' })).not.toBeInTheDocument();
	});

	it('shows the validated install plan first, and cancelling writes nothing', async () => {
		const user = userEvent.setup();
		const proposal = skillProposal();
		const approveProposal = vi.fn();
		const loadApprovalPlan = vi.fn(async () => skillApprovalPlan());
		const store = harness(skillsInventory({ proposals: [proposal] }), {
			loadProposal: async () => ({ ...proposal, body: '# paseka-release\n\n1. Do the thing.\n' }),
			loadApprovalPlan,
			approveProposal
		});
		render(Skills, { store });
		await waitFor(() =>
			expect(screen.getByRole('region', { name: 'Proposal inventory' })).toBeInTheDocument()
		);

		await user.click(
			within(proposalsTable()).getByRole('button', {
				name: `View ${proposal.trail}/${proposal.name}`
			})
		);
		const dialog = await screen.findByRole('dialog');
		await user.click(within(dialog).getByRole('button', { name: 'Approve' }));

		// The confirmation names the skill, its source, the destination, the files,
		// and what happens to the working tree.
		await waitFor(() => expect(within(dialog).getByText('Source trail')).toBeInTheDocument());
		expect(within(dialog).getByText(proposal.trail)).toBeInTheDocument();
		expect(within(dialog).getByText(/untracked file path/)).toHaveTextContent(
			'Committing is yours; this action never commits.'
		);
		expect(within(dialog).getByText('.paseka/skills/paseka-release/SKILL.md')).toBeInTheDocument();
		expect(loadApprovalPlan).toHaveBeenCalledWith(proposal.trail, proposal.name);

		await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));

		expect(approveProposal).not.toHaveBeenCalled();
		expect(cellOf(proposalsTable(), proposal.trail, 'State')).toHaveTextContent('pending');
	});

	it('updates the decided row and the inventory from the server result, without a re-read', async () => {
		const user = userEvent.setup();
		const proposal = skillProposal();
		const installed = skillProposal({ state: 'installed', destination: 'store' });
		const before = skillsInventory({
			skills: [skillRow({ name: 'paseka-ship', provenance: 'hand' })],
			proposals: [proposal]
		});
		const after = skillsInventory({
			skills: [skillRow(), skillRow({ name: 'paseka-ship', provenance: 'hand' })],
			proposals: [installed]
		});
		const loadSkills = vi.fn(async () => before);
		const approveProposal = vi.fn(async () =>
			skillDecisionResult({
				inventory: after,
				proposal: installed,
				message: 'Installed the proposal. Nothing was committed.'
			})
		);
		const store = createSkillsStore({
			loadSkills,
			loadProposal: async () => ({ ...proposal, body: '# paseka-release\n' }),
			loadApprovalPlan: async () => skillApprovalPlan(),
			approveProposal
		});
		render(Skills, { store });
		await waitFor(() => expect(screen.getByText('paseka-ship')).toBeInTheDocument());

		await user.click(
			within(proposalsTable()).getByRole('button', {
				name: `View ${proposal.trail}/${proposal.name}`
			})
		);
		const dialog = await screen.findByRole('dialog');
		await user.click(within(dialog).getByRole('button', { name: 'Approve' }));
		await user.click(await within(dialog).findByRole('button', { name: 'Install skill' }));

		await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
		expect(cellOf(proposalsTable(), proposal.trail, 'State')).toHaveTextContent('installed');
		// The new skill appears in the store pane and the receipt says what the
		// action did; neither needed a second read.
		expect(cellOf(storeTable(), 'paseka-release', 'Provenance')).toHaveTextContent('proposed');
		expect(screen.getByText(/Nothing was committed/)).toBeInTheDocument();
		expect(loadSkills).toHaveBeenCalledTimes(1);
	});

	it('refuses with a reason and shows the decision is terminal', async () => {
		const user = userEvent.setup();
		const proposal = skillProposal();
		const refused = skillProposal({ state: 'refused', reason: 'one incident, not a procedure' });
		const rejectProposal = vi.fn(async () =>
			skillDecisionResult({
				inventory: skillsInventory({ proposals: [refused] }),
				proposal: refused,
				message: 'Refused the proposal; the decision is terminal.'
			})
		);
		const store = harness(skillsInventory({ proposals: [proposal] }), {
			loadProposal: async () => ({ ...proposal, body: '# paseka-release\n' }),
			rejectProposal
		});
		render(Skills, { store });
		await waitFor(() =>
			expect(screen.getByRole('region', { name: 'Proposal inventory' })).toBeInTheDocument()
		);

		await user.click(
			within(proposalsTable()).getByRole('button', {
				name: `View ${proposal.trail}/${proposal.name}`
			})
		);
		const dialog = await screen.findByRole('dialog');
		await user.click(within(dialog).getByRole('button', { name: 'Refuse' }));

		// The dialog says the decision is terminal for the bundle before it is taken.
		expect(within(dialog).getByText(/terminal for this bundle/)).toBeInTheDocument();
		await user.type(
			within(dialog).getByRole('textbox', { name: 'Reason (optional)' }),
			'one incident, not a procedure'
		);
		await user.click(within(dialog).getByRole('button', { name: 'Refuse proposal' }));

		await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
		expect(rejectProposal).toHaveBeenCalledWith(
			proposal.trail,
			proposal.name,
			'one incident, not a procedure'
		);
		expect(cellOf(proposalsTable(), proposal.trail, 'State')).toHaveTextContent(
			'refused: one incident, not a procedure'
		);
	});

	it('refuses without a reason, which is still one decided bundle', async () => {
		const user = userEvent.setup();
		const proposal = skillProposal();
		const refused = skillProposal({ state: 'refused' });
		const rejectProposal = vi.fn(async () =>
			skillDecisionResult({
				inventory: skillsInventory({ proposals: [refused] }),
				proposal: refused,
				message: 'Refused the proposal; the decision is terminal.'
			})
		);
		const store = harness(skillsInventory({ proposals: [proposal] }), {
			loadProposal: async () => ({ ...proposal, body: '# paseka-release\n' }),
			rejectProposal
		});
		render(Skills, { store });
		await waitFor(() =>
			expect(screen.getByRole('region', { name: 'Proposal inventory' })).toBeInTheDocument()
		);

		await user.click(
			within(proposalsTable()).getByRole('button', {
				name: `View ${proposal.trail}/${proposal.name}`
			})
		);
		const dialog = await screen.findByRole('dialog');
		await user.click(within(dialog).getByRole('button', { name: 'Refuse' }));
		await user.click(within(dialog).getByRole('button', { name: 'Refuse proposal' }));

		await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
		expect(rejectProposal).toHaveBeenCalledWith(proposal.trail, proposal.name, '');
		expect(cellOf(proposalsTable(), proposal.trail, 'State')).toHaveTextContent('refused');
	});

	it('reports the server reason for a failed action and keeps the programme as it was', async () => {
		const user = userEvent.setup();
		const proposal = skillProposal();
		const store = harness(skillsInventory({ proposals: [proposal] }), {
			loadProposal: async () => ({ ...proposal, body: '# paseka-release\n' }),
			loadApprovalPlan: async () => skillApprovalPlan(),
			approveProposal: async () => {
				throw new Error('skills: the store already holds a skill named "paseka-release"');
			}
		});
		render(Skills, { store });
		await waitFor(() =>
			expect(screen.getByRole('region', { name: 'Proposal inventory' })).toBeInTheDocument()
		);

		await user.click(
			within(proposalsTable()).getByRole('button', {
				name: `View ${proposal.trail}/${proposal.name}`
			})
		);
		const dialog = await screen.findByRole('dialog');
		await user.click(within(dialog).getByRole('button', { name: 'Approve' }));
		await user.click(await within(dialog).findByRole('button', { name: 'Install skill' }));

		await waitFor(() =>
			expect(within(dialog).getByRole('alert')).toHaveTextContent(
				'the store already holds a skill named "paseka-release"'
			)
		);
		// The failure is not a decision: the dialog stays on the plan and the row
		// is still pending.
		expect(screen.getByRole('dialog')).toBeInTheDocument();
		expect(cellOf(proposalsTable(), proposal.trail, 'State')).toHaveTextContent('pending');
	});
});
