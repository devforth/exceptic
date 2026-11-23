<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { post, type Item } from '#lib/api/client.ts';
	import Button from '#lib/components/Button.svelte';
	import type { MenuItem } from '#lib/components/menu.ts';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import RoleBadge from '#lib/components/RoleBadge.svelte';
	import RowMenu from '#lib/components/RowMenu.svelte';
	import Table from '#lib/components/Table.svelte';
	import type { TableColumn } from '#lib/components/table.ts';
	import Time from '#lib/components/Time.svelte';
	import { handOver } from '#lib/handoff.ts';
	import type { PageProps } from './$types';
	import { deleteConfirmation, deleteUser, resetConfirmation } from './users.ts';

	let { data }: PageProps = $props();

	type Row = Item<'list_users'>;
	type UserColumn = TableColumn<'user' | 'role' | 'login' | 'added' | 'actions'>;

	const columns: UserColumn[] = [
		{ key: 'user', label: 'User' },
		{ key: 'role', label: 'Role' },
		{ key: 'login', label: 'Last login' },
		{ key: 'added', label: 'Added', wideOnly: true },
		{ key: 'actions', label: 'Actions', actions: true }
	];

	const viewer = $derived(data.session.user.id);

	function pageOf(id: string) {
		return resolve('/(app)/users/[id]', { id });
	}

	// The new password is shown once, on the user's page.
	async function reset(id: string) {
		const { temporary_password } = await post('reset_user_password', { user_id: id });

		handOver(id, temporary_password);
		await goto(pageOf(id));
	}

	function menuOf(id: string, username: string): MenuItem[] {
		return [
			{ label: 'Edit', icon: 'edit', href: pageOf(id) },
			{
				label: 'Reset password',
				icon: 'token',
				confirm: resetConfirmation(username),
				action: () => reset(id)
			},
			{
				label: 'Delete',
				icon: 'delete',
				danger: true,
				separatorBefore: true,
				confirm: deleteConfirmation(username),
				action: async () => {
					await deleteUser(id);
					void invalidateAll();
				}
			}
		];
	}
</script>

<PageHeading title="Users">
	{#snippet actions()}
		<Button variant="primary" icon="add" href={resolve('/(app)/users/new')}>Add user</Button>
	{/snippet}
</PageHeading>

<p class="mb-4 text-muted">
	Users change issue statuses, read logs and connect their own coding agents. Admins also manage
	projects, notifications and users. There is one superadmin.
</p>

<Table {columns} rows={data.users} rowKey={(row: Row) => row.user.id} label="Users">
	{#snippet cell({ user, can_manage }: Row, column: UserColumn)}
		{#if column.key === 'user'}
			{#if user.id === viewer}
				<Button variant="title" href={resolve('/(app)/account')}>{user.display_name}</Button>
				<span class="text-xs text-muted">(you)</span>
			{:else}
				<Button variant="title" href={pageOf(user.id)}>{user.display_name}</Button>
			{/if}
			{#if user.username !== user.display_name}
				<div class="text-xs text-muted">{user.username}</div>
			{/if}
		{:else if column.key === 'role'}
			<RoleBadge role={user.role} />
			{#if user.must_change_password}
				<span class="text-xs text-muted">temporary password</span>
			{/if}
		{:else if column.key === 'login'}
			{#if user.last_login_at}<Time at={user.last_login_at} />{:else}<span class="text-muted">
					never
				</span>{/if}
		{:else if column.key === 'added'}
			<Time at={user.created_at} />
		{:else if can_manage}
			<RowMenu label="Actions for {user.display_name}" items={menuOf(user.id, user.username)} />
		{/if}
	{/snippet}
</Table>
