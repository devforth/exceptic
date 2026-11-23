<!--
@component
One exception of the chain: "Caused by" for all but the outermost, its type, whether the SDK
caught it, its value and its stack trace (closed), newest frame first.
-->
<script lang="ts">
	import Badge from '#lib/components/Badge.svelte';
	import Details from '#lib/components/Details.svelte';
	import { handledLabel, type Exception } from './issue.ts';
	import StackFrame from './StackFrame.svelte';

	let { exception, cause }: { exception: Exception; cause: boolean } = $props();

	const handled = $derived(handledLabel(exception.handled));
	const frames = $derived(exception.frames ?? []);
</script>

<div class="rounded-box border border-border bg-surface p-3">
	<h3 class="mb-1.5 flex flex-wrap items-center gap-2 font-semibold">
		{#if cause}<span class="text-muted">Caused by</span>{/if}
		{#if exception.type}<span class="text-error">{exception.type}</span>{/if}
		{#if handled}
			<Badge tone={exception.handled ? 'debug' : 'error'} label={handled} />
		{/if}
	</h3>
	{#if exception.value}
		<p class="mb-2.5 font-mono break-words whitespace-pre-wrap">{exception.value}</p>
	{/if}
	{#if frames.length}
		<Details
			variant="nested"
			title="Stack trace"
			count={frames.length}
			unit={frames.length === 1 ? 'frame' : 'frames'}
		>
			{#each frames as frame, index (index)}
				<StackFrame {frame} />
			{/each}
		</Details>
	{/if}
</div>
