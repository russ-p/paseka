<script lang="ts">
	import {
		consoleThemeLabels,
		themeStore,
		type ThemeStore
	} from '$lib/stores/theme.svelte';

	let { store = themeStore }: { store?: ThemeStore } = $props();

	function handleChange(event: Event): void {
		store.set((event.currentTarget as HTMLSelectElement).value);
	}
</script>

<fieldset class="fieldset">
	<legend class="fieldset-legend">Theme</legend>
	<select class="select select-bordered w-full max-w-sm" value={store.current} onchange={handleChange}>
		{#each store.themes as theme}
			<option value={theme}>{consoleThemeLabels[theme]}</option>
		{/each}
	</select>
</fieldset>
