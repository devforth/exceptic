<!--
@component
A range of UTC days, both included: two date fields that stop at today.
-->
<script lang="ts">
	import { today } from '#lib/filters.ts';
	import { controlClasses } from './field.ts';

	type Props = { from: string; to: string; onchange: (from: string, to: string) => void };

	let { from, to, onchange }: Props = $props();

	const last = today();
</script>

<input
	type="date"
	class={controlClasses}
	value={from}
	max={to && to < last ? to : last}
	aria-label="From date"
	title="From (UTC)"
	onchange={(event) => onchange(event.currentTarget.value, to)}
/>
<input
	type="date"
	class={controlClasses}
	value={to}
	min={from || undefined}
	max={last}
	aria-label="To date"
	title="To, inclusive (UTC)"
	onchange={(event) => onchange(from, event.currentTarget.value)}
/>
