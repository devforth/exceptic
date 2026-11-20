<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import { levels } from '#lib/api/catalog.gen.ts';
	import { ApiError, post, type Item } from '#lib/api/client.ts';
	import Badge from '#lib/components/Badge.svelte';
	import Button from '#lib/components/Button.svelte';
	import Card from '#lib/components/Card.svelte';
	import CopyField from '#lib/components/CopyField.svelte';
	import LevelBadge from '#lib/components/LevelBadge.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import Table from '#lib/components/Table.svelte';
	import type { TableColumn } from '#lib/components/table.ts';
	import UsageBar from '#lib/components/UsageBar.svelte';
	import { oldUI, withQuery } from '#lib/nav.ts';
	import { toast } from '#lib/toast.svelte.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	type Project = Item<'list_projects'>;
	type ProjectColumn = TableColumn<'project' | 'dsn' | 'unresolved' | 'records' | 'actions'>;

	const columns: ProjectColumn[] = [
		{ key: 'project', label: 'Project' },
		{ key: 'dsn', label: 'DSN' },
		{ key: 'unresolved', label: 'Unresolved', number: true },
		{ key: 'records', label: 'Records', wideOnly: true },
		{ key: 'actions', label: 'Actions', actions: true }
	];

	// Also what the old project page says after deleting (it returns here with ?notice=deleted).
	const deleted = 'Project and its data deleted.';

	const canAdminister = $derived(data.session.can_administer);
	const storage = $derived(data.storage);

	// What a project records: every level, none, or the ones listed.
	function recorded(own: string[] | null, known: readonly string[]) {
		const list = own ?? [];

		return list.length === known.length ? 'all' : list.length === 0 ? 'none' : list;
	}

	async function remove(project: Project) {
		if (
			!confirm(
				`Delete the project ${project.name} with all its issues, events and logs? Its DSN stops working. This cannot be undone.`
			)
		) {
			return;
		}

		try {
			await post('delete_project', { project: String(project.id) });
			toast.show(deleted);
			await invalidateAll();
		} catch (failure) {
			toast.show(failure instanceof ApiError ? failure.message : 'Could not delete the project.');
		}
	}
</script>

<PageHeading title="Projects">
	{#snippet actions()}
		{#if canAdminister}
			<Button variant="primary" icon="add" href={oldUI.newProject}>Add project</Button>
		{/if}
	{/snippet}
</PageHeading>

{#if page.url.searchParams.get('notice') === 'deleted'}
	<div class="mb-4"><Notice tone="success">{deleted}</Notice></div>
{/if}

<Card title="Storage">
	<p>
		Database: <b>{storage.used}</b>{#if storage.limit}
			of {storage.limit}{/if}
	</p>
	{#if storage.limit}
		<UsageBar percent={storage.percent ?? 0} label="Database size against MAX_DATABASE_SIZE" />
	{/if}
	<p class="text-xs text-muted">
		{#if storage.limit}
			At the limit the least useful data is deleted until the database is at {storage.cleanup_stop_percent}%:
			old logs first, errors last.
		{:else}
			Nothing is deleted: the limit is off.
		{/if}
		<code>MAX_DATABASE_SIZE</code>
	</p>
	<p><Button variant="link" href={oldUI.cleanups}>Cleanup history</Button></p>
</Card>

<p class="mb-4 text-muted">
	Each service (backend, frontend, …) gets its own project. Put a project's DSN into the Sentry SDK
	config of that service.
</p>

<Table {columns} rows={data.projects} rowKey={(project: Project) => project.id} label="Projects">
	{#snippet cell(project: Project, column: ProjectColumn)}
		{#if column.key === 'project'}
			<Badge
				tone="project"
				label={project.name}
				href={withQuery(oldUI.issues, { project: project.name })}
				title="Issues of {project.name}"
			/>
			<div class="text-xs text-muted">
				#{project.id}{project.from_env ? ' · from PROJECTS' : ''}
			</div>
		{:else if column.key === 'dsn'}
			<CopyField
				value={project.dsn ?? ''}
				label="DSN of {project.name}"
				copyTitle="Copy the DSN of {project.name}"
				copied="DSN copied to clipboard."
			/>
		{:else if column.key === 'unresolved'}
			<Button variant="link" href={withQuery(oldUI.issues, { project: project.name })}>
				{project.unresolved_issues}
			</Button>
		{:else if column.key === 'records'}
			{@const events = recorded(project.event_levels, levels.event)}
			{@const logs = recorded(project.log_levels, levels.log)}
			<div class="flex flex-wrap gap-1 text-xs">
				{#if events === 'all'}all errors{:else if events === 'none'}<span class="text-muted">
						no errors
					</span>{:else}{#each events as level (level)}<LevelBadge
							{level}
							href={withQuery(oldUI.issues, { level })}
						/>{/each}{/if}
			</div>
			<div class="text-xs text-muted">logs: {typeof logs === 'string' ? logs : logs.join(' ')}</div>
		{:else}
			<div class="flex gap-1.5">
				{#if canAdminister}
					<Button
						icon="settings"
						href={oldUI.project(project.id)}
						title="Settings of {project.name}"
					/>
				{/if}
				<Button
					icon="logs"
					href={withQuery(oldUI.logs, { project: project.name })}
					title="Logs of {project.name}"
				/>
				{#if canAdminister && !project.from_env}
					<Button
						variant="danger"
						icon="delete"
						title="Delete {project.name}"
						onclick={() => remove(project)}
					/>
				{/if}
			</div>
		{/if}
	{/snippet}
	{#snippet empty()}
		No projects yet. Add one, or set <code>PROJECTS=1:backend:&lt;public key&gt;</code>.
	{/snippet}
</Table>
