<!--
@component
A number chosen on a scale (a range input), with its label and Hint, marks under the thumb
positions of their values, and what the value means beside it (output). The scale may start below
the least value it takes (least): the thumb stops there. `locked` disables it with why.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import Hint from './Hint.svelte';
	import { thumbCenter } from './slider.ts';

	type Props = {
		label: string;
		name?: string;
		value: number;
		min?: number;
		max?: number;
		step?: number;
		/** The least value taken; below it the thumb goes back to it. */
		least?: number;
		/** Values labelled under the scale. */
		marks?: number[];
		/** After each mark and nothing else: "%". */
		unit?: string;
		hint?: string;
		error?: string;
		/** What the value means, beside the scale. */
		output?: Snippet;
		/** Why it cannot be changed: disabled, the reason under it. */
		locked?: string;
	};

	let {
		label,
		name,
		value = $bindable(),
		min = 0,
		max = 100,
		step = 1,
		least = min,
		marks = [],
		unit = '',
		hint,
		error,
		output,
		locked
	}: Props = $props();

	const id = $props.id();

	function input(event: Event & { currentTarget: HTMLInputElement }) {
		const next = Math.max(least, Number(event.currentTarget.value));

		event.currentTarget.value = String(next);
		value = next;
	}
</script>

<div class="flex flex-col gap-2.5">
	<span class="flex items-center gap-1.25 text-label text-muted">
		<label for={id}>{label}</label>
		{#if hint}<Hint text={hint} />{/if}
	</span>
	<div class="flex flex-wrap items-center gap-x-3.5 gap-y-2">
		<div class="relative min-w-50 flex-1 pb-6">
			<input
				{id}
				{name}
				type="range"
				{min}
				{max}
				{step}
				{value}
				disabled={Boolean(locked)}
				oninput={input}
				aria-describedby={[output && `${id}-out`, locked && `${id}-locked`, error && `${id}-error`]
					.filter(Boolean)
					.join(' ') || undefined}
				aria-invalid={error ? 'true' : undefined}
				class={[
					'block h-2.5 w-full cursor-pointer appearance-none rounded-full focus-visible:shadow-[0_0_0_3px_var(--color-focus)] focus-visible:outline-hidden disabled:cursor-not-allowed disabled:opacity-60',
					'bg-[linear-gradient(to_right,var(--color-accent)_var(--fill),color-mix(in_srgb,var(--color-muted)_25%,var(--color-surface))_var(--fill))]',
					'[&::-webkit-slider-thumb]:size-5 [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:border-2 [&::-webkit-slider-thumb]:border-accent [&::-webkit-slider-thumb]:bg-surface [&::-webkit-slider-thumb]:shadow-[0_1px_3px_rgb(0_0_0/0.25)]',
					'[&::-moz-range-thumb]:box-border [&::-moz-range-thumb]:size-5 [&::-moz-range-thumb]:rounded-full [&::-moz-range-thumb]:border-2 [&::-moz-range-thumb]:border-accent [&::-moz-range-thumb]:bg-surface [&::-moz-range-thumb]:shadow-[0_1px_3px_rgb(0_0_0/0.25)]',
					'forced-colors:bg-[linear-gradient(to_right,Highlight_var(--fill),GrayText_var(--fill))] forced-colors:forced-color-adjust-none'
				]}
				style:--fill={thumbCenter(value, min, max)}
			/>
			<div class="absolute inset-x-0 bottom-0 h-4 text-xs text-muted" aria-hidden="true">
				{#each marks as mark (mark)}
					<span class="absolute top-0 -translate-x-1/2" style:left={thumbCenter(mark, min, max)}>
						{mark}{unit}
					</span>
				{/each}
			</div>
		</div>
		{#if output}
			<output id="{id}-out" for={id} class="min-w-37.5 text-md tabular-nums">
				{@render output()}
			</output>
		{/if}
	</div>
	{#if locked}
		<p id="{id}-locked" class="text-xs text-muted">{locked}</p>
	{/if}
	{#if error}
		<p id="{id}-error" class="text-xs text-error">{error}</p>
	{/if}
</div>
