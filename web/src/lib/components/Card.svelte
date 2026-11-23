<!--
@component
A section of a page on a raised surface, with its title.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	import { cardClasses } from './surface.ts';

	type Props = { title?: string; children: Snippet };

	let { title, children }: Props = $props();
</script>

<section class={[cardClasses, 'mb-4 px-[22px] py-5']}>
	{#if title}
		<h2 class="mb-2 text-h2 font-bold">{title}</h2>
	{/if}
	<div class="flex flex-col gap-2">{@render children()}</div>
</section>
