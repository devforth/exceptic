<!--
@component
A log line: color codes removed, cut to a few lines with Expand when longer, and Full screen
to read it whole, JSON fields as lines (readableLog).
-->
<script lang="ts">
	import { cleanLog, readableLog } from '#lib/format.ts';
	import Button from './Button.svelte';
	import Dialog from './Dialog.svelte';

	let { text }: { text: string } = $props();

	const clean = $derived(cleanLog(text));

	let body: HTMLPreElement;
	let overflowing = $state(false);
	let expanded = $state(false);
	let viewing = $state(false);

	$effect(() => {
		void clean;
		overflowing = body.scrollHeight > body.clientHeight + 2;
	});
</script>

<pre
	bind:this={body}
	class={[
		'overflow-auto font-mono text-code break-words whitespace-pre-wrap',
		expanded ? 'max-h-none' : 'max-h-40'
	]}>{clean}</pre>
<div class="mt-1 flex gap-3 text-xs">
	{#if overflowing || expanded}
		<Button variant="link" {expanded} onclick={() => (expanded = !expanded)}>
			{expanded ? 'Collapse' : 'Expand'}
		</Button>
	{/if}
	<Button variant="link" onclick={() => (viewing = true)}>Full screen</Button>
</div>

<Dialog title="Log" bind:open={viewing}>
	<pre class="font-mono text-code break-words whitespace-pre-wrap">{readableLog(clean)}</pre>
</Dialog>
