<!--
@component
The 404 page: also what a page the viewer's role does not allow looks like.
-->
<script lang="ts">
	import { resolve } from '$app/paths';
	import Button from './Button.svelte';
	import PageTitle from './PageTitle.svelte';
</script>

<PageTitle title="Page not found" />

<section class="flex min-h-[70vh] flex-col items-center justify-center gap-2 text-center">
	<p class="text-display leading-none font-bold text-muted">404</p>
	<h1 class="text-h1 font-bold">Page not found</h1>
	<p class="text-muted">The page does not exist or was removed.</p>
	<div class="mt-3">
		<Button variant="primary" href={resolve('/(app)/issues')}>Back to issues</Button>
	</div>
</section>
