<!--
@component
The role of a user form, a card: the roles an administrator may give, each with what it may do,
both as the server has them; `roles` are those the viewer may give (get_session's
assignable_roles: an admin gives only user), the current role always shown; locked disables it
with why, under the current role.
-->
<script lang="ts">
	import { choices, help, roleSummaries } from '#lib/api/catalog.gen.ts';
	import Choice from './Choice.svelte';
	import ChoiceGroup from './ChoiceGroup.svelte';
	import type { Role } from './role.ts';

	type Props = { value: Role; error?: string; locked?: string; roles?: readonly string[] };

	let { value = $bindable(), error, locked, roles }: Props = $props();

	const offered = $derived(
		choices.create_user.role.filter((role) => !roles || roles.includes(role) || role === value)
	);
</script>

<ChoiceGroup legend="Role" hint={help.fields.Users.Role} {error} card>
	{#each offered as role (role)}
		<Choice
			type="radio"
			name="role"
			value={role}
			label={role}
			description={roleSummaries[role]}
			checked={value === role}
			disabled={Boolean(locked)}
			reason={value === role ? locked : undefined}
			onchange={() => (value = role)}
		/>
	{/each}
</ChoiceGroup>
