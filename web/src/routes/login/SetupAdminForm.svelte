<!--
The first admin: shown by the login page after a login with ADMIN_EMAIL and ADMIN_PASSWORD while
the server has no admin. The server creates the admin and logs them in; a full load opens the app.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { limits } from '#lib/api/catalog.gen.ts';
	import { send } from '#lib/api/client.ts';
	import { formClasses } from '#lib/components/field.ts';
	import FormActions from '#lib/components/FormActions.svelte';
	import TextField from '#lib/components/TextField.svelte';
	import { FormState } from '#lib/form.svelte.ts';

	const form = new FormState();
	let email = $state('');
	let password = $state('');
	let confirmation = $state('');
	let element: HTMLFormElement | undefined = $state();

	onMount(() => element?.querySelector<HTMLInputElement>('input[name="email"]')?.focus());

	async function submit(event: SubmitEvent) {
		event.preventDefault();

		const created = await form.submit(() =>
			send('setup_admin', { email: email.trim(), password, confirm_password: confirmation })
		);

		if (created) location.assign('/');
	}
</script>

<form bind:this={element} onsubmit={submit} class={formClasses}>
	<p class="text-sm text-muted">
		This creates the first admin. Choose your email and a password other than ADMIN_PASSWORD.
	</p>
	<TextField
		label="Email"
		name="email"
		type="email"
		bind:value={email}
		autocomplete="username"
		required
		error={form.fields.email}
	/>
	<TextField
		label="Password"
		name="password"
		type="password"
		bind:value={password}
		note="at least {limits.minPassword} characters"
		minlength={limits.minPassword}
		maxlength={limits.maxPassword}
		autocomplete="new-password"
		required
		error={form.fields.password}
	/>
	<TextField
		label="Repeat the password"
		name="confirm_password"
		type="password"
		bind:value={confirmation}
		minlength={limits.minPassword}
		maxlength={limits.maxPassword}
		autocomplete="new-password"
		required
		error={form.fields.confirm_password}
	/>
	<FormActions {form} stretch>Create admin</FormActions>
</form>
