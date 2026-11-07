import { getSkills } from '$lib/api/client';
import type { SkillsInventory } from '$lib/api/types';

interface SkillsStoreOptions {
	loadSkills?: () => Promise<SkillsInventory>;
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

/**
 * The colony skill store and the trails' proposals, behind `/next/skills`.
 *
 * It does not poll, for the reason the Bees roster does not: the store is
 * committed configuration and the role lists that gate it live in committed bee
 * YAML, so both change when a commit lands rather than on a clock. A proposal
 * decision is the one fact the shell writes, and Refresh is how an operator sees
 * it — a timer would re-read two directories for a change nobody made in the
 * browser.
 *
 * One read answers both panes, so the inventory and the proposals cannot land
 * out of step with each other.
 */
export function createSkillsStore(options: SkillsStoreOptions = {}) {
	const readSkills = options.loadSkills ?? getSkills;

	let inventory = $state<SkillsInventory | null>(null);
	let loading = $state(true);
	let error = $state('');
	let started = false;

	async function refresh(): Promise<void> {
		loading = true;
		try {
			inventory = await readSkills();
			error = '';
		} catch (cause) {
			// A failed read keeps the last good inventory on screen: the skills it
			// held are still the last real ones, and blanking them would hide what
			// the operator was reading behind the failure.
			error = errorMessage(cause);
		} finally {
			loading = false;
		}
	}

	/** Guarded, so a remount or a re-run effect does not read twice. */
	function start(): void {
		if (started) return;
		started = true;
		void refresh();
	}

	return {
		get inventory(): SkillsInventory | null {
			return inventory;
		},
		get storeRoot(): string {
			return inventory?.store ?? '';
		},
		get skills() {
			return inventory?.skills ?? [];
		},
		get proposals() {
			return inventory?.proposals ?? [];
		},
		get issues(): string[] {
			return inventory?.issues ?? [];
		},
		get proposalIssues(): string[] {
			return inventory?.proposalIssues ?? [];
		},
		/** Skeletons only before the first payload; a later failure keeps the rows. */
		get showSkeletons(): boolean {
			return loading && inventory === null;
		},
		get loading(): boolean {
			return loading;
		},
		get lastError(): string {
			return error;
		},
		/** A colony with no store has not written a procedure, which is not a read failure. */
		get isEmpty(): boolean {
			return inventory !== null && inventory.skills.length === 0;
		},
		get hasProposals(): boolean {
			return (inventory?.proposals.length ?? 0) > 0;
		},
		refresh,
		start
	};
}

export type SkillsStore = ReturnType<typeof createSkillsStore>;
