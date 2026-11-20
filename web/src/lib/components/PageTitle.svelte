<!--
@component
The title of the browser tab: the page, then the app name.
-->
<script lang="ts">
	import { page } from '$app/state';

	let { title }: { title: string } = $props();

	const appName = $derived(page.data.session?.app_name);
</script>

<svelte:head>
	<title>{appName ? `${title} · ${appName}` : title}</title>
</svelte:head>
