<!--
@component
A toggle switch: a button with role switch, its track in the accent color when on, the knob
sliding over. Decorative: the track alone, hidden from screen readers, for a row that is itself
the switch (ThemeSwitch through NavLink).
-->
<script lang="ts">
	type Props = {
		checked: boolean;
		/** Shown beside the track; the switch's name. */
		label?: string;
		onchange?: (checked: boolean) => void;
		disabled?: boolean;
		decorative?: boolean;
	};

	let {
		checked = $bindable(),
		label,
		onchange,
		disabled = false,
		decorative = false
	}: Props = $props();

	const track = [
		'relative inline-flex h-5 w-9 shrink-0 items-center rounded-full p-0.5',
		'bg-switch-off transition-colors duration-150 motion-reduce:transition-none data-checked:bg-accent',
		'forced-colors:border forced-colors:border-[CanvasText] forced-colors:bg-[Canvas] forced-colors:forced-color-adjust-none forced-colors:data-checked:bg-[Highlight]'
	];

	const knob = [
		'size-4 rounded-full bg-knob shadow-sm transition-transform duration-150 motion-reduce:transition-none',
		'group-data-checked:translate-x-4',
		'forced-colors:bg-[CanvasText] forced-colors:group-data-checked:bg-[HighlightText]'
	];

	function toggle() {
		checked = !checked;
		onchange?.(checked);
	}
</script>

{#snippet drawn()}
	<span class={['group', track]} data-checked={checked || undefined} aria-hidden="true">
		<span class={knob}></span>
	</span>
{/snippet}

{#if decorative}
	{@render drawn()}
{:else}
	<button
		type="button"
		role="switch"
		aria-checked={checked}
		{disabled}
		onclick={toggle}
		class="inline-flex cursor-pointer items-center gap-2.5 rounded-full text-sm text-text focus-ring disabled:cursor-not-allowed disabled:opacity-50 forced-colors:disabled:text-[GrayText]"
	>
		{@render drawn()}
		{#if label}<span>{label}</span>{/if}
	</button>
{/if}
