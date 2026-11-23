<!-- Cards side by side, as many as fit (each at least 260px); a card's own margin is dropped. -->
<script lang="ts">
	import type { Snippet } from 'svelte';

	let { children }: { children: Snippet } = $props();
</script>

<div class="grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-4 *:mb-0">
	{@render children()}
</div>
