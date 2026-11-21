<!--
@component
One connection per address or chat: the one that already sends there, linked.
-->
<script lang="ts">
	import { resolve } from '$app/paths';
	import Notice from '#lib/components/Notice.svelte';

	let { duplicate }: { duplicate: { id: string; name: string } } = $props();

	const href = $derived(resolve('/(app)/notifications/[id=connectionId]', { id: duplicate.id }));
</script>

<Notice tone="error">
	Already added as <a {href}>{duplicate.name}</a>: edit that connection instead.
</Notice>
