<!--
@component
Which projects something covers: All projects, the projects as badges, or a muted dash for none
(named "No projects" for screen readers). links, beside names, makes each badge a link.
-->
<script lang="ts">
	import Badge from './Badge.svelte';

	type Props = {
		all: boolean;
		/** The project names, when not all. */
		names: readonly string[];
		/** Where each project's badge leads, in the order of names (resolved paths). */
		links?: readonly string[];
	};

	let { all, names, links }: Props = $props();
</script>

{#if all}
	All projects
{:else if names.length}
	<span class="inline-flex flex-wrap gap-1">
		{#each names as name, index (name)}<Badge
				tone="project"
				label={name}
				href={links?.[index]}
			/>{/each}
	</span>
{:else}
	<span class="text-muted" title="No projects">
		<span aria-hidden="true">—</span><span class="sr-only">No projects</span>
	</span>
{/if}
