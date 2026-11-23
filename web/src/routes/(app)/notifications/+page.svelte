<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { post, type Item } from '#lib/api/client.ts';
	import Badge from '#lib/components/Badge.svelte';
	import Button from '#lib/components/Button.svelte';
	import LevelSummary from '#lib/components/LevelSummary.svelte';
	import type { MenuItem } from '#lib/components/menu.ts';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import RowMenu from '#lib/components/RowMenu.svelte';
	import Table from '#lib/components/Table.svelte';
	import type { TableColumn } from '#lib/components/table.ts';
	import Time from '#lib/components/Time.svelte';
	import { withQuery } from '#lib/nav.ts';
	import {
		connectionState,
		deleteConfirmation,
		deleteConnection,
		deliveryHealth,
		shortError,
		typeLabel
	} from '#lib/notifications.ts';
	import { toast } from '#lib/toast.svelte.ts';
	import RetryNowButton from './RetryNowButton.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	type Connection = Item<'list_connections'>;
	type ConnectionColumn = TableColumn<
		'name' | 'type' | 'destination' | 'projects' | 'levels' | 'delivery' | 'status' | 'actions'
	>;

	const columns: ConnectionColumn[] = [
		{ key: 'name', label: 'Name' },
		{ key: 'type', label: 'Type', wideOnly: true },
		{ key: 'destination', label: 'Destination' },
		{ key: 'projects', label: 'Projects', wideOnly: true },
		{ key: 'levels', label: 'Levels', wideOnly: true },
		{ key: 'delivery', label: 'Delivery' },
		{ key: 'status', label: 'Status' },
		{ key: 'actions', label: 'Actions', actions: true }
	];

	const page = (connection: Connection) =>
		resolve('/(app)/notifications/[id=connectionId]', { id: connection.id });
	const log = (connection: Connection) =>
		withQuery(resolve('/(app)/notifications/log'), { connection: connection.id });

	async function setActive(connection: Connection, active: boolean) {
		await post('set_connection_active', { connection_id: connection.id, active });
		toast.show(active ? 'Connection activated.' : 'Connection deactivated.');
		void invalidateAll();
	}

	function menuOf(connection: Connection): MenuItem[] {
		const items: MenuItem[] = [{ label: 'Edit', icon: 'edit', href: page(connection) }];

		if (connection.active) {
			items.push({
				label: 'Pause notifications',
				icon: 'pause',
				action: () => setActive(connection, false)
			});
		} else if (connection.confirmed) {
			items.push({
				label: 'Resume notifications',
				icon: 'resume',
				action: () => setActive(connection, true)
			});
		}

		items.push({
			label: 'Delete',
			icon: 'delete',
			danger: true,
			separatorBefore: true,
			confirm: deleteConfirmation(connection.name),
			action: async () => {
				await deleteConnection(connection.id);
				// The delete is done: a failed reload of the list is not a failed delete.
				void invalidateAll();
			}
		});

		return items;
	}
</script>

<PageHeading title="Notifications">
	{#snippet actions()}
		<Button icon="logs" href={resolve('/(app)/notifications/log')}>Delivery log</Button>
		<Button variant="primary" icon="add" href={resolve('/(app)/notifications/new')}>
			Add connection
		</Button>
	{/snippet}
</PageHeading>

<p class="mb-4 text-muted">
	New issues and regressions are sent to every active connection that has the project and the level
	selected.
</p>

<Table
	{columns}
	rows={data.connections}
	rowKey={(connection: Connection) => connection.id}
	label="Connections"
>
	{#snippet cell(connection: Connection, column: ConnectionColumn)}
		{#if column.key === 'name'}
			<Button variant="link" href={page(connection)}>{connection.name}</Button>
		{:else if column.key === 'type'}
			{typeLabel(connection.type)}
		{:else if column.key === 'destination'}
			<span class="text-muted">{connection.destination}</span>
		{:else if column.key === 'projects'}
			{#if connection.all_projects}
				All projects
			{:else}
				<span class="inline-flex flex-wrap gap-1">
					{#each connection.projects ?? [] as project (project)}
						<Badge tone="project" label={project} />
					{/each}
				</span>
			{/if}
		{:else if column.key === 'levels'}
			<LevelSummary kind="event" chosen={connection.levels ?? []} />
		{:else if column.key === 'delivery'}
			{@const health = deliveryHealth(connection.delivery)}
			<div class="flex flex-col items-start gap-1 text-xs">
				{#if health.kind === 'failing'}
					<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolved, with its query -->
					<a href={log(connection)} title={health.error} class="text-error">
						failing: {shortError(health.error)}
					</a>
					{#if health.retrying > 0}
						<RetryNowButton connectionId={connection.id} waiting={health.retrying} />
					{/if}
				{:else if health.kind === 'sent'}
					<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolved, with its query -->
					<a href={log(connection)} class="text-muted">ok · <Time at={health.at} /></a>
				{:else if health.kind === 'queued'}
					<span class="text-muted">{health.count} queued</span>
				{:else}
					<span class="text-muted">—</span>
				{/if}
			</div>
		{:else if column.key === 'status'}
			{@const state = connectionState(connection)}
			<Badge tone={state.tone} label={state.label} />
		{:else}
			<RowMenu label="Actions for {connection.name}" items={menuOf(connection)} />
		{/if}
	{/snippet}
	{#snippet empty()}No connections yet. Nothing is sent until you add one.{/snippet}
</Table>
