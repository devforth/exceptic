<!--
A user as an administrator sees them: name and role to change, the password to reset, the
user to delete; read-only when the viewer may not change them (the superadmin for an admin).
A temporary password made here or on the way here is shown once.
-->
<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { post } from '#lib/api/client.ts';
	import Breadcrumbs from '#lib/components/Breadcrumbs.svelte';
	import ConfirmButton from '#lib/components/ConfirmButton.svelte';
	import Facts from '#lib/components/Facts.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import RoleBadge from '#lib/components/RoleBadge.svelte';
	import SecretReveal from '#lib/components/SecretReveal.svelte';
	import { takeOver } from '#lib/handoff.ts';
	import { toast } from '#lib/toast.svelte.ts';
	import UserForm from '../UserForm.svelte';
	import { deleteConfirmation, deleteUser, resetConfirmation, type UserInput } from '../users.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const user = $derived(data.managed.user);

	// The temporary password, in this component only: shown for its own user, gone on leaving.
	let temporary: { owner: string; secret: string } | undefined = $state(handedOver());
	const secret = $derived(temporary?.owner === user.id ? temporary.secret : undefined);

	function handedOver() {
		const owner = page.params.id ?? '';
		const handed = takeOver(owner);

		return handed ? { owner, secret: handed } : undefined;
	}

	async function save(input: UserInput) {
		await post('update_user', { user_id: user.id, name: input.name, role: input.role });
		toast.show('Saved.');
		await invalidateAll();
	}

	async function reset() {
		const { temporary_password } = await post('reset_user_password', { user_id: user.id });

		temporary = { owner: user.id, secret: temporary_password };
		void invalidateAll();
	}

	async function remove() {
		await deleteUser(user.id);
		await goto(resolve('/(app)/users'));
	}

	type Fact = { label: string };

	const facts: Fact[] = [{ label: 'Username' }, { label: 'Role' }];
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
			<ConfirmButton confirm={deleteConfirmation(user.username)} action={remove}>
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

{#if data.managed.can_manage}
	<UserForm
		user={{ username: user.username, name: user.display_name, role: user.role }}
		submitLabel="Save"
		{save}
	/>
	<div class="mt-6">
		<ConfirmButton variant="default" confirm={resetConfirmation(user.username)} action={reset}>
			Reset password
		</ConfirmButton>
	</div>
{:else}
	<Facts {facts}>
		{#snippet value(fact: Fact)}
			{#if fact.label === 'Username'}{user.username}{:else}<RoleBadge role={user.role} />{/if}
		{/snippet}
	</Facts>
	<p class="text-muted">Your role cannot change this user.</p>
{/if}
