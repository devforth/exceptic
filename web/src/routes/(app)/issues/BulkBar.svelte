<!--
@component
Above the issue list, in view while it scrolls: how many issues are checked (or why they
cannot be changed together) and the actions on them.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import { selectionProblem } from '#lib/components/issueActions.ts';

	let { count, children }: { count: number; children: Snippet } = $props();

	const problem = $derived(selectionProblem(count));
</script>

<div class="sticky top-0 z-2 flex flex-wrap items-center gap-2 bg-bg py-2">
	<span class={['mr-auto text-sm', problem && 'text-muted']} aria-live="polite">
		{problem ?? `${count} selected`}
	</span>
	{@render children()}
</div>
