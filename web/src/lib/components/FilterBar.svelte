<!--
@component
The filter fields above a list, in one wrapping row.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	let { label, children }: { label: string; children: Snippet } = $props();
</script>

<search aria-label={label} class="mb-4 flex flex-wrap items-center gap-2">
	{@render children()}
</search>
