<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { post, type Item } from '#lib/api/client.ts';
	import Button from '#lib/components/Button.svelte';
	import { unavailable, type MenuItem } from '#lib/components/menu.ts';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import ProjectsSummary from '#lib/components/ProjectsSummary.svelte';
	import RoleBadge from '#lib/components/RoleBadge.svelte';
	import RowMenu from '#lib/components/RowMenu.svelte';
	import Table from '#lib/components/Table.svelte';
	import type { TableColumn } from '#lib/components/table.ts';
	import Time from '#lib/components/Time.svelte';
	import { handOver } from '#lib/handoff.ts';
	import type { PageProps } from './$types';
	import {
		deleteConfirmation,
		deleteUser,
		refreshingOnRefusal,
		resetConfirmation,
		takeOverLock
	} from './users.ts';

	let { data }: PageProps = $props();

	type Row = Item<'list_users'>;
	type UserColumn = TableColumn<
		'user' | 'role' | 'projects' | 'login' | 'active' | 'added' | 'actions'
	>;

	const columns: UserColumn[] = [
		{ key: 'user', label: 'User' },
		{ key: 'role', label: 'Role' },
		{ key: 'projects', label: 'Projects' },
		{ key: 'login', label: 'Last login' },
		{ key: 'active', label: 'Last active', wideOnly: true },
		{ key: 'added', label: 'Added', wideOnly: true },
		{ key: 'actions', label: 'Actions', actions: true }
	];

	const viewer = $derived(data.session.user.id);

	function pageOf(id: string) {
		return resolve('/(app)/users/[id]', { id });
	}

	// The new password is shown once, on the user's page.
	async function reset(id: string) {
		const { temporary_password } = await refreshingOnRefusal(() =>
			post('reset_user_password', { user_id: id })
		);

		handOver(id, temporary_password);
		await goto(pageOf(id));
	}

	// The viewer's own row: name and password change on the account page.
	const ownMenu: MenuItem[] = [{ label: 'Edit', icon: 'edit', href: resolve('/(app)/account') }];

	// The actions of another user's row; each one the viewer may not take says why. Edit opens
	// the user's page while their projects may change, though the rest may not.
	function menuOf(row: Row) {
		const { user, can_take_over, access_editable } = row;
		const locked = takeOverLock(row);

		return [
			unavailable(
				{ label: 'Edit', icon: 'edit', href: pageOf(user.id) },
				can_take_over || access_editable ? undefined : locked
			),
			unavailable(
				{
					label: 'Reset password',
					icon: 'token',
					confirm: resetConfirmation(user.email),
					action: () => reset(user.id)
				},
				locked
			),
			unavailable(
				{
					label: 'Delete',
					icon: 'delete',
					danger: true,
					separatorBefore: true,
					confirm: deleteConfirmation(user.email),
					action: async () => {
						await refreshingOnRefusal(() => deleteUser(user.id));
						void invalidateAll();
					}
				},
				locked
			)
		];
	}
</script>

<PageHeading title="Users">
	{#snippet actions()}
		<Button variant="primary" icon="add" href={resolve('/(app)/users/new')}>Add user</Button>
	{/snippet}
	{#snippet description()}
		Admins manage everything. Users work in their projects: issues, logs, notifications, settings
		and the users of those projects.
	{/snippet}
</PageHeading>

<Table {columns} rows={data.users} rowKey={(row: Row) => row.user.id} label="Users">
	{#snippet cell(row: Row, column: UserColumn)}
		{@const user = row.user}
		{#if column.key === 'user'}
			{#if user.id === viewer}
				<Button variant="title" href={resolve('/(app)/account')}>{user.display_name}</Button>
				<span class="text-xs text-muted">(you)</span>
			{:else}
				<Button variant="title" href={pageOf(user.id)}>{user.display_name}</Button>
			{/if}
			{#if user.email !== user.display_name}
				<div class="text-xs text-muted">{user.email}</div>
			{/if}
		{:else if column.key === 'role'}
			<RoleBadge role={user.role} />
			{#if user.must_change_password}
				<span class="text-xs text-muted">temporary password</span>
			{/if}
		{:else if column.key === 'projects'}
			<!-- The viewer's part of the user's projects: others they do not see are left out. -->
			<ProjectsSummary
				all={user.all_projects}
				names={user.projects ?? []}
				links={(user.project_ids ?? []).map((id) =>
					resolve('/(app)/projects/[id=projectId]', { id })
				)}
			/>
		{:else if column.key === 'active'}
			{#if user.last_active_at}<Time at={user.last_active_at} />{:else}<span class="text-muted">
					never
				</span>{/if}
		{:else if column.key === 'login'}
			{#if user.last_login_at}<Time at={user.last_login_at} />{:else}<span class="text-muted">
					never
				</span>{/if}
		{:else if column.key === 'added'}
			<Time at={user.created_at} />
		{:else}
			<RowMenu
				label="Actions for {user.display_name}"
				items={user.id === viewer ? ownMenu : menuOf(row)}
			/>
		{/if}
	{/snippet}
</Table>
