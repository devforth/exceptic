<!--
@component
A section of a page on a raised surface, with its title.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = { title?: string; children: Snippet };

	let { title, children }: Props = $props();
</script>

<section class="mb-4 rounded-card bg-surface px-[22px] py-5 shadow-card">
	{#if title}
		<h2 class="mb-2 text-h2 font-bold">{title}</h2>
	{/if}
	<div class="flex flex-col gap-2">{@render children()}</div>
</section>
