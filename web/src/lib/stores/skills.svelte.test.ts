import { describe, expect, it, vi } from 'vitest';
import { createSkillsStore } from './skills.svelte';
import { skillsInventory } from '../../tests/fixtures';
import type { SkillsInventory } from '$lib/api/types';

/** The store's own reader, so the endpoint on the default path is assertable. */
const getSkills = vi.fn(async (): Promise<SkillsInventory> => skillsInventory());
vi.mock('$lib/api/client', () => ({ getSkills: () => getSkills() }));

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
});
