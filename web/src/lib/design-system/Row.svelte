<!-- A line of specimens with what they show; wide gives one specimen the full width. -->
<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = { caption: string; wide?: boolean; children: Snippet };

	let { caption, wide = false, children }: Props = $props();
</script>

<div class="flex flex-col gap-1.5">
	<code class="text-muted">{caption}</code>
	<div class={wide ? 'w-full' : 'flex flex-wrap items-center gap-2'}>{@render children()}</div>
</div>
