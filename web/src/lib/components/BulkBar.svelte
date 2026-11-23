<!--
@component
Above a list with checkboxes, in view while it scrolls: how many rows are checked (or why
they cannot be acted on together) and the actions on them.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = {
		count: number;
		/** Why the checked rows cannot be acted on together, if they cannot. */
		problem?: string;
		children: Snippet;
	};

	let { count, problem, children }: Props = $props();
</script>

<div class="sticky top-0 z-2 flex flex-wrap items-center gap-2 bg-bg py-2">
	<span class={['mr-auto text-sm', problem && 'text-muted']} aria-live="polite">
		{problem ?? `${count} selected`}
	</span>
	{@render children()}
</div>
