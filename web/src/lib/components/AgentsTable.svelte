<!--
@component
A user's connected coding agents (OAuth apps and a used personal token) with Disconnect: the MCP
page's own and, in Sessions and sign-ins, those of a user the viewer manages. Only the connected
ones are listed; ondisconnect runs after one is disconnected.
-->
<script lang="ts">
	import { personalTokenKind } from '#lib/api/catalog.gen.ts';
	import { post, type Out } from '#lib/api/client.ts';
	import { toast } from '#lib/toast.svelte.ts';
	import ConfirmButton from './ConfirmButton.svelte';
	import Table from './Table.svelte';
	import type { TableColumn } from './table.ts';
	import Time from './Time.svelte';

	type Agent = NonNullable<Out<'list_agents'>['agents']>[number];
	type Props = {
		agents: Agent[];
		ondisconnect: () => void;
		/** Smaller rows inside a dialog. */
		compact?: boolean;
		/** Why the viewer may not disconnect them (another user they may not take over). */
		unavailable?: string;
	};

	let { agents, ondisconnect, compact = false, unavailable }: Props = $props();

	type AgentColumn = TableColumn<'app' | 'connected' | 'active' | 'actions'>;

	const columns: AgentColumn[] = [
		{ key: 'app', label: 'App' },
		{ key: 'connected', label: 'Connected' },
		{ key: 'active', label: 'Last active' },
		{ key: 'actions', label: 'Actions', actions: true }
	];

	async function disconnect(agent: Agent) {
		await post('disconnect_agent', { grant_id: agent.grant_id });
		toast.show(`${agent.name} disconnected.`);
		ondisconnect();
	}
</script>

<Table
	{columns}
	rows={agents}
	rowKey={(agent: Agent) => agent.grant_id}
	label="Connected apps"
	flat
	{compact}
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
				variant="danger"
				{unavailable}
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
