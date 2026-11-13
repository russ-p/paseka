<script lang="ts">
	import { base } from '$app/paths';
	import { RefreshCw } from 'lucide-svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { DataColumn } from '$lib/components/DataTable.svelte';
	import {
		skillProposalStateLabel,
		skillProposalTimeLabel,
		skillProposalsHeadline,
		skillProvenanceLabel,
		skillRolesLabel,
		skillStateLabel,
		skillStoreHeadline
	} from '$lib/format';
	import { traceDetailPath } from '$lib/navigation';
	import { createSkillsStore, type SkillsStore } from '$lib/stores/skills.svelte';
	import type { SkillProposal, SkillRow } from '$lib/api/types';
	import SkillProposalModal from './SkillProposalModal.svelte';

	let { store = createSkillsStore() }: { store?: SkillsStore } = $props();

	$effect(() => {
		store.start();
	});

	/** The proposal opened read-only; the modal owns its own body read. */
	let selected = $state<{ trail: string; name: string } | null>(null);

	const skillColumns: DataColumn<SkillRow>[] = [
		{
			key: 'name',
			label: 'Name',
			text: (skill) => skill.name,
			mono: true
		},
		{
			key: 'state',
			label: 'State',
			// The badge carries the whole verdict, reason included: an invalid
			// procedure without the rule it broke would send the operator looking
			// without telling them where.
			text: (skill) => skillStateLabel(skill),
			badge: (skill) => ({ status: skill.state, label: skillStateLabel(skill) })
		},
		{
			key: 'roles',
			label: 'Roles',
			// The server expands `*` and patterns, so this is the gate's real
			// answer: one name per role that receives the skill, never a wildcard.
			text: (skill) => skillRolesLabel(skill.roles)
		},
		{
			key: 'description',
			label: 'Description',
			text: (skill) => skill.description || '—',
			grow: true
		},
		{
			key: 'provenance',
			label: 'Provenance',
			// `hand` is the store's word for "nobody recorded where this came
			// from"; the page says what that means for the reader.
			text: (skill) => skillProvenanceLabel(skill.provenance)
		}
	];

	const proposalColumns: DataColumn<SkillProposal>[] = [
		{
			key: 'trail',
			label: 'Trail',
			text: (proposal) => proposal.trail,
			mono: true,
			// Every row links its source trail: the proposal is that trail's
			// output, and the trail is where the evidence lives.
			href: (proposal) => traceDetailPath(base, proposal.trail)
		},
		{
			key: 'skill',
			label: 'Skill',
			text: (proposal) => proposal.name,
			mono: true,
			action: (proposal) => ({
				label: 'View',
				onselect: () => (selected = { trail: proposal.trail, name: proposal.name })
			})
		},
		{
			key: 'proposed',
			label: 'Proposed',
			text: (proposal) => skillProposalTimeLabel(proposal.proposedAt)
		},
		{
			key: 'state',
			label: 'State',
			// A trial names its destination trail and a refusal its reason, so the
			// one cell answers what happened without a second column.
			text: (proposal) => skillProposalStateLabel(proposal),
			badge: (proposal) => ({
				status: proposal.state,
				label: skillProposalStateLabel(proposal)
			}),
			grow: true
		}
	];
</script>

<svelte:head>
	<title>Skills · Queen Console</title>
</svelte:head>

