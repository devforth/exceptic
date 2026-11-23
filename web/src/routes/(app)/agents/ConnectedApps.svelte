<!--
@component
The connected apps of a user, with Disconnect. Admins choose whose (?user=, left out for their
own); the server lists only the users they may manage.
-->
<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import { personalTokenKind } from '#lib/api/catalog.gen.ts';
	import { post, type Out } from '#lib/api/client.ts';
	import ConfirmButton from '#lib/components/ConfirmButton.svelte';
	import EmptyState from '#lib/components/EmptyState.svelte';
	import Select from '#lib/components/Select.svelte';
	import Table from '#lib/components/Table.svelte';
	import type { TableColumn } from '#lib/components/table.ts';
	import Time from '#lib/components/Time.svelte';
	import { changed } from '#lib/filters.ts';
	import { toast } from '#lib/toast.svelte.ts';

	type Props = { agents: Out<'list_agents'>; viewerId: string };

	let { agents, viewerId }: Props = $props();

	type Agent = NonNullable<Out<'list_agents'>['agents']>[number];
	type AgentColumn = TableColumn<'app' | 'connected' | 'active' | 'actions'>;

	const columns: AgentColumn[] = [
		{ key: 'app', label: 'App' },
		{ key: 'connected', label: 'Connected' },
		{ key: 'active', label: 'Last active' },
		{ key: 'actions', label: 'Actions', actions: true }
	];

	const connected = $derived((agents.agents ?? []).filter((agent) => agent.connected));

	const owners = $derived(
		(agents.owners ?? []).map((owner) => ({
			value: owner.id,
			label: owner.id === viewerId ? `${owner.name} (you)` : owner.name
		}))
	);

	function showOwner(id: string) {
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- this page with another query
		void goto(changed(page.url, { user: id === viewerId ? null : id }));
	}

	async function disconnect(agent: Agent) {
		await post('disconnect_agent', { grant_id: agent.grant_id });
		toast.show(`${agent.name} disconnected.`);
		void invalidateAll();
	}
</script>

<section class="mt-5">
	<div class="mb-2.5 flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
		<h2 class="text-h2 font-bold">Connected apps</h2>
		{#if owners.length > 0}
			<Select label="Whose apps" value={agents.user_id} options={owners} onchange={showOwner} />
		{/if}
	</div>

	{#if connected.length === 0}
		<EmptyState variant="card">
			<p>No apps connected yet.</p>
			<p class="text-xs text-muted">
				Connect a client using the instructions above. It will appear here after you authorize
				access.
			</p>
		</EmptyState>
	{:else}
		<Table
			{columns}
			rows={connected}
			rowKey={(agent: Agent) => agent.grant_id}
			label="Connected apps"
			flat
		>
			{#snippet cell(agent: Agent, column: AgentColumn)}
				{#if column.key === 'app'}
					<b>{agent.name}</b>
					<div class="text-xs text-muted">
						{agent.kind === personalTokenKind ? 'Personal token' : 'OAuth'}
					</div>
				{:else if column.key === 'connected'}
					<Time at={agent.created_at} />
				{:else if column.key === 'active'}
					{#if agent.last_used_at}<Time at={agent.last_used_at} />{:else}<span class="text-muted">
							Not used yet
						</span>{/if}
				{:else}
					<ConfirmButton
						confirm={{
							title: 'Disconnect app',
							message: `Disconnect ${agent.name}? Its tokens will stop working immediately.`,
							confirmLabel: 'Disconnect'
						}}
						action={() => disconnect(agent)}
					>
						Disconnect
					</ConfirmButton>
				{/if}
			{/snippet}
		</Table>
	{/if}
</section>
