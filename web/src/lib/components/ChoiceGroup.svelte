<!--
@component
A group of Choice options under its legend (with a Hint), and the error the server gave it.
`inline` puts short options side by side; `card` makes the group a card of its own;
`hideLegend` keeps the legend for screen readers only, when a heading around it names the group;
`compact`: a small legend over a tight row of short options (levels); `beside` puts that legend
before the options on their line; `lead` is an option above the others, such as "All projects";
the others' row is left out while it is empty.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import Hint from './Hint.svelte';
	import { cardClasses } from './surface.ts';

	type Props = {
		legend: string;
		hint?: string;
		error?: string;
		inline?: boolean;
		card?: boolean;
		hideLegend?: boolean;
		compact?: boolean;
		beside?: boolean;
		lead?: Snippet;
		children: Snippet;
	};

	let {
		legend,
		hint,
		error,
		inline = false,
		card = false,
		hideLegend = false,
		compact = false,
		beside = false,
		lead,
		children
	}: Props = $props();

	const id = $props.id();

	// Floated, the legend is an item of the fieldset's flex box: a full row, or beside the options.
	const legendClasses = $derived(
		hideLegend
			? 'sr-only'
			: [
					'flex items-center gap-1.25 text-muted',
					compact ? 'text-xs' : 'text-label',
					beside
						? 'float-left min-w-[130px]'
						: [compact ? 'mb-0.5' : 'mb-2', (card || compact) && 'float-left w-full']
				]
	);
</script>

<fieldset
	class={[
		'flex min-w-0',
		beside ? 'flex-row flex-wrap items-center gap-x-3.5 gap-y-1' : 'flex-col gap-1',
		compact && !beside && 'my-1',
		card && [cardClasses, 'px-5 py-4']
	]}
	aria-describedby={error ? `${id}-error` : undefined}
>
	<legend class={legendClasses}>
		{legend}
		{#if hint}<Hint text={hint} />{/if}
	</legend>
	{#if lead}{@render lead()}{/if}
	<div
		class={[
			beside
				? 'contents'
				: inline
					? ['flex flex-wrap', compact ? 'gap-x-3.5 gap-y-1' : 'gap-x-4.5']
					: 'flex flex-col',
			'empty:hidden'
		]}
	>
		{@render children()}
	</div>
	{#if error}
		<p id="{id}-error" class={['text-xs text-error', beside && 'basis-full']}>{error}</p>
	{/if}
</fieldset>
