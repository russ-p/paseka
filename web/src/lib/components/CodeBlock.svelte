<script lang="ts">
	import { jsonTokenClasses, tokenizeJson } from '$lib/tokenize';

	let {
		code,
		label,
		class: extra = ''
	}: {
		/** The text to show, already serialized by the caller — a raw event is data, not a file. */
		code: string;
		/** Names the block for a reader, since the text inside it is punctuation by then. */
		label?: string;
		/** Extra classes for the block, e.g. the `max-h-80` a folded view needs. */
		class?: string;
	} = $props();

	/**
	 * The text is the payload, so the colouring is decoration that must not be able to
	 * change what is on screen: `tokenizeJson` cannot reject, reorder, or drop a
	 * character, and a payload that is not JSON renders as itself. `label` therefore
	 * carries the meaning a `<pre>` of punctuation no longer speaks for itself.
	 */
	const tokens = $derived(tokenizeJson(code));
</script>

<!-- The `each` is one line on purpose: Svelte preserves whitespace inside `<pre>`, so an
     indented template here would inject the template's own newlines into the JSON. -->
<pre aria-label={label} class="overflow-auto rounded-box bg-base-200/50 p-2 font-mono text-xs whitespace-pre-wrap {extra}">{#each tokens as token, index (index)}<span class={jsonTokenClasses[token.role]}>{token.text}</span>{/each}</pre>
