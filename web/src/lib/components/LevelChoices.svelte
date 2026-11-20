<!--
@component
The event or log levels as checkboxes, each a LevelBadge, highest first (the generated levels).
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
		/** The form field of each checkbox. */
		name?: string;
		selected: readonly string[];
		onchange: (selected: string[]) => void;
		hint?: string;
		error?: string;
	};

	let { kind, legend, name, selected, onchange, hint, error }: Props = $props();

	const known: readonly string[] = $derived(levels[kind]);
</script>

<ChoiceGroup {legend} {hint} {error} inline>
	{#each known as level (level)}
		<Choice
			type="checkbox"
			{name}
			value={level}
			checked={selected.includes(level)}
			onchange={(checked: boolean) => onchange(toggled(known, selected, level, checked))}
		>
			<LevelBadge {level} />
		</Choice>
	{/each}
</ChoiceGroup>
