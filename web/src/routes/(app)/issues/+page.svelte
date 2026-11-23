<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { choices } from '#lib/api/catalog.gen.ts';
	import type { Item } from '#lib/api/client.ts';
	import ActiveFilters from '#lib/components/ActiveFilters.svelte';
	import AgentCopyButton from '#lib/components/AgentCopyButton.svelte';
	import Badge from '#lib/components/Badge.svelte';
	import BulkBar from '#lib/components/BulkBar.svelte';
	import Button from '#lib/components/Button.svelte';
	import DateRange from '#lib/components/DateRange.svelte';
	import FilterBar from '#lib/components/FilterBar.svelte';
	import FilterPicker from '#lib/components/FilterPicker.svelte';
	import IssueActions from '#lib/components/IssueActions.svelte';
	import { doneMessage, selectionProblem, type IssueAction } from '#lib/components/issueActions.ts';
	import { statusLabel, statusRowTone, statusTitle } from '#lib/components/issueStatus.ts';
	import LevelBadge from '#lib/components/LevelBadge.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import Pager from '#lib/components/Pager.svelte';
	import SearchField from '#lib/components/SearchField.svelte';
	import Select from '#lib/components/Select.svelte';
	import Table from '#lib/components/Table.svelte';
	import type { TableColumn } from '#lib/components/table.ts';
	import Time from '#lib/components/Time.svelte';
	import { changed, cleared } from '#lib/filters.ts';
	import { showChanged } from '#lib/navigate.ts';
	import { toast } from '#lib/toast.svelte.ts';
	import type { PageProps } from './$types';
	import { columnSorts, issueChips, showsResolved, sortedBy, statusChange } from './issues.ts';

	let { data }: PageProps = $props();

	type Issue = Item<'list_issues'>;
	type IssueColumn = TableColumn<
		'issue' | 'project' | 'events' | 'firstSeen' | 'lastSeen' | 'resolved' | 'actions'
	>;

	const url = $derived(page.url);
	const canAdminister = $derived(page.data.session?.can_administer ?? false);

	const columns: IssueColumn[] = $derived([
		{ key: 'issue', label: 'Issue', fill: true },
		{ key: 'project', label: 'Project', sort: columnSorts.project },
		{ key: 'events', label: 'Events', number: true, wideOnly: true, sort: columnSorts.events },
		{ key: 'firstSeen', label: 'First seen', wideOnly: true, sort: columnSorts.firstSeen },
		{ key: 'lastSeen', label: 'Last seen', wideOnly: true, sort: columnSorts.lastSeen },
		...(showsResolved(data.filters.status)
			? [{ key: 'resolved' as const, label: 'Resolved', wideOnly: true }]
			: []),
		{ key: 'actions', label: 'Actions', actions: true }
	]);

	const statusOptions = choices.list_issues.status.map((status) => ({
		value: status,
		label: statusLabel(status)
	}));

	let selected: ReadonlySet<string | number> = $state(new Set());
	const selectedIds = $derived([...selected].map(String));

	async function acted(action: IssueAction) {
		toast.show(doneMessage(action, selectedIds.length));
		// The same list again (and the menu's count); the selection empties with the new rows.
		await invalidateAll();
	}

	const issueHref = (issue: Issue) => resolve('/(app)/issues/[id]', { id: issue.id });
</script>

<PageHeading title="Issues" />

<FilterBar label="Issue filters">
	<FilterPicker
		kind="project"
		options={data.projectNames}
		selected={data.filters.project}
		onchange={(project: string[]) => showChanged(url, { project })}
	/>
	<Select
		label="Status"
		value={data.filters.status}
		options={statusOptions}
		onchange={(status: string) => showChanged(url, statusChange(status))}
	/>
	<FilterPicker
		kind="level"
		options={data.levelChoices}
		selected={data.filters.level}
		onchange={(level: string[]) => showChanged(url, { level })}
	/>
	<SearchField
		value={data.filters.query}
		label="Search title or location"
		onchange={(query: string) => showChanged(url, { query }, true)}
	/>
	<DateRange
		from={data.filters.from}
		to={data.filters.to}
		onchange={(from: string, to: string) => showChanged(url, { from, to })}
	/>
</FilterBar>

<ActiveFilters chips={issueChips(url, data.filters)} clear={cleared(url)} />

{#if data.issues.length > 0}
	<BulkBar count={selectedIds.length} problem={selectionProblem(selectedIds.length)}>
		<IssueActions issueIds={selectedIds} canDelete={canAdminister} ondone={acted} />
	</BulkBar>
{/if}

<Table
	{columns}
	rows={data.issues}
	label="Issues"
	rowKey={(issue: Issue) => issue.id}
	bind:selected
	selectLabel={(issue: Issue) => `Select ${issue.title}`}
	sort={{ current: data.filters.sort, href: (sort: string) => sortedBy(url, sort) }}
	rowTone={(issue: Issue) => statusRowTone(issue.status)}
	rowTitle={(issue: Issue) => statusTitle(issue.status)}
>
	{#snippet cell(issue: Issue, column: IssueColumn)}
		{#if column.key === 'issue'}
			<div class="flex min-w-0 items-center gap-2">
				<LevelBadge level={issue.level} href={changed(url, { level: [issue.level] })} />
				<Button variant="title" href={issueHref(issue)} title={issue.title}>{issue.title}</Button>
				<span class="sr-only">({issue.status})</span>
			</div>
			{#if issue.location}
				<div class="truncate text-xs text-muted" title={issue.location}>{issue.location}</div>
			{/if}
		{:else if column.key === 'project'}
			<Badge
				tone="project"
				label={issue.project}
				href={changed(url, { project: [issue.project] })}
				title="Show only {issue.project}"
			/>
		{:else if column.key === 'events'}
			{issue.event_count}
		{:else if column.key === 'firstSeen'}
			<Time at={issue.first_seen} />
		{:else if column.key === 'lastSeen'}
			<Time at={issue.last_seen} />
		{:else if column.key === 'resolved'}
			{#if issue.resolved_at}<Time at={issue.resolved_at} />{:else}<span class="text-muted">
					—
				</span>{/if}
		{:else}
			<span class="flex items-center gap-2">
				<AgentCopyButton request={issue.mcp_reference ?? ''} />
				<Button
					variant="accent"
					icon="view"
					href={issueHref(issue)}
					title="Open the issue {issue.title}"
				/>
			</span>
		{/if}
	{/snippet}
	{#snippet empty()}No issues match these filters.{/snippet}
</Table>

<Pager
	page={data.page}
	perPage={data.perPage}
	total={data.count}
	href={(number: number) => changed(url, { page: number > 1 ? String(number) : null })}
	onperpage={(perPage: number) => showChanged(url, { per_page: String(perPage) })}
/>
