<!--
@component
The page of an error: Not found for a 404 (also what a page the viewer may not open looks
like), the message otherwise.
-->
<script lang="ts">
	import { page } from '$app/state';
	import NotFound from './NotFound.svelte';
	import Notice from './Notice.svelte';
	import PageTitle from './PageTitle.svelte';
</script>

{#if page.status === 404}
	<NotFound />
{:else}
	<PageTitle title="Error" />
	<Notice tone="error">{page.error?.message ?? 'Something went wrong.'}</Notice>
{/if}
