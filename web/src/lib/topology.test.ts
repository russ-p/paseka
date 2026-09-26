import { describe, expect, it } from 'vitest';
import {
	bipartiteLayout,
	bipartiteOrder,
	beeNodeId,
	clearTopologyPositions,
	eventNodeId,
	loadTopologyPositions,
	neighbourIds,
	orderByBarycentre,
	saveTopologyPositions,
	topologyBeeLabel,
	topologyEdgeLabel,
	topologyElements,
	topologyPositionsKey
} from './topology';
import { topology } from '../tests/fixtures';
import { topologyStylesheet } from './topology-theme';

describe('topology node labels', () => {
	it('prefixes ids by side, because a role and an event id can collide', () => {
		expect(beeNodeId('scout')).toBe('bee:scout');
		expect(eventNodeId('SIGNAL/task.ready')).toBe('event:SIGNAL/task.ready');
		// A role that looks like an event kind must stay on its own side of the graph.
		expect(beeNodeId('SIGNAL/task.ready')).not.toBe(eventNodeId('SIGNAL/task.ready'));
	});

	it('gives a bee node its prompt vocabulary, because an intent is what it can be asked', () => {
		expect(topologyBeeLabel({ role: 'scout', adapter: 'cursor-agent', intents: ['triage', 'discovery'] })).toBe(
			'scout\nintents: triage, discovery'
		);
	});

	it('leaves a bee with no intents as just its role', () => {
		expect(topologyBeeLabel({ role: 'drone', adapter: 'pi' })).toBe('drone');
		expect(topologyBeeLabel({ role: 'drone', adapter: 'pi', intents: [] })).toBe('drone');
	});
});

describe('topologyEdgeLabel', () => {
	it('says the direction word and whatever qualifies the rule', () => {
		expect(
			topologyEdgeLabel({ kind: 'subscribe', from: 'a', to: 'b', dispatch: 'direct', implicit: true })
		).toBe('subscribe direct implicit');
		expect(topologyEdgeLabel({ kind: 'publish', from: 'a', to: 'b' })).toBe('publish');
		expect(
			topologyEdgeLabel({ kind: 'invite', from: 'a', to: 'b', intent: 'grilling', beeFrom: 'scout' })
		).toBe('invite from=scout intent=grilling');
	});

	it('leaves out a qualifier the rule did not set', () => {
		expect(topologyEdgeLabel({ kind: 'subscribe', from: 'a', to: 'b' })).toBe('subscribe');
		expect(topologyEdgeLabel({ kind: 'invite', from: 'a', to: 'b' })).toBe('invite');
	});
});

describe('topologyElements', () => {
	it('builds a node per event and per bee, tagged with its side', () => {
		const { nodes } = topologyElements(topology());

		expect(nodes.filter((node) => node.nodeType === 'event')).toHaveLength(5);
		expect(nodes.filter((node) => node.nodeType === 'bee')).toHaveLength(3);
		const signal = nodes.find((node) => node.id === 'event:SIGNAL/task.ready');
		// The contract is what the stylesheet colours by, so it has to reach the node.
		expect(signal?.eventType).toBe('SIGNAL');
	});

	it('points each rule the way the contract reads', () => {
		const { edges } = topologyElements(topology());
		const byKind = (kind: string) => edges.find((edge) => edge.edgeKind === kind);

		// A subscription and an invite both point an event at the bee that reacts.
		expect(byKind('subscribe')).toMatchObject({
			source: 'event:SIGNAL/task.ready',
			target: 'bee:scout'
		});
		expect(byKind('invite')).toMatchObject({
			source: 'event:SIGNAL/feature.classified',
			target: 'bee:drone'
		});
		// A publication points the other way.
		expect(byKind('publish')).toMatchObject({
			source: 'bee:builder',
			target: 'event:MUTATION/code.proposal.isolated'
		});
	});

	it('carries the implicit flag through, since it is what dims the edge', () => {
		const { edges } = topologyElements(topology());

		expect(edges.find((edge) => edge.target === 'bee:scout')?.implicit).toBe(true);
		expect(edges.find((edge) => edge.target === 'bee:builder')?.implicit).toBe(false);
	});

	it('drops an invite with no target instead of drawing an edge to nowhere', () => {
		// An invite that chains off another bee's output has no destination of its own.
		const { edges } = topologyElements(
			topology({ edges: [{ kind: 'invite', from: 'SIGNAL/task.ready', beeFrom: 'scout' }] })
		);

		expect(edges).toHaveLength(0);
	});

	it('drops an edge naming a node the projection never emitted', () => {
		// A dangling edge is a server bug, and cytoscape throws on one rather than
		// drawing a line into the void.
		const { edges } = topologyElements(
			topology({ edges: [{ kind: 'publish', from: 'ghost', to: 'SIGNAL/task.ready' }] })
		);

		expect(edges).toHaveLength(0);
	});

	it('gives every edge a stable id, so positions survive a re-read', () => {
		const first = topologyElements(topology());
		const second = topologyElements(topology());

		expect(first.edges.map((edge) => edge.id)).toEqual(second.edges.map((edge) => edge.id));
	});
});

