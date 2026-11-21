<script lang="ts">
	import { resolve } from '$app/paths';
	import Breadcrumbs from '#lib/components/Breadcrumbs.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import ConnectionForm from '../ConnectionForm.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<Breadcrumbs
	items={[
		{ label: 'Notifications', href: resolve('/(app)/notifications') },
		{ label: 'Add connection', href: resolve('/(app)/notifications/new') }
	]}
/>
<PageHeading title="Add connection" />

<ConnectionForm defaults={data.defaults} projects={data.projects} />
