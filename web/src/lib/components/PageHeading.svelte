<!--
@component
The title of a page, with its actions (buttons, links) at the other end; also the title of
the browser tab, unless tabTitle is false (a sample on the design-system page).
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import PageTitle from './PageTitle.svelte';

	type Props = { title: string; actions?: Snippet; tabTitle?: boolean };

	let { title, actions, tabTitle = true }: Props = $props();
</script>

{#if tabTitle}
	<PageTitle {title} />
{/if}

<div class="mb-4 flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
	<h1 class="text-h1 font-bold wrap-break-word">{title}</h1>
	{#if actions}
		<div class="flex flex-wrap items-center gap-2">{@render actions()}</div>
	{/if}
</div>
