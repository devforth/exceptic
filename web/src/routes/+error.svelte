<script lang="ts">
	import { page } from '$app/state';
	import NotFound from '#lib/components/NotFound.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import PageTitle from '#lib/components/PageTitle.svelte';
</script>

{#if page.status === 404}
	<NotFound />
{:else}
	<PageTitle title="Error" />
	<Notice tone="error">{page.error?.message ?? 'Something went wrong.'}</Notice>
{/if}
