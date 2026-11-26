<!--
@component
The viewer's own connected apps, with Disconnect. Those of other users are in their Sessions and
sign-ins.
-->
<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import type { Out } from '#lib/api/client.ts';
	import AgentsTable from '#lib/components/AgentsTable.svelte';
	import EmptyState from '#lib/components/EmptyState.svelte';

	type Props = { agents: Out<'list_agents'> };

	let { agents }: Props = $props();

	const connected = $derived((agents.agents ?? []).filter((agent) => agent.connected));
</script>

<section class="mt-5">
	<h2 class="mb-2.5 text-h2 font-bold">Connected apps</h2>

	{#if connected.length === 0}
		<EmptyState variant="card">
			<p>No apps connected yet.</p>
			<p class="text-xs text-muted">
				Connect a client using the instructions above. It will appear here after you authorize
				access.
			</p>
		</EmptyState>
	{:else}
		<AgentsTable agents={connected} ondisconnect={() => void invalidateAll()} />
	{/if}
</section>
