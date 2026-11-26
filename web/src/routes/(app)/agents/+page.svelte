<script lang="ts">
	import Card from '#lib/components/Card.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import ConnectedApps from './ConnectedApps.svelte';
	import McpSetup from './McpSetup.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<PageHeading title="MCP" />

<!-- As in the design: the setup and the connected apps in one card, 920px wide. -->
<div class="max-w-230">
	<Card>
		<McpSetup setup={data.setup} appName={data.session.app_name} />
		<ConnectedApps agents={data.agents} />
	</Card>
</div>
