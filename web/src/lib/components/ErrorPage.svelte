<!--
@component
The page of an error: StatusPage for a 404 or a 403 (with the server's reason), the
message otherwise.
-->
<script lang="ts">
	import { page } from '$app/state';
	import Notice from './Notice.svelte';
	import PageTitle from './PageTitle.svelte';
	import StatusPage from './StatusPage.svelte';
</script>

{#if page.status === 404 || page.status === 403}
	<StatusPage status={page.status} detail={page.error?.message} />
{:else}
	<PageTitle title="Error" />
	<Notice tone="error">{page.error?.message ?? 'Something went wrong.'}</Notice>
{/if}
