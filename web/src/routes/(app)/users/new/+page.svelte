<!--
A new user, with a temporary password shown once on their page.
-->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { post } from '#lib/api/client.ts';
	import Breadcrumbs from '#lib/components/Breadcrumbs.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import { handOver } from '#lib/handoff.ts';
	import UserForm from '../UserForm.svelte';
	import type { UserInput } from '../users.ts';

	async function create(input: UserInput) {
		const { user, temporary_password } = await post('create_user', input);

		handOver(user.id, temporary_password);
		// Back from the user's page skips the form, so it is not sent again.
		await goto(resolve('/(app)/users/[id]', { id: user.id }), { replaceState: true });
	}
</script>

<Breadcrumbs
	items={[
		{ label: 'Users', href: resolve('/(app)/users') },
		{ label: 'Add user', href: resolve('/(app)/users/new') }
	]}
/>
<PageHeading title="Add user" />

<UserForm submitLabel="Add user" save={create} />
