<!--
@component
A part of a Details disclosure: a small uppercase heading with its Hint, and a dashed line above
every part but the first.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import Hint from './Hint.svelte';

	type Props = { title: string; hint?: string; children: Snippet };

	let { title, hint, children }: Props = $props();
</script>

<section
	class="flex flex-col gap-2 not-first:border-t not-first:border-dashed not-first:border-border not-first:pt-5"
>
	<h3
		class="mb-0.5 flex flex-wrap items-center gap-1.5 text-label font-semibold tracking-[.06em] text-muted uppercase"
	>
		{title}
		{#if hint}<Hint text={hint} />{/if}
	</h3>
	{@render children()}
</section>
