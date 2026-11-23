<!--
@component
The event or log levels as checkboxes, each a LevelBadge, highest first (the generated levels):
side by side, or in a column (`inline` false); `hideLegend` as in ChoiceGroup.
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
		hideLegend = false
	}: Props = $props();

	const known: readonly string[] = $derived(levels[kind]);
</script>

<ChoiceGroup {legend} {hint} {error} {inline} {hideLegend}>
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
