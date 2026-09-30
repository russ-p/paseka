import type { Topology, TopologyBee, TopologyEdge } from '$lib/api/types';

/**
 * The topology graph, as cytoscape elements plus the layout maths, kept free of
 * the library itself so both can be unit-tested. The legacy console kept the same
 * split inside one vanilla bundle; here it is a module the graph component
 * imports, and a second surface (the wiring table) can read the same edges.
 *
 * Node ids are prefixed `bee:` and `event:` because a role and an event id can
 * collide, and the prefix is what tells the layout which row a node belongs to.
 */

export interface TopologyNodeData {
	id: string;
	label: string;
	nodeType: 'bee' | 'event';
	/** On an event node, the contract, which the stylesheet colours by. */
	eventType?: string;
}

export interface TopologyEdgeData {
	id: string;
	source: string;
	target: string;
	label: string;
	edgeKind: 'subscribe' | 'publish' | 'invite';
	implicit: boolean;
}

export interface TopologyGraphElements {
	nodes: TopologyNodeData[];
	edges: TopologyEdgeData[];
}

export function beeNodeId(role: string): string {
	return `bee:${role}`;
}

export function eventNodeId(id: string): string {
	return `event:${id}`;
}

/** A bee node names its prompt vocabulary, because an intent is what it can be asked. */
export function topologyBeeLabel(bee: TopologyBee): string {
	if (!bee.intents?.length) return bee.role;
	return `${bee.role}\nintents: ${bee.intents.join(', ')}`;
}

/** The edge label says the direction word plus whatever qualifies the rule. */
export function topologyEdgeLabel(edge: TopologyEdge): string {
	if (edge.kind === 'subscribe') {
		const parts = ['subscribe'];
		if (edge.dispatch) parts.push(edge.dispatch);
		if (edge.implicit) parts.push('implicit');
		return parts.join(' ');
	}
	if (edge.kind === 'publish') return 'publish';
	const parts = ['invite'];
	if (edge.beeFrom) parts.push(`from=${edge.beeFrom}`);
	if (edge.intent) parts.push(`intent=${edge.intent}`);
	return parts.join(' ');
}

/**
 * The cytoscape element set. An edge naming a node the projection did not emit is
 * dropped rather than drawn to nowhere: a dangling edge is a server bug, and
 * cytoscape would throw on it.
 */
export function topologyElements(topology: Topology): TopologyGraphElements {
	const nodes: TopologyNodeData[] = topology.events.map((event) => ({
		id: eventNodeId(event.id),
		label: event.id,
		nodeType: 'event',
		eventType: event.type
	}));
	for (const bee of topology.bees) {
		nodes.push({ id: beeNodeId(bee.role), label: topologyBeeLabel(bee), nodeType: 'bee' });
	}
	const nodeIds = new Set(nodes.map((node) => node.id));

	const edges: TopologyEdgeData[] = [];
	topology.edges.forEach((edge, index) => {
		let source: string;
		let target: string;
		if (edge.kind === 'subscribe' || edge.kind === 'invite') {
			// Both point an event at the bee that reacts to it. An invite with no
			// target is a rule that chains off another bee's output, so it has no edge
			// of its own to draw.
			if (!edge.to) return;
			source = eventNodeId(edge.from);
			target = beeNodeId(edge.to);
		} else {
			source = beeNodeId(edge.from);
			target = eventNodeId(edge.to ?? '');
		}
		if (!nodeIds.has(source) || !nodeIds.has(target)) return;
		edges.push({
			// Index-keyed because the projection carries no edge id, and a stable key
			// is what lets cytoscape keep node positions across a re-read.
			id: `edge:${index}`,
			source,
			target,
			label: topologyEdgeLabel(edge),
			edgeKind: edge.kind,
			implicit: Boolean(edge.implicit)
		});
	});
	return { nodes, edges };
}

/** The nodes on the other side of an edge, used by the barycentre ordering below. */
export function neighbourIds(edges: TopologyEdgeData[], nodeId: string): string[] {
	const out: string[] = [];
	for (const edge of edges) {
		if (edge.source === nodeId) out.push(edge.target);
		else if (edge.target === nodeId) out.push(edge.source);
	}
	return out;
}

/**
 * Order one row by the mean position of its neighbours in the other row. A node
 * with no neighbour keeps its current rank rather than jumping to the top, which
 * is what makes an isolated event stay where the operator left it.
 */
export function orderByBarycentre(
	ids: string[],
	edges: TopologyEdgeData[],
	otherIndex: Map<string, number>
): string[] {
	return ids
		.map((id, fallback) => {
			const neighbours = neighbourIds(edges, id);
			const known = neighbours.filter((id) => otherIndex.has(id));
			const rank =
				known.length > 0
					? known.reduce((sum, id) => sum + (otherIndex.get(id) ?? 0), 0) / known.length
					: fallback;
			return { id, rank, fallback };
		})
		.sort((a, b) => a.rank - b.rank || a.fallback - b.fallback)
		.map((entry) => entry.id);
}

