<script lang="ts">
	import { resolve } from '$app/paths';
	import { help } from '#lib/api/catalog.gen.ts';
	import Badge from '#lib/components/Badge.svelte';
	import Breadcrumbs from '#lib/components/Breadcrumbs.svelte';
	import Button from '#lib/components/Button.svelte';
	import Dialog from '#lib/components/Dialog.svelte';
	import Hint from '#lib/components/Hint.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import StatusText from '#lib/components/StatusText.svelte';
	import type { StatusTone } from '#lib/components/status.ts';
	import Table from '#lib/components/Table.svelte';
	import type { TableColumn } from '#lib/components/table.ts';
	import Time from '#lib/components/Time.svelte';
	import { exactTime } from '#lib/format.ts';
	import { withQuery } from '#lib/nav.ts';
	import type { PageProps } from './$types';
	import { arrivedLine, compactionLine, compactionState, heldLine, type Run } from './cleanups.ts';

	let { data }: PageProps = $props();
	const history = $derived(data.history);

	type Step = NonNullable<Run['steps']>[number];
	type RunColumn = TableColumn<
		| 'started'
		| 'status'
		| 'limit'
		| 'before'
		| 'after'
		| 'deleted'
		| 'result'
		| 'took'
		| 'compaction'
		| 'details'
	>;
	type StepColumn = TableColumn<'number' | 'name' | 'freed' | 'took' | 'projects'>;

	const runColumns: RunColumn[] = [
		{ key: 'started', label: 'Started' },
		{ key: 'status', label: 'Status' },
		{ key: 'limit', label: 'Limit', number: true, wideOnly: true },
		{ key: 'before', label: 'Before', number: true, wideOnly: true },
		{ key: 'after', label: 'After' },
		{ key: 'deleted', label: 'Deleted in steps', wideOnly: true },
		{ key: 'result', label: 'Result' },
		{ key: 'took', label: 'Took', number: true, wideOnly: true },
		{ key: 'compaction', label: 'Files compacted', number: true, wideOnly: true },
		{ key: 'details', label: 'Details', actions: true }
	];

	const stepColumns: StepColumn[] = [
		{ key: 'number', label: '#', number: true },
		{ key: 'name', label: 'Step' },
		{ key: 'freed', label: 'Data freed (before compression)', number: true },
		{ key: 'took', label: 'Took', number: true },
		{ key: 'projects', label: 'Deleted' }
	];

	const statusTones: Record<string, StatusTone> = {
		done: 'success',
		running: 'warning',
		failed: 'error'
	};

	let shown: Run | undefined = $state();
	let detailsOpen = $state(false);

	function details(run: Run) {
		shown = run;
		detailsOpen = true;
	}

	// A deleted project's data is named "project N": no list to link to.
	const known = $derived(data.session.project_colors ?? {});
</script>

<Breadcrumbs
	items={[
		{ label: 'Projects', href: resolve('/(app)/projects') },
		{ label: 'Cleanup history', href: resolve('/(app)/projects/cleanups') }
	]}
/>
<PageHeading title="Cleanup history">
	{#snippet description()}
		When the data the database holds reaches {history.start_percent}% of
		<code>MAX_DATABASE_SIZE</code>
		<Hint text={help.settings.MAX_DATABASE_SIZE} />, data is deleted step by step until the database
		holds {history.stop_percent}% of the limit. When its files take over {history.files_percent}% of
		it, deleted data is compacted away first, and if they still do, deleting goes on until they are
		under the limit. Before and after are of the data a run started with, so data arriving meanwhile
		is shown apart; the files shrink when the compaction after the run frees what it deleted. Runs
		recorded before that show the files' size before and no after. The last
		{history.kept} runs are kept.
	{/snippet}
</PageHeading>

<Table
	columns={runColumns}
	rows={history.runs ?? []}
	rowKey={(run: Run) => run.id}
	label="Cleanup runs"
	compact
>
	{#snippet cell(run: Run, column: RunColumn)}
		{#if column.key === 'started'}
			<Time at={run.started_at} />
		{:else if column.key === 'status'}
			<StatusText tone={statusTones[run.status] ?? 'muted'}>{run.status}</StatusText>
		{:else if column.key === 'limit'}
			{run.limit}
		{:else if column.key === 'before'}
			{run.size_before}
			{#if run.before_files}<span class="text-xs text-muted">files</span>{/if}
		{:else if column.key === 'after'}
			{#if run.status === 'running'}…{:else if run.size_after}<span class="whitespace-nowrap">
					{run.size_after}
				</span><span class="block text-xs whitespace-nowrap text-muted">
					−{run.freed}
				</span>{:else}—{/if}
		{:else if column.key === 'deleted'}
			{run.deleted_steps || '—'}
		{:else if column.key === 'result'}
			<span class="text-xs">{run.result}</span>
			{#if run.error}<p class="text-xs text-error">{run.error}</p>{/if}
		{:else if column.key === 'took'}
			{run.took}
		{:else if column.key === 'compaction'}
			{#if run.compaction}
				{@const state = compactionState(run.compaction)}
				<StatusText tone={state.tone}>{state.text}</StatusText>
			{:else}
				—
			{/if}
		{:else if run.steps?.length}
			<Button variant="link" popup="dialog" onclick={() => details(run)}>Details</Button>
		{/if}
	{/snippet}
	{#snippet empty()}No cleanup has run yet.{/snippet}
</Table>

<Dialog
	title={shown ? `Cleanup, ${exactTime(shown.started_at).slice(0, 16)} UTC` : 'Cleanup'}
	bind:open={detailsOpen}
>
	{#if shown}
		<p class="mb-3 text-sm">
			Limit {shown.limit}: deleting goes down to {shown.stop}. {heldLine(shown)}
			{arrivedLine(shown)}
			{#if shown.files}
				Started by the files: {shown.files} after compacting, over {history.files_percent}% of the
				limit, so deleting goes on until they are back under it.
			{/if}
			<br />
			{shown.result}. Took {shown.took}.
			{compactionLine(shown.compaction, history.stale_percent, history.compaction_attempts)}
		</p>
		<Table
			columns={stepColumns}
			rows={shown.steps ?? []}
			rowKey={(step: Step) => step.number}
			label="Steps"
			compact
			flat
		>
			{#snippet cell(step: Step, column: StepColumn)}
				{#if column.key === 'number'}
					{step.number}
				{:else if column.key === 'name'}
					{step.name}
				{:else if column.key === 'freed'}
					{step.freed}
				{:else if column.key === 'took'}
					{step.took}
				{:else}
					{#each step.projects ?? [] as deleted (deleted.project)}
						<div class="flex flex-wrap items-center gap-1.5">
							<Badge
								tone="project"
								label={deleted.project}
								href={deleted.project in known
									? withQuery(resolve('/(app)/issues'), { project: deleted.project })
									: undefined}
							/>
							{deleted.deleted}
						</div>
					{/each}
				{/if}
			{/snippet}
		</Table>
	{/if}
</Dialog>
