<script lang="ts">
	import { page } from '$app/state';
	import TaskDetail from './TaskDetail.svelte';

	// Both ids address a task, so `page.params` is the only source; the body lives
	// in TaskDetail so a test can mount it without a router. A deep link to a task
	// the board no longer lists still resolves, because the detail is its own read.
	const traceId = $derived(page.params.traceId ?? '');
	const taskId = $derived(page.params.taskId ?? '');
</script>

<TaskDetail {traceId} {taskId} />