/** Two crossing-reduction passes, alternating rows, as the legacy console used. */
export function bipartiteOrder(
	beeIds: string[],
	eventIds: string[],
	edges: TopologyEdgeData[]
): { bees: string[]; events: string[] } {
	let bees = beeIds.slice();
	let events = eventIds.slice();
	for (let pass = 0; pass < 2; pass += 1) {
		const beeIndex = new Map(bees.map((id, index) => [id, index]));
		events = orderByBarycentre(events, edges, beeIndex);
		const eventIndex = new Map(events.map((id, index) => [id, index]));
		bees = orderByBarycentre(bees, edges, eventIndex);
	}
	return { bees, events };
}

/** One node's computed home, before any dragged position is applied. */
export interface TopologyPlacement {
	id: string;
	x: number;
	y: number;
}

export interface TopologyLayout {
	bees: TopologyPlacement[];
	events: TopologyPlacement[];
}

/**
 * Spacing, in pixels, estimated rather than measured. The legacy console measured
 * each node's label-inclusive bounding box, which needs a laid-out canvas and so
 * cannot be unit-tested; these constants approximate the same result and the
 * viewport `fit` corrects the remainder.
 */
const columnGap = 240;
const rowGap = 96;
/** Bees on the left, events filling a block to the right. */
const targetAspect = 3.2;

/**
 * Place the two sides. Bees are one column; events wrap into a block whose width
 * to height roughly matches the viewport, because a single row of fourteen event
 * ids is a line several thousand pixels long and the `fit` that frames it would
 * shrink the labels to nothing. Wrapping keeps the aspect close to the container's
 * so the first paint is already readable, and the operator can still drag anything.
 */
export function bipartiteLayout(
	beeIds: string[],
	eventIds: string[],
	edges: TopologyEdgeData[]
): TopologyLayout {
	const ordered = bipartiteOrder(beeIds, eventIds, edges);
	const place = (ids: string[], originX: number, wrap: boolean): TopologyPlacement[] => {
		// The bee side is always one column — that is what makes the graph read as
		// bees on the left and events on the right.
		const columns = wrap
			? Math.max(1, Math.min(ids.length, Math.ceil(Math.sqrt(ids.length * targetAspect))))
			: 1;
		const rows = Math.ceil(ids.length / columns);
		// Centre each block on the origin so the two sides straddle the middle.
		const offsetY = -((rows - 1) * rowGap) / 2;
		return ids.map((id, index) => ({
			id,
			x: originX + (index % columns) * columnGap,
			y: offsetY + Math.floor(index / columns) * rowGap
		}));
	};
	// The bee block is one column at the origin; the event block starts a column away.
	return {
		bees: place(ordered.bees, 0, false),
		events: place(ordered.events, columnGap, true)
	};
}

/** The saved-layout storage key, namespaced by colony so two colonies do not share one. */
export function topologyPositionsKey(colony: string | undefined): string | null {
	const slug = (colony ?? '').trim();
	if (!slug) return null;
	return `paseka:console:topology-positions:${slug}`;
}

export type SavedPositions = Record<string, { x: number; y: number }>;

export function loadTopologyPositions(
	storage: Pick<Storage, 'getItem'> | undefined,
	colony: string | undefined
): SavedPositions | null {
	const key = topologyPositionsKey(colony);
	if (!storage || !key) return null;
	try {
		const raw = storage.getItem(key);
		if (!raw) return null;
		const parsed: unknown = JSON.parse(raw);
		if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return null;
		return parsed as SavedPositions;
	} catch {
		// A corrupt blob must not cost the operator the graph; the layout just
		// recomputes and the next drag rewrites it.
		return null;
	}
}

export function saveTopologyPositions(
	storage: Pick<Storage, 'setItem'> | undefined,
	colony: string | undefined,
	positions: SavedPositions
): void {
	const key = topologyPositionsKey(colony);
	if (!storage || !key) return;
	try {
		storage.setItem(key, JSON.stringify(positions));
	} catch {
		// A full or blocked storage is not worth an error the operator can do nothing about.
	}
}

export function clearTopologyPositions(
	storage: Pick<Storage, 'removeItem'> | undefined,
	colony: string | undefined
): void {
	const key = topologyPositionsKey(colony);
	if (!storage || !key) return;
	try {
		storage.removeItem(key);
	} catch {
		// Same: the layout still recomputes, so a failed clear is cosmetic.
	}
}
