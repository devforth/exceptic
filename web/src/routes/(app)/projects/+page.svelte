<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { help } from '#lib/api/catalog.gen.ts';
	import { post, type Item } from '#lib/api/client.ts';
	import Badge from '#lib/components/Badge.svelte';
	import Button from '#lib/components/Button.svelte';
	import Card from '#lib/components/Card.svelte';
	import Dialog from '#lib/components/Dialog.svelte';
	import FormDialog from '#lib/components/FormDialog.svelte';
	import Hint from '#lib/components/Hint.svelte';
	import LevelSummary from '#lib/components/LevelSummary.svelte';
	import type { MenuItem } from '#lib/components/menu.ts';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import RowMenu from '#lib/components/RowMenu.svelte';
	import Table from '#lib/components/Table.svelte';
	import type { TableColumn } from '#lib/components/table.ts';
	import Time from '#lib/components/Time.svelte';
	import UsageBar from '#lib/components/UsageBar.svelte';
	import { withQuery } from '#lib/nav.ts';
	import {
		deleteConfirmation,
		deleteProject,
		droppedSummary,
		fromEnvDeleteReason,
		errorsKept,
		usageSegments
	} from '#lib/projects.ts';
	import { FormState } from '#lib/form.svelte.ts';
	import { sharePercent } from '#lib/format.ts';
	import { toast } from '#lib/toast.svelte.ts';
	import ColorField from '#lib/components/ColorField.svelte';
	import ProjectNameField from './ProjectNameField.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	type Project = Item<'list_projects'>;
	type ProjectColumn = TableColumn<
		'project' | 'unresolved' | 'logs' | 'size' | 'received' | 'actions'
	>;

	const columns: ProjectColumn[] = [
		{ key: 'project', label: 'Project' },
		{ key: 'unresolved', label: 'Unresolved', number: true },
		{ key: 'logs', label: 'Logs recorded', wideOnly: true },
		{ key: 'size', label: 'Size', number: true },
		{ key: 'received', label: 'Last received', wideOnly: true },
		{ key: 'actions', label: 'Actions', actions: true }
	];

	const canAdminister = $derived(data.session.can_administer);
	const storage = $derived(data.storage);
	const usage = $derived(new Map((storage.projects ?? []).map((part) => [part.id, part])));
	const colors = $derived(data.session.project_colors ?? {});
	const segments = $derived(usageSegments(storage, (name) => colors[name]));

	async function remove(project: Project) {
		await deleteProject(project.id);
		// The delete is done: a failed reload of the list is not a failed delete.
		void invalidateAll();
	}

	// Rename and recolor in one dialog; the server checks both (a refused value stays in the
	// dialog with its problem).
	type DroppedRow = { kind: string; count: number; reason: string; reason_text: string };
	type DroppedColumn = TableColumn<'what' | 'why' | 'count'>;

	// The project whose dropped data the dialog shows.
	let droppedOf: { name: string; rows: DroppedRow[] } | undefined = $state();
	let droppedOpen = $state(false);

	const droppedColumns: DroppedColumn[] = [
		{ key: 'what', label: 'What' },
		{ key: 'why', label: 'Why' },
		{ key: 'count', label: 'Count', number: true }
	];

	let editing: Project | undefined = $state();
	let editOpen = $state(false);
	let newName = $state('');
	let newColor = $state('');
	const editForm = new FormState(() => ({ name: newName.trim(), color: newColor }));

	function startEdit(project: Project) {
		editing = project;
		newName = project.name;
		newColor = colors[project.name] ?? '';
		editForm.reset();
		editOpen = true;
	}

	async function saveEdit() {
		if (!editing) return;

		await post('update_project', {
			project: String(editing.id),
			name: newName.trim(),
			color: newColor
		});
		toast.show('Project saved.');
		// Saved: a failed reload of the list is not a failed save.
		void invalidateAll();
	}

	// Projects from PROJECTS cannot be deleted here (they would come back at the next start).
	function menuOf(project: Project): MenuItem[] {
		const id = project.id;
		const editable = canAdminister && !project.from_env;
		const items: MenuItem[] = [];

		if (canAdminister) {
			items.push({ label: 'Rename', icon: 'edit', action: () => startEdit(project) });
		}

		items.push({
			label: 'Show installation code',
			icon: 'code',
			href: resolve('/(app)/projects/[id=projectId]/setup', { id })
		});

		if (canAdminister) {
			items.push({
				label: 'Settings',
				icon: 'settings',
				href: resolve('/(app)/projects/[id=projectId]', { id }),
				separatorBefore: true
			});
		}

		items.push({
			label: 'Logs',
			icon: 'logs',
			href: withQuery(resolve('/(app)/logs'), { project: project.name }),
			separatorBefore: !canAdminister
		});

		if (editable) {
			items.push({
				label: 'Delete',
				icon: 'delete',
				danger: true,
				separatorBefore: true,
				confirm: deleteConfirmation(project.name),
				action: () => remove(project)
			});
		} else if (canAdminister) {
			items.push({
				label: 'Delete',
				icon: 'delete',
				separatorBefore: true,
				disabled: fromEnvDeleteReason
			});
		}

		return items;
	}
</script>

