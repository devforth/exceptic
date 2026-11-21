<!--
@component
The form of a new user (username, name, role) and of an existing one, whose username stays.
save sends it; a refusal shows at its fields.
-->
<script lang="ts">
	import { limits } from '#lib/api/catalog.gen.ts';
	import Button from '#lib/components/Button.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import RoleChoices from '#lib/components/RoleChoices.svelte';
	import { isAssignable } from '#lib/components/role.ts';
	import TextField from '#lib/components/TextField.svelte';
	import { FormState } from '#lib/form.svelte.ts';
	import type { UserInput } from './users.ts';

	type Props = {
		/** The user as stored; none for a new one. */
		user?: { username: string; name: string; role: string };
		submitLabel: string;
		save: (input: UserInput) => Promise<unknown>;
	};

	let { user, submitLabel, save }: Props = $props();

	const form = new FormState();
	let username = $derived(user?.username ?? '');
	let name = $derived(user?.name ?? '');
	// A new user is a user unless chosen otherwise.
	let role = $derived(user && isAssignable(user.role) ? user.role : 'user');

	function submit(event: SubmitEvent) {
		event.preventDefault();
		void form.submit(() => save({ username, name, role }));
	}
</script>

<form onsubmit={submit} class="flex max-w-xl flex-col gap-4">
	{#if user}
		<TextField label="Username" name="username" value={user.username} readonly />
	{:else}
		<TextField
			label="Username"
			name="username"
			bind:value={username}
			maxlength={limits.username}
			pattern={limits.usernamePattern}
			autocomplete="off"
			required
			error={form.fields.username}
		/>
	{/if}
	<TextField
		label="Name"
		name="name"
		bind:value={name}
		note="optional: the username when empty"
		maxlength={limits.displayName}
		autocomplete="off"
		error={form.fields.name}
	/>
	<RoleChoices bind:value={role} error={form.fields.role} />
	{#if !user}
		<p class="text-xs text-muted">
			A temporary password is generated; the user replaces it at the first login.
		</p>
	{/if}
	{#if form.error}<Notice tone="error">{form.error}</Notice>{/if}
	<div><Button variant="primary" type="submit" busy={form.busy}>{submitLabel}</Button></div>
</form>
