<!--
@component
A row that opens what it names (a dialog): an icon in a square, the label, an optional line under
it and a chevron, on a card's surface. unavailable is why it does nothing now, shown under the label;
status is a state shown before the chevron, as in a table's Status column (Active, Paused).
-->
<script lang="ts">
	import type { IconName } from '#lib/icons.ts';
	import Icon from './Icon.svelte';
	import type { StatusTone } from './status.ts';
	import StatusText from './StatusText.svelte';
	import { cardClasses } from './surface.ts';

	type Props = {
		icon: IconName;
		label: string;
		description?: string;
		onclick: () => void;
		/** What it opens (aria-haspopup). */
		popup?: 'dialog';
		/** Why it does nothing now (aria-disabled, so it keeps the focus). */
		unavailable?: string;
		/** The state of what it changes, before the chevron. */
		status?: { label: string; tone: StatusTone };
	};

	let {
		icon,
		label,
		description,
		onclick,
		popup = 'dialog',
		unavailable,
		status
	}: Props = $props();
</script>

<button
	type="button"
	class={[
		cardClasses,
		'flex w-full items-center gap-3.5 px-4 py-3.5 text-left focus-ring transition-colors',
		unavailable ? 'cursor-not-allowed opacity-60' : 'cursor-pointer hover:bg-hover'
	]}
	aria-haspopup={popup}
	aria-disabled={unavailable ? true : undefined}
	title={unavailable}
	onclick={unavailable ? undefined : onclick}
>
	<span class="grid size-9 shrink-0 place-items-center rounded-control bg-code-bg text-muted">
		<Icon name={icon} size={16} />
	</span>
	<span class="flex min-w-0 flex-1 flex-col">
		<span class="font-semibold">{label}</span>
		{#if unavailable || description}<span class="text-xs text-muted">
				{unavailable ?? description}
			</span>{/if}
	</span>
	{#if status}<StatusText tone={status.tone}>{status.label}</StatusText>{/if}
	<span class="text-muted"><Icon name="chevronRight" size={16} /></span>
</button>
