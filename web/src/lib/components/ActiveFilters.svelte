<!--
@component
The filters a list applies, one chip per field: each value removable on its own (×), and
"Clear all".
-->
<script lang="ts">
	import Badge from './Badge.svelte';
	import Button from './Button.svelte';
	import type { FilterChip } from './chips.ts';
	import LevelBadge from './LevelBadge.svelte';
	import StatusBadge from './StatusBadge.svelte';

	let { chips, clear }: { chips: FilterChip[]; clear: string } = $props();
</script>

{#if chips.length}
	<div class="mb-3 flex flex-wrap items-center gap-2 text-xs">
		{#each chips as chip (chip.label)}
			<span class="inline-flex items-center gap-1.5 rounded-full bg-code-bg py-0.5 pr-1 pl-2.5">
				{chip.label}:
				{#each chip.values as value (value.text)}
					<span class="inline-flex items-center gap-0.5">
						{#if value.kind === 'project'}
							<Badge tone="project" label={value.text} />
						{:else if value.kind === 'level'}
							<LevelBadge level={value.text} />
						{:else if value.kind === 'status'}
							<StatusBadge status={value.text} />
						{:else}
							<strong>{value.text}</strong>
						{/if}
						<!-- eslint-disable svelte/no-navigation-without-resolve -- the list's own address -->
						<a
							href={value.remove}
							class="rounded-full px-1 text-muted no-underline focus-ring hover:text-text hover:no-underline"
							title="Remove this filter"
							aria-label="Remove {chip.label}: {value.text}"
						>
							×
						</a>
						<!-- eslint-enable svelte/no-navigation-without-resolve -->
					</span>
				{/each}
			</span>
		{/each}
		<Button variant="link" href={clear}>Clear all</Button>
	</div>
{/if}
