<!--
A user as an administrator sees them: name and role to change, the password to reset, the
user to delete; for a user with projects the viewer lacks only their projects, and read-only when
the viewer may change nothing (an admin, for a user).
A temporary password made here or on the way here is shown once.
-->
<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { post } from '#lib/api/client.ts';
	import ActionList from '#lib/components/ActionList.svelte';
	import ActionRow from '#lib/components/ActionRow.svelte';
	import Breadcrumbs from '#lib/components/Breadcrumbs.svelte';
	import ConfirmDialog from '#lib/components/ConfirmDialog.svelte';
	import ConfirmButton from '#lib/components/ConfirmButton.svelte';
	import Dialog from '#lib/components/Dialog.svelte';
	import Facts from '#lib/components/Facts.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import RoleBadge from '#lib/components/RoleBadge.svelte';
	import SecretReveal from '#lib/components/SecretReveal.svelte';
	import SignInActivity from '#lib/components/SignInActivity.svelte';
	import { takeOver } from '#lib/handoff.ts';
	import { toast } from '#lib/toast.svelte.ts';
	import UserForm from '../UserForm.svelte';
	import {
		deleteConfirmation,
		deleteUser,
		refreshingOnRefusal,
		resetConfirmation,
		takeOverLock,
		type UserInput
	} from '../users.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const user = $derived(data.managed.user);
	// Why the viewer may not rename, delete or reset the user, or change their role.
	const takeOverLocked = $derived(takeOverLock(data.managed));
	let activityOpen = $state(false);
	let resetOpen = $state(false);
	const editable = $derived(data.managed.can_take_over || data.managed.access_editable);

	// The temporary password, in this component only: shown for its own user, gone on leaving.
	let temporary: { owner: string; secret: string } | undefined = $state(handedOver());
	const secret = $derived(temporary?.owner === user.id ? temporary.secret : undefined);

	function handedOver() {
		const owner = page.params.id ?? '';
		const handed = takeOver(owner);

		return handed ? { owner, secret: handed } : undefined;
	}

	async function save({ email, name, role, access }: UserInput) {
		await refreshingOnRefusal(() =>
			post('update_user', { user_id: user.id, email, name, role, ...access })
		);
		toast.show('Saved.');
		await invalidateAll();
	}

	async function reset() {
		const { temporary_password } = await refreshingOnRefusal(() =>
			post('reset_user_password', { user_id: user.id })
		);

		temporary = { owner: user.id, secret: temporary_password };
		void invalidateAll();
	}

	async function remove() {
		await refreshingOnRefusal(() => deleteUser(user.id));
		await goto(resolve('/(app)/users'));
	}

	type Fact = { label: string };

	const facts: Fact[] = [{ label: 'Email' }, { label: 'Role' }];
</script>

<Breadcrumbs
	items={[
		{ label: 'Users', href: resolve('/(app)/users') },
		{ label: user.display_name, href: resolve('/(app)/users/[id]', { id: user.id }) }
	]}
/>
<PageHeading title={user.display_name}>
	{#snippet actions()}
		{#if data.managed.can_manage}
			<ConfirmButton
				confirm={deleteConfirmation(user.email)}
				action={remove}
				unavailable={takeOverLocked}
			>
				Delete
			</ConfirmButton>
		{/if}
	{/snippet}
</PageHeading>

{#if secret}
	<div class="mb-4 max-w-xl">
		<SecretReveal
			value={secret}
			label="Temporary password"
			copyTitle="Copy the temporary password"
			copied="Temporary password copied to clipboard."
		>
			<b>Temporary password</b>: give it to the user. It is shown only now; they must choose their
			own password when they log in.
		</SecretReveal>
	</div>
{/if}

{#if editable}
	<UserForm
		projects={data.projects}
		viewerAll={data.session.all_projects}
		roles={data.session.assignable_roles ?? []}
		user={{
			email: user.email,
			name: user.display_name,
			role: user.role,
			allProjects: user.all_projects,
			projectIds: user.project_ids ?? [],
			accessEditable: data.managed.access_editable,
			accessReason: data.managed.access_reason,
			takeOverLocked
		}}
		submitLabel="Save"
		{save}
	/>
	<div class="mt-6">
		<ActionList title="Security">
			<ActionRow
				icon="password"
				label="Reset password"
				description="A temporary password, shown once; they are logged out"
				unavailable={takeOverLocked}
				onclick={() => (resetOpen = true)}
			/>
			<ActionRow
				icon="sessions"
				label="Sessions and sign-ins"
				description="Where they are signed in, every attempt and Google"
				onclick={() => (activityOpen = true)}
			/>
		</ActionList>
	</div>
	<ConfirmDialog bind:open={resetOpen} {...resetConfirmation(user.email)} onconfirm={reset} />
	<Dialog title="Sessions and sign-ins of {user.display_name}" bind:open={activityOpen} size="wide">
		{#if activityOpen}
			<SignInActivity userId={user.id} changeable={!takeOverLocked} reason={takeOverLocked} />
		{/if}
	</Dialog>
{:else}
	<Facts {facts}>
		{#snippet value(fact: Fact)}
			{#if fact.label === 'Email'}{user.email}{:else}<RoleBadge role={user.role} />{/if}
		{/snippet}
	</Facts>
	<p class="text-muted">{takeOverLocked}</p>
{/if}
