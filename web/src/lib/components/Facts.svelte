<!--
@component
Facts side by side: each a small muted label over its value, which the value snippet draws.
-->
<script lang="ts" generics="Fact extends { label: string }">
	import type { Snippet } from 'svelte';

	type Props = { facts: Fact[]; value: Snippet<[Fact]> };

	let { facts, value }: Props = $props();
</script>

<dl class="my-3 flex flex-wrap gap-x-6 gap-y-3">
	{#each facts as fact (fact.label)}
		<div class="min-w-0">
			<dt class="text-xs text-muted">{fact.label}</dt>
			<dd class="wrap-break-word">{@render value(fact)}</dd>
		</div>
	{/each}
</dl>
