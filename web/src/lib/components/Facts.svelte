<!--
@component
Facts side by side: each a small muted label over its value, which the value snippet draws.
`boxed`: in cells of a framed grid (what a setting keeps now). `title`: a heading above them,
with its `hint`.
-->
<script lang="ts" generics="Fact extends { label: string }">
	import type { Snippet } from 'svelte';
	import Hint from './Hint.svelte';

	type Props = {
		facts: Fact[];
		value: Snippet<[Fact]>;
		boxed?: boolean;
		title?: string;
		hint?: string;
	};

	let { facts, value, boxed = false, title, hint }: Props = $props();
</script>

{#if title}
	<p class="mt-1 flex items-center gap-1.25 text-label font-semibold">
		{title}
		{#if hint}<Hint text={hint} />{/if}
	</p>
{/if}

<dl
	class={boxed
		? 'my-1 grid grid-cols-[repeat(auto-fit,minmax(140px,1fr))] gap-px overflow-hidden rounded-popup border border-border bg-border'
		: 'my-3 flex flex-wrap gap-6'}
>
	{#each facts as fact (fact.label)}
		<div class={boxed ? 'min-w-0 bg-surface px-3 py-2.5' : 'min-w-0'}>
			<dt class="text-xs text-muted">{fact.label}</dt>
			<dd class={['wrap-break-word', boxed && 'mt-0.5']}>{@render value(fact)}</dd>
		</div>
	{/each}
</dl>
