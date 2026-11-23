<!--
@component
The role of a user form, a card: the roles an administrator may give, each with what it may do,
both as the server has them.
-->
<script lang="ts">
	import { choices, roleSummaries } from '#lib/api/catalog.gen.ts';
	import Choice from './Choice.svelte';
	import ChoiceGroup from './ChoiceGroup.svelte';
	import type { Role } from './role.ts';

	type Props = { value: Role; error?: string };

	let { value = $bindable(), error }: Props = $props();
</script>

<ChoiceGroup legend="Role" {error} card>
	{#each choices.create_user.role as role (role)}
		<Choice
			type="radio"
			name="role"
			value={role}
			label={role}
			description={roleSummaries[role]}
			checked={value === role}
			onchange={() => (value = role)}
		/>
	{/each}
</ChoiceGroup>
