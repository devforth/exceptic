<!--
@component
A list filter of several values: a dropdown of checkboxes whose button says "All projects"
or "Projects: 2"; "All projects" first, set apart by a line. The option snippet draws a value (a badge). Hidden when there is nothing
to choose (one option and none chosen).
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import { toggled } from '#lib/selection.ts';
	import { checkClasses, controlClasses, controlHoverClasses } from './field.ts';
	import { chevronClasses, openControlClasses } from './popup.ts';
	import { popupClasses } from './surface.ts';
	import Icon from './Icon.svelte';

	type Props = {
		/** What the values are, e.g. "Projects". */
		label: string;
		/** The button without a choice, e.g. "All projects". */
		all: string;
		options: readonly string[];
		selected: readonly string[];
		onchange: (selected: string[]) => void;
		option: Snippet<[string]>;
	};

	let { label, all, options, selected, onchange, option }: Props = $props();

	let details: HTMLDetailsElement | undefined = $state();
	let open = $state(false);

	// A click elsewhere or Esc closes it, as a menu does.
	$effect(() => {
		const closeOutside = (event: MouseEvent) => {
			if (details?.open && !details.contains(event.target as Node)) details.open = false;
		};
		const closeOnEscape = (event: KeyboardEvent) => {
			if (details?.open && event.key === 'Escape') {
				// Handled: Esc must not also leave the page (Breadcrumbs).
				event.preventDefault();
				details.open = false;
				details.querySelector('summary')?.focus();
			}
		};

		document.addEventListener('click', closeOutside);
		document.addEventListener('keydown', closeOnEscape);

		return () => {
			document.removeEventListener('click', closeOutside);
			document.removeEventListener('keydown', closeOnEscape);
		};
	});

	const optionClasses =
		'flex cursor-pointer items-center gap-2 rounded-control px-2 py-1.5 whitespace-nowrap hover:bg-hover';
</script>

{#if options.length > 1 || selected.length > 0}
	<details bind:this={details} bind:open class="relative" data-popup>
		<summary
			class={[
				controlClasses,
				'flex cursor-pointer list-none items-center justify-between gap-2.5 whitespace-nowrap transition-[border-color,box-shadow] marker:hidden',
				open ? openControlClasses : controlHoverClasses
			]}
		>
			{selected.length ? `${label}: ${selected.length}` : all}
			<span class={chevronClasses(open)}>
				<Icon name="expand" size={16} />
			</span>
		</summary>
		<div class={[popupClasses, 'absolute top-full left-0 z-20 mt-1 min-w-full']}>
			<label class={[optionClasses, 'mb-0.5 rounded-b-none border-b border-border']}>
				<input
					type="checkbox"
					class={checkClasses.checkbox}
					checked={selected.length === 0}
					onchange={() => onchange([])}
				/>
				{all}
			</label>
			{#each options as value (value)}
				<label class={optionClasses}>
					<input
						type="checkbox"
						class={checkClasses.checkbox}
						checked={selected.includes(value)}
						onchange={(event) =>
							onchange(toggled(options, selected, value, event.currentTarget.checked))}
					/>
					{@render option(value)}
				</label>
			{/each}
		</div>
	</details>
{/if}
