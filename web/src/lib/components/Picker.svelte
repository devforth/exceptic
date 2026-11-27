<!--
@component
A list filter of several values: a dropdown of checkboxes whose button says "All projects"
or "Projects: 2"; "All projects" first, set apart by a line, checked and disabled while nothing
else is (choosing a value unchecks it, unchecking the last value checks it again). The option snippet draws a value (a badge). Hidden when there is nothing
to choose (one option and none chosen). With searchText a field above the values narrows them
as one types (a value's name and email, say); chosen values always stay listed.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import { toggled } from '#lib/selection.ts';
	import { checkClasses, controlClasses, controlHoverClasses } from './field.ts';
	import { chevronClasses, listMinWidthClass, openControlClasses } from './popup.ts';
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
		/** The text a value is found by: adds the search field. */
		searchText?: (value: string) => string;
	};

	let { label, all, options, selected, onchange, option, searchText }: Props = $props();

	let search = $state('');
	let field: HTMLInputElement | undefined = $state();

	// Opened, the search field takes the focus: it is what one opens the list to use.
	$effect(() => {
		if (open) field?.focus();
		else search = '';
	});
	const listed = $derived.by(() => {
		const needle = search.trim().toLowerCase();

		if (!searchText || !needle) return options;

		return options.filter(
			(value) => selected.includes(value) || searchText(value).toLowerCase().includes(needle)
		);
	});

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
		<div class={[popupClasses, listMinWidthClass, 'absolute top-full left-0 z-20 mt-1']}>
			<!-- All and the search stay on top while the values scroll under them. -->
			<div class="sticky -top-1 z-10 -mx-1 -mt-1 bg-surface px-1 pt-1">
				<!-- Chosen while nothing else is: it goes by choosing a value, so it cannot be unchecked. -->
				<label
					class={[
						optionClasses,
						'mb-0.5 rounded-b-none border-b border-border',
						selected.length === 0 && 'cursor-default hover:bg-transparent'
					]}
				>
					<input
						type="checkbox"
						class={checkClasses.checkbox}
						checked={selected.length === 0}
						disabled={selected.length === 0}
						onchange={() => onchange([])}
					/>
					{all}
				</label>
				{#if searchText}
					<input
						bind:this={field}
						type="search"
						bind:value={search}
						placeholder="Find"
						aria-label="Find {label.toLowerCase()}"
						class={[controlClasses, 'mb-1 w-full']}
					/>
				{/if}
			</div>
			{#each listed as value (value)}
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
			{:else}
				<p class="px-2 py-1.5 text-sm text-muted">Nothing found.</p>
			{/each}
		</div>
	</details>
{/if}