describe('bipartite ordering', () => {
	const edges = [
		{ id: 'e0', source: 'event:a', target: 'bee:x', label: '', edgeKind: 'subscribe' as const, implicit: false },
		{ id: 'e1', source: 'event:b', target: 'bee:y', label: '', edgeKind: 'subscribe' as const, implicit: false },
		{ id: 'e2', source: 'event:c', target: 'bee:x', label: '', edgeKind: 'subscribe' as const, implicit: false }
	];

	it('finds the nodes on the other side of an edge', () => {
		expect(neighbourIds(edges, 'event:a')).toEqual(['bee:x']);
		expect(neighbourIds(edges, 'bee:x').sort()).toEqual(['event:a', 'event:c']);
		expect(neighbourIds(edges, 'event:zzz')).toEqual([]);
	});

	it('pulls a node toward the middle of its neighbours, which is what cuts crossings', () => {
		const otherIndex = new Map([
			['bee:x', 0],
			['bee:y', 2]
		]);
		// a hangs off x at 0, c hangs off x at 0, b hangs off y at 2, so b sorts last.
		expect(orderByBarycentre(['event:a', 'event:b', 'event:c'], edges, otherIndex)).toEqual([
			'event:a',
			'event:c',
			'event:b'
		]);
	});

	it('keeps a node with no known neighbour at its own index, so it cannot leapfrog a wired one', () => {
		const edges = [
			{
				id: 'e0',
				source: 'event:conn',
				target: 'bee:y',
				label: '',
				edgeKind: 'subscribe' as const,
				implicit: false
			}
		];
		const otherIndex = new Map([['bee:y', 1]]);
		// `iso` and `tail` have no edges, so each keeps the rank it already had. Had
		// they been given rank 0 instead, both would tie at the top and push `conn`
		// to the end — moving a node the operator never touched.
		expect(orderByBarycentre(['event:iso', 'event:conn', 'event:tail'], edges, otherIndex)).toEqual([
			'event:iso',
			'event:conn',
			'event:tail'
		]);
	});

	it('alternates both rows so the ordering settles rather than oscillating', () => {
		const ordered = bipartiteOrder(['bee:x', 'bee:y'], ['event:a', 'event:b', 'event:c'], edges);

		expect(ordered.bees).toHaveLength(2);
		expect(ordered.events).toHaveLength(3);
		// x serves a and c, y serves b, so the two rows cannot both be sorted the same way.
		expect(ordered.events.indexOf('event:b')).toBeGreaterThan(ordered.events.indexOf('event:a'));
	});

	it('never stacks two nodes on the same point', () => {
		// The first draft gave every node in a side the same Y, which piled all seven
		// bees on top of each other. Every placement has to be distinct.
		const layout = bipartiteLayout(
			['bee:a', 'bee:b', 'bee:c'],
			['event:a', 'event:b', 'event:c', 'event:d', 'event:e'],
			edges
		);
		const points = [...layout.bees, ...layout.events].map((node) => `${node.x},${node.y}`);

		expect(new Set(points).size).toBe(points.length);
	});

	it('keeps the bees in one column and the events clear of it', () => {
		const layout = bipartiteLayout(['bee:a', 'bee:b'], ['event:a', 'event:b'], edges);

		expect(new Set(layout.bees.map((node) => node.x)).size).toBe(1);
		expect(layout.bees.map((node) => node.x)).toEqual([0, 0]);
		for (const event of layout.events) {
			expect(event.x).toBeGreaterThan(0);
		}
	});

	it('wraps a long event side so the first paint is not scaled to nothing', () => {
		// Fourteen ids in one row is several thousand pixels wide; the fit that frames
		// it would shrink the labels away. Wrapping keeps the shape close to the
		// viewport's aspect instead.
		const events = Array.from({ length: 14 }, (_, index) => `event:${index}`);
		const layout = bipartiteLayout(['bee:a'], events, []);

		const columns = new Set(layout.events.map((node) => node.x)).size;
		expect(columns).toBeGreaterThan(1);
		expect(columns).toBeLessThan(events.length);
	});

	it('places a single node on each side without dividing by zero', () => {
		const layout = bipartiteLayout(['bee:only'], ['event:only'], []);

		expect(layout.bees).toEqual([{ id: 'bee:only', x: 0, y: 0 }]);
		expect(layout.events).toHaveLength(1);
	});

	it('keeps every node it was given', () => {
		const bees = ['bee:a', 'bee:b', 'bee:c'];
		const events = ['event:a', 'event:b', 'event:c', 'event:d'];
		const layout = bipartiteLayout(bees, events, edges);

		expect(layout.bees.map((node) => node.id).sort()).toEqual([...bees].sort());
		expect(layout.events.map((node) => node.id).sort()).toEqual([...events].sort());
	});
});

