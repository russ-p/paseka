<script lang="ts">
	import { page } from '$app/state';
	import SessionDetail from './SessionDetail.svelte';

	// The body lives in SessionDetail so a test can mount it without a router, and a
	// deep link to a session the list no longer holds still resolves.
	const sessionId = $derived(page.params.sessionId ?? '');
</script>

<SessionDetail {sessionId} />
