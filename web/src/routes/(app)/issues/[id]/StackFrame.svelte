<!--
@component
A stack frame: its function and where it is, the library's muted. With source or local
variables it unfolds (open when it is the application's frame that threw); without, it is a
plain row.
-->
<script lang="ts">
	import CodeBlock from '#lib/components/CodeBlock.svelte';
	import Details from '#lib/components/Details.svelte';
	import {
		frameFunction,
		frameLocation,
		frameStartsOpen,
		frameUnfolds,
		type Frame
	} from './issue.ts';

	let { frame }: { frame: Frame } = $props();

	const location = $derived(frameLocation(frame));
</script>

{#snippet heading()}
	<span class="font-mono font-semibold">{frameFunction(frame)}</span>
	<span class="wrap-anywhere text-muted">{location}</span>
{/snippet}

<div class={['border-t border-border py-1.5', !frame.in_app && 'text-muted']}>
	{#if frameUnfolds(frame)}
		<details open={frameStartsOpen(frame)}>
			<summary class="cursor-pointer rounded-control focus-ring">{@render heading()}</summary>
			{#if frame.source_lines?.length}
				<pre
					class="mt-1.5 overflow-x-auto rounded-control bg-code-bg py-2 font-mono text-code text-text">{#each frame.source_lines as line (line.number)}<span
							class={['block px-2.5', line.current && 'bg-current-line']}
							><span class="inline-block w-11 text-muted select-none">{line.number}</span
							>{line.text}</span
						>{/each}</pre>
			{/if}
			{#if frame.vars}
				<div class="pl-2.5">
					<Details variant="inline" title="Local variables">
						<CodeBlock code={frame.vars} label="Local variables of {frameFunction(frame)}" />
					</Details>
				</div>
			{/if}
		</details>
	{:else}
		<div class="pl-[1.1em]">{@render heading()}</div>
	{/if}
</div>
