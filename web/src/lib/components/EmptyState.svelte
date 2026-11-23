<!--
@component
The box shown instead of an empty list: dashed and centered, or with `variant="card"` a card
with the text on the left (the MCP page's connected apps, inside its card).
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cardClasses, cardPadding } from './surface.ts';

	type Props = { variant?: 'dashed' | 'card'; children: Snippet };

	let { variant = 'dashed', children }: Props = $props();
</script>

{#if variant === 'card'}
	<div class={[cardClasses, cardPadding, 'flex flex-col gap-1.5']}>
		{@render children()}
	</div>
{:else}
	<div
		class="rounded-note border border-dashed border-border bg-surface p-8 text-center text-muted"
	>
		{@render children()}
	</div>
{/if}
