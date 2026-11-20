<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { levels } from '#lib/api/catalog.gen.ts';
	import { ApiError, post, type Item } from '#lib/api/client.ts';
	import Badge from '#lib/components/Badge.svelte';
	import Button from '#lib/components/Button.svelte';
	import Card from '#lib/components/Card.svelte';
	import CopyField from '#lib/components/CopyField.svelte';
	import Dialog from '#lib/components/Dialog.svelte';
	import LevelBadge from '#lib/components/LevelBadge.svelte';
	import type { MenuItem } from '#lib/components/menu.ts';
	import Notice from '#lib/components/Notice.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import RowMenu from '#lib/components/RowMenu.svelte';
	import Table from '#lib/components/Table.svelte';
	import type { TableColumn } from '#lib/components/table.ts';
	import TextField from '#lib/components/TextField.svelte';
	import UsageBar from '#lib/components/UsageBar.svelte';
	import { comingPages, oldUI, withQuery } from '#lib/nav.ts';
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
		await post('delete_project', { project: String(project.id) });
		toast.show(deleted);
		await invalidateAll();
	}

	// Rename: a dialog with the name; the server checks it (a refused name stays in the dialog).
	let renaming: Project | undefined = $state();
	let renameOpen = $state(false);
	let newName = $state('');
	let renameError = $state('');
	let renameBusy = $state(false);
	let renameForm: HTMLFormElement | undefined = $state();

	function startRename(project: Project) {
		renaming = project;
		newName = project.name;
		renameError = '';
		renameOpen = true;
	}

	async function rename(event: SubmitEvent) {
		event.preventDefault();

		if (!renaming || renameBusy) return;

		renameBusy = true;
		renameError = '';

		try {
			await post('update_project', { project: String(renaming.id), name: newName.trim() });
			renameOpen = false;
			toast.show('Project renamed.');
			await invalidateAll();
		} catch (failure) {
			renameError = failure instanceof ApiError ? failure.message : 'Could not rename the project.';
		} finally {
			renameBusy = false;
		}
	}

	// Projects from PROJECTS keep their name and cannot be deleted.
	function menuOf(project: Project): MenuItem[] {
		const editable = canAdminister && !project.from_env;
		const items: MenuItem[] = [];

		if (editable) {
			items.push({ label: 'Rename', icon: 'edit', action: () => startRename(project) });
		}

		items.push({
			label: 'Show installation code',
			icon: 'code',
			href: comingPages.projectSetup(project.id)
		});

		if (canAdminister) {
			items.push({
				label: 'Settings',
				icon: 'settings',
				href: oldUI.project(project.id),
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
				confirm: {
					title: 'Delete project',
					message: `Delete the project ${project.name} with all its issues, events and logs? Its DSN stops working. This cannot be undone.`,
					confirmLabel: 'Delete'
				},
				action: () => remove(project)
			});
		}

		return items;
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
			<RowMenu label="Actions for {project.name}" items={menuOf(project)} />
		{/if}
	{/snippet}
	{#snippet empty()}
		No projects yet. Add one, or set <code>PROJECTS=1:backend:&lt;public key&gt;</code>.
	{/snippet}
</Table>

<Dialog
	title="Rename project"
	bind:open={renameOpen}
	size="narrow"
	dismissible={!renameBusy}
	onopen={() => renameForm?.querySelector('input')?.select()}
>
	<form id="rename-project" bind:this={renameForm} onsubmit={rename}>
		<TextField
			label="Name"
			name="name"
			bind:value={newName}
			required
			autocomplete="off"
			error={renameError}
		/>
	</form>
	{#snippet actions()}
		<Button disabled={renameBusy} onclick={() => (renameOpen = false)}>Cancel</Button>
		<Button variant="primary" type="submit" form="rename-project" busy={renameBusy}>Save</Button>
	{/snippet}
</Dialog>
