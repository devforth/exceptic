<script lang="ts">
	import Card from '#lib/components/Card.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import ConnectedApps from './ConnectedApps.svelte';
	import McpSetup from './McpSetup.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<PageHeading title="MCP" />

<Card>
	<McpSetup setup={data.setup} appName={data.session.app_name} />
</Card>

<ConnectedApps agents={data.agents} viewerId={data.session.user.id} />
