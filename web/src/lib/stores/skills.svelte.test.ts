import { describe, expect, it, vi } from 'vitest';
import { createSkillsStore } from './skills.svelte';
import { skillDecisionResult, skillProposal, skillsInventory } from '../../tests/fixtures';
import type { SkillDecisionResult, SkillsInventory } from '$lib/api/types';

/** The store's own reader, so the endpoint on the default path is assertable. */
const getSkills = vi.fn(async (): Promise<SkillsInventory> => skillsInventory());
vi.mock('$lib/api/client', () => ({
	getSkills: () => getSkills(),
	getSkillProposal: vi.fn(),
	getSkillApprovalPlan: vi.fn(),
	approveSkillProposal: vi.fn(),
	rejectSkillProposal: vi.fn()
}));

function harness(inventory: SkillsInventory = skillsInventory()) {
	const loadSkills = vi.fn(async () => inventory);
	const store = createSkillsStore({ loadSkills });
	return { store, loadSkills };
}

describe('createSkillsStore', () => {
	it('reads once on mount and never polls, because the store is committed config', async () => {
		const { store, loadSkills } = harness();

		expect(store.showSkeletons).toBe(true);
		await store.start();
		await store.start();

		expect(loadSkills).toHaveBeenCalledTimes(1);
		expect(store.showSkeletons).toBe(false);
		expect(store.skills).toHaveLength(2);
		expect(store.proposals).toHaveLength(1);
	});

	it('reads the shared inventory endpoint', async () => {
		await createSkillsStore().start();

		// One read carries the store and the proposals together, so the two panes
		// cannot land out of step with each other.
		expect(getSkills).toHaveBeenCalledTimes(1);
	});

	it('keeps the last good inventory on screen through a later failure', async () => {
		const loadSkills = vi
			.fn<() => Promise<SkillsInventory>>()
			.mockResolvedValueOnce(skillsInventory())
			.mockRejectedValueOnce(new Error('skills: ledger is corrupt'));
		const store = createSkillsStore({ loadSkills });
		await store.start();

		await store.refresh();

		expect(store.lastError).toBe('skills: ledger is corrupt');
		expect(store.skills).toHaveLength(2);
		expect(store.showSkeletons).toBe(false);
	});

	it('clears the failure once a read succeeds again', async () => {
		const loadSkills = vi
			.fn<() => Promise<SkillsInventory>>()
			.mockRejectedValueOnce(new Error('boom'))
			.mockResolvedValueOnce(skillsInventory());
		const store = createSkillsStore({ loadSkills });
		await store.start();
		expect(store.lastError).toBe('boom');

		await store.refresh();

		expect(store.lastError).toBe('');
		expect(store.skills).toHaveLength(2);
	});

	it('reports an empty store as empty rather than as a failed read', async () => {
		const { store } = harness({
			store: '/colony/.paseka/skills',
			skills: [],
			issues: [],
			proposals: [],
			proposalIssues: []
		});
		await store.start();

		expect(store.isEmpty).toBe(true);
		expect(store.hasProposals).toBe(false);
		expect(store.lastError).toBe('');
		// Before the first read, emptiness is unknown rather than false.
		expect(createSkillsStore().isEmpty).toBe(false);
	});

	it('applies the server inventory of a decision without a second read', async () => {
		const pending = skillsInventory({ proposals: [skillProposal()] });
		const installedProposal = skillProposal({ state: 'installed', destination: 'store' });
		const after = skillsInventory({
			proposals: [installedProposal]
		});
		const loadSkills = vi.fn(async () => pending);
		const store = createSkillsStore({
			loadSkills,
			approveProposal: async () =>
				skillDecisionResult({
					inventory: after,
					proposal: installedProposal,
					message: 'Installed the proposal. Nothing was committed.'
				})
		});
		await store.start();

		await store.approve('trace-a', 'paseka-release');

		expect(store.proposals[0].state).toBe('installed');
		expect(store.lastMessage).toBe('Installed the proposal. Nothing was committed.');
		expect(loadSkills).toHaveBeenCalledTimes(1);
	});

	it('refuses a second decision while one is in flight rather than queueing it', async () => {
		let release: ((result: SkillDecisionResult) => void) | undefined;
		const gate = new Promise<SkillDecisionResult>((resolve) => (release = resolve));
		const approveProposal = vi.fn(() => gate);
		const store = createSkillsStore({
			loadSkills: async () => skillsInventory({ proposals: [skillProposal()] }),
			approveProposal
		});
		await store.start();

		const first = store.approve('trace-a', 'paseka-release');
		await expect(store.approve('trace-a', 'paseka-release')).rejects.toThrow(/in flight/);
		expect(approveProposal).toHaveBeenCalledTimes(1);

		release?.(skillDecisionResult());
		await first;
		expect(store.mutating).toBe(false);
	});
});
