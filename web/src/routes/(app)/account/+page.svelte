<!--
The viewer's own name and password. With a temporary password it is the only page open, and
only the password form shows.
-->
<script lang="ts">
	import { invalidate } from '$app/navigation';
	import { limits } from '#lib/api/catalog.gen.ts';
	import { post, send } from '#lib/api/client.ts';
	import Card from '#lib/components/Card.svelte';
	import { narrowFormClasses } from '#lib/components/field.ts';
	import FormActions from '#lib/components/FormActions.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import PageHeading from '#lib/components/PageHeading.svelte';
	import RoleBadge from '#lib/components/RoleBadge.svelte';
	import TextField from '#lib/components/TextField.svelte';
	import { FormState } from '#lib/form.svelte.ts';
	import { toast } from '#lib/toast.svelte.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const user = $derived(data.session.user);

	// The saved name, until the user types; it follows the session after a save.
	let name = $derived(user.display_name);
	const nameForm = new FormState(() => ({ name: name.trim() }));

	async function saveName(event: SubmitEvent) {
		event.preventDefault();

		if (await nameForm.submit(() => post('update_account_name', { name }))) {
			toast.show('Name saved.');
			await invalidate('app:session');
			// An empty name shows the username once saved.
			nameForm.reset();
		}
	}

	let current = $state('');
	let password = $state('');
	let confirmation = $state('');
	const passwordForm = new FormState();

	async function changePassword(event: SubmitEvent) {
		event.preventDefault();

		const changed = await passwordForm.submit(() =>
			send('change_password', {
				current_password: current,
				password,
				confirm_password: confirmation
			})
		);

		if (changed) {
			current = password = confirmation = '';
			toast.show(
				'Password changed. Your other sessions were logged out; your connected agents keep working.'
			);
			// A temporary password no longer: the other pages open.
			await invalidate('app:session');
		}
	}
</script>

<PageHeading title="Your account">
	{#snippet description()}{user.username} · <RoleBadge role={user.role} />{/snippet}
</PageHeading>

{#if user.must_change_password}
	<div class="mb-4">
		<Notice tone="warning" alert>
			You logged in with a temporary password: choose your own to continue.
		</Notice>
	</div>
{:else}
	<Card title="Name">
		<form onsubmit={saveName} class={narrowFormClasses}>
			<TextField
				label="Name"
				name="name"
				bind:value={name}
				note="shown in the menu and in issue histories; empty: your username"
				maxlength={limits.displayName}
				autocomplete="name"
				error={nameForm.fields.name}
			/>
			<FormActions form={nameForm}>Save name</FormActions>
		</form>
	</Card>
{/if}

<Card title="Password">
	<form onsubmit={changePassword} class={narrowFormClasses}>
		<!-- For password managers: whose password this is. -->
		<input type="hidden" name="username" value={user.username} autocomplete="username" />
		<TextField
			label="Current password"
			name="current_password"
			type="password"
			bind:value={current}
			autocomplete="current-password"
			required
			error={passwordForm.fields.current_password}
		/>
		<TextField
			label="New password"
			name="password"
			type="password"
			bind:value={password}
			note="at least {limits.minPassword} characters"
			minlength={limits.minPassword}
			maxlength={limits.maxPassword}
			autocomplete="new-password"
			required
			error={passwordForm.fields.password}
		/>
		<TextField
			label="Repeat the new password"
			name="confirm_password"
			type="password"
			bind:value={confirmation}
			minlength={limits.minPassword}
			maxlength={limits.maxPassword}
			autocomplete="new-password"
			required
			error={passwordForm.fields.confirm_password}
		/>
		<p class="text-xs text-muted">
			Your other sessions are logged out; your connected coding agents keep working.
		</p>
		<FormActions form={passwordForm}>Change password</FormActions>
	</form>
</Card>
