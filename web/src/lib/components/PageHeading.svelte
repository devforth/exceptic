<!--
@component
The title of a page, with what goes before it (leading: a level badge) and its actions (buttons,
links) at the other end; also the title of the browser tab, unless tabTitle is false (a sample on
the design-system page).
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import PageTitle from './PageTitle.svelte';

	type Props = { title: string; leading?: Snippet; actions?: Snippet; tabTitle?: boolean };

	let { title, leading, actions, tabTitle = true }: Props = $props();
</script>

{#if tabTitle}
	<PageTitle {title} />
{/if}

<div class="mb-4 flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
	<div class="flex min-w-0 items-center gap-2">
		{#if leading}{@render leading()}{/if}
		<h1 class="text-h1 font-bold wrap-break-word">{title}</h1>
	</div>
	{#if actions}
		<div class="flex flex-wrap items-center gap-2">{@render actions()}</div>
	{/if}
</div>
