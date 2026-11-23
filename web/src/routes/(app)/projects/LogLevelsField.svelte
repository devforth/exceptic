<!--
@component
The log levels a project records, with what happens to the others; `beside` puts the legend
before the levels on their line (in a form section).
-->
<script lang="ts">
	import { help } from '#lib/api/catalog.gen.ts';
	import LevelChoices from '#lib/components/LevelChoices.svelte';

	let {
		value = $bindable(),
		error,
		beside = false
	}: { value: string[]; error?: string; beside?: boolean } = $props();
</script>

<p class="text-xs text-muted">
	Logs of levels left unchecked are dropped when they arrive. Errors and messages of every level are
	kept; each notification connection chooses the levels it sends.
</p>
<LevelChoices
	kind="log"
	legend="Logs"
	hint={help.fields.Projects.Logs}
	selected={value}
	onchange={(selected: string[]) => (value = selected)}
	{error}
	{beside}
/>
