<!--
@component
The color of a project: the palette's colors as square swatches (radio buttons, generated from the
server's palette, a check on the chosen one), a rainbow swatch after them that opens the color
input for any other color and then shows it, and the project's badge in the chosen color.
`hideLegend`: under a heading that already names it (the legend stays for screen readers).
-->
<script lang="ts">
	import { projectPalette } from '#lib/api/catalog.gen.ts';
	import Badge from '#lib/components/Badge.svelte';
	import Hint from '#lib/components/Hint.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import { paletteName } from '#lib/projects.ts';

	type Props = {
		/** #rrggbb */
		value: string;
		/** The badge of the preview. */
		name: string;
		hint?: string;
		error?: string;
		hideLegend?: boolean;
	};

	let { value = $bindable(), name, hint, error, hideLegend = false }: Props = $props();

	const id = $props.id();
	const custom = $derived(paletteName(value) === undefined);

	const swatch =
		'relative flex size-9 cursor-pointer items-center justify-center rounded-popup bg-project text-on-project transition-transform duration-100 hover:scale-106 motion-reduce:transition-none forced-colors:forced-color-adjust-none';
	const chosen =
		'shadow-[0_0_0_2px_var(--color-surface),0_0_0_4px_var(--project-color)] forced-colors:outline-3 forced-colors:outline-offset-2 forced-colors:outline-[Highlight]';
	const idle = 'shadow-[inset_0_0_0_1px_rgb(0_0_0/0.08)]';
	const focus =
		'has-focus-visible:outline-2 has-focus-visible:outline-offset-4 has-focus-visible:outline-accent';
</script>

<fieldset
	class="flex min-w-0 flex-col gap-2.5"
	aria-describedby={error ? `${id}-error` : undefined}
>
	<legend class={hideLegend ? 'sr-only' : 'mb-2 flex items-center gap-1.25 text-label text-muted'}>
		Color
		{#if hint && !hideLegend}<Hint text={hint} />{/if}
	</legend>
	<div class="flex flex-wrap items-center gap-2">
		{#each projectPalette as entry (entry.color)}
			<label
				class={[swatch, focus, value === entry.color ? chosen : idle]}
				style:--project-color={entry.color}
				title={entry.name}
			>
				<input
					type="radio"
					name="{id}-color"
					value={entry.color}
					checked={value === entry.color}
					class="absolute inset-0 m-0 cursor-pointer opacity-0"
					onchange={() => (value = entry.color)}
				/>
				<span class="sr-only">{entry.name}</span>
				{#if value === entry.color}<Icon name="check" size={14} />{/if}
			</label>
		{/each}
		<label
			class={[
				swatch,
				focus,
				custom
					? chosen
					: [idle, 'bg-[conic-gradient(#ef4444,#f59e0b,#22c55e,#06b6d4,#6366f1,#d946ef,#ef4444)]']
			]}
			style:--project-color={custom ? value : undefined}
			title="Custom color"
		>
			<input
				type="color"
				bind:value
				aria-label="Custom color"
				class="absolute inset-0 size-full cursor-pointer appearance-none rounded-popup border-0 p-0 opacity-0"
			/>
			{#if custom}<Icon name="check" size={14} />{/if}
		</label>
	</div>
	<p class="flex items-center gap-1.5 text-xs text-muted">
		Preview: <Badge tone="project" label={name || 'project'} color={value} />
	</p>
	{#if error}
		<p id="{id}-error" class="text-xs text-error">{error}</p>
	{/if}
</fieldset>
