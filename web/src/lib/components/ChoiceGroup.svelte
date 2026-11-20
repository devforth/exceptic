<!--
@component
A group of Choice options under its legend (with a Hint), and the error the server gave it.
`inline` puts short options side by side.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import Hint from './Hint.svelte';

	type Props = {
		legend: string;
		hint?: string;
		error?: string;
		inline?: boolean;
		children: Snippet;
	};

	let { legend, hint, error, inline = false, children }: Props = $props();

	const id = $props.id();
</script>

<fieldset class="flex min-w-0 flex-col gap-2" aria-describedby={error ? `${id}-error` : undefined}>
	<legend class="mb-2 flex items-center gap-1.5 text-label text-muted">
		{legend}
		{#if hint}<Hint text={hint} />{/if}
	</legend>
	<div class={inline ? 'flex flex-wrap gap-x-4 gap-y-2' : 'flex flex-col gap-2'}>
		{@render children()}
	</div>
	{#if error}
		<p id="{id}-error" class="text-xs text-error">{error}</p>
	{/if}
</fieldset>
