<!-- A specimen: what it shows above it (caption), then a line of samples; wide gives one sample
the full width. -->
<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = { caption?: string; wide?: boolean; children: Snippet };

	let { caption, wide = false, children }: Props = $props();
</script>

<div class="flex flex-col gap-2.5">
	{#if caption}<code class="text-xs text-muted">{caption}</code>{/if}
	<div class={wide ? 'w-full' : 'flex flex-wrap items-center gap-2.5'}>{@render children()}</div>
</div>
