<!--
@component
A group of Choice options under its legend (with a Hint), and the error the server gave it.
`inline` puts short options side by side; `card` makes the group a card of its own;
`hideLegend` keeps the legend for screen readers only, when a heading around it names the group;
`lead` is an option above the others, such as "All projects".
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import Hint from './Hint.svelte';

	type Props = {
		legend: string;
		hint?: string;
		error?: string;
		inline?: boolean;
		card?: boolean;
		hideLegend?: boolean;
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
		lead,
		children
	}: Props = $props();

	const id = $props.id();
</script>

<fieldset
	class={[
		'flex min-w-0 flex-col gap-2',
		card && 'rounded-card bg-surface px-5 py-4 shadow-card forced-colors:border'
	]}
	aria-describedby={error ? `${id}-error` : undefined}
>
	<legend
		class={hideLegend
			? 'sr-only'
			: ['mb-2 flex items-center gap-1.5 text-label text-muted', card && 'float-left w-full']}
	>
		{legend}
		{#if hint}<Hint text={hint} />{/if}
	</legend>
	{#if lead}{@render lead()}{/if}
	<div class={inline ? 'flex flex-wrap gap-x-5 gap-y-2' : 'flex flex-col gap-2'}>
		{@render children()}
	</div>
	{#if error}
		<p id="{id}-error" class="text-xs text-error">{error}</p>
	{/if}
</fieldset>
