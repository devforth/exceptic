<!-- A section of the page: its heading, a lead under it, the specimens and a footnote. -->
<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = { title: string; id?: string; lead?: Snippet; note?: Snippet; children: Snippet };

	let { title, id, lead, note, children }: Props = $props();
</script>

<section {id} class="mt-10">
	<h2 class="mb-1 text-lg font-bold">{title}</h2>
	{#if lead}<p class="mb-4 text-muted">{@render lead()}</p>{/if}
	<div class="flex flex-col gap-4">{@render children()}</div>
	{#if note}<p class="mt-3 text-xs text-muted">{@render note()}</p>{/if}
</section>
