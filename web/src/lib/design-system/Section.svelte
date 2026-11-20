<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = { title: string; id?: string; children: Snippet };

	let { title, id, children }: Props = $props();
</script>

<section {id} class="mt-8">
	<h2 class="mb-3 text-h2 font-bold">{title}</h2>
	<div class="flex flex-col gap-3">{@render children()}</div>
</section>
