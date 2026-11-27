<!--
@component
Which projects something covers: All projects, the projects as badges, or a muted dash for none
(named "No projects" for screen readers). ids, beside names, links each badge to its project's
settings; links, to any other address (a list filtered by it). limit
keeps a list row on one line: only that many badges, then a MoreButton with the rest; on a phone
(under the table breakpoint) only the first, the rest in the MoreButton.
-->
<script lang="ts">
	import { resolve } from '$app/paths';
	import Badge from './Badge.svelte';
	import MoreButton from './MoreButton.svelte';

	type Props = {
		all: boolean;
		/** The project names, when not all. */
		names: readonly string[];
		/** The projects' ids, in the order of names: each badge leads to its project's settings. */
		ids?: readonly number[];
		/** Where each project's badge leads instead, in the order of names (resolved paths). */
		links?: readonly string[];
		/** At most this many badges in place; the rest behind "+N more". */
		limit?: number;
	};

	let { all, names, ids, links, limit }: Props = $props();

	const hrefs = $derived(
		links ?? ids?.map((id) => resolve('/(app)/projects/[id=projectId]', { id }))
	);

	const shown = $derived(limit !== undefined && names.length > limit ? limit : names.length);
</script>

{#snippet badges(from: number, to: number)}
	{#each names.slice(from, to) as name, index (name)}<Badge
			tone="project"
			label={name}
			href={hrefs?.[from + index]}
		/>{/each}
{/snippet}

{#snippet more(from: number)}
	<MoreButton count={names.length - from} label="{names.length - from} more projects">
		{@render badges(from, names.length)}
	</MoreButton>
{/snippet}

{#if all}
	All projects
{:else if names.length}
	<span class={['inline-flex items-center gap-1', limit === undefined && 'flex-wrap']}>
		{#if limit === undefined}
			{@render badges(0, names.length)}
		{:else}
			{@render badges(0, 1)}
			<span class="contents max-table:hidden">{@render badges(1, shown)}</span>
			{#if names.length > 1}
				<span class="contents table:hidden">
					{@render more(1)}
				</span>
			{/if}
			{#if shown < names.length}
				<span class="contents max-table:hidden">{@render more(shown)}</span>
			{/if}
		{/if}
	</span>
{:else}
	<span class="text-muted" title="No projects">
		<span aria-hidden="true">—</span><span class="sr-only">No projects</span>
	</span>
{/if}
