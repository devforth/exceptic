<!--
@component
Numbered steps (Step children): each with its number in a circle at the left and its content
indented to the text.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	let { children }: { children: Snippet } = $props();
</script>

<ol class="mt-5 flex max-w-guide list-none flex-col gap-5 p-0 [counter-reset:step]">
	{@render children()}
</ol>
