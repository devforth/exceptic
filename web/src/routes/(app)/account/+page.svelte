<!--
The viewer's own name, under Profile the email and under Security the password, each changed in a
dialog.
With a temporary password it is the only page open, and only the password form shows.
-->
<script lang="ts">
	import { invalidate } from '$app/navigation';
	import { limits } from '#lib/api/catalog.gen.ts';
	import { post, send } from '#lib/api/client.ts';
	import ActionList from '#lib/components/ActionList.svelte';
	import ActionRow from '#lib/components/ActionRow.svelte';
	import Card from '#lib/components/Card.svelte';
	import { narrowFormClasses } from '#lib/components/field.ts';
	import FormActions from '#lib/components/FormActions.svelte';
	import FormDialog from '#lib/components/FormDialog.svelte';
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
			// An empty name shows the email once saved.
			nameForm.reset();
		}
	}

	let emailOpen = $state(false);
	let email = $state('');
	let emailPassword = $state('');
	// Sends nothing while the email is the saved one.
	const emailForm = new FormState(() => ({ email: email.trim() }));

	function openEmail() {
		email = user.email;
		emailPassword = '';
		emailForm.reset();
		emailOpen = true;
	}

	async function changeEmail() {
		await send('change_email', { email, current_password: emailPassword });
		toast.show('Email changed: log in with it from now on.');
		await invalidate('app:session');
	}

	let passwordOpen = $state(false);
	let current = $state('');
	let password = $state('');
	let confirmation = $state('');
	const passwordForm = new FormState();

	function openPassword() {
		current = password = confirmation = '';
		passwordOpen = true;
	}

	async function changePassword() {
		await send('change_password', {
			current_password: current,
			password,
			confirm_password: confirmation
		});
		current = password = confirmation = '';
		toast.show(
			'Password changed. Your other sessions were logged out; your connected agents keep working.'
		);
		// A temporary password no longer: the other pages open.
		await invalidate('app:session');
	}

	// The temporary password's form, on the page: nothing else is open then.
	async function replaceTemporary(event: SubmitEvent) {
		event.preventDefault();
		await passwordForm.submit(changePassword);
	}
</script>

<PageHeading title="Your account">
	{#snippet description()}{user.email} · <RoleBadge role={user.role} />{/snippet}
</PageHeading>

{#snippet passwordFields()}
	<!-- For password managers: whose password this is. -->
	<input type="hidden" name="email" value={user.email} autocomplete="username" />
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
{/snippet}

{#if user.must_change_password}
	<div class="mb-4">
		<Notice tone="warning" alert>
			You logged in with a temporary password: choose your own to continue.
		</Notice>
	</div>
	<Card title="Password">
		<form onsubmit={replaceTemporary} class={narrowFormClasses}>
			{@render passwordFields()}
			<FormActions form={passwordForm}>Change password</FormActions>
		</form>
	</Card>
{:else}
	<Card title="Name">
		<form onsubmit={saveName} class={narrowFormClasses}>
			<TextField
				label="Name"
				name="name"
				bind:value={name}
				note="shown in the menu and in issue histories; empty: your email"
				maxlength={limits.displayName}
				autocomplete="name"
				error={nameForm.fields.name}
			/>
			<FormActions form={nameForm}>Save name</FormActions>
		</form>
	</Card>

	<ActionList title="Profile">
		<ActionRow icon="email" label="Change email" description={user.email} onclick={openEmail} />
	</ActionList>

	<ActionList title="Security">
		<ActionRow icon="password" label="Change password" onclick={openPassword} />
	</ActionList>

	<FormDialog
		title="Change email"
		bind:open={emailOpen}
		form={emailForm}
		save={changeEmail}
		submitLabel="Change email"
	>
		<!-- A login from before emails is shown as it is: the server checks a changed one. -->
		<TextField
			label="Email"
			name="email"
			type="text"
			bind:value={email}
			note="you log in with it"
			maxlength={limits.email}
			autocomplete="email"
			required
			error={emailForm.fields.email}
		/>
		<TextField
			label="Current password"
			name="current_password"
			type="password"
			bind:value={emailPassword}
			autocomplete="current-password"
			required
			error={emailForm.fields.current_password}
		/>
	</FormDialog>

	<FormDialog
		title="Change password"
		bind:open={passwordOpen}
		form={passwordForm}
		save={changePassword}
		submitLabel="Change password"
	>
		{@render passwordFields()}
	</FormDialog>
{/if}
