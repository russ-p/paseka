<script lang="ts">
	import {
		CircleAlert,
		CircleDashed,
		Link,
		LoaderCircle,
		Play,
		Square,
		Unlink
	} from 'lucide-svelte';
	import { statusTone, statusToneTextClasses } from '$lib/status';

	export type StatusIconGlyph = 'play' | 'stop' | 'pending' | 'link' | 'broken' | 'alert' | 'unknown';

	let {
		status,
		label,
		glyph: glyphOverride,
		size = 'sm'
	}: {
		status: string;
		label?: string;
		glyph?: StatusIconGlyph;
		size?: 'sm' | 'md' | 'lg';
	} = $props();

	const glyphs = {
		play: Play,
		stop: Square,
		pending: LoaderCircle,
		link: Link,
		broken: Unlink,
		alert: CircleAlert,
		unknown: CircleDashed
	} as const;

	const name = $derived(label ?? status);
	const tone = $derived(statusTone(status));
	const derivedGlyph: StatusIconGlyph = $derived(
		status === 'running' || status === 'live'
			? 'play'
			: status === 'starting' || status === 'stopping'
				? 'pending'
				: status === 'failed' || status === 'error' || status === 'killed'
					? 'alert'
					: status === 'connected'
						? 'link'
						: status === 'disconnected'
							? 'broken'
							: status === 'stopped'
								? 'stop'
								: 'unknown'
	);
	/** Resolved glyph: an explicit override wins over the status-derived one. */
	const glyph: StatusIconGlyph = $derived(glyphOverride ?? derivedGlyph);
	const Glyph = $derived(glyphs[glyph]);
	const sizeClass = $derived(size === 'lg' ? 'h-5 w-5' : size === 'md' ? 'h-4 w-4' : 'h-3 w-3');
</script>

<span
	class={`inline-flex shrink-0 items-center ${glyphOverride ? 'text-current' : statusToneTextClasses[tone]}`}
	role={glyphOverride ? 'presentation' : 'img'}
	aria-label={glyphOverride ? undefined : name}
	title={glyphOverride ? undefined : name}
	data-glyph={glyph}
>
	<Glyph class={sizeClass} strokeWidth={2.5} />
	{#if !glyphOverride}<span class="sr-only">{name}</span>{/if}
</span>
