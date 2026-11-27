<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import { page } from '$app/state';
	import { errorMessage, get } from '#lib/api/client.ts';
	import {
		entryMessage,
		entryTitle,
		recordHref,
		resourceLabel,
		viaText,
		type AuditEntry
	} from '#lib/audit.ts';
	import ActiveFilters from '#lib/components/ActiveFilters.svelte';
	import AuditActionBadge from '#lib/components/AuditActionBadge.svelte';
	import AuditDiff from '#lib/components/AuditDiff.svelte';
	import AuditItems from '#lib/components/AuditItems.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import Button from '#lib/components/Button.svelte';
	import { appliedChips, daysChip, listChip, textChip } from '#lib/components/chips.ts';
	import DateRange from '#lib/components/DateRange.svelte';
	import Dialog from '#lib/components/Dialog.svelte';
	import Facts from '#lib/components/Facts.svelte';
	import FilterBar from '#lib/components/FilterBar.svelte';
	import FilterPicker from '#lib/components/FilterPicker.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import Pager from '#lib/components/Pager.svelte';
	import Picker from '#lib/components/Picker.svelte';
	import ProjectsSummary from '#lib/components/ProjectsSummary.svelte';
	import SearchField from '#lib/components/SearchField.svelte';
	import Table from '#lib/components/Table.svelte';
	import type { TableColumn } from '#lib/components/table.ts';
	import Time from '#lib/components/Time.svelte';
	import { changed, cleared } from '#lib/filters.ts';
	import { reloading } from '#lib/loading.svelte.ts';
	import { showChanged, shown } from '#lib/navigate.svelte.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	type AuditColumn = TableColumn<
		'time' | 'user' | 'action' | 'resource' | 'record' | 'projects' | 'changes'
	>;

	const columns: AuditColumn[] = [
		{ key: 'time', label: 'Time' },
		{ key: 'action', label: 'Action' },
		{ key: 'projects', label: 'Projects', wideOnly: true },
		{ key: 'user', label: 'User' },
		{ key: 'resource', label: 'Record' },
		{ key: 'record', label: 'Message', fill: true },
		{ key: 'changes', label: 'Changes', actions: true }
	];

	const url = $derived(page.url);

	// The users the viewer sees, and users chosen by a click on a name they do not list (a user
	// deleted since), named as the log shows them.
	const usersById = $derived(new Map(data.users.map((user) => [user.id, user])));

	function userName(id: string): string {
		return (
			usersById.get(id)?.name ?? data.entries.find((entry) => entry.user_id === id)?.user_name ?? id
		);
	}
	const canAdminister = $derived(page.data.session?.can_administer ?? false);

	const chips = $derived(
		appliedChips([
			listChip(url, 'Record', 'resource', data.filters.resource),
			listChip(url, 'Action', 'action', data.filters.action),
			listChip(url, 'Project', 'project', data.filters.project, 'project'),
			listChip(url, 'User', 'user', data.filters.user, undefined, userName),
			textChip(url, 'Search', 'query', data.filters.query),
			daysChip(url, data.filters.from, data.filters.to)
		])
	);

	// The entry whose changes the dialog shows.
	let shownEntry: AuditEntry | undefined = $state();
	let dialogOpen = $state(false);

	// A link in the dialog (a project, the user, the record) applies a filter or opens a page: the
	// dialog closes, so the list shows it.
	afterNavigate(() => {
		dialogOpen = false;
	});

	// The records of an entry whose request changed several, read when its dialog opens.
	type Items = NonNullable<Awaited<ReturnType<typeof get<'get_audit_entry'>>>['items']>;
	let items: Items | undefined = $state();
	let itemsError = $state('');

	async function openEntry(entry: AuditEntry) {
		shownEntry = entry;
		items = undefined;
		itemsError = '';
		dialogOpen = true;

		if (!entry.item_count) return;

		try {
			const read = await get('get_audit_entry', { id: entry.id });

			if (shownEntry?.id === entry.id) items = read.items ?? [];
		} catch (failure) {
			if (shownEntry === entry)
				itemsError = errorMessage(failure, 'The records could not be read.');
		}
	}

	function itemHref(entry: AuditEntry, item: Items[number]): string | undefined {
		return recordHref(
			{ resource: entry.resource, record_id: item.record_id, exists: item.exists },
			canAdminister
		);
	}

	type Fact = { label: string; key: 'time' | 'user' | 'via' | 'ip' | 'record' | 'projects' };

	const facts: Fact[] = [
		{ label: 'Time', key: 'time' },
		{ label: 'User', key: 'user' },
		{ label: 'Via', key: 'via' },
		{ label: 'IP address', key: 'ip' },
		{ label: 'Record', key: 'record' },
		{ label: 'Projects', key: 'projects' }
	];
</script>

