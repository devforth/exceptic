<!--
@component
The Projects or Levels filter of a list: a Picker whose options are drawn as project or level
badges.
-->
<script lang="ts">
	import Badge from './Badge.svelte';
	import LevelBadge from './LevelBadge.svelte';
	import Picker from './Picker.svelte';

	type Props = {
		kind: 'project' | 'level';
		options: readonly string[];
		selected: readonly string[];
		onchange: (selected: string[]) => void;
	};

	let { kind, options, selected, onchange }: Props = $props();

	const names = {
		project: { label: 'Projects', all: 'All projects' },
		level: { label: 'Levels', all: 'All levels' }
	};
</script>

<Picker label={names[kind].label} all={names[kind].all} {options} {selected} {onchange}>
	{#snippet option(value: string)}
		{#if kind === 'project'}<Badge tone="project" label={value} />{:else}<LevelBadge
				level={value}
			/>{/if}
	{/snippet}
</Picker>
