<script lang="ts">
	import { page } from '$app/state';
	import TimelineFeed from './TimelineFeed.svelte';

	// `?trace=` is the deep link a trail's "Open timeline" button produces, so the
	// feed can be linked to. It seeds the panel and nothing else: the other filters
	// are ad-hoc refinements that reset on reload, which is what the legacy console
	// did too, and one source of truth is worth more than a shareable query string.
	const initialTrace = $derived(page.url.searchParams.get('trace') ?? undefined);
</script>

<TimelineFeed initialFilters={initialTrace ? { traceId: initialTrace } : {}} />
