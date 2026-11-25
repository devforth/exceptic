<!--
@component
A row that opens what it names (a dialog): an icon in a square, the label, an optional line under
it and a chevron, on a card's surface.
-->
<script lang="ts">
	import type { IconName } from '#lib/icons.ts';
	import Icon from './Icon.svelte';
	import { cardClasses } from './surface.ts';

	type Props = {
		icon: IconName;
		label: string;
		description?: string;
		onclick: () => void;
		/** What it opens (aria-haspopup). */
		popup?: 'dialog';
	};

	let { icon, label, description, onclick, popup = 'dialog' }: Props = $props();
</script>

<button
	type="button"
	class={[
		cardClasses,
		'flex w-full cursor-pointer items-center gap-3.5 px-4 py-3.5 text-left focus-ring transition-colors hover:bg-hover'
	]}
	aria-haspopup={popup}
	{onclick}
>
	<span class="grid size-9 shrink-0 place-items-center rounded-control bg-code-bg text-muted">
		<Icon name={icon} size={16} />
	</span>
	<span class="flex min-w-0 flex-1 flex-col">
		<span class="font-semibold">{label}</span>
		{#if description}<span class="text-xs text-muted">{description}</span>{/if}
	</span>
	<span class="text-muted"><Icon name="chevronRight" size={16} /></span>
</button>
