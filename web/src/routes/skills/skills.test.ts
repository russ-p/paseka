import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Skills from './+page.svelte';
import { createSkillsStore } from '$lib/stores/skills.svelte';
import { skillProposal, skillRow, skillsInventory } from '../../tests/fixtures';
import type { SkillsInventory } from '$lib/api/types';

afterEach(() => {
	vi.unstubAllGlobals();
});

function harness(inventory: SkillsInventory = skillsInventory()) {
	const loadSkills = vi.fn(async () => inventory);
	return createSkillsStore({ loadSkills });
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
		expect(trailLink).toHaveAttribute('href', `/next/traces/${pending.trail}`);
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
});
