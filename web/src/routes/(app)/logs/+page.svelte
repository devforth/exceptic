<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import type { Item } from '#lib/api/client.ts';
	import ActiveFilters from '#lib/components/ActiveFilters.svelte';
	import Badge from '#lib/components/Badge.svelte';
	import BulkBar from '#lib/components/BulkBar.svelte';
	import Button from '#lib/components/Button.svelte';
	import DateRange from '#lib/components/DateRange.svelte';
	import ConfirmButton from '#lib/components/ConfirmButton.svelte';
	import FilterBar from '#lib/components/FilterBar.svelte';
	import { appliedChips, daysChip, listChip, textChip } from '#lib/components/chips.ts';
	import FilterPicker from '#lib/components/FilterPicker.svelte';
	import LevelBadge from '#lib/components/LevelBadge.svelte';
	import LogBody from '#lib/components/LogBody.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import Pager from '#lib/components/Pager.svelte';
	import SearchField from '#lib/components/SearchField.svelte';
	import Table from '#lib/components/Table.svelte';
	import type { TableColumn } from '#lib/components/table.ts';
	import Time from '#lib/components/Time.svelte';
	import { changed, cleared } from '#lib/filters.ts';
	import { reloading } from '#lib/loading.svelte.ts';
	import { showChanged, shown } from '#lib/navigate.svelte.ts';
	import { withQuery } from '#lib/nav.ts';
	import { logSearchSeconds } from '#lib/api/catalog.gen.ts';
	import type { PageProps } from './$types';
	import { deleteConfirmation, deleteLogs, selectionProblem, stoppedSearch } from './logs.ts';

	let { data }: PageProps = $props();

	type Log = Item<'list_logs'>;
	type LogColumn = TableColumn<'time' | 'level' | 'project' | 'message'>;

	const url = $derived(page.url);
	// Deleting logs is for admins: only they get checkboxes.
	const canAdminister = $derived(page.data.session?.can_administer ?? false);

	const columns: LogColumn[] = $derived([
		{ key: 'time', label: 'Time' },
		{ key: 'level', label: 'Level' },
		{ key: 'project', label: 'Project', wideOnly: true },
		{ key: 'message', label: 'Message' }
	]);

	let selected: ReadonlySet<string | number> = $state(new Set());
	const selectedIds = $derived([...selected].map(Number));
	const problem = $derived(selectionProblem(selectedIds.length));

	/** Deletes the logs, then shows the list again; the selection empties with the new rows. */
	async function remove(input: Parameters<typeof deleteLogs>[0]) {
		await deleteLogs(input);
		// The delete is done: a failed reload of the list is not a failed delete.
		void invalidateAll();
	}

	// What a search that ran out of time found.
	const stoppedText = $derived(stoppedSearch(data.count));

	const chips = $derived(
		appliedChips([
			listChip(url, 'Project', 'project', data.filters.project, 'project'),
			listChip(url, 'Level', 'level', data.filters.level, 'level'),
			textChip(url, 'Search', 'query', data.filters.query),
			textChip(url, 'Trace', 'trace_id', data.filters.traceId),
			daysChip(url, data.filters.from, data.filters.to)
		])
	);
</script>

<PageHeading title="Logs" />

<FilterBar label="Log filters">
	<FilterPicker
		kind="project"
		options={data.projectNames}
		selected={shown('project', data.filters.project)}
		onchange={(project: string[]) => showChanged(url, { project })}
	/>
	<FilterPicker
		kind="level"
		options={data.levelChoices}
		selected={shown('level', data.filters.level)}
		onchange={(level: string[]) => showChanged(url, { level })}
	/>
	<SearchField
		value={shown('query', data.filters.query)}
		label="Search text"
		onchange={(query: string, now: boolean) => showChanged(url, { query }, { typing: true, now })}
	/>
	<SearchField
		value={shown('trace_id', data.filters.traceId)}
		label="Trace ID"
		onchange={(traceId: string, now: boolean) =>
			showChanged(url, { trace_id: traceId }, { typing: true, now })}
	/>
	<DateRange
		from={shown('from', data.filters.from)}
		to={shown('to', data.filters.to)}
		onchange={(from: string, to: string) => showChanged(url, { from, to })}
	/>
</FilterBar>

<ActiveFilters {chips} clear={cleared(url)} />

{#if data.count.stopped}
	<div class="mb-3">
		<Notice tone="warning">
			<p>
				The search stopped after {logSearchSeconds} s{#if data.count.searched_to}
					at the logs received <Time at={data.count.searched_to} />{/if}: {stoppedText}
			</p>
		</Notice>
	</div>
{/if}

{#if data.listStopped}
	<div class="mb-3">
		<Notice tone="warning"><p>{data.listStopped}</p></Notice>
	</div>
{/if}

{#if canAdminister && data.logs.length > 0}
	<BulkBar count={selectedIds.length} {problem}>
		<ConfirmButton
			confirm={deleteConfirmation(selectedIds.length)}
			action={() => remove({ log_ids: selectedIds })}
			disabled={problem !== undefined}
			title={problem}
		>
			Delete
		</ConfirmButton>
	</BulkBar>
{/if}

<Table
	busy={reloading()}
	{columns}
	rows={data.logs}
	label="Logs"
	compact
	rowKey={canAdminister ? (log: Log) => log.id : undefined}
	bind:selected
	selectLabel={(log: Log) => `Select the log ${log.body.slice(0, 80)}`}
>
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
					<Button variant="link" href={resolve('/(app)/issues/[id]', { id: log.issue.id })}>
						Issue: {log.issue.title}
					</Button>
				{/if}
				{#if log.trace_id}
					<Button
						variant="muted"
						href={withQuery(resolve('/(app)/logs'), { trace_id: log.trace_id })}
					>
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
	more={data.count.more || data.count.stopped}
	href={(number: number) => changed(url, { page: number > 1 ? String(number) : null })}
	onperpage={(perPage: number) => showChanged(url, { per_page: String(perPage) }, { now: true })}
/>