<PageHeading title="Audit log">
	{#snippet description()}
		Every change admins and users made, in the web app, through MCP or the REST API, with the fields
		before and after. Entries are kept for good; users see only those of their projects.
	{/snippet}
</PageHeading>

<FilterBar label="Audit log filters">
	<Picker
		label="Records"
		all="All records"
		options={data.choices.resource}
		selected={shown('resource', data.filters.resource)}
		onchange={(resource: string[]) => showChanged(url, { resource })}
	>
		{#snippet option(value: string)}{resourceLabel(value)}{/snippet}
	</Picker>
	<Picker
		label="Actions"
		all="All actions"
		options={data.choices.action}
		selected={shown('action', data.filters.action)}
		onchange={(action: string[]) => showChanged(url, { action })}
	>
		{#snippet option(value: string)}<AuditActionBadge action={value} />{/snippet}
	</Picker>
	<Picker
		label="Users"
		all="All users"
		options={data.choices.user}
		selected={shown('user', data.filters.user)}
		onchange={(user: string[]) => showChanged(url, { user })}
		searchText={userName}
	>
		{#snippet option(id: string)}{userName(id)}{/snippet}
	</Picker>
	<FilterPicker
		kind="project"
		options={data.choices.project}
		selected={shown('project', data.filters.project)}
		onchange={(project: string[]) => showChanged(url, { project })}
	/>
	<SearchField
		value={shown('query', data.filters.query)}
		label="Search user, record or address"
		onchange={(query: string, now: boolean) => showChanged(url, { query }, { typing: true, now })}
	/>
	<DateRange
		from={shown('from', data.filters.from)}
		to={shown('to', data.filters.to)}
		onchange={(from: string, to: string) => showChanged(url, { from, to })}
	/>
</FilterBar>

<ActiveFilters {chips} clear={cleared(url)} />

<!-- The record's page, or for a request that changed several the dialog listing them (from the list). -->
{#snippet record(entry: AuditEntry, inList: boolean)}
	{@const href = entry.item_count ? undefined : recordHref(entry, canAdminister)}
	{#if entry.item_count && inList}
		<Button variant="title" popup="dialog" onclick={() => openEntry(entry)}>
			{entry.record_name}
		</Button>
	{:else if href}
		<Button variant="title" truncate title={entry.record_name} {href}>
			{entry.record_name || entry.record_id}
		</Button>
	{:else}
		<span title={entry.record_name}>{entry.record_name || entry.record_id}</span>
	{/if}
{/snippet}

<!-- A user who still exists by their current name, so every row of one user reads alike (the
     filter is by id); the name they had then in the tooltip. -->
{#snippet user(entry: AuditEntry)}
	{#if entry.user_id}
		{@const name = usersById.get(entry.user_id)?.name ?? entry.user_name}
		<Button
			variant="title"
			href={changed(url, { user: [entry.user_id] })}
			title={name === entry.user_name ? undefined : `Named ${entry.user_name} then`}
		>
			{name}
		</Button>
	{:else}
		{entry.user_name}
	{/if}
{/snippet}

{#snippet projects(entry: AuditEntry)}
	{@const names = entry.projects ?? []}
	<ProjectsSummary
		all={false}
		{names}
		links={names.map((name) => changed(url, { project: [name] }))}
		limit={1}
	/>
{/snippet}

<Table
	busy={reloading()}
	{columns}
	rows={data.entries}
	rowKey={(entry: AuditEntry) => entry.id}
	label="Audit log"
	compact
>
	{#snippet cell(entry: AuditEntry, column: AuditColumn)}
		{#if column.key === 'time'}
			<Time at={entry.created_at} />
		{:else if column.key === 'user'}
			{@render user(entry)}
			{#if entry.source !== 'ui'}
				<div class="text-xs whitespace-nowrap text-muted">{viaText(entry)}</div>
			{/if}
		{:else if column.key === 'action'}
			<AuditActionBadge action={entry.action} href={changed(url, { action: [entry.action] })} />
		{:else if column.key === 'resource'}
			<Button variant="muted" href={changed(url, { resource: [entry.resource] })}>
				{resourceLabel(entry.resource)}
			</Button>
		{:else if column.key === 'record'}
			{@const message = entryMessage(entry)}
			<!-- A sentence that wraps, at most two lines, so a narrow screen never pushes it under the eye. -->
			<div class="line-clamp-2 min-w-0 wrap-break-word">
				<span class="text-muted">{message.lead}</span>
				{@render record(entry, true)}
				{#if message.tail}<span class="text-muted">{message.tail}</span>{/if}
			</div>
		{:else if column.key === 'projects'}
			{@render projects(entry)}
		{:else}
			<Button
				variant="accent"
				icon="view"
				popup="dialog"
				title="Show the change"
				onclick={() => openEntry(entry)}
			/>
		{/if}
	{/snippet}
	{#snippet empty()}No changes{chips.length ? ' match these filters' : ' yet'}.{/snippet}
</Table>

<Pager
	page={data.page}
	perPage={data.perPage}
	total={data.count.count}
	more={data.count.more}
	href={(number: number) => changed(url, { page: number > 1 ? String(number) : null })}
	onperpage={(perPage: number) => showChanged(url, { per_page: String(perPage) }, { now: true })}
/>

<Dialog title={shownEntry ? entryTitle(shownEntry) : 'Change'} bind:open={dialogOpen}>
	{#if shownEntry}
		{@const entry = shownEntry}
		<Facts {facts} columns>
			{#snippet value(fact: Fact)}
				{#if fact.key === 'time'}
					<Time at={entry.created_at} />
				{:else if fact.key === 'user'}
					{@render user(entry)}
					{#if entry.user_email}<div class="text-xs text-muted">{entry.user_email}</div>{/if}
				{:else if fact.key === 'via'}
					{viaText(entry)}
				{:else if fact.key === 'ip'}
					<span class="font-mono text-xs">{entry.ip || '—'}</span>
				{:else if fact.key === 'record'}
					<span class="flex min-w-0 flex-wrap items-center gap-2">
						{@render record(entry, false)}
					</span>
				{:else}
					{@render projects(entry)}
				{/if}
			{/snippet}
		</Facts>
		{#if !entry.item_count}
			<AuditDiff before={entry.before} after={entry.after} />
		{:else if itemsError}
			<Notice tone="error">{itemsError}</Notice>
		{:else if items}
			<AuditItems
				items={items.map((item) => ({ ...item, href: itemHref(entry, item) }))}
				query={data.filters.query}
			/>
		{:else}
			<p class="text-sm text-muted">Reading the records…</p>
		{/if}
	{/if}
</Dialog>
