/**
 * Cytoscape is imperative and has no notion of a theme, so its stylesheet needs
 * concrete colours. Rather than paste hex literals into a Svelte file — which the
 * design system forbids, and which would also be wrong the moment the operator
 * switched theme — the graph reads the active DaisyUI custom properties and maps
 * them onto cytoscape's own properties. The source carries token *names*; the
 * values come from the theme the rest of the console is already using.
 */
export interface TopologyThemeTokens {
	surface: string;
	surfaceRaised: string;
	border: string;
	text: string;
	textMuted: string;
	/** SIGNAL and subscribe edges. */
	info: string;
	/** INSIGHT. */
	secondary: string;
	/** MUTATION and invite edges. */
	warning: string;
	/** VERIFICATION and bee nodes. */
	success: string;
}

const tokenVar = (name: string): string => `var(${name})`;

/** Fallbacks so the graph is never invisible if a theme omits a token. */
const fallbacks: Record<keyof TopologyThemeTokens, string> = {
	surface: tokenVar('--color-base-200'),
	surfaceRaised: tokenVar('--color-base-300'),
	border: tokenVar('--color-base-content'),
	text: tokenVar('--color-base-content'),
	textMuted: tokenVar('--color-base-content'),
	info: tokenVar('--color-info'),
	secondary: tokenVar('--color-secondary'),
	warning: tokenVar('--color-warning'),
	success: tokenVar('--color-success')
};

/**
 * DaisyUI 5 exposes its palette as `--color-<name>` on the theme root, and
 * cytoscape writes colours into SVG attributes where `var()` does not resolve
 * consistently across browsers. So the values are read once and handed over
 * resolved. Reading also means a theme switch is a re-read plus a restyle rather
 * than a reload.
 */
export function readTopologyThemeTokens(root: HTMLElement | null = document.documentElement): TopologyThemeTokens {
	const computed = root ? getComputedStyle(root) : null;
	const read = (name: string, fallback: string): string => {
		const value = computed?.getPropertyValue(name).trim();
		return value || fallback;
	};
	return {
		surface: read('--color-base-200', fallbacks.surface),
		surfaceRaised: read('--color-base-300', fallbacks.surfaceRaised),
		border: read('--color-base-300', fallbacks.border),
		text: read('--color-base-content', fallbacks.text),
		textMuted: read('--color-base-content', fallbacks.textMuted),
		info: read('--color-info', fallbacks.info),
		secondary: read('--color-secondary', fallbacks.secondary),
		warning: read('--color-warning', fallbacks.warning),
		success: read('--color-success', fallbacks.success)
	};
}

/** The subset of cytoscape's style block the graph uses. */
export type TopologyStyleBlock = {
	selector: string;
	style: Record<string, string | number>;
};

/**
 * The cytoscape stylesheet, built from theme tokens. It keeps the legacy graph's
 * visual language — rounded nodes, bezier edges with arrowheads, dashed publish
 * edges, dotted invite edges, dimmed implicit rules — so an operator who knows the
 * old console reads this one the same way. What changed is where the colours come
 * from: DaisyUI tokens instead of hard-coded hex, so the graph follows the theme
 * instead of staying dark while the rest of the console goes light.
 *
 * Signal, insight, mutation, and verification keep distinct hues because they are
 * the four contracts the whole page is about; they map onto `info`, `secondary`,
 * `warning`, and `success` rather than inventing colours.
 */
export function topologyStylesheet(tokens: TopologyThemeTokens): TopologyStyleBlock[] {
	return [
		{
			selector: 'node',
			style: {
				label: 'data(label)',
				'text-wrap': 'wrap',
				'text-max-width': '160px',
				'font-size': '11px',
				'font-family': 'ui-monospace, monospace',
				color: tokens.text,
				'text-valign': 'center',
				'text-halign': 'center',
				'background-color': tokens.surface,
				'border-color': tokens.border,
				'border-width': 1,
				shape: 'round-rectangle',
				padding: '10px',
				width: 'label',
				height: 'label'
			}
		},
		{
			selector: 'node[nodeType = "event"]',
			style: { 'background-color': tokens.surface, 'border-color': tokens.border }
		},
		{ selector: 'node[eventType = "SIGNAL"]', style: { 'border-color': tokens.info } },
		{ selector: 'node[eventType = "INSIGHT"]', style: { 'border-color': tokens.secondary } },
		{ selector: 'node[eventType = "MUTATION"]', style: { 'border-color': tokens.warning } },
		{ selector: 'node[eventType = "VERIFICATION"]', style: { 'border-color': tokens.success } },
		{
			selector: 'node[nodeType = "bee"]',
			style: { 'background-color': tokens.surfaceRaised, 'border-color': tokens.success }
		},
		{
			selector: 'edge',
			style: {
				label: 'data(label)',
				'font-size': '9px',
				'font-family': 'ui-monospace, monospace',
				color: tokens.textMuted,
				'text-background-color': tokens.surface,
				'text-background-opacity': 0.85,
				'text-background-padding': '2px',
				'curve-style': 'bezier',
				'target-arrow-shape': 'triangle',
				'arrow-scale': 0.8,
				width: 1.5,
				'line-color': tokens.border,
				'target-arrow-color': tokens.border
			}
		},
		{
			selector: 'edge[edgeKind = "subscribe"]',
			style: { 'line-color': tokens.info, 'target-arrow-color': tokens.info }
		},
		{
			selector: 'edge[edgeKind = "publish"]',
			style: {
				'line-style': 'dashed',
				'line-color': tokens.textMuted,
				'target-arrow-color': tokens.textMuted
			}
		},
		{
			selector: 'edge[edgeKind = "invite"]',
			style: {
				'line-style': 'dotted',
				'line-color': tokens.warning,
				'target-arrow-color': tokens.warning,
				width: 2
			}
		},
		// A bee with no declared `subscribes` still receives task.ready, which is a
		// real rule but not one the operator wrote, so it reads quieter.
		{ selector: 'edge[implicit = "true"]', style: { opacity: 0.65 } }
	];
}
