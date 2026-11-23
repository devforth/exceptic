<!--
@component
Which projects something covers: All projects, the projects as badges, or a muted dash for none
(named "No projects" for screen readers).
-->
<script lang="ts">
	import Badge from './Badge.svelte';

	type Props = {
		all: boolean;
		/** The project names, when not all. */
		names: readonly string[];
	};

	let { all, names }: Props = $props();
</script>

{#if all}
	All projects
{:else if names.length}
	<span class="inline-flex flex-wrap gap-1">
		{#each names as name (name)}<Badge tone="project" label={name} />{/each}
	</span>
{:else}
	<span class="text-muted" title="No projects">
		<span aria-hidden="true">—</span><span class="sr-only">No projects</span>
	</span>
{/if}
