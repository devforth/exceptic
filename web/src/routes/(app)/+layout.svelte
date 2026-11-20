<script lang="ts">
	import { afterNavigate, invalidate } from '$app/navigation';
	import Sidebar from '#lib/components/Sidebar.svelte';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();

	// The unresolved count changes while the user works: recount on every page, not on the first.
	afterNavigate((navigation) => {
		if (navigation.type !== 'enter') {
			void invalidate('app:session');
		}
	});
</script>

<svelte:head>
	<!-- The project colors: a stylesheet, since the CSP allows no inline styles. -->
	<link rel="stylesheet" href={data.session.project_colors_url} />
</svelte:head>

<div class="flex min-h-screen max-sidebar:flex-col">
	<Sidebar session={data.session} />
	<main class="mx-auto w-full max-w-page min-w-0 flex-1 px-4 pt-6 pb-16">
		{@render children()}
	</main>
</div>
