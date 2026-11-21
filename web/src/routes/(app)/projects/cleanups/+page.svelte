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

	let { data }: PageProps = $props();

	type Run = NonNullable<typeof data.history.runs>[number];
	type Step = NonNullable<Run['steps']>[number];
	type RunColumn = TableColumn<
		| 'started'
		| 'status'
		| 'limit'
		| 'before'
		| 'compacted'
		| 'after'
		| 'deleted'
		| 'result'
		| 'took'
		| 'details'
	>;
	type StepColumn = TableColumn<'number' | 'name' | 'freed' | 'took' | 'projects'>;

	const runColumns: RunColumn[] = [
		{ key: 'started', label: 'Started' },
		{ key: 'status', label: 'Status' },
		{ key: 'limit', label: 'Limit', number: true, wideOnly: true },
		{ key: 'before', label: 'Before', number: true, wideOnly: true },
		{ key: 'compacted', label: 'Compacted', number: true, wideOnly: true },
		{ key: 'after', label: 'After', number: true },
		{ key: 'deleted', label: 'Deleted in steps', wideOnly: true },
		{ key: 'result', label: 'Result' },
		{ key: 'took', label: 'Took', number: true, wideOnly: true },
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
<PageHeading title="Cleanup history" />
<p class="mb-4 text-muted">
	When the database reaches <code>MAX_DATABASE_SIZE</code>
	<Hint text={help.settings.MAX_DATABASE_SIZE} />, the ingest queue and the issues are compacted
	first: deleted data keeps its space on disk until then. If the database is still at the limit,
	data is deleted step by step until it is at {data.history.stop_percent}% of the limit. Sizes are
	on disk; the last {data.history.kept} runs are kept.
</p>

<Table
	columns={runColumns}
	rows={data.history.runs ?? []}
	rowKey={(run: Run) => run.id}
	label="Cleanup runs"
	compact
	scroll
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
		{:else if column.key === 'compacted'}
			{run.compacted}
		{:else if column.key === 'after'}
			{run.status === 'running' ? '…' : run.size_after}
		{:else if column.key === 'deleted'}
			{run.deleted_steps || '—'}
		{:else if column.key === 'result'}
			<span class="text-xs">{run.result}</span>
			{#if run.error}<p class="text-xs text-error">{run.error}</p>{/if}
		{:else if column.key === 'took'}
			{run.took}
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
			Limit {shown.limit}: deleting goes down to {shown.stop}. On disk: {shown.size_before} before,
			{shown.compacted} after compacting, so about {shown.needed} to free; {shown.size_after} after.
			<br />
			{shown.result}. Took {shown.took}.
		</p>
		<Table
			columns={stepColumns}
			rows={shown.steps ?? []}
			rowKey={(step: Step) => step.number}
			label="Steps"
			compact
			scroll
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
