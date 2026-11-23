<!--
@component
The page of an address the viewer cannot open: 404, it does not exist; 403, it exists but
belongs to a project they do not have or their role may not open it (detail: the
server's reason).
-->
<script lang="ts">
	import { resolve } from '$app/paths';
	import Button from './Button.svelte';
	import { titleClasses } from './heading.ts';
	import PageTitle from './PageTitle.svelte';

	let { status = 404, detail }: { status?: 403 | 404; detail?: string } = $props();

	const texts = {
		403: {
			title: 'You do not have access to this',
			text: 'It belongs to a project you do not have, or your role may not open it.'
		},
		404: { title: 'Page not found', text: 'The page does not exist or was removed.' }
	};
	const shown = $derived(texts[status]);
</script>

<PageTitle title={shown.title} />

<section class="flex min-h-[70vh] flex-col items-center justify-center gap-2 text-center">
	<p class="text-display leading-none font-bold text-muted">{status}</p>
	<h1 class={titleClasses}>{shown.title}</h1>
	<p class="text-muted">{status === 403 && detail ? detail : shown.text}</p>
	<div class="mt-3">
		<Button variant="primary" href={resolve('/(app)/issues')}>Back to issues</Button>
	</div>
</section>
