<script lang="ts">
	import { afterNavigate, invalidate } from '$app/navigation';
	import Sidebar from '#lib/components/Sidebar.svelte';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();

	// The unresolved count changes while the user works: recount on every page, not on the
	// first, nor when a page only changes its address (a tab, shallow).
	afterNavigate((navigation) => {
		if (navigation.type !== 'enter' && !navigation.shallow) {
			void invalidate('app:session');
		}
	});
</script>

<div class="flex min-h-screen max-sidebar:flex-col">
	<Sidebar session={data.session} />
	<main class="mx-auto w-full max-w-page min-w-0 flex-1 px-4 pt-6 pb-16">
		{@render children()}
	</main>
</div>
