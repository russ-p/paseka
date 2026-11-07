import {
	approveSkillProposal,
	getSkillApprovalPlan,
	getSkillProposal,
	getSkills,
	rejectSkillProposal
} from '$lib/api/client';
import type {
	SkillApprovalPlan,
	SkillDecisionResult,
	SkillProposalDetail,
	SkillsInventory
} from '$lib/api/types';

interface SkillsStoreOptions {
	loadSkills?: () => Promise<SkillsInventory>;
	loadProposal?: (trail: string, name: string) => Promise<SkillProposalDetail>;
	loadApprovalPlan?: (trail: string, name: string) => Promise<SkillApprovalPlan>;
	approveProposal?: (trail: string, name: string) => Promise<SkillDecisionResult>;
	rejectProposal?: (trail: string, name: string, reason: string) => Promise<SkillDecisionResult>;
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
 * decision is the one fact the console writes, and the server answers it with
 * the refreshed inventory — so a successful approval or refusal updates the row
 * and the store together without a manual Refresh.
 *
 * One read answers both panes, so the inventory and the proposals cannot land
 * out of step with each other.
 */
export function createSkillsStore(options: SkillsStoreOptions = {}) {
	const readSkills = options.loadSkills ?? getSkills;
	const readProposal = options.loadProposal ?? getSkillProposal;
	const readPlan = options.loadApprovalPlan ?? getSkillApprovalPlan;
	const approve = options.approveProposal ?? approveSkillProposal;
	const reject = options.rejectProposal ?? rejectSkillProposal;

	let inventory = $state<SkillsInventory | null>(null);
	let loading = $state(true);
	let error = $state('');
	/**
	 * One decision at a time. The server refuses a second in-flight decision as
	 * busy, and the store does the same before it sends one, so a double click
	 * cannot become a queued second write.
	 */
	let mutating = $state(false);
	let lastMessage = $state('');
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

	async function decide(
		action: () => Promise<SkillDecisionResult>
	): Promise<SkillDecisionResult> {
		if (mutating) {
			throw new Error(
				'a proposal decision is already in flight; the console refuses a second one rather than queueing it'
			);
		}
		mutating = true;
		try {
			const result = await action();
			// The result carries the inventory after the action, so the row and the
			// store update in one step and Refresh is never needed to see the effect.
			inventory = result.inventory;
			error = '';
			lastMessage = result.message;
			return result;
		} finally {
			mutating = false;
		}
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
		/** A decision in flight, so every control that would start another is disabled. */
		get mutating(): boolean {
			return mutating;
		},
		/** The receipt of the last successful decision, cleared on the next read. */
		get lastMessage(): string {
			return lastMessage;
		},
		/** A colony with no store has not written a procedure, which is not a read failure. */
		get isEmpty(): boolean {
			return inventory !== null && inventory.skills.length === 0;
		},
		get hasProposals(): boolean {
			return (inventory?.proposals.length ?? 0) > 0;
		},
		proposal: readProposal,
		plan: readPlan,
		approve: (trail: string, name: string) => decide(() => approve(trail, name)),
		reject: (trail: string, name: string, reason: string) =>
			decide(() => reject(trail, name, reason)),
		clearMessage(): void {
			lastMessage = '';
		},
		refresh,
		start
	};
}

export type SkillsStore = ReturnType<typeof createSkillsStore>;
