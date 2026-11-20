<!--
@component
One choice from a list, in the look of the other controls; `all` adds a first option for no
choice (its value is empty).
-->
<script lang="ts">
	import { controlClasses } from './field.ts';
	import type { SelectOption } from './select.ts';

	type Props = {
		/** Its accessible name, e.g. "Rows per page". */
		label: string;
		value: string;
		options: readonly SelectOption[];
		onchange: (value: string) => void;
		/** The option of no choice, e.g. "All statuses". */
		all?: string;
		/** The form field. */
		name?: string;
	};

	let { label, value, options, onchange, all, name }: Props = $props();
</script>

<select
	class={controlClasses}
	aria-label={label}
	{name}
	{value}
	onchange={(event) => onchange(event.currentTarget.value)}
>
	{#if all !== undefined}<option value="">{all}</option>{/if}
	{#each options as option (option.value)}
		<option value={option.value}>{option.label}</option>
	{/each}
</select>