<div class="space-y-8">
	<header class="flex flex-wrap items-start justify-between gap-3">
		<div class="min-w-0 max-w-3xl space-y-1">
			<h1 class="text-3xl font-bold">Skills</h1>
			<p class="text-base-content/70">
				The procedures in <span class="font-mono">.paseka/skills</span>, with the roles whose
				selection carries each one, and the proposals the trails hold.
			</p>
			<p class="text-xs text-base-content/50">
				A proposal is reviewed here and decided here: Approve shows the validated install
				plan before anything is written, and Refuse records the decision. Neither writes a
				commit; trialling, disabling, and removing remain shell verbs.
			</p>
		</div>
		<div class="flex items-center gap-2">
			<button
				id="skills-refresh"
				type="button"
				class="btn btn-ghost btn-sm"
				disabled={store.loading}
				onclick={() => void store.refresh()}
			>
				<RefreshCw class="h-4 w-4" strokeWidth={2.5} />
				Refresh
			</button>
		</div>
	</header>

	{#if store.lastError}
		<div class="alert alert-error" role="alert"><span>{store.lastError}</span></div>
	{/if}

	{#if store.lastMessage}
		<!-- The decision's receipt: installing is a working-tree change the operator
		     has to commit, and refusing is a terminal decision on the bundle. -->
		<div class="alert alert-success" role="status">
			<span>{store.lastMessage}</span>
			<button type="button" class="btn btn-ghost btn-xs" onclick={() => store.clearMessage()}>
				Dismiss
			</button>
		</div>
	{/if}

	{#if store.showSkeletons}
		<div class="space-y-3" aria-busy="true">
			<span class="skeleton block h-3 w-1/3"></span>
			<span class="skeleton block h-52 w-full"></span>
			<span class="skeleton block h-3 w-1/4"></span>
			<span class="skeleton block h-40 w-full"></span>
		</div>
	{:else if store.inventory}
		{#if store.issues.length > 0}
			<!-- A store-wide finding — a stray entry, or a budget breach — has no
			     skill row of its own, so it is reported above the inventory rather
			     than silently folded into a row that does not name it. -->
			<div class="alert alert-warning" role="alert">
				<div class="space-y-1">
					{#each store.issues as issue (issue)}
						<span class="block">{issue}</span>
					{/each}
				</div>
			</div>
		{/if}

		<section class="space-y-3" aria-label="Store inventory">
			<div class="space-y-1">
				<h2 class="text-xl font-semibold">Store</h2>
				<p class="text-sm text-base-content/70">{skillStoreHeadline(store.skills.length)}</p>
				<p class="font-mono text-xs text-base-content/50">{store.storeRoot}</p>
			</div>
			<DataTable
				label="Skill store"
				columns={skillColumns}
				rows={store.skills}
				rowKey={(skill) => skill.name}
				emptyMessage="No skill is in the store yet. A procedure written into .paseka/skills, or approved from a proposal, appears here."
				filterLabel="Filter skills"
				filterPlaceholder="name, state, role, description, provenance"
				pageSize={10}
				stateKey="skills"
			/>
		</section>

		<section class="space-y-3" aria-label="Proposal inventory">
			<div class="space-y-1">
				<h2 class="text-xl font-semibold">Proposals</h2>
				<p class="text-sm text-base-content/70">{skillProposalsHeadline(store.proposals.length)}</p>
				<p class="text-xs text-base-content/50">
					A proposal is pending until a Beekeeper refuses it, installs it into the store, or
					trials it on one trail.
				</p>
			</div>

			{#if store.proposalIssues.length > 0}
				<!-- An unreadable trail must not look like a trail with no proposal. -->
				<div class="alert alert-warning" role="alert">
					<div class="space-y-1">
						{#each store.proposalIssues as issue (issue)}
							<span class="block">{issue}</span>
						{/each}
					</div>
				</div>
			{/if}

			<DataTable
				label="Skill proposals"
				columns={proposalColumns}
				rows={store.proposals}
				rowKey={(proposal) => `${proposal.trail}/${proposal.name}`}
				emptyMessage="No proposal is on record. A trail's Archivist run writes one here when it proposes a skill."
				filterLabel="Filter proposals"
				filterPlaceholder="trail, skill, state, destination"
				pageSize={10}
				stateKey="proposals"
			/>
		</section>
	{/if}
</div>

<SkillProposalModal
	open={selected !== null}
	trail={selected?.trail ?? ''}
	name={selected?.name ?? ''}
	{store}
	onclose={() => (selected = null)}
/>
