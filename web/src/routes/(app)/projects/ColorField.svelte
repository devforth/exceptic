<!--
@component
The color of a project: the palette's colors as swatches (radio buttons, generated from the
server's palette), any other color with the color input after them, and the project's badge in
the chosen color.
-->
<script lang="ts">
	import { projectPalette } from '#lib/api/catalog.gen.ts';
	import Badge from '#lib/components/Badge.svelte';
	import Hint from '#lib/components/Hint.svelte';
	import { paletteName } from '#lib/projects.ts';

	type Props = {
		/** #rrggbb */
		value: string;
		/** The badge of the preview. */
		name: string;
		hint?: string;
		error?: string;
	};

	let { value = $bindable(), name, hint, error }: Props = $props();

	const id = $props.id();
	const custom = $derived(paletteName(value) === undefined);

	const ring = 'ring-offset-2 ring-offset-surface forced-colors:border';
	const chosen = 'ring-2 ring-text';
</script>

<fieldset class="flex min-w-0 flex-col gap-2" aria-describedby={error ? `${id}-error` : undefined}>
	<legend class="mb-2 flex items-center gap-1.5 text-label text-muted">
		Color
		{#if hint}<Hint text={hint} />{/if}
	</legend>
	<div class="flex flex-wrap items-center gap-2.5">
		{#each projectPalette as entry (entry.color)}
			<label class="cursor-pointer" title={entry.name}>
				<input
					type="radio"
					name="{id}-color"
					value={entry.color}
					checked={value === entry.color}
					class="peer sr-only"
					onchange={() => (value = entry.color)}
				/>
				<span
					class={[
						'block size-7 rounded-full bg-project peer-checked:ring-2 peer-checked:ring-text',
						'peer-focus-visible:outline-2 peer-focus-visible:outline-offset-4 peer-focus-visible:outline-focus',
						ring
					]}
					style:--project-color={entry.color}
				></span>
				<span class="sr-only">{entry.name}</span>
			</label>
		{/each}
		<input
			type="color"
			bind:value
			aria-label="Custom color"
			title="Custom color"
			class={[
				'size-7 cursor-pointer rounded-full border border-border bg-surface p-0.5 focus-ring',
				ring,
				custom && chosen
			]}
		/>
	</div>
	<p class="flex items-center gap-1.5 text-xs text-muted">
		Preview: <Badge tone="project" label={name || 'project'} color={value} />
	</p>
	{#if error}
		<p id="{id}-error" class="text-xs text-error">{error}</p>
	{/if}
</fieldset>
