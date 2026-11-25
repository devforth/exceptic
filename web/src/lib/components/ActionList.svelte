<!--
@component
A group of ActionRows under a small uppercase heading (the account's Profile and Security).
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = { title: string; children: Snippet };

	let { title, children }: Props = $props();
</script>

<section class="mb-6 flex flex-col gap-2">
	<h2 class="mb-1 text-label font-bold tracking-[.06em] text-muted uppercase">{title}</h2>
	{@render children()}
</section>
