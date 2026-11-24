<!--
@component
The filters a list applies, one chip per field: the × after a chip removes the whole filter, the
× inside each of several values (a project's or level's badge) only that value; then "Clear all".
-->
<script lang="ts">
	import Badge from './Badge.svelte';
	import Button from './Button.svelte';
	import type { FilterChip, FilterValue } from './chips.ts';
	import LevelBadge from './LevelBadge.svelte';
	import StatusBadge from './StatusBadge.svelte';

	let { chips, clear }: { chips: FilterChip[]; clear: string } = $props();

	// A value's own × only among several: alone it is the whole filter's.
	function valueRemove(chip: FilterChip, value: FilterValue) {
		return chip.values.length > 1
			? { href: value.remove, label: `Remove ${chip.label}: ${value.text}` }
			: undefined;
	}
</script>

{#if chips.length}
	<div class="-mt-1.5 mb-3 flex flex-wrap items-center gap-1.5 text-xs">
		{#each chips as chip (chip.label)}
			<span
				class="inline-flex items-center gap-1 rounded-full border border-border bg-surface py-0.5 pr-1 pl-2.5 whitespace-nowrap"
			>
				{chip.label}:
				{#each chip.values as value (value.text)}
					{#if value.kind === 'project'}
						<Badge tone="project" label={value.text} remove={valueRemove(chip, value)} />
					{:else if value.kind === 'level'}
						<LevelBadge level={value.text} remove={valueRemove(chip, value)} />
					{:else if value.kind === 'status'}
						<StatusBadge status={value.text} />
					{:else}
						<strong class="max-w-[260px] truncate font-semibold" title={value.text}>
							{value.text}
						</strong>
					{/if}
				{/each}
				<!-- eslint-disable svelte/no-navigation-without-resolve -- the list's own address -->
				<a
					href={chip.remove}
					class="inline-flex size-[18px] items-center justify-center rounded-full text-sm leading-none text-muted no-underline focus-ring hover:bg-bg hover:text-text hover:no-underline"
					title="Remove this filter"
					aria-label="Remove the {chip.label} filter"
				>
					×
				</a>
				<!-- eslint-enable svelte/no-navigation-without-resolve -->
			</span>
		{/each}
		<span class="ml-1"><Button variant="link" href={clear}>Clear all</Button></span>
	</div>
{/if}
