<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import type { Item } from '#lib/api/client.ts';
	import ActiveFilters from '#lib/components/ActiveFilters.svelte';
	import Badge from '#lib/components/Badge.svelte';
	import Button from '#lib/components/Button.svelte';
	import DateRange from '#lib/components/DateRange.svelte';
	import FilterBar from '#lib/components/FilterBar.svelte';
	import type { FilterChip } from '#lib/components/chips.ts';
	import LevelBadge from '#lib/components/LevelBadge.svelte';
	import LogBody from '#lib/components/LogBody.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import Pager from '#lib/components/Pager.svelte';
	import Picker from '#lib/components/Picker.svelte';
	import SearchField from '#lib/components/SearchField.svelte';
	import Table from '#lib/components/Table.svelte';
	import type { TableColumn } from '#lib/components/table.ts';
	import Time from '#lib/components/Time.svelte';
	import { changed, cleared, type Change } from '#lib/filters.ts';
	import { oldUI, withQuery } from '#lib/nav.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	type Log = Item<'list_logs'>;
	type LogColumn = TableColumn<'time' | 'level' | 'project' | 'message'>;

	const columns: LogColumn[] = [
		{ key: 'time', label: 'Time' },
		{ key: 'level', label: 'Level' },
		{ key: 'project', label: 'Project', wideOnly: true },
		{ key: 'message', label: 'Message' }
	];

	const url = $derived(page.url);

	/** Shows the list with the change; typing replaces the entry instead of adding one. */
	function show(change: Change, replace = false) {
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- this page with another query
		void goto(changed(url, change), { reset: false, replace });
	}

	const chips: FilterChip[] = $derived(
		[
			{
				label: 'Project',
				values: data.filters.project.map((name) => ({
					text: name,
					kind: 'project' as const,
					remove: changed(url, { project: data.filters.project.filter((other) => other !== name) })
				}))
			},
			{
				label: 'Level',
				values: data.filters.level.map((level) => ({
					text: level,
					kind: 'level' as const,
					remove: changed(url, { level: data.filters.level.filter((other) => other !== level) })
				}))
			},
			{
				label: 'Search',
				values: data.filters.query
					? [{ text: data.filters.query, remove: changed(url, { query: null }) }]
					: []
			},
			{
				label: 'Trace',
				values: data.filters.traceId
					? [{ text: data.filters.traceId, remove: changed(url, { trace_id: null }) }]
					: []
			},
			{
				label: 'Dates',
				values:
					data.filters.from || data.filters.to
						? [
								{
									text: `${data.filters.from || '…'} – ${data.filters.to || '…'}`,
									remove: changed(url, { from: null, to: null })
								}
							]
						: []
			}
		].filter((chip) => chip.values.length > 0)
	);
</script>

<PageHeading title="Logs" />

<FilterBar label="Log filters">
	<Picker
		label="Projects"
		all="All projects"
		options={data.projectNames}
		selected={data.filters.project}
		onchange={(project: string[]) => show({ project })}
	>
		{#snippet option(name)}<Badge tone="project" label={name} />{/snippet}
	</Picker>
	<Picker
		label="Levels"
		all="All levels"
		options={data.levelChoices}
		selected={data.filters.level}
		onchange={(level: string[]) => show({ level })}
	>
		{#snippet option(level)}<LevelBadge {level} />{/snippet}
	</Picker>
	<SearchField
		value={data.filters.query}
		label="Search text"
		onchange={(query: string) => show({ query }, true)}
	/>
	<SearchField
		value={data.filters.traceId}
		label="Trace ID"
		onchange={(traceId: string) => show({ trace_id: traceId }, true)}
	/>
	<DateRange
		from={data.filters.from}
		to={data.filters.to}
		onchange={(from: string, to: string) => show({ from, to })}
	/>
</FilterBar>

<ActiveFilters {chips} clear={cleared(url)} />

<Table {columns} rows={data.logs} label="Logs" compact>
	{#snippet cell(log: Log, column: LogColumn)}
		{#if column.key === 'time'}
			<Time at={log.time} />
		{:else if column.key === 'level'}
			<LevelBadge level={log.level} href={changed(url, { level: [log.level] })} />
		{:else if column.key === 'project'}
			<Badge tone="project" label={log.project} href={changed(url, { project: [log.project] })} />
		{:else}
			<LogBody text={log.body} />
			<div class="flex flex-wrap gap-3 text-xs">
				{#if log.issue}
					<Button variant="link" href={oldUI.issue(log.issue.id)}>Issue: {log.issue.title}</Button>
				{/if}
				{#if log.trace_id}
					<Button variant="link" href={withQuery(resolve('/logs'), { trace_id: log.trace_id })}>
						trace {log.trace_id.slice(0, 8)}
					</Button>
				{/if}
			</div>
		{/if}
	{/snippet}
	{#snippet empty()}No logs match these filters.{/snippet}
</Table>

<Pager
	page={data.page}
	perPage={data.perPage}
	total={data.count.count}
	more={data.count.more}
	href={(number: number) => changed(url, { page: number > 1 ? String(number) : null })}
	onperpage={(perPage: number) => show({ per_page: String(perPage) })}
/>
