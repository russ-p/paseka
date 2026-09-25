<script lang="ts">
	import { statusTone, statusToneClasses } from '$lib/status';

	let { status, label }: { status: string; label?: string } = $props();
</script>

<span class={`badge ${statusToneClasses[statusTone(status)]}`}>{label ?? status}</span>
