<!--
@component
One of Steps: its title (with `optional` a muted "optional" after it), a note under it and the
content (a CodeBlock).
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = { title: string; optional?: boolean; note?: string; children: Snippet };

	let { title, optional = false, note, children }: Props = $props();
</script>

<li
	class="relative pl-10 [counter-increment:step] before:absolute before:top-[-1px] before:left-0 before:flex before:size-[26px] before:items-center before:justify-center before:rounded-full before:border before:border-border before:bg-surface before:text-label before:font-semibold before:text-muted before:content-[counter(step)]"
>
	<h3 class="mt-0.5 mb-1 flex items-baseline gap-2 text-md font-bold">
		{title}
		{#if optional}<span class="text-xs font-normal text-muted">optional</span>{/if}
	</h3>
	{#if note}<p class="mb-1.5 text-xs text-muted">{note}</p>{/if}
	<div class="mt-1.5">{@render children()}</div>
</li>
