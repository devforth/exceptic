<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { choices } from '#lib/api/catalog.gen.ts';
	import type { Item } from '#lib/api/client.ts';
	import Badge from '#lib/components/Badge.svelte';
	import Breadcrumbs from '#lib/components/Breadcrumbs.svelte';
	import Button from '#lib/components/Button.svelte';
	import Dialog from '#lib/components/Dialog.svelte';
	import FilterBar from '#lib/components/FilterBar.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import Pager from '#lib/components/Pager.svelte';
	import Select from '#lib/components/Select.svelte';
	import StatusText from '#lib/components/StatusText.svelte';
	import Table from '#lib/components/Table.svelte';
	import type { TableColumn } from '#lib/components/table.ts';
	import Time from '#lib/components/Time.svelte';
	import { changed } from '#lib/filters.ts';
	import { reloading } from '#lib/loading.svelte.ts';
	import { showChanged, shown } from '#lib/navigate.svelte.ts';
	import { deliveryStatus } from '#lib/notifications.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	type Delivery = Item<'list_deliveries'>;
	type DeliveryColumn = TableColumn<
		'queued' | 'connection' | 'message' | 'status' | 'attempts' | 'error'
	>;
	type Issue = { message: string; project?: string; title?: string; issue_id?: string };

	const columns: DeliveryColumn[] = [
		{ key: 'queued', label: 'Queued' },
		{ key: 'connection', label: 'Connection' },
		{ key: 'message', label: 'Message', fill: true },
		{ key: 'status', label: 'Status' },
		{ key: 'attempts', label: 'Attempts', number: true, wideOnly: true },
		{ key: 'error', label: 'Error' }
	];

	const url = $derived(page.url);
	const names = $derived(
		new Map(data.connections.map((connection) => [connection.id, connection.name]))
	);
	const connectionOptions = $derived(
		data.connections.map((connection) => ({ value: connection.id, label: connection.name }))
	);
	const statusOptions = choices.list_deliveries.status.map((status) => ({
		value: status,
		label: status
	}));
	const filtered = $derived(Boolean(data.filters.connection || data.filters.status));

	// The issues a summary stood for, in a dialog.
	let summary: Delivery | undefined = $state();
	let summaryOpen = $state(false);

	function openSummary(delivery: Delivery) {
		summary = delivery;
		summaryOpen = true;
	}
</script>

<Breadcrumbs
	items={[
		{ label: 'Notifications', href: resolve('/(app)/notifications') },
		{ label: 'Delivery log', href: resolve('/(app)/notifications/log') }
	]}
/>
<PageHeading title="Delivery log">
	{#snippet description()}
		Every notification message and what happened to it. Failed attempts are retried for a while (<code>
			NOTIFY_RETRY_FOR
		</code>); sent messages are kept for <code>NOTIFY_KEEP_SENT</code>, failed ones for
		<code>NOTIFY_KEEP_FAILED</code>.
	{/snippet}
</PageHeading>

<FilterBar label="Delivery filters">
	<Select
		label="Connection"
		all="All connections"
		value={shown('connection', data.filters.connection)}
		options={connectionOptions}
		onchange={(connection: string) => showChanged(url, { connection })}
	/>
	<Select
		label="Status"
		all="Any status"
		value={shown('status', data.filters.status)}
		options={statusOptions}
		onchange={(status: string) => showChanged(url, { status })}
	/>
</FilterBar>

{#snippet issue(item: Issue)}
	<span class="shrink-0 text-xs text-muted">{item.message}</span>
	{#if item.project}<Badge tone="project" label={item.project} />{/if}
	{#if item.issue_id}
		<Button
			variant="link"
			truncate
			title={item.title}
			href={resolve('/(app)/issues/[id]', { id: item.issue_id })}
		>
			{item.title}
		</Button>
	{:else}
		<span class="truncate" title={item.title}>{item.title}</span>
	{/if}
{/snippet}

<Table
	busy={reloading()}
	{columns}
	rows={data.deliveries}
	rowKey={(delivery: Delivery) => delivery.id}
	label="Messages"
	compact
>
	{#snippet cell(delivery: Delivery, column: DeliveryColumn)}
		{#if column.key === 'queued'}
			<Time at={delivery.created_at} />
		{:else if column.key === 'connection'}
			<Button
				variant="link"
				href={resolve('/(app)/notifications/[id=connectionId]', { id: delivery.connection_id })}
			>
				{names.get(delivery.connection_id) ?? delivery.connection_id}
			</Button>
		{:else if column.key === 'message'}
			<!-- One line, the title cut with "…": the table never scrolls sideways. -->
			<div class="flex min-w-0 items-center gap-2">
				{#if delivery.items?.length}
					<span class="shrink-0 text-xs text-muted">{delivery.message}</span>
					<Button
						variant="link"
						truncate
						title={delivery.title}
						popup="dialog"
						onclick={() => openSummary(delivery)}
					>
						{delivery.title}
					</Button>
				{:else}
					{@render issue(delivery)}
				{/if}
			</div>
		{:else if column.key === 'status'}
			{@const status = deliveryStatus(delivery)}
			<StatusText tone={status.tone}>{status.label}</StatusText>
			{#if delivery.retrying}
				<div class="text-xs text-muted">next <Time at={delivery.next_attempt_at} /></div>
			{:else if delivery.status === 'sent'}
				<div class="text-xs text-muted"><Time at={delivery.updated_at} /></div>
			{/if}
		{:else if column.key === 'attempts'}
			{delivery.attempts}
		{:else if delivery.last_error}
			<span class="block max-w-64 truncate text-xs text-error" title={delivery.last_error}>
				{delivery.last_error}
			</span>
		{/if}
	{/snippet}
	{#snippet empty()}No messages{filtered ? ' match these filters' : ''}.{/snippet}
</Table>

<Pager
	page={data.page}
	perPage={data.perPage}
	total={data.count}
	href={(number: number) => changed(url, { page: number > 1 ? String(number) : null })}
	onperpage={(perPage: number) => showChanged(url, { per_page: String(perPage) }, { now: true })}
/>

<Dialog title={summary?.title ?? 'Summary'} bind:open={summaryOpen}>
	{#if summary}
		<ul class="flex flex-col gap-2">
			{#each summary.items ?? [] as item, index (index)}
				<li class="flex flex-wrap items-center gap-2">{@render issue(item)}</li>
			{/each}
		</ul>
		{#if summary.more}<p class="mt-3 text-xs text-muted">and {summary.more} more not listed</p>{/if}
	{/if}
</Dialog>