<PageHeading title="Projects">
	{#snippet actions()}
		<Button icon="history" href={resolve('/(app)/projects/cleanups')}>Cleanup history</Button>
		{#if canAdminister}
			<Button variant="primary" icon="add" href={resolve('/(app)/projects/new')}>
				Add project
			</Button>
		{/if}
	{/snippet}
</PageHeading>

<Card title="Storage">
	<p>
		Database: <b>{storage.used}</b>{#if storage.limit}{` of ${storage.limit}`}
			<span class="text-muted">
				({sharePercent(storage.used_bytes, storage.limit_bytes ?? 0)})
			</span>{/if}
	</p>
	{#if segments.length}
		<UsageBar
			{segments}
			label={`${segments.map((segment) => segment.label).join(', ')}${storage.limit ? ` of ${storage.limit}` : ''}`}
		/>
		<p class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
			{#each storage.projects ?? [] as part (part.id)}
				{#if part.bytes > 0}
					<span class="flex items-center gap-1">
						<Badge
							tone="project"
							label={part.name}
							href={withQuery(resolve('/(app)/issues'), { project: part.name })}
							title="Issues of {part.name}"
						/>{[part.size, errorsKept(part.oldest)].filter(Boolean).join(' · ')}
					</span>
				{/if}
			{/each}
			<span class="text-muted">other {storage.other}</span>
		</p>
	{:else if storage.limit}
		<UsageBar percent={storage.percent ?? 0} label="Database size against MAX_DATABASE_SIZE" />
	{/if}
	<p class="text-xs text-muted">
		Files on disk: {storage.files}, with deleted data until it is compacted and envelopes waiting to
		be stored.
	</p>
	<p class="text-xs text-muted">
		{#if storage.counting}
			Counting what each project holds…
		{/if}
		{#if storage.limit}
			At {storage.cleanup_start_percent}% of the limit the least useful data is deleted until the
			database holds {storage.cleanup_stop_percent}%: old logs first, errors last.
		{:else}
			Nothing is deleted: the limit is off.
		{/if}
		<code>MAX_DATABASE_SIZE</code>
		<Hint text={help.settings.MAX_DATABASE_SIZE} />
	</p>
</Card>

<Table {columns} rows={data.projects} rowKey={(project: Project) => project.id} label="Projects">
	{#snippet cell(project: Project, column: ProjectColumn)}
		{@const part = usage.get(project.id)}
		{@const dropped = droppedSummary(part?.dropped)}
		{#if column.key === 'project'}
			<!-- Admins open its settings; the others, who cannot, its issues. -->
			<Badge
				tone="project"
				label={project.name}
				href={canAdminister
					? resolve('/(app)/projects/[id=projectId]', { id: project.id })
					: withQuery(resolve('/(app)/issues'), { project: project.name })}
				title={canAdminister ? `Settings of ${project.name}` : `Issues of ${project.name}`}
			/>
			<div class="text-xs text-muted">
				#{project.id}{project.from_env ? ' · from PROJECTS' : ''}
			</div>
			{#if dropped}
				<div class="text-xs">
					<Button
						variant="link"
						popup="dialog"
						onclick={() => {
							droppedOf = { name: project.name, rows: part?.dropped ?? [] };
							droppedOpen = true;
						}}
					>
						{dropped}
					</Button>
				</div>
			{/if}
		{:else if column.key === 'unresolved'}
			<Button variant="link" href={withQuery(resolve('/(app)/issues'), { project: project.name })}>
				{project.unresolved_issues}
			</Button>
		{:else if column.key === 'logs'}
			<span class="text-xs"><LevelSummary kind="log" chosen={project.log_levels ?? []} /></span>
		{:else if column.key === 'size'}
			{#if storage.counting}
				<span class="text-xs text-muted">counting…</span>
			{:else}
				{part?.size ?? '—'}
			{/if}
		{:else if column.key === 'received'}
			{#if part?.last_received}
				<Time at={part.last_received} />
			{:else}
				<span class="text-xs text-muted">nothing yet</span>
			{/if}
		{:else}
			<RowMenu label="Actions for {project.name}" items={menuOf(project)} />
		{/if}
	{/snippet}
	{#snippet empty()}
		No projects yet. Add one, or set <code>PROJECTS=1:backend:&lt;public key&gt;</code>.
	{/snippet}
</Table>

<FormDialog title="Edit project" bind:open={editOpen} form={editForm} save={saveEdit}>
	<ProjectNameField bind:value={newName} error={editForm.fields.name} />
	<ColorField bind:value={newColor} name={newName.trim()} error={editForm.fields.color} />
</FormDialog>

<Dialog title="Dropped by {droppedOf?.name ?? ''} in the last 24 hours" bind:open={droppedOpen}>
	<p class="mb-3 text-sm text-muted">{help.fields.Projects.Dropped}</p>
	<Table
		columns={droppedColumns}
		rows={droppedOf?.rows ?? []}
		rowKey={(row: DroppedRow) => `${row.kind}/${row.reason}`}
		label="Dropped data"
		compact
	>
		{#snippet cell(row: DroppedRow, column: DroppedColumn)}
			{#if column.key === 'what'}
				{row.kind}
			{:else if column.key === 'why'}
				{row.reason_text}
			{:else}
				{row.count.toLocaleString('en-US')}
			{/if}
		{/snippet}
	</Table>
</Dialog>
