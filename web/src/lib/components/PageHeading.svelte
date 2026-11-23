<!--
@component
The title of a page, with what goes before it (leading: a level badge) and its actions (buttons,
links) at the other end, and its description under it (what the page is for, a subtitle); also
the title of the browser tab, unless tabTitle is false (a sample on the design-system page).
`heading` draws the title instead of the plain h1 (an EditableTitle).
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import { titleClasses } from './heading.ts';
	import PageTitle from './PageTitle.svelte';

	type Props = {
		title: string;
		leading?: Snippet;
		actions?: Snippet;
		tabTitle?: boolean;
		/** The title drawn in place of the h1, e.g. an EditableTitle. */
		heading?: Snippet;
		/** Muted text under the title. */
		description?: Snippet;
	};

	let { title, leading, actions, tabTitle = true, heading, description }: Props = $props();
</script>

{#if tabTitle}
	<PageTitle {title} />
{/if}

<div class="mb-4">
	<div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
		<div class="flex min-w-0 items-center gap-2">
			{#if leading}{@render leading()}{/if}
			{#if heading}
				{@render heading()}
			{:else}
				<h1 class={titleClasses}>{title}</h1>
			{/if}
		</div>
		{#if actions}
			<div class="flex flex-wrap items-center gap-2">{@render actions()}</div>
		{/if}
	</div>
	{#if description}<p class="mt-2 text-muted">{@render description()}</p>{/if}
</div>
