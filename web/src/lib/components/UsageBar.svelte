<!--
@component
How full something is: a bar filled to percent, or split into segments (each project's part in
its color, the rest grey; each at least a sliver wide), read as a meter by screen readers. Widths and colors are set through
the DOM (style: directive), which the CSP allows, unlike a style attribute.
-->
<script lang="ts">
	import type { UsageSegment } from './usage.ts';

	type Props = { percent?: number; segments?: UsageSegment[]; label: string };

	let { percent = 0, segments, label }: Props = $props();

	const clamp = (value: number) => Math.min(100, Math.max(0, value));

	const filled = $derived(
		clamp(segments ? segments.reduce((total, segment) => total + segment.percent, 0) : percent)
	);
</script>

<div
	role="meter"
	aria-label={label}
	aria-valuenow={Math.round(filled)}
	aria-valuemin={0}
	aria-valuemax={100}
	class="flex h-2.5 w-full max-w-[520px] overflow-hidden rounded-full bg-track forced-colors:border"
>
	{#if segments}
		{#each segments as segment, index (index)}
			<div
				class={[
					'h-full min-w-1.5 forced-color-adjust-none not-first:border-l-2 not-first:border-surface',
					segment.color ? 'bg-project' : 'bg-muted'
				]}
				title={segment.label}
				style:width="{clamp(segment.percent)}%"
				style:--project-color={segment.color}
			></div>
		{/each}
	{:else}
		<div
			class="h-full rounded-full bg-accent forced-color-adjust-none"
			style:width="{filled}%"
		></div>
	{/if}
</div>