describe('saved layout storage', () => {
	function memoryStorage(initial: Record<string, string> = {}) {
		const map = new Map(Object.entries(initial));
		return {
			getItem: (key: string) => map.get(key) ?? null,
			setItem: (key: string, value: string) => void map.set(key, value),
			removeItem: (key: string) => void map.delete(key),
			read: (key: string) => map.get(key) ?? null
		};
	}

	it('namespaces the key by colony, so two colonies do not share one shape', () => {
		expect(topologyPositionsKey('apiary')).toBe('paseka:console:topology-positions:apiary');
		expect(topologyPositionsKey('  ')).toBeNull();
		expect(topologyPositionsKey(undefined)).toBeNull();
	});

	it('round-trips a dragged shape', () => {
		const storage = memoryStorage();
		saveTopologyPositions(storage, 'apiary', { 'bee:scout': { x: 1, y: 2 } });

		expect(loadTopologyPositions(storage, 'apiary')).toEqual({ 'bee:scout': { x: 1, y: 2 } });
	});

	it('survives a corrupt blob by recomputing, rather than losing the graph', () => {
		const storage = memoryStorage({ 'paseka:console:topology-positions:apiary': 'not json' });

		expect(loadTopologyPositions(storage, 'apiary')).toBeNull();
	});

	it('refuses a non-object blob, which would otherwise spread into nothing', () => {
		const storage = memoryStorage({ 'paseka:console:topology-positions:apiary': '[1,2,3]' });

		expect(loadTopologyPositions(storage, 'apiary')).toBeNull();
	});

	it('forgets the shape on reset so it cannot come back on the next read', () => {
		const storage = memoryStorage();
		saveTopologyPositions(storage, 'apiary', { 'bee:scout': { x: 1, y: 2 } });

		clearTopologyPositions(storage, 'apiary');

		expect(loadTopologyPositions(storage, 'apiary')).toBeNull();
	});

	it('does nothing without a colony, rather than writing a shared key', () => {
		const storage = memoryStorage();
		saveTopologyPositions(storage, undefined, { 'bee:scout': { x: 1, y: 2 } });

		expect(storage.read('paseka:console:topology-positions:')).toBeNull();
		expect(loadTopologyPositions(storage, undefined)).toBeNull();
	});
});

describe('topologyStylesheet', () => {
	const tokens = {
		surface: '#111',
		surfaceRaised: '#222',
		border: '#333',
		text: '#444',
		textMuted: '#555',
		info: '#666',
		secondary: '#777',
		warning: '#888',
		success: '#999'
	};

	it('keeps one hue per contract so the four stay tellable apart', () => {
		const sheet = topologyStylesheet(tokens);
		const colourFor = (selector: string) =>
			sheet.find((block) => block.selector === selector)?.style['border-color'];

		expect(colourFor('node[eventType = "SIGNAL"]')).toBe(tokens.info);
		expect(colourFor('node[eventType = "INSIGHT"]')).toBe(tokens.secondary);
		expect(colourFor('node[eventType = "MUTATION"]')).toBe(tokens.warning);
		expect(colourFor('node[eventType = "VERIFICATION"]')).toBe(tokens.success);
	});

	it('keeps the legacy line language, so the graph reads the same as before', () => {
		const sheet = topologyStylesheet(tokens);
		const styleFor = (selector: string) => sheet.find((block) => block.selector === selector)?.style;

		expect(styleFor('edge[edgeKind = "publish"]')?.['line-style']).toBe('dashed');
		expect(styleFor('edge[edgeKind = "invite"]')?.['line-style']).toBe('dotted');
		expect(styleFor('edge[implicit = "true"]')?.opacity).toBe(0.65);
	});

	it('colours everything from tokens, so a theme switch reaches the canvas', () => {
		const sheet = topologyStylesheet(tokens);
		const colours = sheet.flatMap((block) => Object.values(block.style));

		// No hard-coded value may survive, or the graph stays dark on a light console.
		for (const value of colours) {
			if (typeof value !== 'string') continue;
			if (value.startsWith('#')) expect(Object.values(tokens)).toContain(value);
		}
	});
});
