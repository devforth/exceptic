<!--
@component
"Back" before a page's title: returns to the page the viewer came from in the app (the browser's
history), or goes to `fallback` (the page above it) when it was opened directly.
-->
<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import Button from './Button.svelte';

	type Props = { fallback: string };

	let { fallback }: Props = $props();

	let cameFromApp = $state(false);

	afterNavigate(({ from }) => {
		cameFromApp = from !== null;
	});
</script>

{#if cameFromApp}
	<Button variant="ghost" icon="back" title="Back" onclick={() => history.back()} />
{:else}
	<Button variant="ghost" icon="back" title="Back" href={fallback} />
{/if}
