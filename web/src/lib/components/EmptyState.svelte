<!--
@component
The box shown instead of an empty list.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	let { children }: { children: Snippet } = $props();
</script>

<div class="rounded-note border border-dashed border-border bg-surface p-8 text-center text-muted">
	{@render children()}
</div>
