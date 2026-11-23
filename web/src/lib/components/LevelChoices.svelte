<!--
@component
The event or log levels as checkboxes, each a LevelBadge, highest first (the generated levels):
side by side under a small legend, or in a column (`inline` false); `beside` puts the legend before
them on their line (in a form section); `hideLegend` as in ChoiceGroup.
-->
<script lang="ts">
	import { levels } from '#lib/api/catalog.gen.ts';
	import { toggled } from '#lib/selection.ts';
	import Choice from './Choice.svelte';
	import ChoiceGroup from './ChoiceGroup.svelte';
	import LevelBadge from './LevelBadge.svelte';

	type Props = {
		kind: keyof typeof levels;
		legend: string;
		selected: readonly string[];
		onchange: (selected: string[]) => void;
		hint?: string;
		error?: string;
		inline?: boolean;
		beside?: boolean;
		hideLegend?: boolean;
	};

	let {
		kind,
		legend,
		selected,
		onchange,
		hint,
		error,
		inline = true,
		beside = false,
		hideLegend = false
	}: Props = $props();

	const known: readonly string[] = $derived(levels[kind]);
</script>

<ChoiceGroup {legend} {hint} {error} {inline} {beside} {hideLegend} compact>
	{#each known as level (level)}
		<Choice
			type="checkbox"
			value={level}
			checked={selected.includes(level)}
			onchange={(checked: boolean) => onchange(toggled(known, selected, level, checked))}
		>
			<LevelBadge {level} />
		</Choice>
	{/each}
</ChoiceGroup>
