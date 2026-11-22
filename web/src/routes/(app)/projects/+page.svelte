<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { help, levels } from '#lib/api/catalog.gen.ts';
	import { post, type Item } from '#lib/api/client.ts';
	import Badge from '#lib/components/Badge.svelte';
	import Button from '#lib/components/Button.svelte';
	import Card from '#lib/components/Card.svelte';
	import FormDialog from '#lib/components/FormDialog.svelte';
	import Hint from '#lib/components/Hint.svelte';
	import LevelBadge from '#lib/components/LevelBadge.svelte';
	import type { MenuItem } from '#lib/components/menu.ts';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import RowMenu from '#lib/components/RowMenu.svelte';
	import Table from '#lib/components/Table.svelte';
	import type { TableColumn } from '#lib/components/table.ts';
	import Time from '#lib/components/Time.svelte';
	import UsageBar from '#lib/components/UsageBar.svelte';
	import { withQuery } from '#lib/nav.ts';
	import { deleteConfirmation, deleteProject, usageSegments } from '#lib/projects.ts';
	import { FormState } from '#lib/form.svelte.ts';
	import { toast } from '#lib/toast.svelte.ts';
	import ColorField from './ColorField.svelte';
	import ProjectNameField from './ProjectNameField.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	type Project = Item<'list_projects'>;
	type ProjectColumn = TableColumn<
		'project' | 'unresolved' | 'records' | 'size' | 'received' | 'actions'
	>;

	const columns: ProjectColumn[] = [
		{ key: 'project', label: 'Project' },
		{ key: 'unresolved', label: 'Unresolved', number: true },
		{ key: 'records', label: 'Records', wideOnly: true },
		{ key: 'size', label: 'Size', number: true },
		{ key: 'received', label: 'Last received', wideOnly: true },
		{ key: 'actions', label: 'Actions', actions: true }
	];

	const canAdminister = $derived(data.session.can_administer);
	const storage = $derived(data.storage);
	const usage = $derived(new Map((storage.projects ?? []).map((part) => [part.id, part])));
	const colors = $derived(data.session.project_colors ?? {});
	const segments = $derived(usageSegments(storage, (name) => colors[name]));

	// What a project records: every level, none, or the ones listed.
	function recorded(own: string[] | null, known: readonly string[]) {
		const list = own ?? [];

		return list.length === known.length ? 'all' : list.length === 0 ? 'none' : list;
	}

	async function remove(project: Project) {
		await deleteProject(project.id);
		// The delete is done: a failed reload of the list is not a failed delete.
		void invalidateAll();
	}

	// Rename and Change color: a small dialog each; the server checks the value (a refused one
	// stays in the dialog with its problem).
	let editing: Project | undefined = $state();
	let renameOpen = $state(false);
	let colorOpen = $state(false);
	let newName = $state('');
	let newColor = $state('');
	const editForm = new FormState();

	function startEdit(project: Project, dialog: 'name' | 'color') {
		editing = project;
		editForm.error = '';
		editForm.fields = {};

		if (dialog === 'name') {
			newName = project.name;
			renameOpen = true;
		} else {
			newColor = colors[project.name] ?? '';
			colorOpen = true;
		}
	}

	async function saveEdit(change: { name: string } | { color: string }, done: string) {
		if (!editing) return;

		await post('update_project', { project: String(editing.id), ...change });
		toast.show(done);
		// Saved: a failed reload of the list is not a failed save.
		void invalidateAll();
	}

	// Projects from PROJECTS keep their name and cannot be deleted; their color can change.
	function menuOf(project: Project): MenuItem[] {
		const id = project.id;
		const editable = canAdminister && !project.from_env;
		const items: MenuItem[] = [];

		if (canAdminister) {
			items.push(
				project.from_env
					? { label: 'Rename', icon: 'edit', disabled: 'Its name is set in PROJECTS.' }
					: { label: 'Rename', icon: 'edit', action: () => startEdit(project, 'name') },
				{ label: 'Change color', icon: 'palette', action: () => startEdit(project, 'color') }
			);
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
		}

		return items;
	}
</script>

<PageHeading title="Projects">
	{#snippet actions()}
		{#if canAdminister}
			<Button variant="primary" icon="add" href={resolve('/(app)/projects/new')}>
				Add project
			</Button>
		{/if}
	{/snippet}
</PageHeading>

<Card title="Storage">
	<p>
		Database: <b>{storage.used}</b>{#if storage.limit}
			of {storage.limit} <span class="text-muted">({storage.percent ?? 0}%)</span>{/if}
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
						<Badge tone="project" label={part.name} />{part.size}
					</span>
				{/if}
			{/each}
			<span class="text-muted">other {storage.other}</span>
		</p>
	{:else if storage.limit}
		<UsageBar percent={storage.percent ?? 0} label="Database size against MAX_DATABASE_SIZE" />
	{/if}
	<p class="text-xs text-muted">
		{#if storage.counting}
			Counting what each project holds…
		{/if}
		{#if storage.limit}
			At the limit the least useful data is deleted until the database is at {storage.cleanup_stop_percent}%:
			old logs first, errors last.
		{:else}
			Nothing is deleted: the limit is off.
		{/if}
		<code>MAX_DATABASE_SIZE</code>
		<Hint text={help.settings.MAX_DATABASE_SIZE} />
	</p>
	<p><Button variant="link" href={resolve('/(app)/projects/cleanups')}>Cleanup history</Button></p>
</Card>

<p class="mb-4 text-muted">
	Each service (backend, frontend, …) gets its own project. Its installation code connects the
	service's Sentry SDK to it.
</p>

<Table {columns} rows={data.projects} rowKey={(project: Project) => project.id} label="Projects">
	{#snippet cell(project: Project, column: ProjectColumn)}
		{@const part = usage.get(project.id)}
		{#if column.key === 'project'}
			<Badge
				tone="project"
				label={project.name}
				href={withQuery(resolve('/(app)/issues'), { project: project.name })}
				title="Issues of {project.name}"
			/>
			<div class="text-xs text-muted">
				#{project.id}{project.from_env ? ' · from PROJECTS' : ''}
			</div>
		{:else if column.key === 'unresolved'}
			<Button variant="link" href={withQuery(resolve('/(app)/issues'), { project: project.name })}>
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
							href={withQuery(resolve('/(app)/issues'), { level })}
						/>{/each}{/if}
			</div>
			<div class="text-xs text-muted">logs: {typeof logs === 'string' ? logs : logs.join(' ')}</div>
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

<FormDialog
	title="Rename project"
	bind:open={renameOpen}
	form={editForm}
	save={() => saveEdit({ name: newName.trim() }, 'Project renamed.')}
>
	<ProjectNameField bind:value={newName} error={editForm.fields.name} />
</FormDialog>

<FormDialog
	title="Change color"
	bind:open={colorOpen}
	form={editForm}
	save={() => saveEdit({ color: newColor }, 'Color changed.')}
>
	<ColorField bind:value={newColor} name={editing?.name ?? ''} error={editForm.fields.color} />
</FormDialog>
